package api_test

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/api"
	"pocketknife/registry"
)

// listURL builds a properly URL-encoded list path for an entity from filter
// terms (field:op:value) and is the safe way to pass values containing spaces or
// LIKE wildcards.
func listURL(appEntity string, filters ...string) string {
	v := url.Values{}
	for _, f := range filters {
		v.Add("filter", f)
	}
	return "/apps/" + appEntity + "?" + v.Encode()
}

// bootApp writes a single custom manifest into a temp apps dir and boots a
// server over it. Used by gate tests that need schema shapes the example apps
// do not cover (e.g. cascade / restrict references).
func bootApp(t *testing.T, appID, manifest string) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, results, err := registry.Load(root)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app %s failed to load: errors=%v err=%v", r.ManifestPath, r.Errors, r.Err)
		}
	}
	srv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(func() {
		srv.Close()
		reg.Close()
	})
	return srv
}

// TestQuerySevenOperators exercises every v1 filter operator end-to-end plus an
// AND-combined filter, against reading_tracker's integer `rating` and text
// `title`. This pins the full v1 query surface as part of the Phase 1 gate.
func TestQuerySevenOperators(t *testing.T) {
	_, srv, _ := bootFromExamples(t)

	for i, rating := range []int{1, 2, 3, 4, 5} {
		title := string(rune('a' + i)) // a, b, c, d, e
		do(t, srv, "POST", "/apps/reading_tracker/book",
			map[string]any{"title": title, "rating": rating}).wantStatus(t, 201)
	}

	total := func(filters ...string) int {
		r := do(t, srv, "GET", listURL("reading_tracker/book", filters...), nil).wantStatus(t, 200)
		return int(r.body["total"].(float64))
	}

	cases := []struct {
		name    string
		filters []string
		want    int
	}{
		{"eq", []string{"rating:eq:3"}, 1},
		{"ne", []string{"rating:ne:3"}, 4},
		{"gt", []string{"rating:gt:3"}, 2},
		{"gte", []string{"rating:gte:3"}, 3},
		{"lt", []string{"rating:lt:3"}, 2},
		{"lte", []string{"rating:lte:3"}, 3},
		{"like", []string{"title:like:a"}, 1},
		{"and", []string{"rating:gte:2", "rating:lte:4"}, 3},
	}
	for _, c := range cases {
		if got := total(c.filters...); got != c.want {
			t.Errorf("%s (%v): total = %d, want %d", c.name, c.filters, got, c.want)
		}
	}
}

// TestLikeIsCaseInsensitive documents and pins the chosen LIKE semantics:
// SQLite's default ASCII case-insensitive matching. This is an intentional v1
// decision, asserted here rather than left incidental.
func TestLikeIsCaseInsensitive(t *testing.T) {
	_, srv, _ := bootFromExamples(t)

	do(t, srv, "POST", "/apps/reading_tracker/book",
		map[string]any{"title": "The Go Programming Language"}).wantStatus(t, 201)

	matches := func(pattern string) int {
		r := do(t, srv, "GET", listURL("reading_tracker/book", "title:like:"+pattern), nil).wantStatus(t, 200)
		return int(r.body["total"].(float64))
	}

	// Lowercase pattern matches the mixed-case title (ASCII case-insensitive).
	if got := matches("the go%"); got != 1 {
		t.Errorf("lowercase prefix like: total = %d, want 1 (LIKE should be case-insensitive)", got)
	}
	// Uppercase pattern likewise matches.
	if got := matches("%LANGUAGE"); got != 1 {
		t.Errorf("uppercase suffix like: total = %d, want 1", got)
	}
	// A non-matching substring returns nothing.
	if got := matches("%python%"); got != 0 {
		t.Errorf("non-matching like: total = %d, want 0", got)
	}
}

