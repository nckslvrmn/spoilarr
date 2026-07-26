package protect

import (
	"strings"

	"github.com/nckslvrmn/spoilarr/internal/store"
)

// PolicyInherit marks a reveal policy as inheriting from the level above rather
// than an explicit choice.
const PolicyInherit = "inherit"

// Resolver computes the effective spoiler profile and reveal policy for a season,
// walking the inheritance chain: season override, then show, then global default.
type Resolver struct {
	defProfile Profile
	defPolicy  string
	defDays    int

	showProfileRaw string
	showPolicy     string
	showDays       int

	overrides map[int]store.SeasonOverride
}

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

func (r Resolver) ProfileFor(season int) Profile {
	if o, ok := r.overrides[season]; ok && o.Profile != nil && strings.TrimSpace(*o.Profile) != "" {
		return ParseProfile(*o.Profile)
	}
	return r.showProfile()
}

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
