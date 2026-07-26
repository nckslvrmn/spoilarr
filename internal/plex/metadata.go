package plex

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Type codes the Plex edit endpoint expects.
const (
	TypeMovie   = 1
	TypeShow    = 2
	TypeSeason  = 3
	TypeEpisode = 4
)

// Children returns seasons of a show, or episodes of a season.
func (c *Client) Children(ctx context.Context, ratingKey string) ([]MediaItem, error) {
	var out itemsResponse
	u := fmt.Sprintf("%s/library/metadata/%s/children", c.ServerURL, ratingKey)
	if err := c.do(ctx, "GET", u, nil, &out); err != nil {
		return nil, err
	}
	return out.MediaContainer.Metadata, nil
}

func (c *Client) Item(ctx context.Context, ratingKey string) (*MediaItem, error) {
	var out itemsResponse
	u := fmt.Sprintf("%s/library/metadata/%s", c.ServerURL, ratingKey)
	if err := c.do(ctx, "GET", u, nil, &out); err != nil {
		return nil, err
	}
	if len(out.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("item %s not found", ratingKey)
	}
	return &out.MediaContainer.Metadata[0], nil
}

// EditFields sets and/or locks metadata fields, e.g.
// {"summary.value": "", "summary.locked": "1"}. Locking a field is what keeps a
// later Plex metadata refresh from reverting spoilarr's changes.
func (c *Client) EditFields(ctx context.Context, sectionID, ratingKey string, typ int, fields map[string]string) error {
	q := url.Values{}
	q.Set("type", fmt.Sprintf("%d", typ))
	q.Set("id", ratingKey)
	q.Set("includeExternalMedia", "0")
	for k, v := range fields {
		q.Set(k, v)
	}
	u := fmt.Sprintf("%s/library/sections/%s/all?%s", c.ServerURL, sectionID, q.Encode())
	return c.do(ctx, "PUT", u, nil, nil)
}

// UploadPoster uploads JPEG bytes and selects them as the poster/thumb. Works
// for show, season, and episode rating keys.
func (c *Client) UploadPoster(ctx context.Context, ratingKey string, jpeg []byte) error {
	u := fmt.Sprintf("%s/library/metadata/%s/posters", c.ServerURL, ratingKey)
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(jpeg))
	if err != nil {
		return err
	}
	c.plexHeaders(req.Header)
	req.Header.Set("Content-Type", "image/jpeg")

	c.throttle()
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload poster %s: %s", ratingKey, resp.Status)
	}
	return nil
}
