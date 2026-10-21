package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, so the desktop build cross-compiles

	"github.com/vedansh/vans/internal/extract"
)

// MaxProgramsPerKey bounds how many programs are kept for one key.
//
// More than one is kept on purpose — see Lookup. More than a handful is
// hoarding: if four programs have all failed against the same page, the
// problem is the page or the signature, not the supply of programs.
const MaxProgramsPerKey = 4

// Entry is a stored program and everything known about how it has behaved.
type Entry struct {
	ID int64

	Program extract.Program

	// Intent is the phrasing that produced this program, kept verbatim. The
	// key holds only its hash, and a hash cannot be shown to a user or fed
	// back to a model when the program needs rewriting.
	Intent string

	// Model records which backend wrote it. A program synthesized by a weak
	// free model and one written by a frontier model are not equally
	// trustworthy, and V1 switches between them by environment variable.
	Model string

	CreatedAt time.Time

	// LastSuccessAt is the last time this program ran and verified.
	LastSuccessAt time.Time

	// Runs and Failures are lifetime totals.
	Runs     int
	Failures int

	// ConsecutiveFailures resets on any success. It is what ordering uses,
	// because a program that worked for a year and broke yesterday should not
	// outrank one that has worked every time since.
	ConsecutiveFailures int
}

// Store is the program cache.
type Store struct {
	db *sql.DB

	// Now is the clock, swappable so tests can order entries deterministically
	// rather than by sleeping.
	Now func() time.Time
}

// Open opens or creates the store at path. Use ":memory:" for an ephemeral one.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("store: create directory: %w", err)
			}
		}
	}

	// WAL keeps a read from blocking behind a write. busy_timeout covers the
	// rest: SQLite serializes writers, and without it a concurrent write
	// returns SQLITE_BUSY immediately instead of waiting.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	// One connection. This is a single-user desktop runtime, so write
	// throughput is not the constraint — and an in-memory store opened on
	// several connections would be several different databases.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, Now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

const schemaVersion = 20

