package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/poller"
	"github.com/nckslvrmn/spoilarr/internal/protect"
)

func (s *Server) processor() (*protect.Processor, error) {
	client, err := s.plexClient()
	if err != nil {
		return nil, err
	}
	return protect.NewProcessor(s.store, client, s.cfg, s.log), nil
}

type protectedRow struct {
	Item      any
	Protected int
	Revealed  int
	Running   bool
}

func (s *Server) handleProtected(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProtectedItems()
	if err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	rows := make([]protectedRow, 0, len(items))
	for _, it := range items {
		prot, rev, _ := s.store.CountStates(it.RatingKey)
		running, _ := s.jobs.status(it.RatingKey)
		rows = append(rows, protectedRow{Item: it, Protected: prot, Revealed: rev, Running: running})
	}

	lastPoll, _ := s.store.GetConfig(poller.KeyLastPollAt)
	lastSummary, _ := s.store.GetConfig(poller.KeyLastPollSummary)
	s.render(w, "protected.html", map[string]any{
		"Rows":         rows,
		"PollInterval": s.cfg.PollInterval.String(),
		"LastPoll":     humanTime(lastPoll),
		"LastSummary":  lastSummary,
		"WebhookSet":   s.webhookURL(r) != "",
	})
}

func (s *Server) handleProtectNow(w http.ResponseWriter, r *http.Request) {
	s.startItemJob(w, r, "protect")
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	s.startItemJob(w, r, "reveal")
}

func (s *Server) startItemJob(w http.ResponseWriter, r *http.Request, action string) {
	r.ParseForm()
	key := r.FormValue("rating_key")
	pi, ok, err := s.store.GetProtectedItem(key)
	if err != nil || !ok {
		s.fragmentError(w, "", fmt.Errorf("item not found"))
		return
	}
	s.jobs.start(key, func() string {
		proc, err := s.processor()
		if err != nil {
			return "error: " + err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if action == "reveal" {
			res, err := proc.RevealOne(ctx, pi)
			if err != nil {
				return "reveal error: " + err.Error()
			}
			return fmt.Sprintf("Revealed %d, %d errors", res.Revealed, res.Errors)
		}
		res, err := proc.ProtectOne(ctx, pi)
		if err != nil {
			return "protect error: " + err.Error()
		}
		return fmt.Sprintf("Protected %d, %d errors", res.Protected, res.Errors)
	})
	s.renderItemStatus(w, key)
}

func (s *Server) handleItemStatus(w http.ResponseWriter, r *http.Request) {
	s.renderItemStatus(w, r.PathValue("ratingKey"))
}

func (s *Server) renderItemStatus(w http.ResponseWriter, key string) {
	running, msg := s.jobs.status(key)
	prot, rev, _ := s.store.CountStates(key)
	s.render(w, "job_status", map[string]any{
		"Key":       key,
		"Running":   running,
		"Message":   msg,
		"Protected": prot,
		"Revealed":  rev,
	})
}

func humanTime(rfc string) string {
	if rfc == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
