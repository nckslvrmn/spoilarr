// Package plex is spoilarr's client for the Plex ecosystem: the plex.tv cloud
// API (PIN sign-in, server discovery) and a local Plex Media Server (library
// reads, metadata edits, artwork uploads).
package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// Product and Version identify spoilarr to Plex. Product shows up in the
	// user's list of authorized devices on plex.tv.
	Product = "spoilarr"
	Version = "0.1.0"

	plexTVBase = "https://plex.tv/api/v2"
)

// Client talks to plex.tv and (once configured) a local Plex Media Server.
//
// ClientID is a stable per-install identifier (a UUID) that ties the auth token
// to this device on plex.tv; it must be sent on every plex.tv request and be
// identical between creating a PIN and polling it.
type Client struct {
	ClientID  string
	Token     string // empty until the user signs in
	ServerURL string // base URL of the local PMS, e.g. http://10.0.0.5:32400

	http   *http.Client
	minGap time.Duration
	rateMu sync.Mutex
	lastReq time.Time
}

// New returns a Client with a sane HTTP timeout. Token and ServerURL may be
// empty (unconfigured) and set later once the user completes setup.
func New(clientID, token, serverURL string) *Client {
	return &Client{
		ClientID:  clientID,
		Token:     token,
		ServerURL: strings.TrimRight(serverURL, "/"),
		http:      &http.Client{Timeout: 30 * time.Second},
		minGap:    80 * time.Millisecond, // ~12 req/s ceiling so we don't hammer PMS
	}
}

// throttle spaces requests at least minGap apart to be gentle on Plex when
// protecting a whole show fires hundreds of calls.
func (c *Client) throttle() {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	if wait := time.Until(c.lastReq.Add(c.minGap)); wait > 0 {
		time.Sleep(wait)
	}
	c.lastReq = time.Now()
}

// retryable reports whether an error/status warrants a retry (transient).
func retryable(status int, err error) bool {
	if err != nil {
		return true // network/timeout
	}
	return status == 429 || status == 502 || status == 503 || status == 504
}

// plexHeaders sets the identification headers Plex expects on every request.
func (c *Client) plexHeaders(h http.Header) {
	h.Set("X-Plex-Product", Product)
	h.Set("X-Plex-Version", Version)
	h.Set("X-Plex-Client-Identifier", c.ClientID)
	h.Set("X-Plex-Platform", "Docker")
	h.Set("X-Plex-Device", "spoilarr")
	h.Set("Accept", "application/json")
	if c.Token != "" {
		h.Set("X-Plex-Token", c.Token)
	}
}

// do performs an HTTP request with Plex headers applied and decodes a JSON body
// into out (when out is non-nil). Non-2xx responses become errors. Requests are
// throttled and transient failures retried with backoff. body must be nil here
// since it is not rewindable across retries (all JSON calls are bodyless).
func (c *Client) do(ctx context.Context, method, url string, body io.Reader, out any) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c.throttle()
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return err
		}
		c.plexHeaders(req.Header)

		resp, err := c.http.Do(req)
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if err == nil && status >= 200 && status < 300 {
			defer resp.Body.Close()
			if out == nil {
				return nil
			}
			return json.NewDecoder(resp.Body).Decode(out)
		}

		if err != nil {
			lastErr = err
		} else {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastErr = fmt.Errorf("plex %s %s: %s: %s", method, redact(url), resp.Status, strings.TrimSpace(string(snippet)))
		}
		if attempt == maxAttempts || !retryable(status, err) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
		}
	}
	return lastErr
}

// redact hides a token that may appear in a query string when included in errors.
func redact(url string) string {
	if i := strings.Index(url, "X-Plex-Token="); i >= 0 {
		return url[:i] + "X-Plex-Token=REDACTED"
	}
	return url
}
