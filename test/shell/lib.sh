#!/usr/bin/env bash
# Shared assertion and process helpers for the black-box shell stress suite.
# Sourced by run.sh and by every *.test.sh file. Never set -e here: a failed
# assertion must be recorded and the script must keep going so one bad
# expectation doesn't hide every other one in the same file.

RESULTS_FILE="${RESULTS_FILE:?RESULTS_FILE must be set by run.sh}"
BIN="${BIN:?BIN must be set by run.sh}"

# --- low-level result recording -------------------------------------------

pass() {
	local name="$1"
	printf 'PASS\t%s\n' "$name" >>"$RESULTS_FILE"
	echo "  PASS  $name"
}

fail() {
	local name="$1" msg="$2"
	printf 'FAIL\t%s\t%s\n' "$name" "$msg" >>"$RESULTS_FILE"
	echo "  FAIL  $name -- $msg" >&2
}

# --- assertions -------------------------------------------------------------

assert_eq() {
	local actual="$1" expected="$2" name="$3"
	if [[ "$actual" == "$expected" ]]; then
		pass "$name"
	else
		fail "$name" "got '$actual', want '$expected'"
	fi
}

assert_status() {
	local actual="$1" expected="$2" name="$3"
	assert_eq "$actual" "$expected" "$name (status)"
}

# assert_json JSON JQ_EXPR EXPECTED NAME
# Evaluates JQ_EXPR (raw-output) against JSON and compares to EXPECTED.
assert_json() {
	local json="$1" expr="$2" expected="$3" name="$4"
	local got
	got=$(printf '%s' "$json" | jq -r "$expr" 2>/dev/null)
	if [[ $? -ne 0 ]]; then
		fail "$name" "jq expression '$expr' failed against: $json"
		return
	fi
	assert_eq "$got" "$expected" "$name"
}

# assert_json_null JSON JQ_EXPR NAME -- asserts the field is JSON null.
assert_json_null() {
	local json="$1" expr="$2" name="$3"
	assert_json "$json" "$expr" "null" "$name"
}

assert_file_exists() {
	local path="$1" name="$2"
	if [[ -f "$path" ]]; then
		pass "$name"
	else
		fail "$name" "expected file to exist: $path"
	fi
}

assert_dir_exists() {
	local path="$1" name="$2"
	if [[ -d "$path" ]]; then
		pass "$name"
	else
		fail "$name" "expected directory to exist: $path"
	fi
}

assert_files_equal() {
	local a="$1" b="$2" name="$3"
	if cmp -s "$a" "$b"; then
		pass "$name"
	else
		fail "$name" "files differ: $a vs $b"
	fi
}

assert_contains() {
	local haystack="$1" needle="$2" name="$3"
	if [[ "$haystack" == *"$needle"* ]]; then
		pass "$name"
	else
		fail "$name" "expected to find '$needle' in: $haystack"
	fi
}

assert_matches() {
	local haystack="$1" pattern="$2" name="$3"
	if [[ "$haystack" =~ $pattern ]]; then
		pass "$name"
	else
		fail "$name" "expected '$haystack' to match /$pattern/"
	fi
}

# --- HTTP -------------------------------------------------------------------

# http_request METHOD PATH [BODY]
# Sets HTTP_STATUS and HTTP_BODY. PATH is appended to BASE_URL as-is (callers
# must URL-encode query strings themselves, e.g. via jq -sRr @uri or printf).
http_request() {
	local method="$1" path="$2" body="${3:-}"
	local tmp
	tmp=$(mktemp)
	if [[ -n "$body" ]]; then
		HTTP_STATUS=$(curl -s -o "$tmp" -w '%{http_code}' -X "$method" \
			-H 'Content-Type: application/json' -d "$body" "${BASE_URL}${path}")
	else
		HTTP_STATUS=$(curl -s -o "$tmp" -w '%{http_code}' -X "$method" "${BASE_URL}${path}")
	fi
	HTTP_BODY=$(cat "$tmp")
	rm -f "$tmp"
}

wait_for_health() {
	local url="$1" timeout="${2:-10}"
	local waited=0
	while ! curl -s -o /dev/null "$url" 2>/dev/null; do
		sleep 0.2
		waited=$((waited + 1))
		if (( waited > timeout * 5 )); then
			echo "server did not become healthy within ${timeout}s: $url" >&2
			return 1
		fi
	done
	return 0
}

