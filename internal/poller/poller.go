// Package poller runs spoilarr's periodic reconcile: the backstop that protects
// newly-added episodes and applies reveal policies even when a webhook is missed
// (or the app was down when an item was added).
package poller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/config"
	"github.com/nckslvrmn/spoilarr/internal/conn"
	"github.com/nckslvrmn/spoilarr/internal/protect"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

const (
	KeyLastPollAt      = "last_poll_at"
	KeyLastPollSummary = "last_poll_summary"
)

type Poller struct {
	store    *store.Store
	cfg      config.Config
	log      *slog.Logger
	interval time.Duration
	mu       sync.Mutex // prevents overlapping cycles
}

func New(st *store.Store, cfg config.Config, log *slog.Logger, interval time.Duration) *Poller {
	return &Poller{store: st, cfg: cfg, log: log, interval: interval}
}

// Run reconciles immediately, then every interval until ctx is done.
func (p *Poller) Run(ctx context.Context) {
	p.log.Info("poller started", "interval", p.interval)
	// Let the server settle before the first cycle.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			p.log.Info("poller stopped")
			return
		case <-timer.C:
			p.RunOnce(ctx)
			timer.Reset(p.interval)
		}
	}
}

// RunOnce is safe to call concurrently; overlapping calls are serialized.
func (p *Poller) RunOnce(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	client, err := conn.Client(p.store)
	if err != nil || !conn.Configured(client) {
		return // setup not complete yet
	}
	items, err := p.store.ListProtectedItems()
	if err != nil {
		p.log.Error("poll: list protected items failed", "err", err)
		return
	}
	if len(items) == 0 {
		return
	}

	proc := protect.NewProcessor(p.store, client, p.cfg, p.log)
	var totalProtected, totalRevealed, totalErrors int
	for _, pi := range items {
		if !pi.Enabled {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		res, err := proc.Reconcile(cctx, pi)
		cancel()
		if err != nil {
			p.log.Warn("poll: reconcile failed", "item", pi.Title, "err", err)
			totalErrors++
			continue
		}
		if res.Protected > 0 || res.Revealed > 0 {
			p.log.Info("poll: reconciled", "item", pi.Title, "protected", res.Protected, "revealed", res.Revealed)
		}
		totalProtected += res.Protected
		totalRevealed += res.Revealed
		totalErrors += res.Errors
	}

	p.store.SetConfig(KeyLastPollAt, time.Now().Format(time.RFC3339))
	p.store.SetConfig(KeyLastPollSummary,
		fmt.Sprintf("%d protected, %d revealed, %d errors across %d items", totalProtected, totalRevealed, totalErrors, len(items)))
}
