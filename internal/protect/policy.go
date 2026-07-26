package protect

import (
	"context"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/plex"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

// Reveal policies.
const (
	RevealNever     = "never"
	RevealOnWatched = "on_watched"
	RevealAfterDays = "after_days"
	RevealManual    = "manual"
)

// Reconcile brings one protected show/movie in line with its policy: it protects
// items not yet handled and auto-reveals items whose reveal condition is now met
// (watched, or N days past air date). Items already revealed are left alone so
// they are not re-hidden. This is what the poller and webhooks drive.
func (p *Processor) Reconcile(ctx context.Context, pi store.ProtectedItem) (Result, error) {
	prof := ParseProfile(pi.Profile)
	if pi.MediaType == "movie" {
		return p.reconcileMovie(ctx, pi, prof)
	}

	var res Result
	scope := parseScope(pi.SeasonScope)
	seasons, err := p.plex.Children(ctx, pi.RatingKey)
	if err != nil {
		return res, err
	}
	for _, season := range seasons {
		if season.Type != "season" || (scope != nil && !scope[season.Index]) {
			continue
		}
		if prof.SeasonPoster {
			if err := p.protectArtOnly(ctx, pi, prof, season, "season"); err != nil {
				p.log.Warn("season poster protect failed", "season", season.RatingKey, "err", err)
			}
		}
		episodes, err := p.plex.Children(ctx, season.RatingKey)
		if err != nil {
			p.log.Warn("list episodes failed", "season", season.RatingKey, "err", err)
			res.Errors++
			continue
		}
		for _, ep := range episodes {
			if ep.Type != "episode" {
				continue
			}
			n, err := p.reconcileEpisode(ctx, pi, prof, ep)
			if err != nil {
				p.store.SetItemError(ep.RatingKey, err.Error())
				res.Errors++
				continue
			}
			res.Protected += n.Protected
			res.Revealed += n.Revealed
		}
	}
	return res, nil
}

// reconcileEpisode protects a new episode or reveals a protected one per policy.
func (p *Processor) reconcileEpisode(ctx context.Context, pi store.ProtectedItem, prof Profile, ep plex.MediaItem) (Result, error) {
	var res Result
	st, ok, err := p.store.GetItemState(ep.RatingKey)
	if err != nil {
		return res, err
	}
	reveal := shouldReveal(pi, ep)

	switch {
	case !ok:
		if reveal {
			return res, nil // already meets the reveal condition — don't hide it
		}
		if err := p.protectEpisode(ctx, pi, prof, ep); err != nil {
			return res, err
		}
		res.Protected++
	case st.State == store.StateProtected && reveal:
		if err := p.revealItem(ctx, pi, st); err != nil {
			return res, err
		}
		res.Revealed++
	}
	return res, nil
}

func (p *Processor) reconcileMovie(ctx context.Context, pi store.ProtectedItem, prof Profile) (Result, error) {
	var res Result
	movie, err := p.plex.Item(ctx, pi.RatingKey)
	if err != nil {
		return res, err
	}
	st, ok, _ := p.store.GetItemState(pi.RatingKey)
	reveal := shouldReveal(pi, *movie)
	switch {
	case !ok:
		if reveal {
			return res, nil
		}
		return p.protectMovie(ctx, pi, prof)
	case st.State == store.StateProtected && reveal:
		if err := p.revealItem(ctx, pi, st); err != nil {
			return res, err
		}
		res.Revealed++
	}
	return res, nil
}

// shouldReveal evaluates a protected item's reveal policy against an episode/movie.
func shouldReveal(pi store.ProtectedItem, m plex.MediaItem) bool {
	switch pi.RevealPolicy {
	case RevealOnWatched:
		return m.Watched()
	case RevealAfterDays:
		ad := m.AirDate()
		if ad.IsZero() {
			return false
		}
		return time.Since(ad) >= time.Duration(pi.RevealDays)*24*time.Hour
	default: // never, manual
		return false
	}
}
