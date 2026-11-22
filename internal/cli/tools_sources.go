package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arturonavax/anx/internal/domain"
	"github.com/arturonavax/anx/internal/gittransport"
	"github.com/arturonavax/anx/internal/tools"
)

func toolSourceManager(appCtx *AppContext) *tools.SourceManager {
	cfgDir := ""
	dataDir := ""
	binDir := ""
	if appCtx.Platform != nil {
		cfgDir = appCtx.Platform.ConfigDir()
		dataDir = filepath.Join(appCtx.Platform.HomeDir(), ".local", "share", "anx")
		binDir = appCtx.Platform.ToolsBinDir()
	}
	bundledDir := ""
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "..", "tools", "bundled")
		if isDir(cand) {
			bundledDir = cand
		}
	}
	if bundledDir == "" && isDir("tools/bundled") {
		bundledDir, _ = filepath.Abs("tools/bundled")
	}
	mgr := tools.NewSourceManager(cfgDir, dataDir, binDir, bundledDir)
	mgr.Git = newGitAdapter()
	return mgr
}

func newToolsInstallCmd(appCtx *AppContext) *cobra.Command {
	return &cobra.Command{
		Use:   "install <tool>",
		Short: "Install or update a tool from a configured tool source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolID := strings.TrimSpace(args[0])
			mgr := toolSourceManager(appCtx)
			byID, _, err := mgr.DiscoverAll()
			if err != nil {
				return err
			}
			rt, ok := byID[toolID]
			if !ok {
				return domain.NewExitError(domain.ExitUsageError, fmt.Errorf("tool %q not found in any configured tool source (run 'anx tools source list')", toolID))
			}
			if appCtx.Flags.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would install %s from source %s into %s\n", rt.Manifest.ID, rt.SourceID, mgr.BinDir)
				return nil
			}
			dest, err := mgr.InstallTool(cmd.Context(), rt)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s (%s) -> %s\n", rt.Manifest.ID, rt.SourceID, dest)
			return nil
		},
	}
}

func newToolsDoctorCmd(appCtx *AppContext) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose tool sources, collisions, and installed tool executables",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := toolSourceManager(appCtx)
			sources, err := mgr.LoadSources()
			if err != nil {
				return err
			}
			byID, collisions, err := mgr.DiscoverAll()
			if err != nil {
				return err
			}
			report := struct {
				Sources    []tools.ToolSource    `json:"sources"`
				Discovered int                   `json:"discovered"`
				Collisions []tools.ToolCollision `json:"collisions,omitempty"`
				BinDir     string                `json:"bin_dir"`
			}{
				Sources:    sources,
				Discovered: len(byID),
				Collisions: collisions,
				BinDir:     mgr.BinDir,
			}
			return emit(cmd, appCtx, report, func(w io.Writer) {
				st := styles(appCtx)
				fmt.Fprintln(w, st.Header.Render("ANX Tools Diagnostics"))
				fmt.Fprintf(w, "  • Isolated Bin Dir: %s\n", report.BinDir)
				fmt.Fprintf(w, "  • Tool Sources:     %d\n", len(report.Sources))
				fmt.Fprintf(w, "  • Discovered Tools: %d\n", report.Discovered)
				if len(report.Collisions) == 0 {
					fmt.Fprintf(w, "  • ID Collisions:    %s\n", st.Success.Render("none"))
				} else {
					fmt.Fprintf(w, "  • ID Collisions:    %d\n", len(report.Collisions))
					for _, c := range report.Collisions {
						fmt.Fprintf(w, "    - %s: winner=%s shadowed=%s (use <source>/%s to disambiguate)\n",
							c.ToolID, c.WinnerSource, strings.Join(c.ShadowedByIDs, ","), c.ToolID)
					}
				}
			})
		},
	}
}

