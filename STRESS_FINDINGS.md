# Stress Suite — Findings Report (Phase 3)

Verdict and detailed results of the pre-generation stress suite (`STRESS_DISCOVERY.md`
→ Layer 1 black-box shell harness, `test/shell/` → Layer 2 in-process Go tests,
`migrate/*_test.go` + package-level additions). Scope: decide whether pocketknife
is safe to admit an LLM-driven generation phase (Phase 5) on top of it.

Baseline at time of writing: `go build ./...` clean, `go vet ./...` clean,
`go test ./... -short` passes every package except one intentionally-failing
property test (see Finding 1), `bash test/shell/run.sh` reports 130/148 with
exactly one intentionally-failing assertion (the same defect, found
independently).

## Verdict

**Two real product defects found. Neither blocks Phase 5, but both must be
either fixed or explicitly documented as known limitations before an LLM is
allowed to generate manifests/migrations unsupervised.** Everything else
exercised by the suite — CRUD/query semantics, validation, reference integrity
(cascade/restrict/set_null), safe-vs-destructive classification, witness/confirm
gating, snapshot-before-destructive ordering, byte-exact snapshot/restore,
multi-step (v1→v2→v3) schema evolution with live data, append-only enforcement,
FK pragma enforcement across every connection-opening path, and annotation-
irrelevance of the classifier — held up under adversarial black-box and
generative testing with no further defects surfaced.

Recommendation: gate Phase 5 on Finding 1 (data corruption risk, see below)
being fixed or fenced off (e.g. forbid integer→real widening, or switch to a
lossless widen path) before an LLM is trusted to choose "safe" migrations
autonomously, since this is precisely the kind of operation an LLM would pick
unprompted (it is classified `ClassSafe` and requires no human confirmation).
Finding 2 (crash window) is lower urgency — it requires an external SIGKILL/
power-loss at a narrow point in time — but should be fixed before pocketknife
is used unattended in any environment where process kills are plausible
(containers under OOM, orchestrators that SIGKILL on deploy, etc.).

## Findings — real product defects

### Finding 1: `integer → real` widening silently loses precision for large values

