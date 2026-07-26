// Package config loads spoilarr's boot-time settings from the environment.
// Everything the user configures at runtime (Plex token, server URL, protected
// shows) lives in SQLite, not here.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Addr    string
	DataDir string // the /data volume in a container: SQLite DB + images/ tree
	// PublicURL is the reachable base URL used when telling Plex where to send
	// webhooks. Optional; auto-derived from the request otherwise.
	PublicURL string
	// PollInterval is how often the backstop poller reconciles protected items.
	PollInterval time.Duration
	// WebhookSecret, if set, must appear as ?secret= on the Plex webhook URL.
	WebhookSecret string
}

func (c Config) DBPath() string       { return filepath.Join(c.DataDir, "spoilarr.db") }
func (c Config) ImagesDir() string    { return filepath.Join(c.DataDir, "images") }
func (c Config) OriginalsDir() string { return filepath.Join(c.ImagesDir(), "originals") }
func (c Config) GeneratedDir() string { return filepath.Join(c.ImagesDir(), "generated") }

func Load() Config {
	interval, err := time.ParseDuration(envOr("SPOILARR_POLL_INTERVAL", "5m"))
	if err != nil || interval < time.Minute {
		interval = 5 * time.Minute
	}
	return Config{
		Addr:          envOr("SPOILARR_ADDR", ":8484"),
		DataDir:       envOr("SPOILARR_DATA_DIR", "./data"),
		PublicURL:     strings.TrimRight(os.Getenv("SPOILARR_PUBLIC_URL"), "/"),
		PollInterval:  interval,
		WebhookSecret: os.Getenv("SPOILARR_WEBHOOK_SECRET"),
	}
}

func (c Config) EnsureDirs() error {
	for _, dir := range []string{c.DataDir, c.ImagesDir(), c.OriginalsDir(), c.GeneratedDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
