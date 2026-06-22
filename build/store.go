// Package build is the install/activate half of the factory: it turns a
// derived app (Phase 1) plus an optional pre-built frontend bundle into an
// openable app, and turns a new manifest version into a coherent "second
// deploy" — data migration (Phase 3) and frontend rebuild landed as one
// operation with a single rollback contract.
//
// Build-job records live in a platform-level SQLite database, separate from
// every per-app data.db: a build job describes the *act* of building an app,
// it is never part of the app's own data. The job state machine has exactly
// five states and an enforced transition map; a retry never reopens a job, it
// always creates a new one, so the history of every attempt is preserved.
package build

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"

	"pocketknife/store"
)

// State is one stage of a build job's lifecycle.
type State string

const (
	StateQueued     State = "queued"
	StateBuilding   State = "building"
	StateActivating State = "activating"
	StateReady      State = "ready"
	StateFailed     State = "failed"
)

// allowedTransitions is the enforced state machine: failed is reachable from
// every working state, ready only from activating, and both ready and failed
// are terminal — a retry always creates a new job row rather than reopening one.
var allowedTransitions = map[State][]State{
	StateQueued:     {StateBuilding, StateFailed},
	StateBuilding:   {StateActivating, StateFailed},
	StateActivating: {StateReady, StateFailed},
	StateReady:      {},
	StateFailed:     {},
}

// ErrInvalidTransition is returned by Transition when the requested move is
// not in the allowed map — a bug in the caller, not a build-time failure.
var ErrInvalidTransition = errors.New("invalid build job state transition")

// Kind distinguishes a fresh frontend install (no data change) from a second
// deploy (a new manifest version, which may carry a data migration).
type Kind string

const (
	KindInstall Kind = "install"
	KindDeploy  Kind = "deploy"
)

// Job is one build attempt, persisted in the platform database.
type Job struct {
	ID              string
	AppID           string
	Kind            Kind
	ManifestVersion int
	State           State
	Error           string
	AssetDir        string
	CreatedAt       string
	UpdatedAt       string
}

// ActiveBuild is the durable cutover pointer: the asset directory currently
// served for an app, and the job/version that produced it.
type ActiveBuild struct {
	AppID           string
	JobID           string
	AssetDir        string
	ManifestVersion int
	UpdatedAt       string
}

