// Package store owns spoilarr's durable state: a SQLite database for
// configuration and item state, plus a filesystem tree for image backups.
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go, cgo-free driver — keeps the static build simple
)

// Store wraps the SQLite connection pool.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies any
// pending migrations. SQLite is run with WAL journaling and a busy timeout so
// the poller, webhook handler, and web UI can touch it concurrently.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc's driver is not safe to use across arbitrary parallel writers;
	// a single writer connection avoids "database is locked" churn while WAL
	// still allows concurrent reads.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle for packages that build on the store.
func (s *Store) DB() *sql.DB { return s.db }

// GetConfig returns a value from the key/value config table. Missing keys
// return ("", nil) so callers can treat "unset" and "empty" the same.
func (s *Store) GetConfig(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetConfig upserts a value into the key/value config table.
func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
