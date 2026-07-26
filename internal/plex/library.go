package plex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Section is a Plex library section (e.g. "TV Shows", "Movies").
type Section struct {
	Key   string `json:"key"`
	Type  string `json:"type"` // "show" | "movie" | others we ignore
	Title string `json:"title"`
}

// MediaItem is any library entry: a show, season, episode, or movie. Not every
// field is populated for every type — seasons/episodes carry Index (their
// number), episodes carry Summary/ViewCount and grandparent (show) references.
type MediaItem struct {
	RatingKey            string `json:"ratingKey"`
	Type                 string `json:"type"` // show | season | episode | movie
	Title                string `json:"title"`
	Summary              string `json:"summary"`
	Year                 int    `json:"year"`
	Thumb                string `json:"thumb"`      // relative art path, needs token to fetch
	ChildCount           int    `json:"childCount"` // seasons for a show
	LeafCount            int    `json:"leafCount"`  // episodes under a show/season
	Index                int    `json:"index"`      // season or episode number
	ParentIndex          int    `json:"parentIndex"`
	ViewCount            int    `json:"viewCount"` // >0 means watched
	ParentRatingKey      string `json:"parentRatingKey"`
	GrandparentRatingKey string `json:"grandparentRatingKey"`
	GrandparentTitle     string `json:"grandparentTitle"`
	OriginallyAvailableAt string `json:"originallyAvailableAt"` // "2001-01-28"
}

// Watched reports whether the item has been played.
func (m MediaItem) Watched() bool { return m.ViewCount > 0 }

// AirDate parses the original air date, or returns zero time if unavailable.
func (m MediaItem) AirDate() time.Time {
	t, err := time.Parse("2006-01-02", m.OriginallyAvailableAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

type sectionsResponse struct {
	MediaContainer struct {
		Directory []Section `json:"Directory"`
	} `json:"MediaContainer"`
}

type itemsResponse struct {
	MediaContainer struct {
		Metadata []MediaItem `json:"Metadata"`
	} `json:"MediaContainer"`
}

// ListSections returns the library sections, filtered to the show and movie
// types spoilarr can protect.
func (c *Client) ListSections(ctx context.Context) ([]Section, error) {
	var out sectionsResponse
	if err := c.do(ctx, "GET", c.ServerURL+"/library/sections", nil, &out); err != nil {
		return nil, err
	}
	var sections []Section
	for _, s := range out.MediaContainer.Directory {
		if s.Type == "show" || s.Type == "movie" {
			sections = append(sections, s)
		}
	}
	return sections, nil
}

// ListItems returns the top-level items (shows or movies) in a section.
func (c *Client) ListItems(ctx context.Context, sectionKey string) ([]MediaItem, error) {
	var out itemsResponse
	url := fmt.Sprintf("%s/library/sections/%s/all", c.ServerURL, sectionKey)
	if err := c.do(ctx, "GET", url, nil, &out); err != nil {
		return nil, err
	}
	return out.MediaContainer.Metadata, nil
}

// FetchImage streams an image from the Plex server (art paths are relative and
// require the token). Returns the raw bytes and content type.
func (c *Client) FetchImage(ctx context.Context, artPath string) ([]byte, string, error) {
	if artPath == "" {
		return nil, "", fmt.Errorf("empty art path")
	}
	u := c.ServerURL + artPath
	if !hasQuery(artPath) {
		u += "?"
	} else {
		u += "&"
	}
	u += "X-Plex-Token=" + url.QueryEscape(c.Token)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	c.throttle()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch image %s: %s", redact(u), resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 20 MiB cap
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	return body, ct, nil
}

func hasQuery(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return true
		}
	}
	return false
}
