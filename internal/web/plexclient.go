package web

import (
	"net/http"

	"github.com/nckslvrmn/spoilarr/internal/conn"
	"github.com/nckslvrmn/spoilarr/internal/plex"
)

// Config keys re-exported from conn for handler readability.
const (
	cfgClientID   = conn.KeyClientID
	cfgToken      = conn.KeyToken
	cfgServerURL  = conn.KeyServerURL
	cfgServerName = conn.KeyServerName
	cfgMachineID  = conn.KeyMachineID
)

// clientID returns this install's stable Plex client identifier.
func (s *Server) clientID() (string, error) { return conn.ClientID(s.store) }

// plexClient builds a Plex client from stored config.
func (s *Server) plexClient() (*plex.Client, error) { return conn.Client(s.store) }

// webhookURL returns the URL to paste into Plex's webhook settings. It prefers an
// explicit SPOILARR_PUBLIC_URL, but otherwise derives a sensible default from the
// address the user is browsing on — which the Plex server (same LAN/host) can
// almost always reach too. Only needs to be reachable by Plex, not the internet.
func (s *Server) webhookURL(r *http.Request) string {
	base := s.cfg.PublicURL
	if base == "" && r != nil {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	if base == "" {
		return ""
	}
	u := base + "/plex/webhook"
	if s.cfg.WebhookSecret != "" {
		u += "?secret=" + s.cfg.WebhookSecret
	}
	return u
}
