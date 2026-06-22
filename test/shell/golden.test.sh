#!/usr/bin/env bash
# Golden-file suite: pins the exact JSON shape of every CRUD/query success and
# error envelope. The TS client is generated from this contract, so a shape
# change must be a deliberate, reviewed diff to a checked-in fixture -- run
# with UPDATE_GOLDEN=1 to regenerate after an intentional contract change.
set -uo pipefail
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/lib.sh"

GOLDEN_DIR="$TEST_DIR/golden"

# --- success envelopes --------------------------------------------------------

body=$(jq -nc '{label:"Widget"}')
http_request POST /apps/golden/thing "$body"
assert_status "$HTTP_STATUS" 201 "golden: create succeeds"
ID=$(printf '%s' "$HTTP_BODY" | jq -r '.id')
golden_check "$GOLDEN_DIR/create_success.json" "$HTTP_BODY" "golden: create success envelope shape"

http_request GET "/apps/golden/thing/$ID"
assert_status "$HTTP_STATUS" 200 "golden: read succeeds"
golden_check "$GOLDEN_DIR/read_success.json" "$HTTP_BODY" "golden: read success envelope shape"

body=$(jq -nc '{label:"Gadget", qty:7, status:"closed"}')
http_request POST /apps/golden/thing "$body"
assert_status "$HTTP_STATUS" 201 "golden: create second row for list"

http_request GET "/apps/golden/thing?sort=label&limit=10"
assert_status "$HTTP_STATUS" 200 "golden: list succeeds"
golden_check "$GOLDEN_DIR/list_success.json" "$HTTP_BODY" "golden: list success envelope shape"

body=$(jq -nc '{qty:5}')
http_request PATCH "/apps/golden/thing/$ID" "$body"
assert_status "$HTTP_STATUS" 200 "golden: update succeeds"
golden_check "$GOLDEN_DIR/update_success.json" "$HTTP_BODY" "golden: update success envelope shape"

http_request DELETE "/apps/golden/thing/$ID"
assert_status "$HTTP_STATUS" 204 "golden: delete succeeds"
assert_eq "$HTTP_BODY" "" "golden: delete response body is empty"

# --- error envelopes ----------------------------------------------------------

body=$(jq -nc '{qty:1}')
http_request POST /apps/golden/thing "$body"
assert_status "$HTTP_STATUS" 400 "golden: create without required field fails validation"
golden_check "$GOLDEN_DIR/error_validation.json" "$HTTP_BODY" "golden: validation error envelope shape"

http_request GET "/apps/golden/thing/does-not-exist"
assert_status "$HTTP_STATUS" 404 "golden: read of an unknown id fails"
normalized_404=$(printf '%s' "$HTTP_BODY" | jq -c '.error.message = "<NORMALIZED>"')
golden_check "$GOLDEN_DIR/error_not_found.json" "$normalized_404" "golden: not-found error envelope shape"

body=$(jq -nc '{label:"Conflict"}')
http_request POST /apps/golden/thing "$body"
assert_status "$HTTP_STATUS" 201 "golden: create row to provoke a unique conflict"
http_request POST /apps/golden/thing "$body"
assert_status "$HTTP_STATUS" 409 "golden: duplicate unique value fails"
golden_check "$GOLDEN_DIR/error_conflict.json" "$HTTP_BODY" "golden: conflict error envelope shape"
