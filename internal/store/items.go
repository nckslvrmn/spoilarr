package store

import "database/sql"

// ProtectedItem is a show or movie the user opted into protecting, along with
// its spoiler profile and reveal policy.
type ProtectedItem struct {
	ID           int64
	RatingKey    string
	SectionID    string
	MediaType    string // "show" | "movie"
	Title        string
	Profile      string // JSON spoiler profile (see internal/protect)
	RevealPolicy string // never | on_watched | after_days | manual
	RevealDays   int
	SeasonScope  string // "all" or JSON array of season numbers
	Enabled      bool
}

// AddProtectedItem inserts a protected item with default profile/policy, or does
// nothing if the rating key is already protected. Returns the row id.
func (s *Store) AddProtectedItem(ratingKey, sectionID, mediaType, title string) error {
	// New items inherit the global defaults: an empty profile and the "inherit"
	// reveal policy both resolve to the defaults until the show is overridden.
	_, err := s.db.Exec(
		`INSERT INTO protected_items (rating_key, section_id, media_type, title, profile, reveal_policy)
		 VALUES (?, ?, ?, ?, '', 'inherit')
		 ON CONFLICT(rating_key) DO NOTHING`,
		ratingKey, sectionID, mediaType, title,
	)
	return err
}

// RemoveProtectedItem deletes a protected item and (via cascade) its item_state.
func (s *Store) RemoveProtectedItem(ratingKey string) error {
	_, err := s.db.Exec(`DELETE FROM protected_items WHERE rating_key = ?`, ratingKey)
	return err
}

// SetSeasonScope updates which seasons of a protected show are covered. scope is
// "all" or a JSON array of season numbers.
func (s *Store) SetSeasonScope(ratingKey, scope string) error {
	_, err := s.db.Exec(
		`UPDATE protected_items SET season_scope = ?, updated_at = datetime('now') WHERE rating_key = ?`,
		scope, ratingKey)
	return err
}

// UpdateSettings saves a protected item's spoiler profile (JSON) and reveal
// policy.
func (s *Store) UpdateSettings(ratingKey, profileJSON, revealPolicy string, revealDays int) error {
	_, err := s.db.Exec(
		`UPDATE protected_items
		 SET profile = ?, reveal_policy = ?, reveal_days = ?, updated_at = datetime('now')
		 WHERE rating_key = ?`,
		profileJSON, revealPolicy, revealDays, ratingKey)
	return err
}

// ProtectedKeys returns the set of protected rating keys for quick membership
// checks when rendering a section.
func (s *Store) ProtectedKeys() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT rating_key FROM protected_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys[k] = true
	}
	return keys, rows.Err()
}

// GetProtectedItem returns a single protected item by rating key.
func (s *Store) GetProtectedItem(ratingKey string) (ProtectedItem, bool, error) {
	var it ProtectedItem
	err := s.db.QueryRow(
		`SELECT id, rating_key, section_id, media_type, title, profile,
		        reveal_policy, reveal_days, season_scope, enabled
		 FROM protected_items WHERE rating_key = ?`, ratingKey,
	).Scan(&it.ID, &it.RatingKey, &it.SectionID, &it.MediaType, &it.Title,
		&it.Profile, &it.RevealPolicy, &it.RevealDays, &it.SeasonScope, &it.Enabled)
	if err == sql.ErrNoRows {
		return it, false, nil
	}
	return it, err == nil, err
}

// ListProtectedItems returns all protected items, newest first.
func (s *Store) ListProtectedItems() ([]ProtectedItem, error) {
	rows, err := s.db.Query(
		`SELECT id, rating_key, section_id, media_type, title, profile,
		        reveal_policy, reveal_days, season_scope, enabled
		 FROM protected_items ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProtectedItem
	for rows.Next() {
		var it ProtectedItem
		if err := rows.Scan(&it.ID, &it.RatingKey, &it.SectionID, &it.MediaType, &it.Title,
			&it.Profile, &it.RevealPolicy, &it.RevealDays, &it.SeasonScope, &it.Enabled); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
