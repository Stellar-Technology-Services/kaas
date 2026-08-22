package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newConfigCmd exposes the fully merged, runtime-effective configuration.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the effective KaaS configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print configuration after user, project, environment, and flag overrides",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			if jsonlEnabled(cmd) {
				emitJSONL("config", map[string]any{
					"command": cmd.CommandPath(),
					"config":  app.Config,
					"sources": map[string]string{
						"user":    app.Config.UserConfig,
						"project": app.Config.ConfigPath,
					},
				})
				return nil
			}
			enc := yaml.NewEncoder(os.Stdout)
			enc.SetIndent(2)
			defer enc.Close()
			return enc.Encode(struct {
				Effective any `yaml:"effective"`
				Sources   struct {
					User    string `yaml:"user"`
					Project string `yaml:"project"`
				} `yaml:"sources"`
			}{
				Effective: app.Config,
				Sources: struct {
					User    string `yaml:"user"`
					Project string `yaml:"project"`
				}{User: app.Config.UserConfig, Project: app.Config.ConfigPath},
			})
		},
	})
	return cmd
}