- **Classified**: `ClassSafe` (no `-confirm`, no witness, no human review point).
- **Mechanism**: `migrate/execute.go`'s `selectExpr`/`rebuildEntity` path performs
  the widen by reading the old `INTEGER` column straight into the new `REAL`
  column with no `CAST`/range check, relying on SQLite's REAL column affinity,
  which converts via a standard `int64`→`float64` conversion. That conversion is
  lossy for any magnitude beyond 2^53 (`float64`'s exact-integer range).
- **Reproductions** (three independent ones, all hitting the exact same root
  cause):
  1. `test/shell/tracker.test.sh` (Layer 1, black-box): seeds a row with
     `count = 9223372036854775807` (max int64), migrates `integer→real`,
     reads back `9223372036854776000` over the HTTP API. Intentionally left
     failing (`FAIL tracker v2: max-int64 count exact after widening to real`).
  2. `migrate/preserve_property_test.go` →
     `TestPropertyAdditiveWideningPreservesData` (Layer 2, generative): rapid
     fuzzes row count, adversarial text, the full `int64` domain, and
     chained/self-referential pointers through a real `Apply` of a
     rename+widen+add-with-default changeset. It finds the defect in 3
     generated cases and shrinks to a minimal 4-row case with
     `amount = math.MaxInt64`; the shrunk counterexample is committed as a
     rapid regression fixture (`migrate/testdata/rapid/TestPropertyAdditiveWideningPreservesData/*.fail`)
     so every future run reproduces it deterministically (`"failed after 0 tests"`).
  3. Confirmed by direct code reading of `selectExpr` in `migrate/execute.go` —
     no `CAST`, no range guard, no witness requirement for this op kind.
- **Blast radius**: any app whose schema author (human or LLM) widens an
  `integer` field to `real` — a migration explicitly designed to be the "safe,
  no-confirm-needed" path — silently corrupts any existing row whose value
  exceeds 2^53 in magnitude, with no error, no warning, and no audit trail.
  Because it's `ClassSafe`, an automated/LLM-driven pipeline would apply it
  without a human in the loop.
- **Disposition**: left failing in both layers per the stress-suite's
  no-product-changes constraint. This is a real defect, not a test-infra bug.

### Finding 2: a kill between `Execute()`'s commit and `manifest.json` promotion leaves the database and the manifest on different schema versions

- **Mechanism**: in `migrate/apply.go`, `Apply` commits `Execute()`'s rebuild
  transaction, then promotes the schema by `os.WriteFile`-ing the new
  `manifest.json` (a plain overwrite, not write-temp-then-rename), then
  re-registers the new schema in the in-memory registry. A process killed
  between the transaction commit and the `os.WriteFile` return leaves
  `data.db` physically on the new schema while `manifest.json` on disk still
  names the old version. Because `registry.Load`'s boot path
  (`registry/boot.go`) only ever issues idempotent `CREATE TABLE IF NOT EXISTS`
  DDL and never reconciles drift (see `TestBootDoesNotReconcileDriftedManifest`,
  Finding/boundary noted separately below), a subsequent boot does not detect
  or repair this — it just serves the old (now-wrong) schema model in memory
  against a table whose physical shape disagrees with it.
- **Reproduction**: `migrate/crash_test.go` →
  `TestCrashDuringMigrationLeavesConsistentState`. Drives the real
  `pocketknife migrate` binary as a subprocess over a 50,000-row dataset
  through an `integer→real` widening migration, calibrates the uninterrupted
  run's wall-clock duration, then SIGKILLs the subprocess across a swept set
  of delays tail-weighted around that duration (where the commit→write gap
  actually is), asserting after every kill that `data.db` passes
  `PRAGMA integrity_check`, no row is lost or duplicated, and `manifest.json`'s
  declared version always agrees with the physical column type it claims to
  describe. Reproduced on **three independent runs** at multiple distinct kill
  delays each time (run 1: kill_06–kill_09, kill_19, kill_22; run 2: kill_07,
  kill_08, kill_16, kill_26; run 3 — most recent, recorded for this report:
  kill_06, kill_24, kill_26), confirming this is a real, repeatable race rather
  than a one-off flake.
- **Blast radius**: requires an external kill (OOM, power loss, orchestrator
  SIGKILL, manual `kill -9`) landing in a narrow window late in a migration.
  Data itself is never corrupted or lost (`integrity_check` always passes, row
  counts always match) — the defect is purely a manifest/physical-schema
  version mismatch, which can cause the registry to serve a schema model that
  doesn't match the table it's backed by (e.g. reads/writes against a column
  the table doesn't have, or vice versa, depending on which way the operation
  went).
- **Disposition**: left failing per the stress-suite's no-product-changes
  constraint. A real defect; the standard fix is the well-known
  write-temp-then-rename pattern for `manifest.json` (atomic on POSIX rename)
  plus, if full atomicity with the DB commit is required, recording the target
  manifest bytes/version inside the same transaction Execute() commits and
  promoting the on-disk file from that on next boot.

## Findings — real but intentional boundaries (not defects)

These are documented behaviors that a stress test could mistake for bugs;
each was deliberately pinned with a test asserting *today's* behavior, per the
discovery doc, so any future change to them is a conscious decision rather
than a silent regression.

### Boundary A: non-ASCII `LIKE` is case-sensitive

- **Test**: `api/gate_test.go` → `TestLikeCaseFoldingIsASCIIOnly`.
- SQLite's built-in `LIKE` operator only case-folds ASCII `A-Z`/`a-z`. A query
  for `café` will not match a stored `CAFÉ` — the accented letter's case is
  compared literally. `TestLikeIsCaseInsensitive` already pins the ASCII case
  (e.g. `Foo` matches `foo`); this test draws the exact boundary of that
  guarantee.
- **Not a defect**: this is SQLite's documented, standard `LIKE` behavior, not
  a pocketknife bug. Worth surfacing to anyone (human or LLM) writing
  search/filter UX on top of the API, since "case-insensitive search" is not
  unicode-case-insensitive in this system without an explicit `ICU`/collation
  extension pocketknife doesn't load.

### Boundary B: boot never reconciles a hand-edited (drifted) manifest against the physical table

- **Test**: `registry/boot_test.go` → `TestBootDoesNotReconcileDriftedManifest`.
- If `manifest.json` is edited out-of-band (bypassing `pocketknife migrate`)
  to add/rename/retype a field, `registry.Load` registers the *new* schema
  model in memory without issuing any DDL to match it (its only DDL is
  idempotent `CREATE TABLE IF NOT EXISTS`, a no-op against an existing table).
  The API will then expose the new field, but reads against existing rows
  fail at the store level because the physical column doesn't exist.
- **Not a defect**: explicitly documented in `registry/boot.go`'s own comments
  as a v1 boundary ("migration is the only sanctioned path to evolve a
  schema"; the reconciliation seam is named but intentionally unwired). Test
  pins the current, documented behavior.
- **Relevance to Phase 5**: if an LLM-driven generation phase ever writes
  `manifest.json` directly instead of going through `migrate.Apply`, this
  boundary turns into a real production hazard — the API would silently start
  serving a schema that doesn't match the table. **Phase 5 must route every
  schema change through `migrate.Apply`, never through direct manifest edits.**

## Test-infrastructure issues found and fixed during suite development

These were bugs in the *test harness*, not in pocketknife, found and fixed
while building Layer 1 (`test/shell/`) before any product-level assertion was
trusted. Recorded here per the mission's instruction to distinguish test-infra
issues from real product defects — none of them represent product behavior.

1. `assert_json_null` always failed because `jq -e` exits non-zero on a
   matched `null` value (jq's own documented behavior for `-e` plus a JSON
   `null`/`false` result) — fixed in the assertion helper, not the product.
2. `run_migrate` was pointed at a single app's directory instead of the apps
   root that `registry.Load` actually globs (`*/manifest.json`) — fixed the
   harness invocation.
3. `SERVER_LOG` was not exported, crashing every test-file subshell under
   `set -u` — fixed by exporting it.
4. `server_start`/`server_stop` tracked the server via a plain shell variable
   that didn't survive the subshell boundary between `run.sh` and each
   `*.test.sh` file, so every "restart" after a migration was silently talking
   to the stale pre-migration server — fixed with a PID file plus a kill+poll
   loop.
5. `tracker.test.sh` originally asserted an upfront "refusing: ... witness"
   message for enum-narrowing without a remap witness. Reading `witness.go`
   showed `OpChangeEnum` is not gated by `witnessNeeded()` at all — it's
   enforced structurally by the rebuild's `CHECK` constraint, so the real
   failure mode is a mid-rebuild rollback, not an upfront refusal. Corrected
   the assertion to match the actual (still-safe, still-correct) mechanism —
   this was a wrong test expectation, not a product defect.
6. `assert_file_exists` used `-f` against a directory (`.snapshots`); added a
   dedicated `assert_dir_exists` and switched the snapshot-directory check to
   it.

All six were test-only changes; no product code was touched to fix any of
them, consistent with the suite's constraints.

## Invariant coverage table

| # | Invariant | Layer | Test | Result |
|---|---|---|---|---|
| 1 | Full v1 field-type set accepts adversarial data (unicode, quotes, empty strings, int64 extremes, self-references) | 1 | `tracker.test.sh` | PASS (except Finding 1, below) |
| 2 | Append-only operation gating | 1 | `applog.test.sh` | PASS |
| 3 | `onDelete` cascade/restrict/set_null enforced natively, survive entity rename | 1 | `refs.test.sh` | PASS |
| 4 | Safe vs. destructive classification matches the documented `OpKind`→`Class` table | 1, 2 | `tracker.test.sh`; `migrate/classify_test.go` | PASS |
| 5 | `-confirm` / witness gating: no-confirm refusal, confirm-without-remap rebuild failure, confirm+witness success | 1, 2 | `tracker.test.sh`; `migrate/witness_test.go`, `migrate/apply_test.go` | PASS |
| 6 | Pre-destructive snapshot is taken, unconditionally, strictly before `Execute()` | 1, 2 | `tracker.test.sh`; `migrate/apply_test.go::TestApplySnapshotPrecedesDestructiveExecution` | PASS |
| 7 | Refused migrations touch zero bytes of `data.db`/`manifest.json` | 1, 2 | `tracker.test.sh`, `refs.test.sh`; `migrate/apply_test.go::TestApplyDestructiveRefusedWithoutConfirm` | PASS |
| 8 | Failed (post-pre-flight) migrations restore from snapshot and leave the prior schema registered | 2 | `migrate/apply_test.go::TestApplyRestoresOnExecutionFailure` | PASS |
| 9 | CRUD/error envelopes match documented shapes (golden fixtures) | 1 | `golden.test.sh` | PASS |
| 10 | Generic query operators (`eq ne gt gte lt lte like`), sort, pagination | 1, 2 | `tracker.test.sh`, `refs.test.sh`; `api/gate_test.go::TestQuerySevenOperators` | PASS |
| 11 | `LIKE` is ASCII case-insensitive | 2 | `api/gate_test.go::TestLikeIsCaseInsensitive` | PASS |
| 12 | `LIKE` does **not** case-fold non-ASCII | 2 | `api/gate_test.go::TestLikeCaseFoldingIsASCIIOnly` | PASS — pins Boundary A (intentional, not a defect) |
| 13 | Foreign-key enforcement (`PRAGMA foreign_keys`) is on for every connection-opening path | 2 | `store/store_test.go::TestForeignKeysPragmaEnabled`; `migrate/snapshot_test.go::TestForeignKeysPragmaEnabledAfterRestore` | PASS |
| 14 | Boot is idempotent on an unchanged manifest; data and registry survive a process restart | 2 | `registry/boot_test.go::TestBootIsIdempotentOnUnchangedManifests`, `TestRestartPersistsDataAndRederivesRegistry` | PASS |
| 15 | Boot does not reconcile a drifted (hand-edited) manifest against the physical table | 2 | `registry/boot_test.go::TestBootDoesNotReconcileDriftedManifest` | PASS — pins Boundary B (intentional, not a defect) |
| 16 | An invalid manifest is skipped, not served; a sibling valid app is unaffected | 2 | `registry/boot_test.go::TestInvalidManifestIsSkippedNotServed` | PASS |
| 17 | Apps have physically separate database files | 2 | `registry/boot_test.go::TestAppsHavePhysicallySeparateDatabases` | PASS |
| 18 | `Classify`'s output is a pure function of operation structure, never of caller-supplied `Annotation` (hand-picked case) | 2 | `migrate/acceptance_test.go::TestAcceptanceMisAnnotationOverridden`, `migrate/classify_test.go` | PASS |
| 19 | Same property, generalized over arbitrary operations/annotations (every `OpKind`, structurally-populated fields, adversarial `Annotation` values incl. random garbage strings) | 2 (generative) | `migrate/classify_property_test.go::TestPropertyClassifyIgnoresAnnotation` | PASS (100/100 rapid checks) |
| 20 | Same property at the `Changeset.Classify()` batch level | 2 (generative) | `migrate/classify_property_test.go::TestPropertyChangesetClassifyIgnoresAnnotation` | PASS (100/100 rapid checks) |
| 21 | A combined rename+widen+add-with-default migration (all `ClassSafe`) preserves every surviving field's value exactly, across adversarial row counts, unicode text, the full `int64` domain, and chained/self-referential pointers | 2 (generative) | `migrate/preserve_property_test.go::TestPropertyAdditiveWideningPreservesData` | **FAIL — real defect, see Finding 1** |
| 22 | A SIGKILL at any point during a migration leaves `data.db` non-corrupt, with no row lost or duplicated, and the registry able to reboot | 2 (crash) | `migrate/crash_test.go::TestCrashDuringMigrationLeavesConsistentState` (data/reboot assertions) | PASS |
| 23 | A SIGKILL at any point during a migration leaves `manifest.json`'s declared version consistent with the physical schema it describes | 2 (crash) | `migrate/crash_test.go::TestCrashDuringMigrationLeavesConsistentState` (version/physical-type assertion) | **FAIL — real defect, see Finding 2** |
| 24 | Byte-exact snapshot/restore round-trip on a failed destructive migration | 1, 2 | `migrate/acceptance_test.go`; `migrate/snapshot_test.go::TestSnapshotRestoreUnderWAL` | PASS |
| 25 | Snapshot retention prunes to the last N | 2 | `migrate/snapshot_test.go::TestPruneKeepsLastN` | PASS |
| 26 | A required field with a single-quote in its default does not break generated DDL | 2 | `migrate/edge_test.go::TestEdgeAddRequiredFieldWithQuotedDefault` | PASS |
| 27 | Rename + type-change in the same migration both land in one transaction | 2 | `migrate/edge_test.go::TestEdgeRenameAndTypeChangeTogether` | PASS |
| 28 | Re-pointing a reference to a different entity rebuilds and re-enforces the new FK target | 2 | `migrate/edge_test.go::TestEdgeReferenceRetarget` | PASS |
| 29 | Coerce-round witness rounds (vs. truncates) on narrowing | 2 | `migrate/edge_test.go::TestEdgeCoerceRound` | PASS |
| 30 | Dropping a uniqueness constraint is safe, native (no rebuild), and lifts the constraint immediately | 2 | `migrate/edge_test.go::TestEdgeDropUnique` | PASS |
| 31 | A manifest with a dangling/unresolved reference target is rejected by the validator before the migration engine ever sees it | 2 | `migrate/edge_test.go::TestEdgeDanglingReferenceRejectedByValidator` | PASS |
| 32 | Self-referencing entities (a reference field targeting its own entity) are valid and survive migrations, including rebuilds | 1, 2 (generative) | `tracker.test.sh`; `migrate/preserve_property_test.go` (chained/self-ref dimension) | PASS |
| 33 | "JSON escape valve" adversarial data dimension | N/A | — | NOT APPLICABLE — pocketknife's closed field-type set (`text\|integer\|real\|boolean\|datetime\|enum\|reference`) has no JSON/blob type to bind this case to (`STRESS_DISCOVERY.md` §11); dropped, not silently fabricated |
| 34 | Migration gating surfaced as CLI exit code + stderr message + byte-identical `data.db`, not HTTP 4xx (migration is CLI-only, no HTTP route exists) | 0/1 | `tracker.test.sh`, `refs.test.sh` (subprocess exit-code/stderr assertions) | PASS — `STRESS_DISCOVERY.md` §8 adaptation, not a gap |

## Summary counts

- **Real product defects**: 2 (Finding 1: int64→float64 precision loss on
  `integer→real` widening; Finding 2: manifest/physical-schema mismatch on a
  crash during migration).
- **Intentional, documented boundaries** (not defects): 2 (non-ASCII `LIKE`
  case-folding; no drift reconciliation on boot).
- **Test-infrastructure bugs found and fixed** (harness only, zero product
  changes): 6.
- **Not applicable**: 1 (JSON escape valve — no such type exists).
- **Everything else exercised**: PASS.
