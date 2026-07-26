package protect

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nckslvrmn/spoilarr/internal/config"
	"github.com/nckslvrmn/spoilarr/internal/imaging"
	"github.com/nckslvrmn/spoilarr/internal/plex"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

// workers bounds parallelism. The Plex client's rate limiter still caps total
// throughput, so concurrency just overlaps per-request latency instead of
// running a whole-show protect/reveal strictly serially.
const workers = 8

// runTasks runs per-item tasks with bounded concurrency and sums their Results.
// Safe with the store's single writer connection: DB writes serialize while the
// slow Plex calls run in parallel.
func (p *Processor) runTasks(tasks []func() Result) Result {
	if len(tasks) == 0 {
		return Result{}
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		agg Result
	)
	ch := make(chan func() Result)
	n := workers
	if n > len(tasks) {
		n = len(tasks)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range ch {
				res := t()
				mu.Lock()
				agg.Protected += res.Protected
				agg.Revealed += res.Revealed
				agg.Errors += res.Errors
				mu.Unlock()
			}
		}()
	}
	for _, t := range tasks {
		ch <- t
	}
	close(ch)
	wg.Wait()
	return agg
}

// parseScope returns nil for "all"/empty (every season), otherwise the set of
// season numbers from the stored JSON array.
func parseScope(scope string) map[int]bool {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "all" {
		return nil
	}
	var nums []int
	if err := json.Unmarshal([]byte(scope), &nums); err != nil || len(nums) == 0 {
		return nil
	}
	set := make(map[int]bool, len(nums))
	for _, n := range nums {
		set[n] = true
	}
	return set
}

type Processor struct {
	store *store.Store
	plex  *plex.Client
	cfg   config.Config
	log   *slog.Logger
}

func NewProcessor(st *store.Store, pc *plex.Client, cfg config.Config, log *slog.Logger) *Processor {
	return &Processor{store: st, plex: pc, cfg: cfg, log: log}
}

type Result struct {
	Protected int
	Revealed  int
	Errors    int
}

// buildResolver loads the inheritance context (global defaults + season
// overrides) for a protected item.
func (p *Processor) buildResolver(pi store.ProtectedItem) (Resolver, error) {
	defs, err := p.store.GetDefaults()
	if err != nil {
		return Resolver{}, err
	}
	overrides, err := p.store.GetSeasonOverrides(pi.ID)
	if err != nil {
		return Resolver{}, err
	}
	return NewResolver(defs, pi, overrides), nil
}

func (p *Processor) ProtectOne(ctx context.Context, pi store.ProtectedItem) (Result, error) {
	r, err := p.buildResolver(pi)
	if err != nil {
		return Result{}, err
	}
	if pi.MediaType == "movie" {
		return p.protectMovie(ctx, pi, r.ProfileFor(0))
	}
	return p.protectShow(ctx, pi, r)
}

func (p *Processor) protectShow(ctx context.Context, pi store.ProtectedItem, r Resolver) (Result, error) {
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
		if prof.SeasonPoster {
			tasks = append(tasks, func() Result {
				if err := p.protectArtOnly(ctx, pi, prof, season, "season"); err != nil {
					p.log.Warn("season poster protect failed", "season", season.RatingKey, "err", err)
					return Result{Errors: 1}
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
				if err := p.protectEpisode(ctx, pi, prof, ep); err != nil {
					p.log.Warn("episode protect failed", "episode", ep.RatingKey, "err", err)
					p.store.SetItemError(ep.RatingKey, err.Error())
					return Result{Errors: 1}
				}
				return Result{Protected: 1}
			})
		}
	}
	return p.runTasks(tasks), nil
}

func (p *Processor) protectEpisode(ctx context.Context, pi store.ProtectedItem, prof Profile, ep plex.MediaItem) error {
	if st, ok, err := p.store.GetItemState(ep.RatingKey); err != nil {
		return err
	} else if ok && st.State == store.StateProtected {
		return nil
	}

	tag := fmt.Sprintf("S%02dE%02d", ep.ParentIndex, ep.Index)
	thumbRel, err := p.backupAndTransformArt(ctx, ep.RatingKey, ep.Thumb, prof, pi.Title, tag)
	if err != nil {
		return err
	}

	fields := map[string]string{}
	if prof.Description != DescLeave {
		fields["summary.value"] = prof.MaskDescription(pi.Title, ep.ParentIndex, ep.Index)
		fields["summary.locked"] = "1"
	}
	if prof.TitleMode != TitleKeep {
		fields["title.value"] = prof.MaskTitle(ep.Title, ep.Index)
		fields["title.locked"] = "1"
	}
	if len(fields) > 0 {
		if err := p.plex.EditFields(ctx, pi.SectionID, ep.RatingKey, plex.TypeEpisode, fields); err != nil {
			return err
		}
	}

	return p.store.UpsertItemState(store.ItemState{
		RatingKey:         ep.RatingKey,
		ProtectedItemID:   pi.ID,
		ParentShowKey:     pi.RatingKey,
		MediaType:         "episode",
		OriginalSummary:   ep.Summary,
		OriginalTitle:     ep.Title,
		OriginalThumbPath: thumbRel,
		State:             store.StateProtected,
	})
}

// protectArtOnly protects only the artwork, used for season posters.
func (p *Processor) protectArtOnly(ctx context.Context, pi store.ProtectedItem, prof Profile, item plex.MediaItem, mediaType string) error {
	if st, ok, err := p.store.GetItemState(item.RatingKey); err != nil {
		return err
	} else if ok && st.State == store.StateProtected {
		return nil
	}
	tag := fmt.Sprintf("Season %d", item.Index)
	thumbRel, err := p.backupAndTransformArt(ctx, item.RatingKey, item.Thumb, prof, pi.Title, tag)
	if err != nil {
		return err
	}
	return p.store.UpsertItemState(store.ItemState{
		RatingKey:         item.RatingKey,
		ProtectedItemID:   pi.ID,
		ParentShowKey:     pi.RatingKey,
		MediaType:         mediaType,
		OriginalThumbPath: thumbRel,
		State:             store.StateProtected,
	})
}

