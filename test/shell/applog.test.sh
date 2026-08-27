#!/usr/bin/env bash
# Acceptance suite: applog -- an append-only entity (operations: create, read
# only) that should "only ever grow". v1->v2 is purely additive and must apply
# automatically; the v2->v3 manifest (dropping a field) is deliberately never
# allowed to take effect: this file proves the destructive attempt is refused,
# the app stays pinned at v2, and every row is untouched.
set -uo pipefail
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/lib.sh"

APP_DIR="$TEST_DIR/apps/applog"
LIVE_DIR="$APPS_DIR/applog"

# --- v1: append-only semantics + seed data -----------------------------------

body=$(jq -nc '{message:"boot"}')
http_request POST /apps/applog/entry "$body"
assert_status "$HTTP_STATUS" 201 "applog v1: create entry with default level"
E1=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.level' "info" "applog v1: level defaults to info"

body=$(jq -nc '{message:"disk nearly full", level:"warn"}')
http_request POST /apps/applog/entry "$body"
assert_status "$HTTP_STATUS" 201 "applog v1: create warn entry"
E2=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

body=$(jq -nc '{message:"日本語 ログ エントリ", level:"error"}')
http_request POST /apps/applog/entry "$body"
assert_status "$HTTP_STATUS" 201 "applog v1: create unicode error entry"
E3=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

http_request PATCH "/apps/applog/entry/$E1" '{"message":"edited"}'
assert_status "$HTTP_STATUS" 405 "applog v1: update is disabled for an append-only entity"
http_request DELETE "/apps/applog/entry/$E1"
assert_status "$HTTP_STATUS" 405 "applog v1: delete is disabled for an append-only entity"

# --- v1 -> v2: purely additive (source, count default 1) --------------------

server_stop
run_migrate "$APPS_DIR" applog "$APP_DIR/v2.manifest.json"
assert_eq "$MIGRATE_EXIT" 0 "applog v1->v2: additive migration applies without -confirm ($MIGRATE_OUT)"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "applog v1->v2: manifest.json promoted to version 2"
server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"

http_request GET "/apps/applog/entry/$E1"
assert_status "$HTTP_STATUS" 200 "applog v2: entry 1 survives migration"
assert_json "$HTTP_BODY" '.message' "boot" "applog v2: message preserved"
assert_json "$HTTP_BODY" '.count' "1" "applog v2: added integer field backfilled with its declared default"
assert_json_null "$HTTP_BODY" '.source' "applog v2: added nullable field with no default is null on old rows"

http_request GET "/apps/applog/entry/$E3"
assert_json "$HTTP_BODY" '.message' "日本語 ログ エントリ" "applog v2: unicode message preserved"
assert_json "$HTTP_BODY" '.level' "error" "applog v2: level preserved"

body=$(jq -nc '{message:"new style", source:"worker-3", count:5}')
http_request POST /apps/applog/entry "$body"
assert_status "$HTTP_STATUS" 201 "applog v2: create entry exercising new fields"
E4=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.source' "worker-3" "applog v2: new field round-trips on a fresh row"

# --- v2 -> v3: dropping fld_level must be refused; the log must "only ever grow" ---
# Unlike tracker's enum narrowing, a field drop needs no witness (only confirm),
# so there is no separate witness gate to exercise here -- this app's whole job
# is to prove the no-confirm refusal holds and that the log never loses a column
# once added. It must stay at v2 for the rest of this suite's life.

server_stop
before_db=$(mktemp)
cp "$LIVE_DIR/data.db" "$before_db"

run_migrate "$APPS_DIR" applog "$APP_DIR/v3.manifest.json"
assert_eq "$MIGRATE_EXIT" 1 "applog v2->v3: dropping a field without -confirm is refused"
assert_matches "$MIGRATE_OUT" "refusing:.*confirmation" "applog v2->v3: refusal cites required confirmation"
assert_files_equal "$before_db" "$LIVE_DIR/data.db" "applog v2->v3: db file untouched after refusal"
rm -f "$before_db"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "applog v2->v3: manifest.json NOT promoted after refusal -- log stays at v2"

server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"
http_request GET "/apps/applog/entry/$E1"
assert_status "$HTTP_STATUS" 200 "applog v3 (refused): entry 1 still readable"
assert_json "$HTTP_BODY" '.level' "info" "applog v3 (refused): level field still present and untouched"
assert_json "$HTTP_BODY" '.message' "boot" "applog v3 (refused): message untouched"
http_request GET "/apps/applog/entry/$E4"
assert_json "$HTTP_BODY" '.source' "worker-3" "applog v3 (refused): v2-era row untouched -- zero rows touched by the refused migration"