func (s *Store) migrate(ctx context.Context) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if v >= schemaVersion {
		return nil
	}

	const ddl = `
CREATE TABLE IF NOT EXISTS programs (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	origin               TEXT    NOT NULL,
	signature            TEXT    NOT NULL,
	intent_hash          TEXT    NOT NULL,
	intent               TEXT    NOT NULL,
	program              TEXT    NOT NULL,
	model                TEXT    NOT NULL DEFAULT '',
	created_at           INTEGER NOT NULL,
	last_success_at      INTEGER NOT NULL DEFAULT 0,
	runs                 INTEGER NOT NULL DEFAULT 0,
	failures             INTEGER NOT NULL DEFAULT 0,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	UNIQUE (origin, signature, intent_hash, program)
);
CREATE INDEX IF NOT EXISTS programs_key ON programs (origin, signature, intent_hash);
CREATE INDEX IF NOT EXISTS programs_signature ON programs (origin, signature);

-- Routing decisions, keyed by instruction shape (D-039). The decision is kept
-- as the JSON the model produced: its shape belongs to the route package, and
-- storing it as columns would mean a migration every time a kind gains a slot.
CREATE TABLE IF NOT EXISTS routes (
	shape        TEXT    PRIMARY KEY,
	decision     TEXT    NOT NULL,
	model        TEXT    NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	uses         INTEGER NOT NULL DEFAULT 0,
	specs        TEXT    NOT NULL DEFAULT ''
);

-- Programs that act inside an application, keyed by the application and the
-- shape of the action asked for. The same idea as routes one tier along: the
-- shape is the generalisation, so one program per key, replaced rather than
-- accumulated.
CREATE TABLE IF NOT EXISTS actions (
	app          TEXT    NOT NULL,
	shape        TEXT    NOT NULL,
	program      TEXT    NOT NULL,
	model        TEXT    NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	uses         INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (app, shape)
);

-- One step inside an application's window, keyed the same way as actions. The
-- rung below a scripting dictionary: reached when an application declares
-- nothing and is not a page.
CREATE TABLE IF NOT EXISTS window_actions (
	app          TEXT    NOT NULL,
	shape        TEXT    NOT NULL,
	program      TEXT    NOT NULL,
	model        TEXT    NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	uses         INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (app, shape)
);

-- A whole drive run that finished, keyed like window_actions. The same
-- primitive one step longer: where window_actions holds a single step validated
-- against one reading of the window, a trace holds the sequence a task took and
-- every step of it is validated against a fresh reading as it is replayed.
--
-- Cached only when the run landed every step and the model said it was done, so
-- nothing in here is a guess about whether the task works. pinned is empty for a
-- trace that was fully templated and answers for any values of its shape; where a
-- literal could not be lifted out safely it holds the values the trace was
-- recorded with, and the trace answers only for those.
CREATE TABLE IF NOT EXISTS drive_traces (
	app          TEXT    NOT NULL,
	shape        TEXT    NOT NULL,
	program      TEXT    NOT NULL,
	pinned       TEXT    NOT NULL DEFAULT '',
	model        TEXT    NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	uses         INTEGER NOT NULL DEFAULT 0,
	divergences  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (app, shape)
);

CREATE TABLE IF NOT EXISTS instructions (
	text    TEXT    PRIMARY KEY,
	kind    TEXT    NOT NULL DEFAULT '',
	used_at INTEGER NOT NULL,
	done_at INTEGER NOT NULL DEFAULT 0,
	calls   INTEGER NOT NULL DEFAULT 0
);

-- What VANS has actually done, as opposed to what it has learned how to do.
-- Every other table here is a cache: losing one costs a model call. This one is
-- the record of changes made to somebody's machine, and it is what an undo has
-- to be built on — there was no way to answer "what has this thing done" before
-- it existed, which is a bad thing for a program that acts on your behalf.
CREATE TABLE IF NOT EXISTS deeds (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	at        INTEGER NOT NULL,
	kind      TEXT    NOT NULL,
	where_    TEXT    NOT NULL DEFAULT '',
	asked     TEXT    NOT NULL DEFAULT '',
	did       TEXT    NOT NULL DEFAULT '',
	changed   TEXT    NOT NULL DEFAULT '',
	verified  INTEGER NOT NULL DEFAULT 0,
	undo      TEXT    NOT NULL DEFAULT '',
	undone_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS deeds_at ON deeds (at DESC);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS routines (
	name        TEXT    PRIMARY KEY,
	shown       TEXT    NOT NULL,
	instruction TEXT    NOT NULL,
	at          INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS facts (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	text TEXT    NOT NULL UNIQUE,
	at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS app_map (
	app        TEXT    NOT NULL,
	role       TEXT    NOT NULL,
	title      TEXT    NOT NULL,
	leads      TEXT    NOT NULL,
	seen       INTEGER NOT NULL DEFAULT 1,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (app, role, title)
);

CREATE TABLE IF NOT EXISTS runs (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	at       INTEGER NOT NULL,
	app      TEXT    NOT NULL DEFAULT '',
	shape    TEXT    NOT NULL DEFAULT '',
	mode     TEXT    NOT NULL DEFAULT '',
	steps    INTEGER NOT NULL DEFAULT 0,
	replayed INTEGER NOT NULL DEFAULT 0,
	calls    INTEGER NOT NULL DEFAULT 0,
	finished INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS runs_at ON runs (at DESC);

CREATE TABLE IF NOT EXISTS clicks (
	id    INTEGER PRIMARY KEY AUTOINCREMENT,
	at    INTEGER NOT NULL,
	app   TEXT    NOT NULL,
	role  TEXT    NOT NULL,
	title TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS clicks_at ON clicks (at);

CREATE TABLE IF NOT EXISTS timed (
	key         TEXT    PRIMARY KEY,
	trigger     TEXT    NOT NULL,
	action      TEXT    NOT NULL,
	days        INTEGER NOT NULL,
	yes         INTEGER NOT NULL DEFAULT 0,
	skips       INTEGER NOT NULL DEFAULT 0,
	always      INTEGER NOT NULL DEFAULT 0,
	offered_on  TEXT    NOT NULL DEFAULT '',
	later_until INTEGER NOT NULL DEFAULT 0,
	stopped     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS habits (
	key      TEXT    PRIMARY KEY,
	name     TEXT    NOT NULL,
	steps    TEXT    NOT NULL,
	times    INTEGER NOT NULL,
	found_at INTEGER NOT NULL,
	state    TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS schedules (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	what       TEXT    NOT NULL,
	rule       TEXT    NOT NULL,
	next       INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);
`
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("store: create schema: %w", err)
	}
	// Older databases predate the column; adding it is the whole migration.
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE routes ADD COLUMN specs TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("store: add routes.specs: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE runs ADD COLUMN failed TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("store: add runs.failed: %w", err)
	}
	for _, column := range []string{"tokens_in", "tokens_out", "clicked", "next_pressed", "seen"} {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE runs ADD COLUMN `+column+` INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("store: add runs.%s: %w", column, err)
		}
	}

	for _, column := range []string{"done_at", "calls"} {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE instructions ADD COLUMN `+column+` INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("store: add instructions.%s: %w", column, err)
		}
	}

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("store: set schema version: %w", err)
	}
	return nil
}