func (p *Processor) protectMovie(ctx context.Context, pi store.ProtectedItem, prof Profile) (Result, error) {
	var res Result
	if st, ok, _ := p.store.GetItemState(pi.RatingKey); ok && st.State == store.StateProtected {
		return res, nil
	}
	movie, err := p.plex.Item(ctx, pi.RatingKey)
	if err != nil {
		return res, err
	}
	thumbRel, err := p.backupAndTransformArt(ctx, movie.RatingKey, movie.Thumb, prof, movie.Title, "")
	if err != nil {
		return res, err
	}
	if prof.Description != DescLeave {
		fields := map[string]string{
			"summary.value":  prof.MaskDescription(movie.Title, 0, 0),
			"summary.locked": "1",
		}
		if err := p.plex.EditFields(ctx, pi.SectionID, movie.RatingKey, plex.TypeMovie, fields); err != nil {
			return res, err
		}
	}
	err = p.store.UpsertItemState(store.ItemState{
		RatingKey:         movie.RatingKey,
		ProtectedItemID:   pi.ID,
		ParentShowKey:     "",
		MediaType:         "movie",
		OriginalSummary:   movie.Summary,
		OriginalTitle:     movie.Title,
		OriginalThumbPath: thumbRel,
		State:             store.StateProtected,
	})
	if err == nil {
		res.Protected++
	}
	return res, err
}

// backupAndTransformArt returns "" when there's no art or thumbnails are left
// alone.
func (p *Processor) backupAndTransformArt(ctx context.Context, ratingKey, thumb string, prof Profile, cardTitle, cardTag string) (string, error) {
	if thumb == "" || prof.Thumbnail == ThumbLeave {
		return "", nil
	}
	// The on-disk backup is the source of truth for the ORIGINAL art. Reuse it if
	// present (e.g. re-protecting after a reveal) so we never back up or blur an
	// already-spoilered image.
	rel := filepath.Join("originals", ratingKey+".jpg")
	abs := filepath.Join(p.cfg.ImagesDir(), rel)
	orig, err := os.ReadFile(abs)
	if err != nil {
		orig, _, err = p.plex.FetchImage(ctx, thumb)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(abs, orig, 0o644); err != nil {
			return "", err
		}
	}

	var out []byte
	switch prof.Thumbnail {
	case ThumbBlur:
		out, err = imaging.Blur(orig, prof.BlurSigma)
	case ThumbSolid:
		out, err = imaging.SolidColor(orig, color.RGBA{R: 20, G: 22, B: 26, A: 255})
	case ThumbGenerated:
		w, h := imaging.DimensionsOf(orig)
		out, err = imaging.GeneratedCard(w, h, cardTitle, cardTag)
	}
	if err != nil {
		return rel, err
	}
	if out != nil {
		if err := p.plex.UploadPoster(ctx, ratingKey, out); err != nil {
			return rel, err
		}
	}
	return rel, nil
}

func (p *Processor) RevealOne(ctx context.Context, pi store.ProtectedItem) (Result, error) {
	var res Result
	// For a movie the protected key is the movie itself. For a show it's the
	// children keyed by the show rating key.
	keys := []string{}
	if pi.MediaType == "movie" {
		keys = append(keys, pi.RatingKey)
	} else {
		states, err := p.store.ListItemStatesByParent(pi.RatingKey)
		if err != nil {
			return res, err
		}
		for _, st := range states {
			keys = append(keys, st.RatingKey)
		}
	}

	var tasks []func() Result
	for _, key := range keys {
		key := key
		tasks = append(tasks, func() Result {
			st, ok, err := p.store.GetItemState(key)
			if err != nil || !ok {
				return Result{}
			}
			if err := p.revealItem(ctx, pi, st); err != nil {
				p.log.Warn("reveal failed", "item", key, "err", err)
				p.store.SetItemError(key, err.Error())
				return Result{Errors: 1}
			}
			return Result{Revealed: 1}
		})
	}
	return p.runTasks(tasks), nil
}

func (p *Processor) revealItem(ctx context.Context, pi store.ProtectedItem, st store.ItemState) error {
	// Restore the original art first (uploading re-locks thumb), then the edit
	// below unlocks every field we touched, leaving the item exactly as Plex would
	// manage it on its own.
	if st.OriginalThumbPath != "" {
		orig, err := os.ReadFile(filepath.Join(p.cfg.ImagesDir(), st.OriginalThumbPath))
		if err == nil {
			if err := p.plex.UploadPoster(ctx, st.RatingKey, orig); err != nil {
				return err
			}
		}
	}

	fields := map[string]string{"thumb.locked": "0"}
	typ := plex.TypeSeason
	switch st.MediaType {
	case "episode", "movie":
		typ = plex.TypeEpisode
		if st.MediaType == "movie" {
			typ = plex.TypeMovie
		}
		fields["summary.value"] = st.OriginalSummary
		fields["summary.locked"] = "0"
		fields["title.value"] = st.OriginalTitle
		fields["title.locked"] = "0"
	}
	if err := p.plex.EditFields(ctx, pi.SectionID, st.RatingKey, typ, fields); err != nil {
		return err
	}
	return p.store.SetState(st.RatingKey, store.StateRevealed)
}
