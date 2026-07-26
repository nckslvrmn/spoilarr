package plex

import (
	"context"
	"fmt"
	"strings"
)

type Connection struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
}

type Resource struct {
	Name             string       `json:"name"`
	Product          string       `json:"product"`
	Provides         string       `json:"provides"`
	Owned            bool         `json:"owned"`
	ClientIdentifier string       `json:"clientIdentifier"`
	AccessToken      string       `json:"accessToken"`
	Connections      []Connection `json:"connections"`
}

// ServerConn is a connection shown to the user with a friendly label instead of
// the raw plex.direct URI.
type ServerConn struct {
	URI    string // actual connect target (TLS hostname)
	Label  string // "Local network" | "Remote" | "Relay"
	Detail string // "192.168.1.61:32400"
}

type Server struct {
	Name             string
	ClientIdentifier string
	AccessToken      string
	Conns            []ServerConn // best-first (local, then remote, then relay)
}

func (s Server) Best() ServerConn {
	if len(s.Conns) == 0 {
		return ServerConn{}
	}
	return s.Conns[0]
}

// ListServers excludes shared servers, since spoilarr can only lock metadata on
// servers the account owns.
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("list servers: not authenticated")
	}
	var resources []Resource
	if err := c.do(ctx, "GET", plexTVBase+"/resources?includeHttps=1&includeRelay=1", nil, &resources); err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}

	var servers []Server
	for _, r := range resources {
		if !strings.Contains(r.Provides, "server") || !r.Owned {
			continue
		}
		servers = append(servers, Server{
			Name:             r.Name,
			ClientIdentifier: r.ClientIdentifier,
			AccessToken:      r.AccessToken,
			Conns:            labelConnections(r.Connections),
		})
	}
	return servers, nil
}

// labelConnections ranks connections and labels them, dropping the confusing raw
// URIs. Real LAN ranges (192.168/10) beat the 172.16/12 range Docker bridges use,
// so the recommended connection is one a browser can actually reach rather than a
// container bridge IP.
func labelConnections(conns []Connection) []ServerConn {
	rank := func(c Connection) int {
		category := 1 // remote direct
		switch {
		case c.Relay:
			category = 2
		case c.Local:
			category = 0
		}
		return category*10 + localScore(c)
	}
	sorted := make([]Connection, len(conns))
	copy(sorted, conns)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && rank(sorted[j]) < rank(sorted[j-1]); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	out := make([]ServerConn, 0, len(sorted))
	for _, c := range sorted {
		if c.URI == "" {
			continue
		}
		label := "Remote"
		switch {
		case c.Relay:
			label = "Relay (via plex.tv)"
		case c.Local:
			label = "Local network"
		}
		out = append(out, ServerConn{
			URI:    c.URI,
			Label:  label,
			Detail: fmt.Sprintf("%s:%d", c.Address, c.Port),
		})
	}
	return out
}

// localScore ranks local addresses so real LAN ranges beat Docker/other bridges.
// Lower is better.
func localScore(c Connection) int {
	if !c.Local {
		return 0
	}
	switch {
	case strings.HasPrefix(c.Address, "192.168."):
		return 0
	case strings.HasPrefix(c.Address, "10."):
		return 1
	case strings.HasPrefix(c.Address, "172."):
		return 3 // Docker default bridges live here
	default:
		return 2
	}
}
