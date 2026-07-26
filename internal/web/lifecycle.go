package web

import (
	"context"
	"fmt"
	"time"

	"github.com/nckslvrmn/spoilarr/internal/store"
)

// jobCtx gives a whole-show run room, since it can touch hundreds of items.
func jobCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}

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

// revealAllBlocking runs before a destructive reset, restoring the library while
// the backups still exist.
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
