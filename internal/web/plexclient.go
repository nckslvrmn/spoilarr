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

func (s *Server) clientID() (string, error) { return conn.ClientID(s.store) }

func (s *Server) plexClient() (*plex.Client, error) { return conn.Client(s.store) }

// webhookURL prefers SPOILARR_PUBLIC_URL, otherwise derives from the address the
// user is browsing on, which the Plex server on the same LAN can almost always
// reach too. It only needs to be reachable by Plex, not the internet.
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
