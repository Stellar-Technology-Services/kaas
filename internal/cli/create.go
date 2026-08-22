package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

type createOpts struct {
	Port   int
	CPUs   int
	Memory int
	Disk   int
	Arch   string
}

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start [name]",
		Short: "Start a stopped sandbox while preserving its existing state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			crepo := contract.NewStoreRepository(store)
			if c, adopted, err := ensureContract(cmd.Context(), app, name, crepo); err != nil {
				return err
			} else if adopted {
				printContractAdoption(cmd, name, c)
			}

			svc := newLifecycleService(store, app)
			app.Log.Info("starting stopped sandbox", "name", name)
			if err := svc.Start(cmd.Context(), name); err != nil {
				return err
			}
			// Reload port after SSH reconcile.
			if refreshed, getErr := store.GetSandbox(name); getErr == nil {
				sb = refreshed
			}
			if sb.EgressMode != "" && sb.EgressMode != "open" {
				if err := configureEgress(cmd, name); err != nil {
					return fmt.Errorf("restore egress: %w", err)
				}
			}
			if _, err := syncSandboxServices(cmd, store, app, name); err != nil {
				return fmt.Errorf("sync services: %w", err)
			}
			printStyled(cmd, styleSuccess, "Started sandbox %s (port %d)\n", name, sb.SSHPort)
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var opts createOpts

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create or start a Colima sandbox VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), cmd, args[0], opts)
		},
	}

	cmd.Flags().IntVar(&opts.Port, "port", 0, "SSH port (default: next free from ssh_port_base)")
	cmd.Flags().IntVar(&opts.CPUs, "cpus", 0, "vCPUs (default from config)")
	cmd.Flags().IntVar(&opts.Memory, "memory", 0, "memory GiB (default from config)")
	cmd.Flags().IntVar(&opts.Disk, "disk", 0, "disk GiB (default from config)")
	cmd.Flags().StringVar(&opts.Arch, "arch", "", "VM arch (default from config)")

	return cmd
}

func runCreate(ctx context.Context, cmd *cobra.Command, name string, opts createOpts) error {
	store, app, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	repoRoot := app.Config.RepoRoot
	if repoRoot == "" {
		repoRoot, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}
	repoName := filepath.Base(repoRoot)

	existing, err := store.GetSandbox(name)
	if err == nil {
		svc := newLifecycleService(store, app)
		app.Log.Info("starting existing sandbox", "name", name)
		ident, idErr := identity.FromState(*existing)
		if idErr != nil {
			return idErr
		}
		res, createErr := svc.Create(ctx, lifecycle.CreateInput{Identity: ident, Port: existing.SSHPort})
		if createErr != nil {
			return createErr
		}
		if res.AlreadyRunning {
			printStyled(cmd, styleInfo, "Sandbox %s already running on port %d\n", name, res.Identity.Endpoint.Port)
			return nil
		}
		printStyled(cmd, styleSuccess, "Started sandbox %s (port %d)\n", name, res.Identity.Endpoint.Port)
		return nil
	}
	if !errors.Is(err, state.ErrNotFound) {
		return err
	}

	sandboxID, err := identity.ParseSandboxID(name)
	if err != nil {
		return err
	}
	name = sandboxID.Name

	cpus, memory, disk, arch := opts.CPUs, opts.Memory, opts.Disk, opts.Arch
	if cpus == 0 {
		cpus = app.Config.Defaults.CPUs
	}
	if memory == 0 {
		memory = app.Config.Defaults.MemoryGiB
	}
	if disk == 0 {
		disk = app.Config.Defaults.DiskGiB
	}
	if arch == "" {
		arch = app.Config.Defaults.Arch
	}

	// Port may be 0 (allocate inside lifecycle.Create). Build the identity
	// value here; Create validates fully after port assignment.
	primary, _ := app.Config.Workspace.Primary()
	ident := identity.Identity{
		ID:       sandboxID,
		Provider: identity.ProviderIdentity{ProfileName: sandboxID.Name},
		ControlRepo: identity.ControlRepository{
			Path: repoRoot,
			Name: repoName,
		},
		Workspace: identity.WorkspaceIdentity{
			Repository: primary.Repository,
			Branch:     primary.Branch,
			Directory:  primary.Directory,
		},
		Endpoint: identity.Endpoint{
			Host:         identity.DefaultSSHHost,
			Port:         opts.Port,
			IdentityFile: app.Config.Paths.SSHKey,
		},
		Resources: identity.Resources{
			CPUs:      cpus,
			MemoryGiB: memory,
			DiskGiB:   disk,
			Arch:      arch,
		},
		ConfigSource: app.Config.ConfigPath,
	}

	svc := newLifecycleService(store, app)
	app.Log.Info("creating sandbox", "name", name, "port", opts.Port, "repo", repoRoot)
	res, err := svc.Create(ctx, lifecycle.CreateInput{Identity: ident, Port: opts.Port})
	if err != nil {
		return err
	}

	flags := contract.FlagsLayer(opts.CPUs, opts.Memory, opts.Disk, opts.Port, opts.Arch, "flags")
	resolved, err := resolveContractStack(app.Config, nil, flags, true)
	if err != nil {
		return fmt.Errorf("resolve contract: %w", err)
	}
	// Bind allocated SSH port into the persisted resource contract.
	resolved.Contract.Resources.SSHPort = res.Identity.Endpoint.Port
	if hashed, hashErr := resolved.Contract.WithHash(); hashErr == nil {
		resolved.Contract = hashed
	}
	if err := contract.NewStoreRepository(store).Put(ctx, name, resolved.Contract); err != nil {
		return fmt.Errorf("persist contract: %w", err)
	}

	printStyled(cmd, styleSuccess, "Created sandbox %s (port %d)\n", name, res.Identity.Endpoint.Port)
	printMessage(cmd, "  ssh: kaas ssh %s\n", name)
	return nil
}
