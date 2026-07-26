package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// plexWebhook is the subset of the payload spoilarr acts on.
type plexWebhook struct {
	Event    string `json:"event"`
	Metadata struct {
		Type                 string `json:"type"`
		RatingKey            string `json:"ratingKey"`
		GrandparentRatingKey string `json:"grandparentRatingKey"` // the show, for an episode
		GrandparentTitle     string `json:"grandparentTitle"`
	} `json:"Metadata"`
}

// handlePlexWebhook reconciles the affected show in the background (so Plex gets
// a fast 200) on library.new and media.scrobble.
func (s *Server) handlePlexWebhook(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WebhookSecret != "" && r.URL.Query().Get("secret") != s.cfg.WebhookSecret {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		w.WriteHeader(http.StatusOK) // don't make Plex retry a payload we can't read
		return
	}
	payload := r.FormValue("payload")
	if payload == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var hook plexWebhook
	if err := json.Unmarshal([]byte(payload), &hook); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if hook.Event != "library.new" && hook.Event != "media.scrobble" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// The protected item is the show (grandparent) for an episode, else the item itself.
	showKey := hook.Metadata.GrandparentRatingKey
	if showKey == "" {
		showKey = hook.Metadata.RatingKey
	}
	s.log.Info("webhook", "event", hook.Event, "type", hook.Metadata.Type, "show", hook.Metadata.GrandparentTitle)

	go s.reconcileFromWebhook(showKey)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) reconcileFromWebhook(showKey string) {
	pi, ok, err := s.store.GetProtectedItem(showKey)
	if err != nil || !ok {
		return // not a protected show, so ignore
	}
	proc, err := s.processor()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res, err := proc.Reconcile(ctx, pi)
	if err != nil {
		s.log.Warn("webhook reconcile failed", "show", pi.Title, "err", err)
		return
	}
	if res.Protected > 0 || res.Revealed > 0 {
		s.log.Info("webhook reconciled", "show", pi.Title, "protected", res.Protected, "revealed", res.Revealed)
	}
}
