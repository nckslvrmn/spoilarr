package web

import (
	"encoding/json"
	"net/http"

	"github.com/nckslvrmn/spoilarr/internal/protect"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	defs, err := s.store.GetDefaults()
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	s.render(w, "defaults.html", map[string]any{
		"Profile":      protect.ParseProfile(defs.Profile),
		"RevealPolicy": defs.RevealPolicy,
		"RevealDays":   defs.RevealDays,
	})
}

func (s *Server) handleSaveDefaults(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.SetDefaults(store.Defaults{
		Profile:      string(blob),
		RevealPolicy: r.FormValue("reveal_policy"),
		RevealDays:   parseInt(r.FormValue("reveal_days"), 0),
	}); err != nil {
		s.fragmentError(w, "", err)
		return
	}
	s.render(w, "settings_saved", map[string]any{
		"Message": "Defaults saved. Shows and seasons that inherit will use these.",
	})
}
