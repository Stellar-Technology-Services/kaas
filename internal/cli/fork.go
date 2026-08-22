package cli

import (
	"context"
	"errors"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

func newForkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fork <sandbox-id> <new-name>",
		Short: "Clone an existing sandbox VM under a new name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFork(cmd.Context(), cmd, args[0], args[1])
		},
	}
}

func runFork(ctx context.Context, cmd *cobra.Command, source, target string) error {
	store, app, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	svc := newLifecycleService(store, app)
	app.Log.Info("forking sandbox", "source", source, "target", target)
	res, err := svc.Fork(ctx, lifecycle.ForkInput{Source: source, Target: target})
	if err != nil {
		var pe *lifecycle.PartialError
		if errors.As(err, &pe) {
			printStyled(cmd, styleSuccess, "Forked sandbox %s as %s (port %d)\n", source, pe.Target, res.Target.Endpoint.Port)
			printMessage(cmd, "  ssh: kaas ssh %s\n", pe.Target)
			return err
		}
		return err
	}
	printStyled(cmd, styleSuccess, "Forked sandbox %s as %s (port %d)\n", source, res.Target.ID.Name, res.Target.Endpoint.Port)
	printMessage(cmd, "  ssh: kaas ssh %s\n", res.Target.ID.Name)
	return nil
}
