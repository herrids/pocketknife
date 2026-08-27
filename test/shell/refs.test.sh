#!/usr/bin/env bash
# Acceptance suite: refs -- one parent entity referenced by three children, one
# per onDelete behavior (cascade/restrict/set_null), proving (a) those FK
# actions are enforced both before and after a schema migration, and (b)
# renaming the *referenced* entity and field (v1->v2, safe, zero-SQL since
# references resolve by stable id) and dropping a non-reference field on a
# child (v2->v3, destructive, confirm-only) never disturbs the references
# themselves.
set -uo pipefail
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/lib.sh"

APP_DIR="$TEST_DIR/apps/refs"
LIVE_DIR="$APPS_DIR/refs"

mkparent() { # mkparent LABEL -> sets ID
	local body
	body=$(jq -nc --arg l "$1" '{label:$l}')
	http_request POST /apps/refs/parent "$body"
	ID=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
}

# --- v1: seed the long-lived set (P1 + one child of each onDelete kind) -----

mkparent "Root"; P1="$ID"
assert_status "$HTTP_STATUS" 201 "refs v1: create long-lived parent"

body=$(jq -nc --arg p "$P1" '{name:"a-casc", parent:$p}')
http_request POST /apps/refs/casc "$body"
A_CASC=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_status "$HTTP_STATUS" 201 "refs v1: create long-lived cascade child"

body=$(jq -nc --arg p "$P1" '{name:"a-rstr", parent:$p}')
http_request POST /apps/refs/rstr "$body"
A_RSTR=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_status "$HTTP_STATUS" 201 "refs v1: create long-lived restrict child"

body=$(jq -nc --arg p "$P1" '{name:"a-snul", parent:$p}')
http_request POST /apps/refs/snul "$body"
A_SNUL=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_status "$HTTP_STATUS" 201 "refs v1: create long-lived set_null child"

# --- v1: exercise each onDelete behavior with disposable parents ------------

mkparent "CascadeMe"; P2="$ID"
body=$(jq -nc --arg p "$P2" '{name:"b-casc", parent:$p}')
http_request POST /apps/refs/casc "$body"
B_CASC=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

http_request DELETE "/apps/refs/parent/$P2"
assert_status "$HTTP_STATUS" 204 "refs v1: cascade -- deleting the parent succeeds"
http_request GET "/apps/refs/casc/$B_CASC"
assert_status "$HTTP_STATUS" 404 "refs v1: cascade -- child was deleted along with its parent"

mkparent "RestrictMe"; P3="$ID"
body=$(jq -nc --arg p "$P3" '{name:"c-rstr", parent:$p}')
http_request POST /apps/refs/rstr "$body"
C_RSTR=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

http_request DELETE "/apps/refs/parent/$P3"
assert_status "$HTTP_STATUS" 409 "refs v1: restrict -- deleting a referenced parent is blocked"
http_request DELETE "/apps/refs/rstr/$C_RSTR"
assert_status "$HTTP_STATUS" 204 "refs v1: restrict -- deleting the child first succeeds"
http_request DELETE "/apps/refs/parent/$P3"
assert_status "$HTTP_STATUS" 204 "refs v1: restrict -- parent deletion now succeeds once unreferenced"

mkparent "SetNullMe"; P4="$ID"
body=$(jq -nc --arg p "$P4" '{name:"d-snul", parent:$p}')
http_request POST /apps/refs/snul "$body"
D_SNUL=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

http_request DELETE "/apps/refs/parent/$P4"
assert_status "$HTTP_STATUS" 204 "refs v1: set_null -- deleting the parent succeeds"
http_request GET "/apps/refs/snul/$D_SNUL"
assert_status "$HTTP_STATUS" 200 "refs v1: set_null -- child survives its parent's deletion"
assert_json_null "$HTTP_BODY" '.parent' "refs v1: set_null -- child's reference is nulled"

# --- v1 -> v2: rename the referenced entity (parent->owner) and its field (label->title) ---

server_stop
run_migrate "$APPS_DIR" refs "$APP_DIR/v2.manifest.json"
assert_eq "$MIGRATE_EXIT" 0 "refs v1->v2: safe rename-on-referenced-entity applies without -confirm ($MIGRATE_OUT)"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "refs v1->v2: manifest.json promoted to version 2"
server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"

http_request GET "/apps/refs/parent/$P1"
assert_status "$HTTP_STATUS" 404 "refs v2: old entity name 'parent' no longer routes"
http_request GET "/apps/refs/owner/$P1"
assert_status "$HTTP_STATUS" 200 "refs v2: renamed entity routes under its new name 'owner'"
assert_json "$HTTP_BODY" '.title' "Root" "refs v2: renamed field holds the original value"

