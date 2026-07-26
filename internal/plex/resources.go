package plex

import (
	"context"
	"fmt"
	"strings"
)

// Connection is one reachable address for a Plex server.
type Connection struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
}

// Resource is an entry from plex.tv's resources list. spoilarr cares about the
// servers the account owns.
type Resource struct {
	Name             string       `json:"name"`
	Product          string       `json:"product"`
	Provides         string       `json:"provides"`
	Owned            bool         `json:"owned"`
	ClientIdentifier string       `json:"clientIdentifier"`
	AccessToken      string       `json:"accessToken"`
	Connections      []Connection `json:"connections"`
}

// ServerConn is a connection option presented to the user with a friendly label
// instead of a raw plex.direct URI.
type ServerConn struct {
	URI    string // actual connect target (TLS hostname)
	Label  string // "Local network" | "Remote" | "Relay"
	Detail string // "192.168.1.61:32400"
}

// Server is a discovered, owned Plex Media Server with ranked connection options.
type Server struct {
	Name             string
	ClientIdentifier string
	AccessToken      string
	Conns            []ServerConn // best-first (local, then remote, then relay)
}

// Best returns the recommended connection (first after ranking).
func (s Server) Best() ServerConn {
	if len(s.Conns) == 0 {
		return ServerConn{}
	}
	return s.Conns[0]
}

// ListServers returns the Plex Media Servers the signed-in account OWNS. Shared
// servers are excluded — spoilarr can only lock metadata on servers you own.
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

// labelConnections ranks connections (local, remote, relay) and gives each a
// friendly label + address:port detail, dropping the confusing raw URIs. Among
// local addresses, real LAN ranges (192.168/10) are preferred over the 172.16/12
// range that Docker bridge interfaces live in, so the recommended connection is
// the one a browser can actually reach — not a container bridge IP.
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

// localScore orders local addresses so real LAN ranges beat Docker/other bridge
// ranges. Lower is better. Only meaningful for local connections.
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
