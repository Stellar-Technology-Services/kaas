package cli

import (
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// newLifecycleService builds a lifecycle.Service for CLI commands.
// Confirmation prompts stay in the CLI; use cases live in the domain package.
func newLifecycleService(store *state.Store, app *App) *lifecycle.Service {
	portBase := 6022
	egressRoot := ""
	if app != nil && app.Config != nil {
		if app.Config.Defaults.SSHPortBase > 0 {
			portBase = app.Config.Defaults.SSHPortBase
		}
		egressRoot = app.Config.EgressRuntime.Root
	}
	return lifecycle.NewService(lifecycle.Deps{
		Provider: lifecycle.NewColimaProvider(newColimaClient(app)),
		Store:    store,
		Events:   lifecycle.StoreEvents{Store: store},
		Egress:   lifecycle.RuntimeEgress{Root: egressRoot},
		Ports:    lifecycle.NewStorePortAllocator(store),
		PortBase: portBase,
	})
}