http_request GET "/apps/refs/casc/$A_CASC"
assert_json "$HTTP_BODY" '.parent' "$P1" "refs v2: cascade child's reference survives the parent rename"
http_request GET "/apps/refs/rstr/$A_RSTR"
assert_json "$HTTP_BODY" '.parent' "$P1" "refs v2: restrict child's reference survives the parent rename"
http_request GET "/apps/refs/snul/$A_SNUL"
assert_json "$HTTP_BODY" '.parent' "$P1" "refs v2: set_null child's reference survives the parent rename"

# --- v2: re-prove all three onDelete behaviors still hold after the rebuild ---

mkparent2() { # uses the post-rename route
	local body
	body=$(jq -nc --arg l "$1" '{title:$l}')
	http_request POST /apps/refs/owner "$body"
	ID=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
}

mkparent2 "CascadeMe2"; P5="$ID"
body=$(jq -nc --arg p "$P5" '{name:"e-casc", parent:$p}')
http_request POST /apps/refs/casc "$body"
E_CASC=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
http_request DELETE "/apps/refs/owner/$P5"
assert_status "$HTTP_STATUS" 204 "refs v2: cascade still applies post-rename -- parent delete succeeds"
http_request GET "/apps/refs/casc/$E_CASC"
assert_status "$HTTP_STATUS" 404 "refs v2: cascade still applies post-rename -- child deleted too"

mkparent2 "RestrictMe2"; P6="$ID"
body=$(jq -nc --arg p "$P6" '{name:"f-rstr", parent:$p}')
http_request POST /apps/refs/rstr "$body"
http_request DELETE "/apps/refs/owner/$P6"
assert_status "$HTTP_STATUS" 409 "refs v2: restrict still applies post-rename -- referenced parent delete is blocked"

mkparent2 "SetNullMe2"; P7="$ID"
body=$(jq -nc --arg p "$P7" '{name:"g-snul", parent:$p}')
http_request POST /apps/refs/snul "$body"
G_SNUL=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
http_request DELETE "/apps/refs/owner/$P7"
assert_status "$HTTP_STATUS" 204 "refs v2: set_null still applies post-rename -- parent delete succeeds"
http_request GET "/apps/refs/snul/$G_SNUL"
assert_json_null "$HTTP_BODY" '.parent' "refs v2: set_null still applies post-rename -- child's reference is nulled"

# --- v2 -> v3: drop a non-reference field on a child (destructive, confirm-only) ---

server_stop
before_db=$(mktemp)
cp "$LIVE_DIR/data.db" "$before_db"
run_migrate "$APPS_DIR" refs "$APP_DIR/v3.manifest.json"
assert_eq "$MIGRATE_EXIT" 1 "refs v2->v3: dropping casc.name without -confirm is refused"
assert_matches "$MIGRATE_OUT" "refusing:.*confirmation" "refs v2->v3: refusal cites required confirmation"
assert_files_equal "$before_db" "$LIVE_DIR/data.db" "refs v2->v3: db file untouched after refusal"
rm -f "$before_db"

run_migrate "$APPS_DIR" refs "$APP_DIR/v3.manifest.json" --confirm
assert_eq "$MIGRATE_EXIT" 0 "refs v2->v3: confirmed field drop applies ($MIGRATE_OUT)"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "3" "refs v2->v3: manifest.json promoted to version 3"
server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"

http_request GET "/apps/refs/casc/$A_CASC"
assert_status "$HTTP_STATUS" 200 "refs v3: cascade child survives the drop of its non-reference field"
if printf '%s' "$HTTP_BODY" | jq -e 'has("name")' >/dev/null 2>&1; then
	fail "refs v3: casc.name field is dropped" "response still has a name key: $HTTP_BODY"
else
	pass "refs v3: casc.name field is dropped"
fi
assert_json "$HTTP_BODY" '.parent' "$P1" "refs v3: reference field is untouched by an unrelated field drop"

http_request GET "/apps/refs/owner/$P1"
assert_status "$HTTP_STATUS" 200 "refs v3: renamed parent entity still reachable after a second rebuild"
assert_json "$HTTP_BODY" '.title' "Root" "refs v3: parent's renamed field value still intact"

http_request DELETE "/apps/refs/owner/$P1"
assert_status "$HTTP_STATUS" 409 "refs v3: restrict still enforced post-drop -- P1 still has rstr child a-rstr"
http_request DELETE "/apps/refs/rstr/$A_RSTR"
assert_status "$HTTP_STATUS" 204 "refs v3: clearing the restrict child for final cascade check"
http_request DELETE "/apps/refs/owner/$P1"
assert_status "$HTTP_STATUS" 204 "refs v3: cascade still enforced post-drop -- deleting P1 succeeds once unrestricted"
http_request GET "/apps/refs/casc/$A_CASC"
assert_status "$HTTP_STATUS" 404 "refs v3: cascade still enforced post-drop -- a-casc deleted with its parent"
http_request GET "/apps/refs/snul/$A_SNUL"
assert_status "$HTTP_STATUS" 200 "refs v3: set_null still enforced post-drop -- a-snul survives"
assert_json_null "$HTTP_BODY" '.parent' "refs v3: set_null still enforced post-drop -- a-snul's reference is nulled"
