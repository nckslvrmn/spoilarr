package protect

import (
	"testing"

	"github.com/nckslvrmn/spoilarr/internal/store"
)

func strptr(s string) *string { return &s }

// TestResolverInheritance checks the defaults → show → season precedence for
// both the spoiler profile and the reveal policy.
func TestResolverInheritance(t *testing.T) {
	defaults := store.Defaults{
		Profile:      `{"thumbnail":"generated","title_mode":"letters"}`,
		RevealPolicy: RevealNever,
		RevealDays:   0,
	}

	t.Run("show inherits defaults", func(t *testing.T) {
		pi := store.ProtectedItem{Profile: "", RevealPolicy: PolicyInherit}
		r := NewResolver(defaults, pi, nil)
		if got := r.ProfileFor(1).Thumbnail; got != ThumbGenerated {
			t.Fatalf("thumbnail = %q, want inherited %q", got, ThumbGenerated)
		}
		if pol, _ := r.PolicyFor(1); pol != RevealNever {
			t.Fatalf("policy = %q, want inherited %q", pol, RevealNever)
		}
	})

	t.Run("show overrides defaults", func(t *testing.T) {
		pi := store.ProtectedItem{Profile: `{"thumbnail":"blur"}`, RevealPolicy: RevealOnWatched}
		r := NewResolver(defaults, pi, nil)
		if got := r.ProfileFor(1).Thumbnail; got != ThumbBlur {
			t.Fatalf("thumbnail = %q, want show override %q", got, ThumbBlur)
		}
		if pol, _ := r.PolicyFor(1); pol != RevealOnWatched {
			t.Fatalf("policy = %q, want show override %q", pol, RevealOnWatched)
		}
	})

	t.Run("season overrides show", func(t *testing.T) {
		pi := store.ProtectedItem{Profile: `{"thumbnail":"blur"}`, RevealPolicy: RevealNever}
		overrides := map[int]store.SeasonOverride{
			2: {SeasonIndex: 2, Profile: strptr(`{"thumbnail":"solid"}`), RevealPolicy: strptr(RevealAfterDays), RevealDays: 7},
		}
		r := NewResolver(defaults, pi, overrides)

		// Season 2 uses its own override.
		if got := r.ProfileFor(2).Thumbnail; got != ThumbSolid {
			t.Fatalf("season 2 thumbnail = %q, want %q", got, ThumbSolid)
		}
		pol, days := r.PolicyFor(2)
		if pol != RevealAfterDays || days != 7 {
			t.Fatalf("season 2 policy = %q/%d, want %q/7", pol, days, RevealAfterDays)
		}
		// Season 1 (no override) falls back to the show.
		if got := r.ProfileFor(1).Thumbnail; got != ThumbBlur {
			t.Fatalf("season 1 thumbnail = %q, want show %q", got, ThumbBlur)
		}
	})
}
