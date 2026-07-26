package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/protect"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

type seasonView struct {
	Index      int
	Title      string
	Overridden bool
	Profile    protect.Profile
	Policy     string
	Days       int
}

func (s *Server) handleItemSettings(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	pi, ok, err := s.store.GetProtectedItem(ratingKey)
	if err != nil || !ok {
		s.render(w, "error.html", map[string]any{"Error": "not a protected item"})
		return
	}

	defs, _ := s.store.GetDefaults()
	overrides, _ := s.store.GetSeasonOverrides(pi.ID)
	resolver := protect.NewResolver(defs, pi, overrides)

	inheriting := pi.Profile == "" && (pi.RevealPolicy == "" || pi.RevealPolicy == protect.PolicyInherit)
	showProfile := resolver.ProfileFor(-1) // -1 has no override, so it resolves to show/default
	showPolicy, showDays := resolver.PolicyFor(-1)

	data := map[string]any{
		"Item":         pi,
		"Inheriting":   inheriting,
		"Profile":      showProfile,
		"RevealPolicy": showPolicy,
		"RevealDays":   showDays,
	}

	if pi.MediaType == "show" {
		if client, err := s.plexClient(); err == nil {
			if seasons, err := client.Children(r.Context(), ratingKey); err == nil {
				scope := parseScopeSet(pi.SeasonScope)
				var rows []seasonView
				for _, se := range seasons {
					if se.Type != "season" {
						continue
					}
					if scope != nil && !scope[se.Index] {
						continue
					}
					prof := resolver.ProfileFor(se.Index)
					pol, days := resolver.PolicyFor(se.Index)
					_, overridden := overrides[se.Index]
					rows = append(rows, seasonView{
						Index: se.Index, Title: se.Title, Overridden: overridden,
						Profile: prof, Policy: pol, Days: days,
					})
				}
				data["Seasons"] = rows
			}
		}
	}
	s.render(w, "settings.html", data)
}

// parseScopeSet mirrors the processor's parseScope for the settings view.
func parseScopeSet(scope string) map[int]bool {
	if scope == "" || scope == "all" {
		return nil
	}
	var nums []int
	if json.Unmarshal([]byte(scope), &nums) != nil || len(nums) == 0 {
		return nil
	}
	set := map[int]bool{}
	for _, n := range nums {
		set[n] = true
	}
	return set
}

func profileFromForm(r *http.Request, prefix string) protect.Profile {
	return protect.Profile{
		Thumbnail:    r.FormValue(prefix + "thumbnail"),
		BlurSigma:    parseFloat(r.FormValue(prefix+"blur_sigma"), 20),
		Description:  r.FormValue(prefix + "description"),
		DescTemplate: r.FormValue(prefix + "desc_template"),
		TitleMode:    r.FormValue(prefix + "title_mode"),
		SeasonPoster: r.FormValue(prefix+"season_poster") == "1",
	}
}

// handleSaveSettings either stores an explicit profile or, when source=inherit,
// resets the show to follow the global defaults.
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	if err := r.ParseForm(); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	if r.FormValue("source") == "inherit" {
		if err := s.store.UpdateSettings(ratingKey, "", protect.PolicyInherit, 0); err != nil {
			s.fragmentError(w, "", err)
			return
		}
		s.render(w, "settings_saved", map[string]any{"Message": "This show now inherits the global defaults."})
		return
	}
	blob, _ := json.Marshal(profileFromForm(r, ""))
	if err := s.store.UpdateSettings(ratingKey, string(blob), r.FormValue("reveal_policy"), parseInt(r.FormValue("reveal_days"), 0)); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	s.render(w, "settings_saved", map[string]any{
		"Message": "Settings saved. Use “Re-apply” to update already-protected episodes.",
	})
}

func (s *Server) handleSaveSeasonOverride(w http.ResponseWriter, r *http.Request) {
	pi, ok, _ := s.store.GetProtectedItem(r.PathValue("ratingKey"))
	if !ok {
		s.fragmentError(w, "", fmt.Errorf("item not found"))
		return
	}
	idx, _ := strconv.Atoi(r.PathValue("index"))
	r.ParseForm()
	blob, _ := json.Marshal(profileFromForm(r, ""))
	profStr := string(blob)
	policy := r.FormValue("reveal_policy")
	if err := s.store.SetSeasonOverride(pi.ID, store.SeasonOverride{
		SeasonIndex:  idx,
		Profile:      &profStr,
		RevealPolicy: &policy,
		RevealDays:   parseInt(r.FormValue("reveal_days"), 0),
	}); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	s.render(w, "settings_saved", map[string]any{"Message": fmt.Sprintf("Season %d override saved.", idx)})
}

func (s *Server) handleResetSeasonOverride(w http.ResponseWriter, r *http.Request) {
	pi, ok, _ := s.store.GetProtectedItem(r.PathValue("ratingKey"))
	if !ok {
		s.fragmentError(w, "", fmt.Errorf("item not found"))
		return
	}
	idx, _ := strconv.Atoi(r.PathValue("index"))
	if err := s.store.DeleteSeasonOverride(pi.ID, idx); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	s.render(w, "settings_saved", map[string]any{"Message": fmt.Sprintf("Season %d back to inheriting the show.", idx)})
}

// handleReapply reveals then re-protects so profile changes reach
// already-processed episodes. The on-disk backups keep the originals intact.
func (s *Server) handleReapply(w http.ResponseWriter, r *http.Request) {
	ratingKey := r.PathValue("ratingKey")
	pi, ok, err := s.store.GetProtectedItem(ratingKey)
	if err != nil || !ok {
		s.fragmentError(w, "", fmt.Errorf("item not found"))
		return
	}
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
