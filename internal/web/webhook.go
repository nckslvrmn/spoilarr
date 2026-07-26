package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// plexWebhook is the subset of the Plex webhook payload spoilarr acts on.
type plexWebhook struct {
	Event    string `json:"event"`
	Metadata struct {
		Type                 string `json:"type"`
		RatingKey            string `json:"ratingKey"`
		GrandparentRatingKey string `json:"grandparentRatingKey"` // show, for an episode
		GrandparentTitle     string `json:"grandparentTitle"`
	} `json:"Metadata"`
}

// handlePlexWebhook receives Plex's multipart webhook. On library.new for an
// episode of a protected show it protects immediately; on media.scrobble it
// re-runs reconcile so an on_watched policy reveals the just-watched episode.
// Work is done in the background so Plex gets a fast 200.
func (s *Server) handlePlexWebhook(w http.ResponseWriter, r *http.Request) {
	// Optional shared secret guards the unauthenticated webhook endpoint.
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

	// Only these events are actionable.
	if hook.Event != "library.new" && hook.Event != "media.scrobble" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// The protected item is the show (grandparent) for an episode, else the item.
	showKey := hook.Metadata.GrandparentRatingKey
	if showKey == "" {
		showKey = hook.Metadata.RatingKey
	}
	s.log.Info("webhook", "event", hook.Event, "type", hook.Metadata.Type, "show", hook.Metadata.GrandparentTitle)

	go s.reconcileFromWebhook(showKey)
	w.WriteHeader(http.StatusOK)
}

// reconcileFromWebhook reconciles a single protected show/movie in the background.
func (s *Server) reconcileFromWebhook(showKey string) {
	pi, ok, err := s.store.GetProtectedItem(showKey)
	if err != nil || !ok {
		return // not a protected show — ignore
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