func newToolsSourceCmd(appCtx *AppContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage multi-source tool repositories (builtin, single-tool, collection)",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured tool sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := toolSourceManager(appCtx)
			sources, err := mgr.LoadSources()
			if err != nil {
				return err
			}
			return emit(cmd, appCtx, sources, func(w io.Writer) {
				st := styles(appCtx)
				fmt.Fprintln(w, st.Header.Render("Configured ANX Tool Sources"))
				for _, s := range sources {
					loc := s.URL
					if loc == "" {
						loc = s.Path
					}
					if loc == "" {
						loc = "(built-in)"
					}
					fmt.Fprintf(w, "  • %-18s %-11s %s\n", s.ID, s.Type, loc)
				}
			})
		},
	}

	var (
		srcID       string
		srcType     string
		srcRef      string
		srcAuth     string
		srcSSHKey   string
		srcOptional bool
	)
	addCmd := &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Add a single-tool or multi-tool collection source repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(args[0])
			norm := gittransport.NormalizeURL(raw)
			src := tools.ToolSource{
				ID:       srcID,
				Type:     tools.ToolSourceType(srcType),
				Ref:      srcRef,
				Auth:     srcAuth,
				SSHKey:   strings.TrimSpace(srcSSHKey),
				Optional: srcOptional,
			}
			mgr := toolSourceManager(appCtx)
			if norm.IsLocal {
				abs, err := filepath.Abs(appCtx.Platform.ExpandPath(raw))
				if err != nil {
					return err
				}
				src.Path = abs
			} else {
				src.URL = raw
				if src.SSHKey != "" || strings.EqualFold(src.Auth, string(gittransport.AuthSSH)) {
					probe, err := mgr.VerifySSHAccess(cmd.Context(), raw, src.SSHKey)
					if err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "SSH access verified: %s\n", probe.ResolvedURL)
					if src.SSHKey != "" && (src.Auth == "" || strings.EqualFold(src.Auth, string(gittransport.AuthAuto))) {
						src.Auth = string(gittransport.AuthSSH)
					}
				}
			}
			saved, discovered, err := mgr.AddSource(cmd.Context(), src)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added tool source %q (%s): discovered %d tool(s)\n", saved.ID, saved.Type, len(discovered))
			for _, rt := range discovered {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", rt.Manifest.ID, rt.Manifest.Name)
			}
			return nil
		},
	}
	addCmd.Flags().StringVar(&srcID, "id", "", "Explicit source identifier")
	addCmd.Flags().StringVar(&srcType, "type", "", "Source type (single|collection; auto-detected if omitted)")
	addCmd.Flags().StringVar(&srcRef, "ref", "", "Git branch, tag, or commit")
	addCmd.Flags().StringVar(&srcAuth, "auth", "auto", "Git auth mode (auto|ssh|https)")
	addCmd.Flags().StringVar(&srcSSHKey, "ssh-key", "", "Path to SSH private key for private Git repositories")
	addCmd.Flags().BoolVar(&srcOptional, "optional", false, "Mark this tool source as optional (e.g., private repository)")

	removeCmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a configured tool source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := toolSourceManager(appCtx)
			if err := mgr.RemoveSource(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed tool source %q\n", args[0])
			return nil
		},
	}

	updateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a Git-backed tool source and re-validate its manifests",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := toolSourceManager(appCtx)
			found, err := mgr.UpdateSource(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated tool source %q (%d tool(s) discovered)\n", args[0], len(found))
			return nil
		},
	}

	var (
		verifySSHKey  string
		verifySSHOnly bool
	)
	verifyCmd := &cobra.Command{
		Use:   "verify <id-or-url>",
		Short: "Verify a tool source's SSH remote accessibility and manifest schemas",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := strings.TrimSpace(args[0])
			mgr := toolSourceManager(appCtx)
			sources, err := mgr.LoadSources()
			if err != nil {
				return err
			}

			for _, s := range sources {
				if s.ID != target {
					continue
				}
				key := strings.TrimSpace(verifySSHKey)
				if key == "" {
					key = s.SSHKey
				}
				if s.URL != "" && !gittransport.NormalizeURL(s.URL).IsLocal &&
					(verifySSHOnly || key != "" || strings.EqualFold(s.Auth, string(gittransport.AuthSSH))) {
					probe, pErr := mgr.VerifySSHAccess(cmd.Context(), s.URL, key)
					if pErr != nil {
						return pErr
					}
					fmt.Fprintf(cmd.OutOrStdout(), "SSH access verified for %q (%s)\n", s.ID, probe.ResolvedURL)
					if verifySSHOnly {
						return nil
					}
				}
				found, vErr := mgr.VerifySource(cmd.Context(), s.ID)
				if vErr != nil {
					return vErr
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Verified tool source %q: %d valid tool(s)\n", s.ID, len(found))
				return nil
			}

			if looksLikeRepoLocator(target) {
				probe, pErr := mgr.VerifySSHAccess(cmd.Context(), target, strings.TrimSpace(verifySSHKey))
				if pErr != nil {
					return pErr
				}
				fmt.Fprintf(cmd.OutOrStdout(), "SSH access verified for %s (%s)\n", probe.Normalized.Redacted, probe.ResolvedURL)
				return nil
			}

			return fmt.Errorf("tool source %q not found", target)
		},
	}
	verifyCmd.Flags().StringVar(&verifySSHKey, "ssh-key", "", "Path to SSH private key for verifying SSH remote access")
	verifyCmd.Flags().BoolVar(&verifySSHOnly, "ssh", false, "Verify SSH remote access only (without requiring local cache)")

	cmd.AddCommand(listCmd, addCmd, removeCmd, updateCmd, verifyCmd)
	return cmd
}

func looksLikeRepoLocator(s string) bool {
	return strings.Contains(s, "://") ||
		strings.Contains(s, "@") ||
		strings.Contains(s, ":") ||
		strings.Contains(s, "/")
}
