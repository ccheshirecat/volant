// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package standard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/volantvm/volant/internal/pluginspec"
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
	var manifestURL string

	cmd := &cobra.Command{
		Use:   "install [manifest]",
		Short: "Install a plugin from manifest JSON (file path or URL)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Allow positional arg as shorthand for --manifest or --url
			if len(args) == 1 {
                token := cleanToken(strings.TrimSpace(args[0]))
				if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
					manifestURL = token
				} else {
					manifestPath = token
				}
			}
            // If --manifest was provided with an http(s) URL, treat it as --url
            if u := cleanToken(strings.TrimSpace(manifestPath)); u != "" {
                if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
                    manifestURL = u
                    manifestPath = ""
                } else {
                    manifestPath = u
                }
            }
            if u := cleanToken(strings.TrimSpace(manifestURL)); u != "" {
                manifestURL = u
            }
			var data []byte
			var err error
			if strings.TrimSpace(manifestURL) != "" {
				data, err = fetchURL(cmd.Context(), manifestURL)
				if err != nil {
					return err
				}
			} else if strings.TrimSpace(manifestPath) != "" {
				data, err = os.ReadFile(manifestPath)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("provide a manifest via --manifest, --url, or positional argument")
			}
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
			initramfsPath := strings.TrimSpace(manifest.Initramfs.URL)
			if initramfsPath != "" && !strings.HasPrefix(initramfsPath, "http://") && !strings.HasPrefix(initramfsPath, "https://") && !strings.HasPrefix(initramfsPath, "file://") && !filepath.IsAbs(initramfsPath) {
				resolved := filepath.Join(filepath.Dir(manifestPath), initramfsPath)
				manifest.Initramfs.URL = filepath.Clean(resolved)
			}
			for i := range manifest.Disks {
				src := strings.TrimSpace(manifest.Disks[i].Source)
				if src == "" {
					continue
				}
				if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "file://") || filepath.IsAbs(src) {
					continue
				}
				resolved := filepath.Join(filepath.Dir(manifestPath), src)
				manifest.Disks[i].Source = filepath.Clean(resolved)
			}
			if manifest.CloudInit != nil {
				base := filepath.Dir(manifestPath)
				process := func(doc pluginspec.CloudInitDoc) (pluginspec.CloudInitDoc, error) {
					path := strings.TrimSpace(doc.Path)
					if path == "" {
						return doc, nil
					}
					if !filepath.IsAbs(path) {
						path = filepath.Join(base, path)
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return doc, err
					}
					doc.Path = ""
					doc.Content = string(data)
					doc.Inline = true
					return doc, nil
				}
				var err error
				if manifest.CloudInit != nil {
					if manifest.CloudInit.UserData, err = process(manifest.CloudInit.UserData); err != nil {
						return err
					}
					if manifest.CloudInit.MetaData, err = process(manifest.CloudInit.MetaData); err != nil {
						return err
					}
					if manifest.CloudInit.NetworkCfg, err = process(manifest.CloudInit.NetworkCfg); err != nil {
						return err
					}
				}
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
	cmd.Flags().StringVar(&manifestURL, "url", "", "URL to plugin manifest JSON")
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

func fetchURL(ctx context.Context, raw string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download manifest: http %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// cleanToken removes simple chat/markup wrappers like <user-mention ...>URL</user-mention>
// and trims spaces so users can paste URLs from rich UIs without errors.
func cleanToken(s string) string {
    if s == "" {
        return s
    }
    // Strip any XML/HTML-like tags: repeatedly remove substrings like <...>
    for {
        start := strings.IndexByte(s, '<')
        if start == -1 {
            break
        }
        end := strings.IndexByte(s[start:], '>')
        if end == -1 {
            break
        }
        s = s[:start] + s[start+end+1:]
    }
    return strings.TrimSpace(s)
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
