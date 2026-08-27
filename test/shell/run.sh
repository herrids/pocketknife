#!/usr/bin/env bash
# Black-box stress harness for pocketknife. Builds the real binary, boots one
# server over a fresh temp apps dir on an ephemeral port, runs every
# *.test.sh file in this directory against it, and tears everything down on
# exit. Exits non-zero if any assertion failed or any test file crashed.
#
# Usage: bash test/shell/run.sh [UPDATE_GOLDEN=1 to regenerate golden files]
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

BIN="$REPO_ROOT/bin/pocketknife-stress"
export BIN

TMPROOT=$(mktemp -d)
APPS_DIR="$TMPROOT/apps"
RESULTS_FILE="$TMPROOT/results.log"
SERVER_LOG="$TMPROOT/server.log"
SERVER_PIDFILE="$TMPROOT/server.pid"
mkdir -p "$APPS_DIR"
: >"$RESULTS_FILE"
export APPS_DIR RESULTS_FILE SERVER_LOG SERVER_PIDFILE

source "$SCRIPT_DIR/lib.sh"

cleanup() {
	server_stop
	if [[ "${KEEP_TMP:-0}" == "1" ]]; then
		echo "KEEP_TMP=1: leaving $TMPROOT in place"
	else
		rm -rf "$TMPROOT"
	fi
}
trap cleanup EXIT

echo "==> building $BIN"
if ! (cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/pocketknife) >"$TMPROOT/build.log" 2>&1; then
	echo "build failed:" >&2
	cat "$TMPROOT/build.log" >&2
	exit 1
fi

echo "==> seeding v1 manifests for each acceptance app into $APPS_DIR"
for app_dir in "$SCRIPT_DIR"/apps/*/; do
	app=$(basename "$app_dir")
	mkdir -p "$APPS_DIR/$app"
	cp "$app_dir/v1.manifest.json" "$APPS_DIR/$app/manifest.json"
done

PORT=$(free_port)
export PORT
echo "==> starting server on :$PORT (apps dir: $APPS_DIR)"
if ! server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"; then
	exit 1
fi
export BASE_URL

echo "==> running test files"
shopt -s nullglob
test_files=("$SCRIPT_DIR"/*.test.sh)
shopt -u nullglob
if [[ ${#test_files[@]} -eq 0 ]]; then
	echo "no *.test.sh files found in $SCRIPT_DIR" >&2
	exit 1
fi

for f in "${test_files[@]}"; do
	name=$(basename "$f")
	echo "--- $name ---"
	before=$(wc -l <"$RESULTS_FILE")
	TEST_FILE_NAME="$name" bash "$f"
	rc=$?
	after=$(wc -l <"$RESULTS_FILE")
	if [[ "$rc" -ne 0 && "$after" -eq "$before" ]]; then
		# The script died before recording even one assertion -- a crash, not a
		# failed expectation. Record it so the summary doesn't silently miss it.
		printf 'FAIL\t%s\t%s\n' "$name" "test file exited $rc with no recorded assertions" >>"$RESULTS_FILE"
	fi
done

echo
echo "==> summary"
total=$(wc -l <"$RESULTS_FILE")
passed=$(grep -c '^PASS' "$RESULTS_FILE" || true)
failed=$(grep -c '^FAIL' "$RESULTS_FILE" || true)
echo "  $passed/$total passed"
if [[ "$failed" -gt 0 ]]; then
	echo
	echo "failures:"
	grep '^FAIL' "$RESULTS_FILE" | sed 's/^/  /'
	exit 1
fi
exit 0
