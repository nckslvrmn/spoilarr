package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/protect"
)

// handleItemSettings renders the spoiler profile + reveal policy editor for one
// protected show/movie.
func (s *Server) handleItemSettings(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	pi, ok, err := s.store.GetProtectedItem(ratingKey)
	if err != nil || !ok {
		s.render(w, "error.html", map[string]any{"Error": "not a protected item"})
		return
	}
	s.render(w, "settings.html", map[string]any{
		"Item":    pi,
		"Profile": protect.ParseProfile(pi.Profile),
	})
}

// handleSaveSettings persists the edited profile + reveal policy.
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	if err := r.ParseForm(); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	prof := protect.Profile{
		Thumbnail:    r.FormValue("thumbnail"),
		BlurSigma:    parseFloat(r.FormValue("blur_sigma"), 20),
		Description:  r.FormValue("description"),
		DescTemplate: r.FormValue("desc_template"),
		TitleMode:    r.FormValue("title_mode"),
		SeasonPoster: r.FormValue("season_poster") == "1",
	}
	blob, _ := json.Marshal(prof)

	policy := r.FormValue("reveal_policy")
	days := parseInt(r.FormValue("reveal_days"), 0)
	if err := s.store.UpdateSettings(ratingKey, string(blob), policy, days); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	s.render(w, "settings_saved", map[string]any{
		"Message": "Settings saved. New changes apply as episodes are processed — use “Re-apply” to update already-protected items.",
	})
}

// handleReapply reveals then re-protects an item so profile changes take effect
// on already-processed episodes. Backups on disk keep the originals intact.
func (s *Server) handleReapply(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	// Persist whatever is in the form first, so re-apply uses the latest settings.
	if err := r.ParseForm(); err == nil {
		prof := protect.Profile{
			Thumbnail:    r.FormValue("thumbnail"),
			BlurSigma:    parseFloat(r.FormValue("blur_sigma"), 20),
			Description:  r.FormValue("description"),
			DescTemplate: r.FormValue("desc_template"),
			TitleMode:    r.FormValue("title_mode"),
			SeasonPoster: r.FormValue("season_poster") == "1",
		}
		blob, _ := json.Marshal(prof)
		s.store.UpdateSettings(ratingKey, string(blob), r.FormValue("reveal_policy"), parseInt(r.FormValue("reveal_days"), 0))
	}

	pi, ok, err := s.store.GetProtectedItem(ratingKey)
	if err != nil || !ok {
		s.fragmentError(w, "", fmt.Errorf("item not found"))
		return
	}
	// Reveal-then-reprotect can be lengthy on a big show; run it in the background
	// and report progress via the item status fragment.
	s.jobs.start(ratingKey, func() string {
		proc, err := s.processor()
		if err != nil {
			return "error: " + err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if _, err := proc.RevealOne(ctx, pi); err != nil {
			return "re-apply failed during reveal: " + err.Error()
		}
		res, err := proc.ProtectOne(ctx, pi)
		if err != nil {
			return "re-apply failed during protect: " + err.Error()
		}
		return fmt.Sprintf("Re-applied: %d re-protected, %d errors", res.Protected, res.Errors)
	})
	s.renderItemStatus(w, ratingKey)
}

func parseFloat(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return def
}

func parseInt(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
