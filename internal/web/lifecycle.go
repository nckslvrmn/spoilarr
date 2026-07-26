package web

import (
	"context"
	"fmt"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/store"
)

// jobCtx is the timeout for a background protect/reveal run. Whole-show
// operations can touch hundreds of items, so give them room.
func jobCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}

// runProtect applies protection to a whole protected item and returns a summary.
func (s *Server) runProtect(pi store.ProtectedItem) string {
	proc, err := s.processor()
	if err != nil {
		return "error: " + err.Error()
	}
	ctx, cancel := jobCtx()
	defer cancel()
	res, err := proc.ProtectOne(ctx, pi)
	if err != nil {
		return "protect error: " + err.Error()
	}
	return fmt.Sprintf("Protected %d, %d errors", res.Protected, res.Errors)
}

// runReveal restores a whole protected item to its originals.
func (s *Server) runReveal(pi store.ProtectedItem) string {
	proc, err := s.processor()
	if err != nil {
		return "error: " + err.Error()
	}
	ctx, cancel := jobCtx()
	defer cancel()
	res, err := proc.RevealOne(ctx, pi)
	if err != nil {
		return "reveal error: " + err.Error()
	}
	return fmt.Sprintf("Revealed %d, %d errors", res.Revealed, res.Errors)
}

// revealAllBlocking reveals every protected item synchronously. Used before a
// destructive reset so the library is restored while the backups still exist.
func (s *Server) revealAllBlocking() (revealed int, errs int) {
	proc, err := s.processor()
	if err != nil {
		return 0, 1
	}
	items, err := s.store.ListProtectedItems()
	if err != nil {
		return 0, 1
	}
	for _, pi := range items {
		ctx, cancel := jobCtx()
		res, err := proc.RevealOne(ctx, pi)
		cancel()
		if err != nil {
			errs++
			continue
		}
		revealed += res.Revealed
		errs += res.Errors
	}
	return revealed, errs
}
