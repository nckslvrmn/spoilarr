// Package store owns spoilarr's durable state: a SQLite database for
// configuration and item state, plus a filesystem tree for image backups.
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go, cgo-free driver, so the binary stays static
)

type Store struct {
	db *sql.DB
}

// Open applies any pending migrations. WAL journaling plus a busy timeout let
// the poller, webhook handler, and web UI touch the DB concurrently.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc's driver isn't safe across arbitrary parallel writers, so a single
	// writer connection avoids "database is locked" churn while WAL still allows
	// concurrent reads.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

// GetConfig treats a missing key and an empty value the same: both return "".
func (s *Store) GetConfig(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

func (s *Store) DeleteConfig(keys ...string) error {
	for _, k := range keys {
		if _, err := s.db.Exec(`DELETE FROM config WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return nil
}

// ResetData clears protected items, per-item state, season overrides, and the
// global defaults. The Plex connection and client id are deliberately left
// alone so setup doesn't have to run again.
func (s *Store) ResetData() error {
	stmts := []string{
		`DELETE FROM season_overrides`,
		`DELETE FROM item_state`,
		`DELETE FROM protected_items`,
		`DELETE FROM config WHERE key IN ('default_profile','default_reveal_policy','default_reveal_days','last_poll_at','last_poll_summary')`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
