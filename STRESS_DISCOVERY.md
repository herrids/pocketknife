# Stress Suite — Phase 0 Discovery

Read-only pass over the codebase to bind the test suite to the real APIs before
writing any tests. Baseline: `go test ./...` passes clean on
`claude/pocketknife-stress-suite-8pi3j4` (api, materialize, migrate, registry,
store, validate — all `ok`).

Two places where reality differs from the build prompt's assumptions are
called out explicitly below (§2, §8), with the adaptation I'm using instead of
inventing an API. Everything else matches.

## 1. Migration engine entry point

`migrate.Apply(ctx context.Context, reg *registry.Registry, appID string, newManifest []byte, opts Options) (*Result, error)` — `migrate/apply.go:43`.

```go
type Options struct {
    Confirm   bool
    Witnesses map[string]*Witness // keyed by stable field id
}
type Result struct {
    Changeset    *Changeset
    NoChange     bool
    SnapshotPath string // set only when a snapshot was taken
}
```

Flow: validate(new) → `Diff(old,new)` → `Classify()` → apply caller witnesses
onto ops → if empty, `NoChange` → if destructive, gate on `Confirm` then
`MissingWitnesses()` → `Snapshot()` (only if `cs.HasDestructive()`) →
`Execute()` in one transaction → on failure, `restoreInPlace` (close → file
`Restore` → reopen → re-register) and return the *original* error → on
success, promote `manifest.json` on disk and re-register, then `Prune`.

The only caller of `migrate.Apply` in the whole tree is
`cmd/pocketknife/main.go:105` (`runMigrate`). There is no HTTP migration
route — see §8.

## 2. Connection-opening paths — fewer than assumed

The prompt assumes three distinct paths (runtime, boot loader, migration
engine). Grepping `store.Open|sql.Open|_pragma` across the tree shows there is
exactly **one** production opener:

`store.Open(path string) (*Store, error)` — `store/store.go:48`, DSN
`path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"`,
`db.SetMaxOpenConns(1)`.

Every other "path" reuses it:
- **Boot loader** (`registry.Load`, `registry/boot.go:70`) calls `store.Open` once per app — this is the only path used by both `serve` and `migrate` subcommands (both call `registry.Load` first).
- **Migration engine** never opens a connection itself. `RunMigration` (`store/store.go:146`) pins a connection from the *same* pool via `s.db.Conn(ctx)` — meaningless as a distinct "path" since `MaxOpenConns(1)`.
- **Snapshot restore** (`restoreInPlace`, `migrate/apply.go:119`) closes the store and calls `store.Open` again on the same path after a byte-level file restore.

