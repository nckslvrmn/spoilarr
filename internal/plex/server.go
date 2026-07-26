package plex

import (
	"context"
	"fmt"
)

type Identity struct {
	MachineIdentifier string
	Version           string
}

type identityResponse struct {
	MediaContainer struct {
		MachineIdentifier string `json:"machineIdentifier"`
		Version           string `json:"version"`
	} `json:"MediaContainer"`
}

// Ping validates that ServerURL + Token reach a working server by hitting
// /identity.
func (c *Client) Ping(ctx context.Context) (*Identity, error) {
	if c.ServerURL == "" {
		return nil, fmt.Errorf("ping: no server URL configured")
	}
	var out identityResponse
	if err := c.do(ctx, "GET", c.ServerURL+"/identity", nil, &out); err != nil {
		return nil, err
	}
	return &Identity{
		MachineIdentifier: out.MediaContainer.MachineIdentifier,
		Version:           out.MediaContainer.Version,
	}, nil
}