// TestLikeCaseFoldingIsASCIIOnly documents the precise boundary of the
// case-insensitivity pinned by TestLikeIsCaseInsensitive above: SQLite's default
// LIKE only folds case for ASCII A-Z/a-z. Per SQLite's own documentation, 'a' LIKE
// 'A' is true but a non-ASCII letter's accented form is compared case-sensitively.
// This is not a bug to fix -- it is the real, intentional behavior of the
// "no ICU extension loaded" choice, pinned here so a future change in that
// behavior is a deliberate decision rather than a silent regression.
func TestLikeCaseFoldingIsASCIIOnly(t *testing.T) {
	_, srv, _ := bootFromExamples(t)

	do(t, srv, "POST", "/apps/reading_tracker/book",
		map[string]any{"title": "café"}).wantStatus(t, 201)

	matches := func(pattern string) int {
		r := do(t, srv, "GET", listURL("reading_tracker/book", "title:like:"+pattern), nil).wantStatus(t, 200)
		return int(r.body["total"].(float64))
	}

	// The exact stored form always matches.
	if got := matches("café"); got != 1 {
		t.Errorf("exact-case like: total = %d, want 1", got)
	}
	// Only the ASCII letters case-fold: an uppercase ASCII prefix still matches the
	// lowercase-accented suffix unchanged.
	if got := matches("CAFé"); got != 1 {
		t.Errorf("ASCII-only-uppercase like (accented letter untouched): total = %d, want 1 (ASCII c/a/f should still fold)", got)
	}
	// The accented letter's own case is NOT folded: 'é' LIKE 'É' is false, so an
	// otherwise-identical pattern with the accented letter uppercased does not match.
	if got := matches("cafÉ"); got != 0 {
		t.Errorf("non-ASCII-letter-case like: total = %d, want 0 (SQLite's default LIKE does not case-fold beyond ASCII)", got)
	}
}

// fkManifest declares one parent with three children, one per onDelete action,
// so cascade / restrict / set_null can each be proven natively enforced.
const fkManifest = `{
  "app": { "id": "fk_app", "name": "FK App", "version": 1 },
  "entities": [
    { "id": "ent_parent", "name": "parent", "fields": [
      { "id": "fld_parent_label", "name": "label", "type": "text", "required": true }
    ]},
    { "id": "ent_casc", "name": "casc", "fields": [
      { "id": "fld_casc_name", "name": "name", "type": "text", "required": true },
      { "id": "fld_casc_ref", "name": "parent", "type": "reference", "target": "ent_parent", "onDelete": "cascade" }
    ]},
    { "id": "ent_rstr", "name": "rstr", "fields": [
      { "id": "fld_rstr_name", "name": "name", "type": "text", "required": true },
      { "id": "fld_rstr_ref", "name": "parent", "type": "reference", "target": "ent_parent", "onDelete": "restrict" }
    ]},
    { "id": "ent_snul", "name": "snul", "fields": [
      { "id": "fld_snul_name", "name": "name", "type": "text", "required": true },
      { "id": "fld_snul_ref", "name": "parent", "type": "reference", "target": "ent_parent", "onDelete": "set_null" }
    ]}
  ]
}`

// TestReferenceIntegrityIsNativelyEnforced proves all three onDelete actions are
// enforced by SQLite's foreign keys (not application logic): deleting a parent
// cascades to its cascade-children, is blocked by a restrict-child, and nulls a
// set_null child's reference.
func TestReferenceIntegrityIsNativelyEnforced(t *testing.T) {
	srv := bootApp(t, "fk_app", fkManifest)

	mkParent := func() string {
		p := do(t, srv, "POST", "/apps/fk_app/parent", map[string]any{"label": "p"}).wantStatus(t, 201)
		return p.body["id"].(string)
	}
	mkChild := func(entity, parentID string) string {
		c := do(t, srv, "POST", "/apps/fk_app/"+entity,
			map[string]any{"name": "c", "parent": parentID}).wantStatus(t, 201)
		return c.body["id"].(string)
	}

	// cascade: deleting the parent removes the child.
	pCasc := mkParent()
	cCasc := mkChild("casc", pCasc)
	do(t, srv, "DELETE", "/apps/fk_app/parent/"+pCasc, nil).wantStatus(t, 204)
	do(t, srv, "GET", "/apps/fk_app/casc/"+cCasc, nil).wantStatus(t, 404)

	// restrict: deleting a referenced parent is blocked by the DB (409).
	pRstr := mkParent()
	mkChild("rstr", pRstr)
	do(t, srv, "DELETE", "/apps/fk_app/parent/"+pRstr, nil).wantStatus(t, 409)
	// The parent still exists because the delete was refused.
	do(t, srv, "GET", "/apps/fk_app/parent/"+pRstr, nil).wantStatus(t, 200)

	// set_null: deleting the parent nulls the child's reference, child survives.
	pSnul := mkParent()
	cSnul := mkChild("snul", pSnul)
	do(t, srv, "DELETE", "/apps/fk_app/parent/"+pSnul, nil).wantStatus(t, 204)
	got := do(t, srv, "GET", "/apps/fk_app/snul/"+cSnul, nil).wantStatus(t, 200)
	if got.body["parent"] != nil {
		t.Fatalf("set_null reference not nulled: %v", got.body["parent"])
	}
}