Adaptation: rather than three FK-pragma tests bound to three openers, Layer 2
has one test asserting the pragma on `store.Open` directly (already exists,
`store/store_test.go`), plus a second asserting it survives the
close→restore→reopen cycle (`restoreInPlace`'s reopen), since that's the one
other place a fresh `*sql.DB` gets created. No invented "runtime opener" or
"migration opener" API.

## 3. Schema / diff / classification types

- `schema.App`, `schema.Entity`, `schema.Field` — `schema/schema.go`. Field lookup by name (`Entity.Field`, used by the API/store logical layer) vs by stable id (`Entity.FieldByID`, `App.EntityByID`, used by the migration engine and reference resolution).
- `migrate.Diff(oldApp, newApp *schema.App) *Changeset` — `migrate/diff.go:1`. Deterministic, id-keyed; a rename is same-id+new-name, never drop+add.
- `migrate.Classify(op Operation) Class` — `migrate/classify.go:16`, pure function of `op`'s structure only; **never reads `op.Annotation`**. `(*Changeset).Classify()` overwrites every op's `Class` unconditionally.
- `Operation.Annotation Class` (`migrate/changeset.go:93`) exists only for "the (unimplemented) verification seam" per its own doc comment — there is no model-verification step to test; the only testable property is "Annotation never affects Class," which is already covered (`TestClassifyIgnoresAnnotation` in `migrate/classify_test.go`, `TestAcceptanceMisAnnotationOverridden` in `migrate/acceptance_test.go:163`). The property test in Layer 2 §5(a) below is reframed accordingly: generate random changesets, assign adversarial `Annotation` values, assert `Classify` is unaffected and `Apply`'s gating still fires from the *computed* class.
- `OpKind` (10 constants), `Class` (`""`/`safe`/`destructive`) — full enumeration in `migrate/changeset.go:29-63`.
- `isWidening(from,to)` (`migrate/classify.go:111`) — **only** `integer → real` in v1. Every other type change is destructive.

## 4. Snapshot mechanism

`migrate.Snapshot(st *store.Store, dir string) (string, error)` —
`migrate/snapshot.go:31`: `st.Checkpoint()` (`PRAGMA wal_checkpoint(TRUNCATE)`)
**then** `copyFile` (full read + `os.Create` + `io.Copy` + `out.Sync()` fsync
+ close). `Restore(snapPath, dbPath)` copies back and removes stale
`-wal`/`-shm` sidecars. `Prune(dir, keep)` keeps the most recent `keep`
(`DefaultRetention = 5`), filenames `data-<UTC nanosecond timestamp>.db` under
`<app_dir>/.snapshots/` (`SnapshotDirName`).

Ordering proof for Layer 2 §4 (snapshot-before-destructive): in
`apply.go:84-94`, `Snapshot()` is called and **fully returns** (file written,
fsynced, closed) before `Execute()` is invoked at all. This is sequential Go
code, not concurrent, so the strongest test is the crash-kill loop itself
(Layer 2 §3): if the snapshot file is written-and-synced strictly before
`Execute`'s transaction opens, then a SIGKILL at *any* point during/after
`Execute` must always find an intact, complete snapshot file on disk. A
dedicated ordering test (assert `os.Stat` on the snapshot path succeeds before
calling `Execute`, using the real `Apply` call sequence via a hook) is cheap
extra insurance but the kill loop is the real proof and will be written
either way.

## 5. Registry boot / re-derive path

`registry.Load(appsDir) (*Registry, []LoadResult, error)` —
`registry/boot.go:34`. Globs `*/manifest.json` (sorted), per app: read →
`validate.Manifest` → `materialize.Statements` → `store.Open` →
`st.ApplyDDL(stmts)` → `reg.Register`. Never stops on one bad manifest;
records `LoadResult{OK,Errors,Err}` per app.

**Nuance worth flagging (not a bug, a documented boundary):**
`materialize.Statements` only ever emits `CREATE TABLE IF NOT EXISTS` /
`CREATE UNIQUE INDEX IF NOT EXISTS` (`materialize/materialize.go:30`,
explicit package doc: "re-running boot on an unchanged manifest is a
no-op"). Boot **never reconciles** an existing `data.db` against a
manifest.json that was edited out-of-band (i.e., not through `migrate`):
`registry/boot.go:83` literally comments "Seam: migrate(storedManifest, app)
would go here before serving" — that seam is not wired up. If a manifest is
hand-edited to add/rename/retype a field and the process restarts without
going through `pocketknife migrate`, `Load` will silently register the *new*
schema model in memory while the physical table keeps the *old* shape (the
`IF NOT EXISTS` create is a no-op against the already-existing table). This
is in-contract today (the README and code comments treat "manifest and
data.db are consistent" as a v1 assumption, enforced by procedure, not by
code) — but it is exactly the kind of silent-drift hazard worth a Layer 2
edge test pinning the *current, documented* behavior, so a regression (or an
LLM that writes manifest.json directly instead of calling `migrate`) is
caught rather than discovered as silent runtime breakage. Added to the Layer
2 plan as `TestBootDoesNotReconcileDriftedManifest` (asserts today's
behavior; not a recommendation to fix it now, per the no-product-changes
constraint).

## 6. Generic CRUD/query handler

`api.NewServer(reg) http.Handler` — `api/api.go:24`. Routes (`net/http`
1.22+ pattern mux): `POST/GET /apps/{app}/{entity}`,
`GET/PATCH/DELETE /apps/{app}/{entity}/{id}`.

- Success envelopes: create/read/update return the row map directly (200/201); list returns `{"data":[...],"total":N,"limit":N,"offset":N}` (200); delete returns 204 with no body.
- Error envelope (`api/errors.go:8`): `{"error":{"code":"...","message":"...","details":[...]}}` (`details` omitted when empty).
- Status codes confirmed in code: 400 `validation_failed`/`invalid_body`/`invalid_query`, 404 `app_not_found`/`entity_not_found`/`row_not_found`, 405 `operation_disabled`, 409 `unique_violation`/`reference_conflict` (mapped from `store.ErrUnique`/`store.ErrForeignKey` via `writeStoreError`, `api/api.go:308`), 500 `internal_error` (catch-all).
- List query syntax (`api/query.go:35`): `filter=field:op:value` (repeatable, AND-only; ops `eq ne gt gte lt lte like`), `sort=field`/`sort=-field` (repeatable), `limit` (default 50, max 200), `offset`. `id`, `created_at`, `updated_at` are filterable/sortable even though not declared fields (`resolveColumn`, `api/query.go:97`).
- **LIKE case-sensitivity is already decided and pinned**, not an open question: `TestLikeIsCaseInsensitive` (`api/gate_test.go:96`) asserts SQLite's default ASCII case-insensitive `LIKE`. Layer 2 doesn't need to "decide" this — it should add one property/edge case the existing test doesn't cover: **non-ASCII case-folding**. SQLite's built-in `LIKE` only case-folds ASCII; a query like `title:like:café` vs `CAFÉ` will *not* match. That's a real, currently-undocumented edge worth one explicit test + a `STRESS_FINDINGS.md` note (not a fix).

## 7. Witness + explicit-confirm mechanism

`migrate.Witness{Kind, Coerce, Backfill, Remap}` — `migrate/witness.go:27`.
`WitnessKind` is closed: `coerce | backfill | remap`. `CoerceMode` is closed:
`truncate | round | fail`. `witnessNeeded(op)` (`witness.go:67`) determines
which kind a destructive op requires; `(*Changeset).MissingWitnesses()`
(`witness.go:89`) is what `Apply` checks before running.

CLI wiring (`cmd/pocketknife/main.go:94-103`): `-confirm` is a bare bool flag;
`-witnesses <file.json>` points at a JSON object **keyed by stable field id**,
unmarshaled straight into `opts.Witnesses map[string]*Witness`, e.g.:

```json
{
  "fld_count": {"kind": "coerce", "coerce": "truncate"},
  "fld_bio":   {"kind": "backfill", "backfill": "n/a"},
  "fld_status": {"kind": "remap", "remap": {"archived": "done"}}
}
```

## 8. Migration is CLI-only — not exposed over HTTP

This is the second real deviation from the prompt's implicit assumption.
There is no HTTP route for migration anywhere in `api/api.go` — the
mux only registers the five CRUD routes (§6). `migrate.Apply` is called
exclusively from `runMigrate` in `cmd/pocketknife/main.go`, which on any
non-nil error calls `log.Fatalf` (prints to stderr, **process exits 1**).

So the prompt's "destructive without witness/confirm → 4xx" framing doesn't
apply literally to migrations — 4xx is an HTTP concept and migrations never
go over HTTP. Adaptation (mechanical, not a product-behavior judgment call):
Layer 1's migration-gating tests drive the built `pocketknife migrate`
binary directly as a subprocess and assert:
- non-zero exit code,
- a stderr message matching `refusing:.*confirmation` or `refusing:.*witness` (the exact strings `Apply` returns, `migrate/apply.go:73,78`),
- the app's `data.db` is byte-identical before/after (refused migrations touch nothing — confirmed by `Apply`'s own gate ordering: the refusal returns before `Snapshot`/`Execute` ever run).

The CRUD/query 4xx assertions from §6 (validation, not-found, op-disabled,
conflict) remain real HTTP tests against the running server — only the
*migration* gating tests move from "curl expecting 4xx" to "subprocess
expecting exit≠0", since that's what the actual surface is.

## 9. Existing test layout / helpers

No `test/` directory exists yet — Layer 1 (`test/shell/`) is new from
scratch. Go tests live beside their packages, per convention:

| Package | Files | Helper functions found |
|---|---|---|
| `migrate` | `apply_test.go`, `acceptance_test.go`, `classify_test.go`, `changeset_test.go`, `diff_test.go`, `edge_test.go`, `execute_test.go`, `snapshot_test.go`, `witness_test.go` | `setupReg`/`seedReg` (`apply_test.go:14,37`), `openApp`/`seed` (`execute_test.go:17,34`), `parseApp` (`diff_test.go:12`) |
| `api` | `api_test.go`, `gate_test.go` | `bootFromExamples`/`copyExampleApps` (`api_test.go:22,44`), `do`/`resp.wantStatus` (`api_test.go:78,102`), `bootApp`/`listURL` (`gate_test.go:17,28`) |
| `store` | `store_test.go` | none yet (one test, `TestForeignKeysPragmaEnabled`) |
| `registry` | `boot_test.go` | — |
| `validate` | `validate_test.go` | `mustValid`/`mustInvalid`/`hasCode` (`validate_test.go:11,33,24`) |

All existing migration tests are **single-step** (v1→v2 only). No test
currently chains v1→v2→v3 with data alive and re-asserted at every step —
confirming that's genuinely new work for Layer 1, not duplicated effort.

Existing coverage that Layer 2's new tests must not duplicate:
- FK pragma on `store.Open`: `store/store_test.go` ✓ (extend, don't replace — add the reopen-after-restore case from §2).
- Native cascade/restrict/set_null enforcement: `api/gate_test.go:148` ✓ (already at the DB level via real HTTP+store, not application logic).
- Byte-exact snapshot/restore: `migrate/acceptance_test.go:99` ✓ (single failure-path case; the kill-loop is still new).
- Classify safe/destructive boundaries: `migrate/classify_test.go` ✓ but hand-enumerated table tests, **not generative** — the property test is still new, complementary work.
- Witness gating / missing-witness refusal: `migrate/witness_test.go`, `migrate/apply_test.go:92` ✓.
- Mis-annotation override: `migrate/acceptance_test.go:163`, `migrate/classify_test.go` ✓.

## 10. Property-testing library

`pgregory.net/rapid` is not in `go.mod`. Choosing **rapid** over
`testing/quick`: it has structured generators (`rapid.SliceOfN`,
`rapid.OneOf`, custom generators for the closed field-type set) and shrinking,
both of which matter for generating valid-by-construction random manifests
and adversarial row data without hand-rolling a generator framework.
`testing/quick` only generates from a type's reflect shape, which doesn't fit
"valid manifest pairs that differ by exactly the operations under test." Will
run `go get pgregory.net/rapid` when Layer 2 property tests are added.

## 11. Adversarial data — one closed-type-set gap

The prompt's adversarial data list includes a "JSON escape valve." Pocketknife
v1's closed type set (`schema/schema.go`) is exactly
`text|integer|real|boolean|datetime|enum|reference` — there is **no JSON/blob
type**. There is nothing to bind that adversarial case to; the closest
analogue is arbitrary unicode/control characters inside a `text` field, which
is already in scope as "unicode" in the same list. No invented field type;
this case is dropped from the data-preservation property test's dimensions
and noted once here instead of silently fabricated.

Self-references (a `reference` field whose `target` is its own entity) are
not forbidden by `validate/semantic.go`'s `unresolved_reference` check
(`app.EntityByID(f.Target) == nil` — an entity can resolve to itself), so that
adversarial case is real and stays in scope.

## 12. Plan adjustments arising from this discovery

1. Layer 2 FK-pragma test targets the one real opener (`store.Open`) at both call sites that matter (fresh boot, post-restore reopen) instead of three invented paths.
2. Layer 1's migration negative/gating tests drive the `pocketknife migrate` CLI as a subprocess (exit code + stderr + byte-identical `data.db`), not curl/4xx; HTTP 4xx assertions stay for the CRUD/query surface only.
3. The "model-supplied annotation disagrees with diff" property test is reframed around the real (inert) `Annotation` seam: generate adversarial annotations, assert they never affect `Classify` or `Apply`'s gating.
4. Add one Layer 2 edge test pinning today's no-reconciliation boot behavior on a drifted manifest (§5) — documents a real boundary without changing product code.
5. Add one Layer 2 edge test for non-ASCII `LIKE` case-folding (§6) — a real, currently-untested edge of an already-pinned decision.
6. Drop "JSON escape valve" from the adversarial data dimensions (no such type exists); keep self-references and unicode.
7. `pgregory.net/rapid` is the property-testing library; added to `go.mod` when Layer 2 lands.

No gap found rises to "stop and wait" — both deviations (§2, §8) are
mechanical (test against the real call site / real CLI surface) rather than
product-behavior judgment calls, so proceeding directly to Layer 1 + Layer 2
implementation.