# --- golden files -------------------------------------------------------------
# golden_check FIXTURE_PATH ACTUAL_JSON NAME
# Normalizes ACTUAL_JSON's dynamic id/created_at/updated_at fields (at the top
# level and, for a list envelope, inside each element of "data") and compares the
# result to FIXTURE_PATH. With UPDATE_GOLDEN=1, (re)writes the fixture from
# ACTUAL_JSON instead of comparing -- the regeneration path the TS client
# generator is expected to run when the contract changes on purpose.
golden_check() {
	local fixture="$1" actual="$2" name="$3"
	local normalized
	normalized=$(printf '%s' "$actual" | jq -S '
		def norm: . as $o
			| $o
			+ (if $o|has("id") then {id:"<ID>"} else {} end)
			+ (if $o|has("created_at") then {created_at:"<TS>"} else {} end)
			+ (if $o|has("updated_at") then {updated_at:"<TS>"} else {} end);
		if (has("data") and (.data|type) == "array")
		then (.data |= map(norm)) | norm
		else norm
		end
	')
	if [[ "${UPDATE_GOLDEN:-0}" == "1" ]]; then
		mkdir -p "$(dirname "$fixture")"
		printf '%s\n' "$normalized" >"$fixture"
		pass "$name (golden updated)"
		return
	fi
	if [[ ! -f "$fixture" ]]; then
		fail "$name" "golden fixture missing: $fixture (run with UPDATE_GOLDEN=1 to create it)"
		return
	fi
	local want
	want=$(jq -S . "$fixture")
	assert_eq "$normalized" "$want" "$name"
}

# --- ephemeral ports ---------------------------------------------------------

free_port() {
	python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'
}

# --- server lifecycle ---------------------------------------------------------
# A single shared server process is started by run.sh over $APPS_DIR and
# reused by every test file. Migration test files that need to evolve an
# app's schema must stop it, run `pocketknife migrate`, then start it again --
# the running server's in-memory registry never learns about an out-of-band
# migration (see STRESS_DISCOVERY.md §5/§8).

server_start() {
	local apps_dir="$1" port="$2" log="$3"
	"$BIN" -apps "$apps_dir" -addr ":$port" >"$log" 2>&1 &
	# Persist the PID to a file rather than only a shell variable: each
	# *.test.sh file runs as its own bash subprocess, so a plain variable set
	# here (in run.sh's process) is invisible to server_stop calls made from
	# inside a test file, and vice versa for restarts a test file performs
	# itself. A file under $TMPROOT is the only state both processes share.
	echo "$!" >"${SERVER_PIDFILE:?SERVER_PIDFILE must be set}"
	BASE_URL="http://127.0.0.1:$port"
	# There is no dedicated health route; any HTTP response (even a 404) proves
	# the listener is up, since registry.Load runs to completion -- including
	# every app's boot/materialize/DDL step -- before ListenAndServe is ever
	# called. A 404 on an unrouted path is therefore a perfectly good probe.
	if ! wait_for_health "$BASE_URL/__health_probe__" 10; then
		echo "server failed to start; log:" >&2
		cat "$log" >&2
		return 1
	fi
}

server_stop() {
	local pidfile="${SERVER_PIDFILE:?SERVER_PIDFILE must be set}"
	[[ -f "$pidfile" ]] || return 0
	local pid
	pid=$(cat "$pidfile" 2>/dev/null)
	[[ -n "$pid" ]] || { rm -f "$pidfile"; return 0; }
	if kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null
		# Not `wait`: $pid may belong to a different subshell than the one
		# calling server_stop, so it is not a job-control child here. Poll
		# instead, with a kill -9 fallback if it ignores SIGTERM.
		local waited=0
		while kill -0 "$pid" 2>/dev/null; do
			sleep 0.05
			waited=$((waited + 1))
			if (( waited > 200 )); then
				kill -9 "$pid" 2>/dev/null
				break
			fi
		done
	fi
	rm -f "$pidfile"
}

# --- migration CLI -----------------------------------------------------------
# run_migrate APPS_DIR APP_ID TO_MANIFEST [--confirm] [--witnesses FILE]
# Sets MIGRATE_EXIT and MIGRATE_OUT (combined stdout+stderr; the CLI logs
# exclusively via the `log` package, which writes to stderr).
run_migrate() {
	local apps_dir="$1" app_id="$2" to="$3"
	shift 3
	local args=(migrate -apps "$apps_dir" -app "$app_id" -to "$to")
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--confirm)
			args+=(-confirm)
			shift
			;;
		--witnesses)
			args+=(-witnesses "$2")
			shift 2
			;;
		*)
			echo "run_migrate: unknown arg $1" >&2
			return 2
			;;
		esac
	done
	MIGRATE_OUT=$("$BIN" "${args[@]}" 2>&1)
	MIGRATE_EXIT=$?
}
