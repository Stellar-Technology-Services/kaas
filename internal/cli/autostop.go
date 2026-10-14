package cli

import (
	"fmt"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

// armAutoStop records a TTL on the sandbox and, when it is running, sets the
// deadline to now + TTL. An empty spec falls back to defaults.auto_stop; the
// flag is only applied when explicitly set so resumes keep the stored TTL.
func armAutoStop(store *state.Store, name, spec string, fallback string, explicit bool) error {
	sb, err := store.GetSandbox(name)
	if err != nil {
		return err
	}
	if sb.IsTemplate() {
		return nil // templates are never auto-stopped
	}
	if explicit {
		ttl, err := config.ParseAutoStop(spec)
		if err != nil {
			return fmt.Errorf("--auto-stop: %w", err)
		}
		sb.AutoStopTTL = int(ttl / time.Second)
	} else if sb.AutoStopTTL == 0 && sb.AutoStopAt == nil && fallback != "" {
		ttl, err := config.ParseAutoStop(fallback)
		if err != nil {
			return fmt.Errorf("defaults.auto_stop: %w", err)
		}
		sb.AutoStopTTL = int(ttl / time.Second)
	}
	return rearmAutoStop(store, sb)
}

// rearmAutoStop derives the deadline from the stored TTL: now + TTL while
// running, cleared otherwise. Boat semantics — the deadline counts from each
// start, not from activity.
func rearmAutoStop(store *state.Store, sb *state.Sandbox) error {
	if sb.AutoStopTTL <= 0 || sb.ObservedState != state.ObservedRunning {
		sb.AutoStopAt = nil
	} else {
		at := time.Now().UTC().Add(time.Duration(sb.AutoStopTTL) * time.Second)
		sb.AutoStopAt = &at
	}
	return store.UpdateSandbox(*sb)
}

// clearAutoStopDeadline drops the deadline (on stop) but keeps the TTL for the
// next start.
func clearAutoStopDeadline(store *state.Store, name string) {
	sb, err := store.GetSandbox(name)
	if err != nil {
		return
	}
	sb.AutoStopAt = nil
	_ = store.UpdateSandbox(*sb)
}

func newExtendCmd() *cobra.Command {
	var forSpec string
	var never bool
	cmd := &cobra.Command{
		Use:   "extend [name]",
		Short: "Set the auto-stop deadline to now + duration (or turn auto-stop off)",
		Long:  "kaas extend sets the remaining time, it does not add to it: `kaas extend my-app --for 4h` stops the sandbox 4h from now and keeps 4h as the TTL for later starts. `--never` disables auto-stop.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			if never && forSpec != "" {
				return fmt.Errorf("--for and --never are mutually exclusive")
			}
			if !never && forSpec == "" {
				return fmt.Errorf("pass --for <duration> or --never")
			}
			spec := forSpec
			if never {
				spec = "never"
			}
			if err := armAutoStop(store, name, spec, "", true); err != nil {
				return err
			}
			sb, err := store.GetSandbox(name)
			if err != nil {
				return err
			}
			recordEvent(store, name, "auto_stop", "ok", "ttl="+config.FormatAutoStop(time.Duration(sb.AutoStopTTL)*time.Second))
			if jsonlEnabled(cmd) {
				fields := map[string]any{"sandbox": name, "auto_stop_ttl_seconds": sb.AutoStopTTL}
				if sb.AutoStopAt != nil {
					fields["auto_stop_at"] = sb.AutoStopAt.Format(time.RFC3339)
				}
				emitJSONL("auto_stop", fields)
				return nil
			}
			if sb.AutoStopAt != nil {
				printStyled(cmd, styleSuccess, "Sandbox %s stops at %s (in %s)\n", name, sb.AutoStopAt.Local().Format("2006-01-02 15:04"), time.Until(*sb.AutoStopAt).Round(time.Minute))
			} else if sb.AutoStopTTL > 0 {
				printStyled(cmd, styleInfo, "Sandbox %s is not running; it will stop %s after its next start\n", name, config.FormatAutoStop(time.Duration(sb.AutoStopTTL)*time.Second))
			} else {
				printStyled(cmd, styleSuccess, "Auto-stop disabled for %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&forSpec, "for", "", "stop this long from now, e.g. 4h, 90m, 2d")
	cmd.Flags().BoolVar(&never, "never", false, "disable auto-stop")
	return cmd
}
