package protect

import (
	"strings"

	"github.com/nckslvrmn/spoilarr/internal/store"
)

// PolicyInherit marks a show/season reveal policy as "inherit from the level
// above" rather than an explicit choice.
const PolicyInherit = "inherit"

// Resolver computes the effective spoiler profile and reveal policy for a given
// season, walking the inheritance chain: season override → show → global default.
type Resolver struct {
	defProfile Profile
	defPolicy  string
	defDays    int

	showProfileRaw string
	showPolicy     string
	showDays       int

	overrides map[int]store.SeasonOverride
}

// NewResolver builds a Resolver from the global defaults, a protected item, and
// its season overrides.
func NewResolver(d store.Defaults, pi store.ProtectedItem, overrides map[int]store.SeasonOverride) Resolver {
	pol := d.RevealPolicy
	if pol == "" || pol == PolicyInherit {
		pol = RevealNever
	}
	return Resolver{
		defProfile:     ParseProfile(d.Profile),
		defPolicy:      pol,
		defDays:        d.RevealDays,
		showProfileRaw: pi.Profile,
		showPolicy:     pi.RevealPolicy,
		showDays:       pi.RevealDays,
		overrides:      overrides,
	}
}

// ProfileFor returns the effective spoiler profile for a season number.
func (r Resolver) ProfileFor(season int) Profile {
	if o, ok := r.overrides[season]; ok && o.Profile != nil && strings.TrimSpace(*o.Profile) != "" {
		return ParseProfile(*o.Profile)
	}
	return r.showProfile()
}

// PolicyFor returns the effective reveal policy and day count for a season.
func (r Resolver) PolicyFor(season int) (string, int) {
	if o, ok := r.overrides[season]; ok && o.RevealPolicy != nil && *o.RevealPolicy != "" && *o.RevealPolicy != PolicyInherit {
		return *o.RevealPolicy, o.RevealDays
	}
	return r.showPol()
}

func (r Resolver) showProfile() Profile {
	if strings.TrimSpace(r.showProfileRaw) == "" {
		return r.defProfile
	}
	return ParseProfile(r.showProfileRaw)
}

func (r Resolver) showPol() (string, int) {
	if r.showPolicy == "" || r.showPolicy == PolicyInherit {
		return r.defPolicy, r.defDays
	}
	return r.showPolicy, r.showDays
}
