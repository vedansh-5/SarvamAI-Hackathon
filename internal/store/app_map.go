package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type MapEdge struct {
	Role  string
	Title string
	Leads []string
	Seen  int
}

const MostMapEdges = 300

func (s *Store) LearnEdge(ctx context.Context, app string, e MapEdge) error {
	app, role, title := strings.TrimSpace(app), strings.TrimSpace(e.Role), strings.TrimSpace(e.Title)
	if app == "" || role == "" || title == "" || len(e.Leads) == 0 {
		return nil
	}
	leads, err := json.Marshal(e.Leads)
	if err != nil {
		return fmt.Errorf("store: encode map edge: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO app_map (app, role, title, leads, seen, updated_at) VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT (app, role, title) DO UPDATE SET leads = excluded.leads, seen = seen + 1, updated_at = excluded.updated_at`,
		app, role, title, string(leads), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: keep map edge: %w", err)
	}
	return nil
}

func (s *Store) KnownEdges(ctx context.Context, app string) ([]MapEdge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, title, leads, seen FROM app_map WHERE app = ? ORDER BY seen DESC, updated_at DESC LIMIT ?`,
		strings.TrimSpace(app), MostMapEdges)
	if err != nil {
		return nil, fmt.Errorf("store: read the map of %s: %w", app, err)
	}
	defer rows.Close()

	var out []MapEdge
	for rows.Next() {
		var e MapEdge
		var leads string
		if err := rows.Scan(&e.Role, &e.Title, &leads, &e.Seen); err != nil {
			return nil, fmt.Errorf("store: read map edge: %w", err)
		}
		if json.Unmarshal([]byte(leads), &e.Leads) != nil {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ForgetEdge(ctx context.Context, app, role, title string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_map WHERE app = ? AND role = ? AND title = ?`, app, role, title)
	return err
}

type Run struct {
	At       time.Time
	App      string
	Shape    string
	Mode     string
	Steps    int
	Replayed int
	Calls    int
	Finished bool
}

func (s *Store) RecordTask(ctx context.Context, r Run) error {
	finished := 0
	if r.Finished {
		finished = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (at, app, shape, mode, steps, replayed, calls, finished) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), r.App, r.Shape, r.Mode, r.Steps, r.Replayed, r.Calls, finished)
	if err != nil {
		return fmt.Errorf("store: record run: %w", err)
	}
	return nil
}

func (s *Store) RecentTasks(ctx context.Context, most int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT at, app, shape, mode, steps, replayed, calls, finished FROM runs ORDER BY id DESC LIMIT ?`, most)
	if err != nil {
		return nil, fmt.Errorf("store: read runs: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		var at int64
		var finished int
		if err := rows.Scan(&at, &r.App, &r.Shape, &r.Mode, &r.Steps, &r.Replayed, &r.Calls, &finished); err != nil {
			return nil, fmt.Errorf("store: read run: %w", err)
		}
		r.At, r.Finished = time.Unix(at, 0), finished == 1
		out = append(out, r)
	}
	return out, rows.Err()
}
