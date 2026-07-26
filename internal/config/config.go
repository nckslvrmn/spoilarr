// Package config loads spoilarr runtime configuration from the environment.
//
// Only process-level, boot-time settings live here (where to listen, where to
// put data). Everything the user configures at runtime — Plex token, server
// URL, protected shows — lives in SQLite, not here.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds boot-time settings resolved from the environment.
type Config struct {
	// Addr is the listen address for the HTTP server, e.g. ":8484".
	Addr string
	// DataDir is the root of the durable state volume. Holds the SQLite DB
	// and the images/ tree. In a container this is the mounted /data volume.
	DataDir string
	// PublicURL is the externally reachable base URL of this app, used when
	// telling Plex where to send webhooks. Optional; can be set later in the UI.
	PublicURL string
	// PollInterval is how often the backstop poller reconciles protected items.
	PollInterval time.Duration
	// WebhookSecret, if set, must be present as ?secret= on the Plex webhook URL.
	WebhookSecret string
}

// DBPath is the location of the SQLite database file.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "spoilarr.db") }

// ImagesDir is the root of the on-disk image store.
func (c Config) ImagesDir() string { return filepath.Join(c.DataDir, "images") }

// OriginalsDir holds backups of original artwork, used to restore on reveal.
func (c Config) OriginalsDir() string { return filepath.Join(c.ImagesDir(), "originals") }

// GeneratedDir holds generated spoiler cards.
func (c Config) GeneratedDir() string { return filepath.Join(c.ImagesDir(), "generated") }

// Load reads configuration from the environment, applying defaults.
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

// EnsureDirs creates the data directory tree if it does not exist.
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
