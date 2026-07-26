package protect

import (
	"context"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/plex"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

const (
	RevealNever     = "never"
	RevealOnWatched = "on_watched"
	RevealAfterDays = "after_days"
	RevealManual    = "manual"
)

// Reconcile protects items not yet handled and auto-reveals ones whose condition
// is now met (watched, or N days past air date). Already-revealed items are left
// alone so they aren't re-hidden. Driven by the poller and webhooks.
func (p *Processor) Reconcile(ctx context.Context, pi store.ProtectedItem) (Result, error) {
	r, err := p.buildResolver(pi)
	if err != nil {
		return Result{}, err
	}
	if pi.MediaType == "movie" {
		return p.reconcileMovie(ctx, pi, r)
	}

	scope := parseScope(pi.SeasonScope)
	seasons, err := p.plex.Children(ctx, pi.RatingKey)
	if err != nil {
		return Result{}, err
	}
	var tasks []func() Result
	for _, season := range seasons {
		if season.Type != "season" || (scope != nil && !scope[season.Index]) {
			continue
		}
		season := season
		prof := r.ProfileFor(season.Index)
		policy, days := r.PolicyFor(season.Index)
		if prof.SeasonPoster {
			tasks = append(tasks, func() Result {
				if err := p.protectArtOnly(ctx, pi, prof, season, "season"); err != nil {
					p.log.Warn("season poster protect failed", "season", season.RatingKey, "err", err)
				}
				return Result{}
			})
		}
		episodes, err := p.plex.Children(ctx, season.RatingKey)
		if err != nil {
			p.log.Warn("list episodes failed", "season", season.RatingKey, "err", err)
			continue
		}
		for _, ep := range episodes {
			if ep.Type != "episode" {
				continue
			}
			ep := ep
			tasks = append(tasks, func() Result {
				n, err := p.reconcileEpisode(ctx, pi, prof, policy, days, ep)
				if err != nil {
					p.store.SetItemError(ep.RatingKey, err.Error())
					return Result{Errors: 1}
				}
				return n
			})
		}
	}
	return p.runTasks(tasks), nil
}

func (p *Processor) reconcileEpisode(ctx context.Context, pi store.ProtectedItem, prof Profile, policy string, days int, ep plex.MediaItem) (Result, error) {
	var res Result
	st, ok, err := p.store.GetItemState(ep.RatingKey)
	if err != nil {
		return res, err
	}
	reveal := shouldReveal(policy, days, ep)

	switch {
	case !ok:
		if reveal {
			return res, nil // already meets the reveal condition, so don't hide it
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

func (p *Processor) reconcileMovie(ctx context.Context, pi store.ProtectedItem, r Resolver) (Result, error) {
	var res Result
	movie, err := p.plex.Item(ctx, pi.RatingKey)
	if err != nil {
		return res, err
	}
	policy, days := r.PolicyFor(0)
	st, ok, _ := p.store.GetItemState(pi.RatingKey)
	reveal := shouldReveal(policy, days, *movie)
	switch {
	case !ok:
		if reveal {
			return res, nil
		}
		return p.protectMovie(ctx, pi, r.ProfileFor(0))
	case st.State == store.StateProtected && reveal:
		if err := p.revealItem(ctx, pi, st); err != nil {
			return res, err
		}
		res.Revealed++
	}
	return res, nil
}

func shouldReveal(policy string, days int, m plex.MediaItem) bool {
	switch policy {
	case RevealOnWatched:
		return m.Watched()
	case RevealAfterDays:
		ad := m.AirDate()
		if ad.IsZero() {
			return false
		}
		return time.Since(ad) >= time.Duration(days)*24*time.Hour
	default: // never, manual, or inherit that resolved to those
		return false
	}
}
