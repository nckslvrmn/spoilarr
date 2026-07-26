package web

import (
	"net/http"
	"strings"
)

// handleLibraries lists the show/movie library sections.
func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	client, err := s.plexClient()
	if err != nil || client.Token == "" || client.ServerURL == "" {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	sections, err := client.ListSections(r.Context())
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	s.render(w, "libraries.html", map[string]any{"Sections": sections})
}

// handleSection lists the shows/movies in a section with protect toggles.
func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	client, err := s.plexClient()
	if err != nil || client.Token == "" || client.ServerURL == "" {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	sectionKey := r.PathValue("section")
	items, err := client.ListItems(r.Context(), sectionKey)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	protected, err := s.store.ProtectedKeys()
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}

	// Flatten each item into the fields the tile + protect button need, so the
	// template needs no dict helper.
	type row struct {
		RatingKey string
		SectionID string
		MediaType string
		Title     string
		Thumb     string
		Year      int
		Protected bool
	}
	rows := make([]row, 0, len(items))
	for _, it := range items {
		rows = append(rows, row{
			RatingKey: it.RatingKey,
			SectionID: sectionKey,
			MediaType: it.Type,
			Title:     it.Title,
			Thumb:     it.Thumb,
			Year:      it.Year,
			Protected: protected[it.RatingKey],
		})
	}
	s.render(w, "section.html", map[string]any{
		"SectionKey": sectionKey,
		"Rows":       rows,
	})
}

// handleProtect enrolls an item and immediately applies protection in the
// background, so checking the box actually does something visible rather than
// just waiting for the next poll.
func (s *Server) handleProtect(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ratingKey := r.FormValue("rating_key")
	if err := s.store.AddProtectedItem(ratingKey, r.FormValue("section_id"), r.FormValue("media_type"), r.FormValue("title")); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	if pi, ok, _ := s.store.GetProtectedItem(ratingKey); ok {
		s.jobs.start(ratingKey, func() string { return s.runProtect(pi) })
	}
	s.renderToggle(w, r, true)
}

// handleUnprotect reveals everything first (restoring originals while the
// backups still exist), then removes the item, all in the background.
func (s *Server) handleUnprotect(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ratingKey := r.FormValue("rating_key")
	if pi, ok, _ := s.store.GetProtectedItem(ratingKey); ok {
		s.jobs.start(ratingKey, func() string {
			msg := s.runReveal(pi)
			if err := s.store.RemoveProtectedItem(ratingKey); err != nil {
				return "reveal ok but remove failed: " + err.Error()
			}
			return msg + ", protection removed"
		})
	}
	s.renderToggle(w, r, false)
}

func (s *Server) renderToggle(w http.ResponseWriter, r *http.Request, protected bool) {
	s.render(w, "protect_button", map[string]any{
		"RatingKey": r.FormValue("rating_key"),
		"SectionID": r.FormValue("section_id"),
		"MediaType": r.FormValue("media_type"),
		"Title":     r.FormValue("title"),
		"Protected": protected,
	})
}

// handlePlexImage proxies artwork from Plex so the browser never sees the token
// and cross-origin issues are avoided. Only /library/ paths are allowed.
func (s *Server) handlePlexImage(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if !strings.HasPrefix(path, "/library/") {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	client, err := s.plexClient()
	if err != nil || client.Token == "" {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
		return
	}
	body, ct, err := client.FetchImage(r.Context(), path)
	if err != nil {
		http.Error(w, "image fetch failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(body)
}
