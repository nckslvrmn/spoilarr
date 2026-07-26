package store

import (
	"database/sql"
	"errors"
)

const (
	StateProtected = "protected"
	StateRevealed  = "revealed"
)

// ItemState holds the original metadata for one episode/season/movie so a reveal
// can restore it exactly.
type ItemState struct {
	RatingKey         string
	ProtectedItemID   int64
	ParentShowKey     string
	MediaType         string // episode | season | movie
	OriginalSummary   string
	OriginalTitle     string
	OriginalThumbPath string // relative to images/
	State             string
	LastError         string
}

// UpsertItemState writes original_* only on first insert, so a re-run never
// overwrites a good backup with already-spoilered values.
func (s *Store) UpsertItemState(it ItemState) error {
	_, err := s.db.Exec(
		`INSERT INTO item_state
		   (rating_key, protected_item_id, parent_show_key, media_type,
		    original_summary, original_title, original_thumb_path, state, protected_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(rating_key) DO UPDATE SET
		    state       = excluded.state,
		    last_error  = NULL,
		    updated_at  = datetime('now')`,
		it.RatingKey, it.ProtectedItemID, it.ParentShowKey, it.MediaType,
		it.OriginalSummary, it.OriginalTitle, it.OriginalThumbPath, it.State,
	)
	return err
}

func (s *Store) GetItemState(ratingKey string) (ItemState, bool, error) {
	var it ItemState
	var origSummary, origTitle, origThumb, lastErr sql.NullString
	err := s.db.QueryRow(
		`SELECT rating_key, protected_item_id, parent_show_key, media_type,
		        original_summary, original_title, original_thumb_path, state, last_error
		 FROM item_state WHERE rating_key = ?`, ratingKey,
	).Scan(&it.RatingKey, &it.ProtectedItemID, &it.ParentShowKey, &it.MediaType,
		&origSummary, &origTitle, &origThumb, &it.State, &lastErr)
	if errors.Is(err, sql.ErrNoRows) {
		return it, false, nil
	}
	if err != nil {
		return it, false, err
	}
	it.OriginalSummary = origSummary.String
	it.OriginalTitle = origTitle.String
	it.OriginalThumbPath = origThumb.String
	it.LastError = lastErr.String
	return it, true, nil
}

func (s *Store) SetState(ratingKey, state string) error {
	_, err := s.db.Exec(
		`UPDATE item_state
		 SET state = ?, revealed_at = CASE WHEN ? = 'revealed' THEN datetime('now') ELSE revealed_at END,
		     updated_at = datetime('now')
		 WHERE rating_key = ?`,
		state, state, ratingKey,
	)
	return err
}

func (s *Store) SetItemError(ratingKey, msg string) error {
	_, err := s.db.Exec(
		`UPDATE item_state SET last_error = ?, updated_at = datetime('now') WHERE rating_key = ?`,
		msg, ratingKey)
	return err
}

// CountStates counts protected vs revealed items under a parent show key. For a
// movie, pass its own rating key.
func (s *Store) CountStates(parentOrSelfKey string) (protected, revealed int, err error) {
	rows, err := s.db.Query(
		`SELECT state, COUNT(*) FROM item_state
		 WHERE parent_show_key = ? OR rating_key = ?
		 GROUP BY state`, parentOrSelfKey, parentOrSelfKey)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return 0, 0, err
		}
		switch state {
		case StateProtected:
			protected = n
		case StateRevealed:
			revealed = n
		}
	}
	return protected, revealed, rows.Err()
}

func (s *Store) ListItemStatesByParent(showKey string) ([]ItemState, error) {
	rows, err := s.db.Query(
		`SELECT rating_key, media_type, original_thumb_path, state
		 FROM item_state WHERE parent_show_key = ?`, showKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemState
	for rows.Next() {
		var it ItemState
		var thumb sql.NullString
		if err := rows.Scan(&it.RatingKey, &it.MediaType, &thumb, &it.State); err != nil {
			return nil, err
		}
		it.OriginalThumbPath = thumb.String
		out = append(out, it)
	}
	return out, rows.Err()
}
