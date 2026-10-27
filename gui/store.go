package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// store keeps settings and the download queue in a SQLite database under the
// user config directory, so both survive restarts.
type store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS kv (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
	id       INTEGER PRIMARY KEY,
	position INTEGER NOT NULL,
	title    TEXT NOT NULL,
	status   TEXT NOT NULL,
	error    TEXT NOT NULL DEFAULT '',
	output   TEXT NOT NULL DEFAULT ''
);`

// migrations add columns to databases created by earlier versions. Each fails
// harmlessly once its column exists.
var migrations = []string{
	`ALTER TABLE jobs ADD COLUMN account TEXT NOT NULL DEFAULT ''`,
}

func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crunchyroll-downloader"), nil
}

func openStore() (*store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "data.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	for _, m := range migrations {
		_, _ = db.Exec(m)
	}
	// The database holds the account cookies, so keep it private to the user.
	_ = os.Chmod(path, 0o600)
	return &store{db: db}, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) get(key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

func (s *store) put(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO kv (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(raw))
	return err
}

// loadSettings reads the settings, importing the settings.json written by
// earlier versions the first time.
func (s *store) loadSettings(defaults Settings) Settings {
	out := defaults
	if ok, err := s.get("settings", &out); ok && err == nil {
		return out
	}
	out = defaults
	dir, err := configDir()
	if err != nil {
		return out
	}
	legacy := filepath.Join(dir, "settings.json")
	if data, err := os.ReadFile(legacy); err == nil && json.Unmarshal(data, &out) == nil {
		if s.put("settings", out) == nil {
			_ = os.Remove(legacy)
		}
	}
	return out
}

func (s *store) saveSettings(v Settings) error { return s.put("settings", v) }

// savedAccountState is accountState as stored in the database.
type savedAccountState struct {
	ReadyAt  time.Time `json:"readyAt"`
	LastUsed time.Time `json:"lastUsed"`
	Hits     int       `json:"hits"`
	Reason   string    `json:"reason,omitempty"`
}

// loadAccountState restores the mixer's per-account rests and cooldowns, so a
// restart doesn't put a rate-limited account straight back to work.
func (s *store) loadAccountState() (map[string]*accountState, error) {
	saved := map[string]savedAccountState{}
	if _, err := s.get("accountState", &saved); err != nil {
		return nil, err
	}
	out := make(map[string]*accountState, len(saved))
	for id, v := range saved {
		out[id] = &accountState{readyAt: v.ReadyAt, lastUsed: v.LastUsed, hits: v.Hits, reason: v.Reason}
	}
	return out, nil
}

func (s *store) saveAccountState(state map[string]*accountState) error {
	saved := make(map[string]savedAccountState, len(state))
	for id, v := range state {
		saved[id] = savedAccountState{ReadyAt: v.readyAt, LastUsed: v.lastUsed, Hits: v.hits, Reason: v.reason}
	}
	return s.put("accountState", saved)
}

// loadQueue restores the saved jobs. Jobs that were mid-download when the app
// closed go back to the queue.
func (s *store) loadQueue() (jobs []*Job, paused bool, err error) {
	if _, err := s.get("paused", &paused); err != nil {
		return nil, false, err
	}
	rows, err := s.db.Query(`SELECT id, title, status, error, output, account FROM jobs ORDER BY position`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id    int64
			title string
			j     = &Job{Tracks: []Track{}}
		)
		if err := rows.Scan(&id, &title, &j.Status, &j.Error, &j.Output, &j.AccountID); err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal([]byte(title), &j.Title); err != nil {
			continue
		}
		j.ID = strconv.FormatInt(id, 10)
		if j.Status == JobDownloading || j.Status == JobMerging || j.Status == JobWaiting {
			j.Status, j.Error = JobQueued, ""
		}
		jobs = append(jobs, j)
	}
	return jobs, paused, rows.Err()
}

// saveQueue replaces the stored queue with jobs, in order.
func (s *store) saveQueue(jobs []*Job, paused bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM jobs`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO jobs (id, position, title, status, error, output, account) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, j := range jobs {
		title, err := json.Marshal(j.Title)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(j.ID, i, string(title), j.Status, j.Error, j.Output, j.AccountID); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(paused)
	if _, err := tx.Exec(`INSERT INTO kv (key, value) VALUES ('paused', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}
