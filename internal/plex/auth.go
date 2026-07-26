package plex

import (
	"context"
	"fmt"
	"net/url"
)

// PIN is a plex.tv sign-in PIN: the user authorizes it by visiting the AuthURL
// and signing in, then polling yields an AuthToken.
type PIN struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

func (c *Client) CreatePIN(ctx context.Context) (*PIN, error) {
	var pin PIN
	if err := c.do(ctx, "POST", plexTVBase+"/pins?strong=true", nil, &pin); err != nil {
		return nil, fmt.Errorf("create pin: %w", err)
	}
	return &pin, nil
}

// CheckPIN returns a PIN whose AuthToken stays empty until the user finishes
// signing in, at which point it holds the account token.
func (c *Client) CheckPIN(ctx context.Context, id int) (*PIN, error) {
	var pin PIN
	if err := c.do(ctx, "GET", fmt.Sprintf("%s/pins/%d", plexTVBase, id), nil, &pin); err != nil {
		return nil, fmt.Errorf("check pin: %w", err)
	}
	return &pin, nil
}

// AuthURL is the plex.tv page the user visits to authorize a PIN. Signing in
// there flips the PIN so CheckPIN returns a token.
func (c *Client) AuthURL(code string) string {
	q := url.Values{}
	q.Set("clientID", c.ClientID)
	q.Set("code", code)
	q.Set("context[device][product]", Product)
	return "https://app.plex.tv/auth#?" + q.Encode()
}