// Store is the platform database handle: build job history plus one active
// build pointer per app. It is wholly separate from any app's data.db.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the platform database at path.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open platform db %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping platform db %s: %w", path, err)
	}
	if err := applyDDL(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func applyDDL(db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS build_jobs (
	id TEXT PRIMARY KEY,
	app_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	manifest_version INTEGER NOT NULL,
	state TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	asset_dir TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_build_jobs_app ON build_jobs(app_id);
CREATE TABLE IF NOT EXISTS active_builds (
	app_id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	asset_dir TEXT NOT NULL,
	manifest_version INTEGER NOT NULL,
	updated_at TEXT NOT NULL
);`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("apply platform ddl: %w", err)
	}
	return nil
}

// Close releases the platform database handle.
func (s *Store) Close() error { return s.db.Close() }

// CreateJob inserts a new job in StateQueued and returns it.
func (s *Store) CreateJob(appID string, kind Kind, manifestVersion int) (*Job, error) {
	j := &Job{
		ID:              store.NewID(),
		AppID:           appID,
		Kind:            kind,
		ManifestVersion: manifestVersion,
		State:           StateQueued,
		CreatedAt:       store.NowUTC(),
	}
	j.UpdatedAt = j.CreatedAt
	_, err := s.db.Exec(
		`INSERT INTO build_jobs (id, app_id, kind, manifest_version, state, error, asset_dir, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', '', ?, ?)`,
		j.ID, j.AppID, string(j.Kind), j.ManifestVersion, string(j.State), j.CreatedAt, j.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create build job: %w", err)
	}
	return j, nil
}

// Transition moves a job to a new state, enforcing the allowed-transition map.
// detail is recorded as the job's diagnosable error message; it is only ever
// meaningful when to is StateFailed.
func (s *Store) Transition(jobID string, to State, detail string) (*Job, error) {
	j, err := s.Get(jobID)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, fmt.Errorf("transition: no build job %q", jobID)
	}
	ok := false
	for _, allowed := range allowedTransitions[j.State] {
		if allowed == to {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s -> %s (job %s)", ErrInvalidTransition, j.State, to, jobID)
	}
	now := store.NowUTC()
	if _, err := s.db.Exec(
		`UPDATE build_jobs SET state = ?, error = ?, updated_at = ? WHERE id = ?`,
		string(to), detail, now, jobID,
	); err != nil {
		return nil, fmt.Errorf("transition job %s: %w", jobID, err)
	}
	j.State = to
	j.Error = detail
	j.UpdatedAt = now
	return j, nil
}

// SetAssetDir records the build artifact directory a job produced, once it
// exists on disk and before activation promotes it.
func (s *Store) SetAssetDir(jobID, dir string) error {
	if _, err := s.db.Exec(`UPDATE build_jobs SET asset_dir = ?, updated_at = ? WHERE id = ?`, dir, store.NowUTC(), jobID); err != nil {
		return fmt.Errorf("set asset dir for job %s: %w", jobID, err)
	}
	return nil
}

// Get returns one job by id, or nil if it does not exist.
func (s *Store) Get(jobID string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, app_id, kind, manifest_version, state, error, asset_dir, created_at, updated_at
		 FROM build_jobs WHERE id = ?`, jobID)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get build job %s: %w", jobID, err)
	}
	return j, nil
}

// ListForApp returns every job for an app, most recent first.
func (s *Store) ListForApp(appID string) ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, app_id, kind, manifest_version, state, error, asset_dir, created_at, updated_at
		 FROM build_jobs WHERE app_id = ? ORDER BY rowid DESC`, appID)
	if err != nil {
		return nil, fmt.Errorf("list build jobs for %s: %w", appID, err)
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan build job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// InFlightJobs returns every job not yet in a terminal state (queued,
// building or activating) across all apps — the set boot reconciliation must
// resolve, since none of them can still be legitimately in progress after a
// fresh process start.
func (s *Store) InFlightJobs() ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, app_id, kind, manifest_version, state, error, asset_dir, created_at, updated_at
		 FROM build_jobs WHERE state IN ('queued', 'building', 'activating') ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("list in-flight build jobs: %w", err)
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan build job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// PromoteActive durably records the cutover: appID now serves assetDir, built
// by jobID at manifestVersion. This row is the source of truth boot
// reconciliation reads to avoid darkening a previously-ready app.
func (s *Store) PromoteActive(appID, jobID, assetDir string, manifestVersion int) error {
	_, err := s.db.Exec(
		`INSERT INTO active_builds (app_id, job_id, asset_dir, manifest_version, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(app_id) DO UPDATE SET job_id = excluded.job_id, asset_dir = excluded.asset_dir,
			manifest_version = excluded.manifest_version, updated_at = excluded.updated_at`,
		appID, jobID, assetDir, manifestVersion, store.NowUTC(),
	)
	if err != nil {
		return fmt.Errorf("promote active build for %s: %w", appID, err)
	}
	return nil
}

// ActiveBuildFor returns the current cutover pointer for an app, or nil if it
// has never been activated.
func (s *Store) ActiveBuildFor(appID string) (*ActiveBuild, error) {
	row := s.db.QueryRow(
		`SELECT app_id, job_id, asset_dir, manifest_version, updated_at FROM active_builds WHERE app_id = ?`, appID)
	var ab ActiveBuild
	err := row.Scan(&ab.AppID, &ab.JobID, &ab.AssetDir, &ab.ManifestVersion, &ab.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("active build for %s: %w", appID, err)
	}
	return &ab, nil
}

// rowScanner abstracts *sql.Row and *sql.Rows so scanJob serves both Get and
// the list queries.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(r rowScanner) (*Job, error) {
	var j Job
	var kind, state string
	if err := r.Scan(&j.ID, &j.AppID, &kind, &j.ManifestVersion, &state, &j.Error, &j.AssetDir, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, err
	}
	j.Kind = Kind(kind)
	j.State = State(state)
	return &j, nil
}
