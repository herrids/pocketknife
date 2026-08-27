#!/usr/bin/env bash
# Acceptance suite: tracker -- a single entity (with a self-reference) covering
# the full v1 field type set, evolved through a safe rename+widen+add (v1->v2)
# and a destructive enum-narrow+drop (v2->v3), with adversarial data seeded at
# v1 kept alive and asserted at every step. Doubles as the suite's negative/
# gating fixture: no-confirm refusal, confirm-without-witness refusal, and a
# confirm+witness success, each checked against the real CLI subprocess.
set -uo pipefail
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/lib.sh"

APP_DIR="$TEST_DIR/apps/tracker"
LIVE_DIR="$APPS_DIR/tracker"

# --- v1: seed adversarial data -----------------------------------------------

body=$(jq -nc '{title:"Minimal"}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 201 "tracker v1: create minimal row"
R1=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.active' 'true' "tracker v1: active defaults to true"
assert_json "$HTTP_BODY" '.kind' 'a' "tracker v1: kind defaults to a"
assert_json_null "$HTTP_BODY" '.count' "tracker v1: count defaults to null"
assert_json_null "$HTTP_BODY" '.score' "tracker v1: score defaults to null"
assert_json_null "$HTTP_BODY" '.parent' "tracker v1: parent defaults to null"

body=$(jq -nc '{title:"Unicode 🚀 Tëst", count:0, score:3.14, active:false, due:"2024-01-01T00:00:00Z", kind:"b"}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 201 "tracker v1: create unicode/full row"
R2=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.title' "Unicode 🚀 Tëst" "tracker v1: unicode title round-trips"

body=$(jq -nc --arg t "O'Brien \"quoted\" <tag>&amp;" '{title:$t, count:9223372036854775807, kind:"c"}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 201 "tracker v1: create quote/markup row with max-int64 count"
R3=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.title' "O'Brien \"quoted\" <tag>&amp;" "tracker v1: quote/markup title round-trips"
assert_contains "$HTTP_BODY" "9223372036854775807" "tracker v1: max-int64 count survives JSON round-trip as-is"

body=$(jq -nc '{title:""}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 201 "tracker v1: empty-string title is accepted (required != non-empty)"
R4=$(printf '%s' "$HTTP_BODY" | jq -r '.id')

body=$(jq -nc --arg p "$R1" '{title:"Child", parent:$p}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 201 "tracker v1: create self-referencing row"
R5=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
assert_json "$HTTP_BODY" '.parent' "$R1" "tracker v1: self-reference stored"

body=$(jq -nc '{title:"Bad", count:-1}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 400 "tracker v1: count below min is rejected"

body=$(jq -nc '{title:"Bad", kind:"z"}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 400 "tracker v1: invalid enum value is rejected"

body=$(jq -nc '{title:"Bad", bogus:"x"}')
http_request POST /apps/tracker/item "$body"
assert_status "$HTTP_STATUS" 400 "tracker v1: unknown field is rejected"

# --- v1 -> v2: safe rename(title->headline) + widen(count int->real) + add(note, default "") ---

server_stop
run_migrate "$APPS_DIR" tracker "$APP_DIR/v2.manifest.json"
assert_eq "$MIGRATE_EXIT" 0 "tracker v1->v2: safe migration applies without -confirm ($MIGRATE_OUT)"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "tracker v1->v2: manifest.json promoted to version 2"
server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"

http_request GET "/apps/tracker/item/$R1"
assert_status "$HTTP_STATUS" 200 "tracker v2: row 1 survives migration"
assert_json "$HTTP_BODY" '.headline' "Minimal" "tracker v2: title renamed to headline, value preserved"
assert_json "$HTTP_BODY" '.note' "" "tracker v2: added field backfilled with its declared default"

http_request GET "/apps/tracker/item/$R2"
assert_json "$HTTP_BODY" '.headline' "Unicode 🚀 Tëst" "tracker v2: unicode value survives rename+widen"
assert_json "$HTTP_BODY" '.count' "0" "tracker v2: count=0 survives widening to real"

http_request GET "/apps/tracker/item/$R3"
assert_json "$HTTP_BODY" '.headline' "O'Brien \"quoted\" <tag>&amp;" "tracker v2: quote/markup title survives rename"
assert_contains "$HTTP_BODY" "9223372036854775807" "tracker v2: max-int64 count exact after widening to real (NOTE: float64 cannot represent all int64 exactly -- a mismatch here is a real precision-loss defect in a Safe-classified op, not a test bug)"

http_request GET "/apps/tracker/item/$R4"
assert_json "$HTTP_BODY" '.headline' "" "tracker v2: empty-string title survives migration"

http_request GET "/apps/tracker/item/$R5"
assert_json "$HTTP_BODY" '.parent' "$R1" "tracker v2: self-reference survives table rebuild"

# --- v2 -> v3 negative gating: enum narrow (a,b,c -> a,b) + drop fld_score (needs confirm only) ---
# NOTE on the enum-narrow case: witnessNeeded() (migrate/witness.go) does not
# gate OpChangeEnum behind a mandatory witness the way it does for
# OpChangeType/OpChangeRequired/OpAddField -- per that file's own doc comment,
# enum value removal is enforced structurally by the rebuild's CHECK
# constraint instead. So confirm+no-witness is not refused up front with a
# "refusing: ... witness" message; it gets as far as the table-rebuild
# transaction, where the CHECK constraint on fld_kind rejects the still-"c"
# row, and the whole migration fails and rolls back. The net safety property
# (no data loss, no partial schema) holds either way -- this just asserts the
# real mechanism instead of an upfront gate that doesn't exist for this op.

server_stop
before_db=$(mktemp)
cp "$LIVE_DIR/data.db" "$before_db"

run_migrate "$APPS_DIR" tracker "$APP_DIR/v3.manifest.json"
assert_eq "$MIGRATE_EXIT" 1 "tracker v2->v3: destructive migration without -confirm is refused"
assert_matches "$MIGRATE_OUT" "refusing:.*confirmation" "tracker v2->v3: refusal cites required confirmation"
assert_files_equal "$before_db" "$LIVE_DIR/data.db" "tracker v2->v3: db file untouched after no-confirm refusal"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "tracker v2->v3: manifest.json NOT promoted after no-confirm refusal"

run_migrate "$APPS_DIR" tracker "$APP_DIR/v3.manifest.json" --confirm
assert_eq "$MIGRATE_EXIT" 1 "tracker v2->v3: confirmed but witnessless migration still fails"
assert_matches "$MIGRATE_OUT" "rolled back.*CHECK constraint" "tracker v2->v3: failure is the rebuild's CHECK constraint rejecting the un-remapped enum value, not an upfront witness gate"
assert_files_equal "$before_db" "$LIVE_DIR/data.db" "tracker v2->v3: db file untouched after no-witness failure"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "2" "tracker v2->v3: manifest.json NOT promoted after no-witness failure"
rm -f "$before_db"

server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"
http_request GET "/apps/tracker/item/$R3"
assert_json "$HTTP_BODY" '.kind' "c" "tracker v2->v3: row untouched by either refusal -- kind still c"
http_request GET "/apps/tracker/item/$R2"
assert_json "$HTTP_BODY" '.count' "0" "tracker v2->v3: row untouched by either refusal -- count still 0"
server_stop

# --- v2 -> v3 success: confirm + remap witness applied ---

run_migrate "$APPS_DIR" tracker "$APP_DIR/v3.manifest.json" --confirm --witnesses "$APP_DIR/v3.witnesses.json"
assert_eq "$MIGRATE_EXIT" 0 "tracker v2->v3: confirmed + witnessed migration applies ($MIGRATE_OUT)"
got_version=$(jq -r '.app.version' "$LIVE_DIR/manifest.json")
assert_eq "$got_version" "3" "tracker v2->v3: manifest.json promoted to version 3"
assert_dir_exists "$LIVE_DIR/.snapshots" "tracker v2->v3: a pre-destructive snapshot directory exists"
snap_count=$(find "$LIVE_DIR/.snapshots" -type f 2>/dev/null | wc -l)
if [[ "$snap_count" -ge 1 ]]; then
	pass "tracker v2->v3: at least one snapshot file was written"
else
	fail "tracker v2->v3: at least one snapshot file was written" "found $snap_count files in $LIVE_DIR/.snapshots"
fi

server_start "$APPS_DIR" "$PORT" "$SERVER_LOG"

http_request GET "/apps/tracker/item/$R3"
assert_status "$HTTP_STATUS" 200 "tracker v3: row 3 survives destructive migration"
assert_json "$HTTP_BODY" '.kind' "b" "tracker v3: removed enum value c remapped to b per witness"
if printf '%s' "$HTTP_BODY" | jq -e 'has("score")' >/dev/null 2>&1; then
	fail "tracker v3: score field is dropped" "response still has a score key: $HTTP_BODY"
else
	pass "tracker v3: score field is dropped"
fi

http_request GET "/apps/tracker/item/$R1"
assert_json "$HTTP_BODY" '.kind' "a" "tracker v3: untouched enum value a is unaffected by remap"
http_request GET "/apps/tracker/item/$R2"
assert_json "$HTTP_BODY" '.kind' "b" "tracker v3: pre-existing value b is unaffected by remap"
http_request GET "/apps/tracker/item/$R5"
assert_json "$HTTP_BODY" '.parent' "$R1" "tracker v3: self-reference survives a second rebuild"
