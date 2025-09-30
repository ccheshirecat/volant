package standard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage engine plugins",
	}

	cmd.AddCommand(newPluginsListCmd())
	cmd.AddCommand(newPluginsShowCmd())
	cmd.AddCommand(newPluginsEnableCmd())
	cmd.AddCommand(newPluginsDisableCmd())
	// For now install/remove expect manifest JSON files.
	cmd.AddCommand(newPluginsInstallCmd())
	cmd.AddCommand(newPluginsRemoveCmd())

	return cmd
}

func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			plugins, err := api.ListPlugins(ctx)
			if err != nil {
				return err
			}
			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-8s %-10s\n", "NAME", "VERSION", "ENABLED", "RUNTIME")
			for _, plugin := range plugins {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-8t %-10s\n", plugin.Name, plugin.Version, plugin.Enabled, plugin.Runtime)
			}
			return nil
		},
	}
}

func newPluginsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show plugin manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			manifest, err := api.GetPlugin(ctx, args[0])
			if err != nil {
				return err
			}
			if manifest == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s not found\n", args[0])
				return nil
			}
			return encodeAsJSON(cmd.OutOrStdout(), manifest)
		},
	}
}

func newPluginsEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return togglePlugin(cmd, args[0], true)
		},
	}
}

func newPluginsDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return togglePlugin(cmd, args[0], false)
		},
	}
}

func newPluginsInstallCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a plugin from manifest JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(manifestPath) == "" {
				return fmt.Errorf("--manifest path required")
			}
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				return err
			}
			var manifest pluginspec.Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return fmt.Errorf("decode manifest: %w", err)
			}
			rootfsPath := strings.TrimSpace(manifest.RootFS.URL)
			if rootfsPath != "" && !strings.HasPrefix(rootfsPath, "http://") && !strings.HasPrefix(rootfsPath, "https://") && !strings.HasPrefix(rootfsPath, "file://") && !filepath.IsAbs(rootfsPath) {
				resolved := filepath.Join(filepath.Dir(manifestPath), rootfsPath)
				manifest.RootFS.URL = filepath.Clean(resolved)
			}
			manifest.Normalize()
			if err := manifest.Validate(); err != nil {
				return err
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			return api.InstallPlugin(ctx, manifest)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to plugin manifest JSON")
	return cmd
}

func newPluginsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			return api.RemovePlugin(ctx, args[0])
		},
	}
}

func togglePlugin(cmd *cobra.Command, name string, enabled bool) error {
	api, err := clientFromCmd(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	if err := api.SetPluginEnabled(ctx, name, enabled); err != nil {
		return err
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s %s\n", name, state)
	return nil
}
