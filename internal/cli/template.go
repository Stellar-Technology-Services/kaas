package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

// Templates are boat.dev's "named snapshot" on a Lima budget: a stopped golden
// sandbox whose disk `limactl clone` copies for every `--from` deploy. Lima has
// no live snapshots on the VZ driver, so the template itself is a VM profile
// that stays stopped and is never auto-stopped.

func newTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Save a sandbox as a reusable template, list templates, or remove one",
		Long:  "A template is a stopped sandbox marked role=template. `kaas up <name> --from <template>` clones its disk (toolchains, agents, caches) and skips provision/agents, then runs secrets/egress, checkout, and setup for the new sandbox. Secrets, egress runtime, and auto-stop are never copied.",
	}
	cmd.AddCommand(newTemplateSaveCmd(), newTemplateListCmd(), newTemplateRemoveCmd())
	return cmd
}

func newTemplateSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save <sandbox> <template>",
		Short: "Clone a sandbox's disk under a template name (the source is stopped briefly)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, target := args[0], args[1]
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			if _, err := loadSandbox(store, source); err != nil {
				return err
			}
			svc := newLifecycleService(store, app)
			if existing, err := store.GetSandbox(target); err == nil {
				if !existing.IsTemplate() {
					return fmt.Errorf("%q exists and is not a template; choose another name", target)
				}
				// Boat: saving the same name again replaces the old copy.
				app.Log.Info("replacing template", "template", target)
				if err := svc.Destroy(cmd.Context(), lifecycle.DestroyInput{Name: target}); err != nil {
					return fmt.Errorf("remove previous template %s: %w", target, err)
				}
			} else if !errors.Is(err, state.ErrNotFound) {
				return err
			}
			if err := cloneAndMarkTemplate(cmd.Context(), svc, store, source, target); err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Saved template %s from %s\n", target, source)
			printMessage(cmd, "  deploy: kaas up <name> --from %s\n", target)
			return nil
		},
	}
}

// cloneAndMarkTemplate forks source → target, then stops target and tags it.
func cloneAndMarkTemplate(ctx context.Context, svc *lifecycle.Service, store *state.Store, source, target string) error {
	_, err := svc.Fork(ctx, lifecycle.ForkInput{Source: source, Target: target})
	var pe *lifecycle.PartialError
	if err != nil && !errors.As(err, &pe) {
		return err
	}
	// The clone comes up running; a template must be at rest so every deploy
	// starts from the exact saved disk.
	if stopErr := svc.Stop(ctx, target); stopErr != nil {
		var ee *lifecycle.EgressError
		if !errors.As(stopErr, &ee) {
			return fmt.Errorf("stop template %s: %w", target, stopErr)
		}
	}
	tpl, getErr := store.GetSandbox(target)
	if getErr != nil {
		return getErr
	}
	tpl.Role = state.RoleTemplate
	tpl.DesiredState = state.DesiredStopped
	tpl.AutoStopTTL, tpl.AutoStopAt = 0, nil
	if updErr := store.UpdateSandbox(*tpl); updErr != nil {
		return updErr
	}
	recordEvent(store, target, "template", "ok", "saved from "+source)
	return err // nil, or the source-restore PartialError (target is fine)
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List templates",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			all, err := store.ListSandboxes("")
			if err != nil {
				return err
			}
			var rows [][]string
			for _, sb := range all {
				if !sb.IsTemplate() {
					continue
				}
				if jsonlEnabled(cmd) {
					emitJSONL("template", map[string]any{"name": sb.Name, "cpus": sb.CPUs, "memory_gib": sb.MemoryGiB, "disk_gib": sb.DiskGiB, "observed": sb.ObservedState, "created_at": sb.CreatedAt.Format(time.RFC3339)})
					continue
				}
				rows = append(rows, []string{sb.Name, fmt.Sprintf("%d/%d/%d", sb.CPUs, sb.MemoryGiB, sb.DiskGiB), sb.ObservedState, sb.CreatedAt.Local().Format("2006-01-02 15:04")})
			}
			if jsonlEnabled(cmd) {
				return nil
			}
			if len(rows) == 0 {
				printStyled(cmd, styleInfo, "No templates. Save one with: kaas template save <sandbox> <template>\n")
				return nil
			}
			writeTable(commandOutput(cmd), []string{"TEMPLATE", "CPU/GIB/DISK", "OBSERVED", "CREATED"}, rows, tableOpts{})
			return nil
		},
	}
}

func newTemplateRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "rm <template>",
		Aliases: []string{"remove", "delete"},
		Short:   "Delete a template VM and its registry row",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			if !sb.IsTemplate() {
				return fmt.Errorf("%q is a sandbox, not a template; use kaas destroy", name)
			}
			if !yes && !jsonlEnabled(cmd) {
				if !confirmPrompt(cmd, fmt.Sprintf("Remove template %q and delete its disk? [y/N] ", name)) {
					printStyled(cmd, styleWarn, "Cancelled\n")
					return nil
				}
			}
			svc := newLifecycleService(store, app)
			if err := svc.Destroy(cmd.Context(), lifecycle.DestroyInput{Name: name}); err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Removed template %s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

// deployFromTemplate clones template → name and persists a contract for the
// new sandbox from the current config stack (resources come from the clone).
// Provision/agents are skipped by the caller: the template already has them.
func deployFromTemplate(ctx context.Context, cmd *cobra.Command, template, name string, opts createOpts) error {
	store, app, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := identity.ParseSandboxID(name); err != nil {
		return err
	}
	tpl, err := loadSandbox(store, template)
	if err != nil {
		return fmt.Errorf("template %q: %w", template, err)
	}
	if !tpl.IsTemplate() {
		printStyled(cmd, styleWarn, "%s is a working sandbox, not a template; cloning its newest disk anyway\n", template)
	}
	if _, err := store.GetSandbox(name); err == nil {
		return fmt.Errorf("%w: %s", state.ErrAlreadyExists, name)
	}
	svc := newLifecycleService(store, app)
	app.Log.Info("deploying from template", "template", template, "name", name)
	res, err := svc.Fork(ctx, lifecycle.ForkInput{Source: template, Target: name})
	var pe *lifecycle.PartialError
	if err != nil && !errors.As(err, &pe) {
		return err
	}
	// A template must stay stopped; restoreSource already handles that, but a
	// PartialError means it may not have. Try once more, then report.
	if pe != nil {
		_ = svc.Stop(ctx, template)
	}
	flags := contract.FlagsLayer(opts.CPUs, opts.Memory, opts.Disk, 0, opts.Arch, "flags")
	resolved, err := resolveContractStack(app.Config, nil, flags, true)
	if err != nil {
		return fmt.Errorf("resolve contract: %w", err)
	}
	resolved.Contract.Resources.SSHPort = res.Target.Endpoint.Port
	if hashed, hashErr := resolved.Contract.WithHash(); hashErr == nil {
		resolved.Contract = hashed
	}
	if err := contract.NewStoreRepository(store).Put(ctx, name, resolved.Contract); err != nil {
		return fmt.Errorf("persist contract: %w", err)
	}
	recordEvent(store, name, "create", "ok", "from template "+template)
	recordEvent(store, name, "provision", "skipped", "disk cloned from template "+template)
	recordEvent(store, name, "agents", "skipped", "disk cloned from template "+template)
	printStyled(cmd, styleSuccess, "Deployed %s from template %s (port %d)\n", name, template, res.Target.Endpoint.Port)
	return nil
}
