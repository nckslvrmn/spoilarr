package store

import (
	"database/sql"
	"strconv"
)

const (
	keyDefaultProfile = "default_profile"
	keyDefaultPolicy  = "default_reveal_policy"
	keyDefaultDays    = "default_reveal_days"
)

// Defaults are the global spoiler settings a show/season inherits when it has no
// override. An empty Profile falls back to the built-in profile.
type Defaults struct {
	Profile      string
	RevealPolicy string
	RevealDays   int
}

func (s *Store) GetDefaults() (Defaults, error) {
	profile, err := s.GetConfig(keyDefaultProfile)
	if err != nil {
		return Defaults{}, err
	}
	policy, err := s.GetConfig(keyDefaultPolicy)
	if err != nil {
		return Defaults{}, err
	}
	if policy == "" {
		policy = "never"
	}
	daysStr, _ := s.GetConfig(keyDefaultDays)
	days := 0
	if n, err := strconv.Atoi(daysStr); err == nil {
		days = n
	}
	return Defaults{Profile: profile, RevealPolicy: policy, RevealDays: days}, nil
}

func (s *Store) SetDefaults(d Defaults) error {
	if err := s.SetConfig(keyDefaultProfile, d.Profile); err != nil {
		return err
	}
	if err := s.SetConfig(keyDefaultPolicy, d.RevealPolicy); err != nil {
		return err
	}
	return s.SetConfig(keyDefaultDays, strconv.Itoa(d.RevealDays))
}

// SeasonOverride overrides a show's spoiler settings for one season. A nil
// Profile or RevealPolicy means inherit that dimension from the show.
type SeasonOverride struct {
	SeasonIndex  int
	Profile      *string
	RevealPolicy *string
	RevealDays   int
}

func (s *Store) GetSeasonOverrides(protectedItemID int64) (map[int]SeasonOverride, error) {
	rows, err := s.db.Query(
		`SELECT season_index, profile, reveal_policy, reveal_days
		 FROM season_overrides WHERE protected_item_id = ?`, protectedItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]SeasonOverride{}
	for rows.Next() {
		var o SeasonOverride
		var prof, pol sql.NullString
		if err := rows.Scan(&o.SeasonIndex, &prof, &pol, &o.RevealDays); err != nil {
			return nil, err
		}
		if prof.Valid {
			o.Profile = &prof.String
		}
		if pol.Valid {
			o.RevealPolicy = &pol.String
		}
		out[o.SeasonIndex] = o
	}
	return out, rows.Err()
}

func (s *Store) SetSeasonOverride(protectedItemID int64, o SeasonOverride) error {
	_, err := s.db.Exec(
		`INSERT INTO season_overrides (protected_item_id, season_index, profile, reveal_policy, reveal_days)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(protected_item_id, season_index) DO UPDATE SET
		   profile = excluded.profile, reveal_policy = excluded.reveal_policy, reveal_days = excluded.reveal_days`,
		protectedItemID, o.SeasonIndex, nullStr(o.Profile), nullStr(o.RevealPolicy), o.RevealDays)
	return err
}

func (s *Store) DeleteSeasonOverride(protectedItemID int64, seasonIndex int) error {
	_, err := s.db.Exec(
		`DELETE FROM season_overrides WHERE protected_item_id = ? AND season_index = ?`,
		protectedItemID, seasonIndex)
	return err
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
