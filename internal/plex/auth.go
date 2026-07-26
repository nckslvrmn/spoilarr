package plex

import (
	"context"
	"fmt"
	"net/url"
)

// PIN represents a plex.tv sign-in PIN. The user authorizes it by visiting the
// AuthURL and signing in; polling then yields an AuthToken.
type PIN struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

// CreatePIN starts a plex.tv PIN sign-in. The returned PIN's ID and Code are
// used to build the AuthURL and to poll for completion.
func (c *Client) CreatePIN(ctx context.Context) (*PIN, error) {
	var pin PIN
	if err := c.do(ctx, "POST", plexTVBase+"/pins?strong=true", nil, &pin); err != nil {
		return nil, fmt.Errorf("create pin: %w", err)
	}
	return &pin, nil
}

// CheckPIN polls a PIN by ID. AuthToken is empty until the user completes
// sign-in in their browser, then holds the account token.
func (c *Client) CheckPIN(ctx context.Context, id int) (*PIN, error) {
	var pin PIN
	if err := c.do(ctx, "GET", fmt.Sprintf("%s/pins/%d", plexTVBase, id), nil, &pin); err != nil {
		return nil, fmt.Errorf("check pin: %w", err)
	}
	return &pin, nil
}

// AuthURL builds the plex.tv page the user visits to authorize a PIN. Signing
// in there flips the PIN so CheckPIN returns a token.
func (c *Client) AuthURL(code string) string {
	q := url.Values{}
	q.Set("clientID", c.ClientID)
	q.Set("code", code)
	q.Set("context[device][product]", Product)
	return "https://app.plex.tv/auth#?" + q.Encode()
}
