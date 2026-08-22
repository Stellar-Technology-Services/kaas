package lifecycle

import (
	"context"
	"errors"
	"os"

	eruntime "github.com/Stellar-Technology-Services/kaas/internal/egress/runtime"
)

// RuntimeEgress adapts the host egress runtime directory to EgressCoordinator.
// Restore is a no-op here; CLI may reconfigure egress after Start.
type RuntimeEgress struct {
	Root string
}

func (e RuntimeEgress) Stop(_ context.Context, name string) error {
	if e.Root == "" || name == "" {
		return nil
	}
	if _, err := os.Stat(eruntime.Dir(e.Root, name)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_, err := eruntime.Call(e.Root, name, eruntime.Request{Action: "stop"})
	if err != nil {
		// No control socket means the proxy is already down.
		if _, statErr := os.Stat(eruntime.Socket(e.Root, name)); errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
	}
	return err
}

func (e RuntimeEgress) Restore(context.Context, string) error {
	return nil
}

func (e RuntimeEgress) RemoveRuntime(_ context.Context, name string) error {
	if e.Root == "" || name == "" {
		return nil
	}
	return os.RemoveAll(eruntime.Dir(e.Root, name))
}
