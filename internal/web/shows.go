package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/nckslvrmn/spoilarr/internal/plex"
)

// handleShow renders a show detail page with per-season protection controls.
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	client, err := s.plexClient()
	if err != nil || client.Token == "" || client.ServerURL == "" {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	ratingKey := r.PathValue("ratingKey")
	section := r.URL.Query().Get("section")

	item, err := client.Item(r.Context(), ratingKey)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	seasons, err := client.Children(r.Context(), ratingKey)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}

	// Current protection state → which seasons show as selected.
	pi, isProtected, _ := s.store.GetProtectedItem(ratingKey)
	wholeShow := isProtected && (pi.SeasonScope == "all" || pi.SeasonScope == "")
	selected := map[int]bool{}
	if isProtected && !wholeShow {
		var nums []int
		if json.Unmarshal([]byte(pi.SeasonScope), &nums) == nil {
			for _, n := range nums {
				selected[n] = true
			}
		}
	}

	type seasonRow struct {
		Index    int
		Title    string
		Thumb    string
		Selected bool
	}
	var rows []seasonRow
	for _, se := range seasons {
		if se.Type != "season" {
			continue
		}
		rows = append(rows, seasonRow{
			Index:    se.Index,
			Title:    se.Title,
			Thumb:    se.Thumb,
			Selected: wholeShow || selected[se.Index],
		})
	}

	s.render(w, "show.html", map[string]any{
		"RatingKey": ratingKey,
		"SectionID": section,
		"Title":     item.Title,
		"Thumb":     item.Thumb,
		"Seasons":   rows,
		"Protected": isProtected,
		"WholeShow": wholeShow,
	})
}

// handleProtectSeasons saves the season-level protection choice for a show.
func (s *Server) handleProtectSeasons(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	showKey := r.FormValue("show_key")
	section := r.FormValue("section_id")
	title := r.FormValue("title")

	// "Entire show" wins and also covers future seasons.
	if r.FormValue("whole") == "1" {
		if err := s.store.AddProtectedItem(showKey, section, "show", title); err != nil {
			s.fragmentError(w, "", err)
			return
		}
		s.store.SetSeasonScope(showKey, "all")
		s.render(w, "season_save_result", map[string]any{"Message": "Protecting the entire show (all current and future seasons)."})
		return
	}

	selected := r.Form["seasons"]
	if len(selected) == 0 {
		// Nothing selected — remove protection entirely.
		s.store.RemoveProtectedItem(showKey)
		s.render(w, "season_save_result", map[string]any{"Message": "Protection removed for this show."})
		return
	}

	// How many seasons does the show have? If every one is selected, treat it as
	// whole-show ("all"); otherwise store the explicit list.
	client, err := s.plexClient()
	if err != nil {
		s.fragmentError(w, "", err)
		return
	}
	total := countSeasons(r, client, showKey)

	nums := make([]int, 0, len(selected))
	for _, v := range selected {
		if n, err := strconv.Atoi(v); err == nil {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)

	if err := s.store.AddProtectedItem(showKey, section, "show", title); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	scope := "all"
	msg := "Protecting the entire show."
	if total == 0 || len(nums) < total {
		blob, _ := json.Marshal(nums)
		scope = string(blob)
		msg = "Protecting selected seasons: " + scope + "."
	}
	s.store.SetSeasonScope(showKey, scope)
	s.render(w, "season_save_result", map[string]any{"Message": msg})
}

// countSeasons returns how many season children a show has.
func countSeasons(r *http.Request, client *plex.Client, showKey string) int {
	seasons, err := client.Children(r.Context(), showKey)
	if err != nil {
		return 0
	}
	n := 0
	for _, se := range seasons {
		if se.Type == "season" {
			n++
		}
	}
	return n
}
