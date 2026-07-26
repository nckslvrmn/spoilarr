package store

import "fmt"

// migrations are applied in order. Each entry runs exactly once; the index+1 is
// recorded in schema_version. Never edit or reorder an existing migration once
// it has shipped — append a new one instead.
var migrations = []string{
	// 0001 — config key/value store.
	`CREATE TABLE config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,

	// 0002 — shows/movies the user opted into protecting, with their spoiler
	// profile and reveal policy. profile is a JSON blob (see internal/protect).
	`CREATE TABLE protected_items (
		id            INTEGER PRIMARY KEY,
		rating_key    TEXT NOT NULL UNIQUE,
		section_id    TEXT NOT NULL,
		media_type    TEXT NOT NULL,             -- 'show' | 'movie'
		title         TEXT NOT NULL,
		profile       TEXT NOT NULL DEFAULT '{}',-- JSON spoiler profile
		reveal_policy TEXT NOT NULL DEFAULT 'never', -- never|on_watched|after_days|manual
		reveal_days   INTEGER NOT NULL DEFAULT 0,
		season_scope  TEXT NOT NULL DEFAULT 'all',-- 'all' or JSON array of season numbers
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
	);`,

	// 0003 — per-item state for everything we actually acted on (episodes,
	// seasons, movies). Holds the original metadata so reveal restores exactly.
	`CREATE TABLE item_state (
		id                  INTEGER PRIMARY KEY,
		rating_key          TEXT NOT NULL UNIQUE,
		protected_item_id   INTEGER REFERENCES protected_items(id) ON DELETE CASCADE,
		parent_show_key     TEXT,
		media_type          TEXT NOT NULL,        -- 'episode' | 'season' | 'movie'
		original_summary    TEXT,
		original_title      TEXT,
		original_thumb_path TEXT,                 -- backup image on disk, relative to images/
		state               TEXT NOT NULL DEFAULT 'protected', -- 'protected' | 'revealed'
		last_error          TEXT,
		protected_at        TEXT,
		revealed_at         TEXT,
		updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
	);`,
	`CREATE INDEX idx_item_state_parent ON item_state(parent_show_key);`,
	`CREATE INDEX idx_item_state_state  ON item_state(state);`,

	// 0004 — per-season overrides. A NULL profile/reveal_policy means "inherit
	// from the show" (which may itself inherit the global defaults).
	`CREATE TABLE season_overrides (
		id                INTEGER PRIMARY KEY,
		protected_item_id INTEGER NOT NULL REFERENCES protected_items(id) ON DELETE CASCADE,
		season_index      INTEGER NOT NULL,
		profile           TEXT,
		reveal_policy     TEXT,
		reveal_days       INTEGER NOT NULL DEFAULT 0,
		UNIQUE(protected_item_id, season_index)
	);`,
}

// migrate applies any migrations not yet recorded in schema_version.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}

	var current int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}

	for i := current; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %04d: %w", i+1, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %04d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