// Lookup returns the programs cached for key, best first.
//
// It returns several rather than one because re-synthesis does not replace
// what came before. If a program fails verification, the ladder writes a
// replacement — but the original may have been right and the page merely
// showing a variant the signature collapsed. Overwriting would make the two
// variants evict each other on every alternating visit, so each visit would
// call the model. Keeping both means the second one is tried from cache, and
// the model is called only when every stored program has failed.
//
// Ordering puts the program most likely to work first: fewest consecutive
// failures, then most recently successful.
//
// A stored program that no longer validates is deleted and omitted. Programs
// outlive the schema that produced them, and replaying one the runner cannot
// execute would surface as a page failure rather than as what it is.
func (s *Store) Lookup(ctx context.Context, key Key) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, program, intent, model, created_at, last_success_at, runs, failures, consecutive_failures
		FROM programs
		WHERE origin = ? AND signature = ? AND intent_hash = ?
		ORDER BY consecutive_failures ASC, last_success_at DESC, id ASC`,
		key.Origin, key.Signature, key.IntentHash)
	if err != nil {
		return nil, fmt.Errorf("store: lookup: %w", err)
	}
	defer rows.Close()

	var out []Entry
	var stale []int64

	for rows.Next() {
		var (
			e                             Entry
			raw                           string
			createdAt, lastSuccessAtEpoch int64
		)
		if err := rows.Scan(&e.ID, &raw, &e.Intent, &e.Model, &createdAt, &lastSuccessAtEpoch,
			&e.Runs, &e.Failures, &e.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("store: scan: %w", err)
		}

		if err := json.Unmarshal([]byte(raw), &e.Program); err != nil {
			stale = append(stale, e.ID)
			continue
		}
		if err := e.Program.Validate(); err != nil {
			stale = append(stale, e.ID)
			continue
		}

		e.CreatedAt = time.Unix(createdAt, 0).UTC()
		if lastSuccessAtEpoch > 0 {
			e.LastSuccessAt = time.Unix(lastSuccessAtEpoch, 0).UTC()
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: lookup: %w", err)
	}

	for _, id := range stale {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM programs WHERE id = ?", id); err != nil {
			return out, fmt.Errorf("store: drop unusable program %d: %w", id, err)
		}
	}
	return out, nil
}

// Put stores a program under key and returns its id.
//
// Storing a program identical to one already there returns the existing id and
// leaves its statistics alone. A model handed failure reasons sometimes
// returns what it just wrote, and inserting that as a fresh entry would reset
// the failure count that proves it does not work.
func (s *Store) Put(ctx context.Context, key Key, program extract.Program, intent, model string) (int64, error) {
	if err := program.Validate(); err != nil {
		return 0, fmt.Errorf("store: refusing to cache an invalid program: %w", err)
	}

	raw, err := json.Marshal(program)
	if err != nil {
		return 0, fmt.Errorf("store: encode program: %w", err)
	}

	var existing int64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM programs
		WHERE origin = ? AND signature = ? AND intent_hash = ? AND program = ?`,
		key.Origin, key.Signature, key.IntentHash, string(raw)).Scan(&existing)
	switch {
	case err == nil:
		return existing, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("store: check for duplicate: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO programs (origin, signature, intent_hash, intent, program, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key.Origin, key.Signature, key.IntentHash, intent, string(raw), model, s.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("store: insert: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert id: %w", err)
	}

	if err := s.prune(ctx, key); err != nil {
		return id, err
	}
	return id, nil
}

// prune keeps at most MaxProgramsPerKey programs per key, dropping the worst.
func (s *Store) prune(ctx context.Context, key Key) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM programs
		WHERE id IN (
			SELECT id FROM programs
			WHERE origin = ? AND signature = ? AND intent_hash = ?
			ORDER BY consecutive_failures ASC, last_success_at DESC, id ASC
			LIMIT -1 OFFSET ?
		)`, key.Origin, key.Signature, key.IntentHash, MaxProgramsPerKey)
	if err != nil {
		return fmt.Errorf("store: prune: %w", err)
	}
	return nil
}

// RecordRun updates a program's statistics after it ran.
//
// The engine calls this after verification, not after extraction. A program
// that produced rows the verifier rejected did not succeed, and recording it
// as a success would promote it to the front of the next lookup.
func (s *Store) RecordRun(ctx context.Context, id int64, ok bool) error {
	var q string
	var args []any

	if ok {
		q = `UPDATE programs
		     SET runs = runs + 1, consecutive_failures = 0, last_success_at = ?
		     WHERE id = ?`
		args = []any{s.Now().Unix(), id}
	} else {
		q = `UPDATE programs
		     SET runs = runs + 1, failures = failures + 1, consecutive_failures = consecutive_failures + 1
		     WHERE id = ?`
		args = []any{id}
	}

	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("store: record run: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("store: no program with id %d", id)
	}
	return nil
}

// Delete removes one program.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM programs WHERE id = ?", id); err != nil {
		return fmt.Errorf("store: delete: %w", err)
	}
	return nil
}

// InvalidateSignature drops every program cached for one page shape, across
// all intents, and reports how many went.
//
// Rung 1 of the failure ladder uses this when a page has changed in a way the
// signature did not catch. It is deliberately blunt: the signature is the
// runtime's claim about what a page shape is, and once that claim is shown to
// be wrong for one intent there is no reason to trust it for the others.
func (s *Store) InvalidateSignature(ctx context.Context, origin, signature string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM programs WHERE origin = ? AND signature = ?", origin, signature)
	if err != nil {
		return 0, fmt.Errorf("store: invalidate signature: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}

// Stats summarizes the cache.
type Stats struct {
	Programs int
	Origins  int
	Runs     int
	Failures int
}

// Stats reports what the cache holds.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT origin), COALESCE(SUM(runs), 0), COALESCE(SUM(failures), 0)
		FROM programs`).Scan(&st.Programs, &st.Origins, &st.Runs, &st.Failures)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats: %w", err)
	}
	return st, nil
}
