package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/arturonavax/anx/internal/catalog"
	"github.com/arturonavax/anx/internal/domain"
	"github.com/arturonavax/anx/internal/gittransport"
)

// DefaultConfigRepoURL is the default declarative configuration repository.
const DefaultConfigRepoURL = "https://github.com/arturonavax/env"

var (
	gitAdapterMu      sync.RWMutex
	gitAdapterFactory = gittransport.NewAdapter
)

func newGitAdapter() *gittransport.Adapter {
	gitAdapterMu.RLock()
	fn := gitAdapterFactory
	gitAdapterMu.RUnlock()
	if fn != nil {
		return fn()
	}
	return gittransport.NewAdapter()
}

// SetGitAdapterFactoryForTest overrides the gittransport adapter factory during tests
// and returns a restore function.
func SetGitAdapterFactoryForTest(fn func() *gittransport.Adapter) func() {
	gitAdapterMu.Lock()
	prev := gitAdapterFactory
	gitAdapterFactory = fn
	gitAdapterMu.Unlock()
	return func() {
		gitAdapterMu.Lock()
		gitAdapterFactory = prev
		gitAdapterMu.Unlock()
	}
}

type repoStatusInfo struct {
	RepoRoot      string `json:"repo_root"`
	ConfigRepoURL string `json:"config_repo_url,omitempty"`
	ConfigRef     string `json:"config_ref,omitempty"`
	Exists        bool   `json:"exists"`
	HasFiles      bool   `json:"has_files"`
	HasManifests  bool   `json:"has_manifests"`
	IsGit         bool   `json:"is_git"`
	RemoteURL     string `json:"remote_url,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Clean         bool   `json:"clean"`
	Profiles      int    `json:"profiles"`
}

func newRepoCmd(appCtx *AppContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the linked declarative configuration repository",
		Long: `Inspect, clone, switch, update, or verify the external configuration repository (default: https://github.com/arturonavax/env).
Supports public/private Git repositories (with automatic SSH discovery and HTTPS fallback) as well as local directories.`,
	}

	cmd.AddCommand(
		newRepoStatusCmd(appCtx),
		newRepoCloneCmd(appCtx),
		newRepoSetCmd(appCtx),
		newRepoUpdateCmd(appCtx),
		newRepoVerifyCmd(appCtx),
	)

	return cmd
}

func resolveConfigRepoURL(appCtx *AppContext, explicit string) string {
	for _, cand := range []string{
		explicit,
		appCtx.Flags.ConfigRepo,
		os.Getenv("ANX_CONFIG_REPO"),
		os.Getenv("ANX_DOTFILES_URL"),
		os.Getenv("ANX_GIT_URL"),
	} {
		if strings.TrimSpace(cand) != "" {
			return strings.TrimSpace(cand)
		}
	}
	if appCtx.Viper != nil {
		if v := strings.TrimSpace(appCtx.Viper.GetString("config_repo")); v != "" {
			return v
		}
	}
	return DefaultConfigRepoURL
}

func resolveConfigRef(appCtx *AppContext, explicit string) string {
	for _, cand := range []string{
		explicit,
		appCtx.Flags.ConfigRef,
		os.Getenv("ANX_CONFIG_REF"),
	} {
		if strings.TrimSpace(cand) != "" {
			return strings.TrimSpace(cand)
		}
	}
	if appCtx.Viper != nil {
		if v := strings.TrimSpace(appCtx.Viper.GetString("config_ref")); v != "" {
			return v
		}
	}
	return ""
}

func resolveConfigSSHKey(appCtx *AppContext, explicit string) string {
	for _, cand := range []string{
		explicit,
		os.Getenv("ANX_SSH_KEY"),
	} {
		if strings.TrimSpace(cand) != "" {
			return strings.TrimSpace(cand)
		}
	}
	if appCtx.Viper != nil {
		if v := strings.TrimSpace(appCtx.Viper.GetString("config_ssh_key")); v != "" {
			return v
		}
	}
	return ""
}

func newRepoStatusCmd(appCtx *AppContext) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Inspect the status of the currently linked configuration repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := appCtx.RepoRoot
			info := inspectRepoStatus(cmd.Context(), root)
			info.ConfigRepoURL = resolveConfigRepoURL(appCtx, "")
			info.ConfigRef = resolveConfigRef(appCtx, "")
			if appCtx.Catalog != nil {
				info.Profiles = len(appCtx.Catalog.ListProfiles())
			}

			return emit(cmd, appCtx, info, func(w io.Writer) {
				st := styles(appCtx)
				fmt.Fprintln(w, st.Header.Render("ANX Configuration Repository Status"))
				fmt.Fprintf(w, "  • Location:      %s\n", info.RepoRoot)
				fmt.Fprintf(w, "  • Default/Source:%s\n", " "+info.ConfigRepoURL)
				if info.ConfigRef != "" {
					fmt.Fprintf(w, "  • Pinned Ref:    %s\n", info.ConfigRef)
				}
				if !info.Exists {
					fmt.Fprintf(w, "  • State:         %s (directory does not exist)\n", st.Error.Render("[NOT FOUND]"))
					fmt.Fprintln(w, "                   Run 'anx repo clone' or 'anx repo set <path-or-url>' to configure.")
					return
				}

				filesBadge := st.Success.Render("[YES]")
				if !info.HasFiles {
					filesBadge = st.Warning.Render("[NO] (files/ directory missing)")
				}
				fmt.Fprintf(w, "  • Has Dotfiles:  %s\n", filesBadge)

				manBadge := st.Success.Render("[YES]")
				if !info.HasManifests {
					manBadge = st.Muted.Render("[NO] (using ANX minimal engine defaults)")
				}
				fmt.Fprintf(w, "  • Has Manifests: %s\n", manBadge)
				fmt.Fprintf(w, "  • Profiles:      %d\n", info.Profiles)

				if info.IsGit {
					fmt.Fprintf(w, "  • Git Remote:    %s\n", info.RemoteURL)
					fmt.Fprintf(w, "  • Git Branch:    %s\n", info.Branch)
					cleanBadge := st.Success.Render("clean")
					if !info.Clean {
						cleanBadge = st.Warning.Render("uncommitted changes")
					}
					fmt.Fprintf(w, "  • Working Tree:  %s\n", cleanBadge)
				} else {
					fmt.Fprintf(w, "  • Version Control: %s\n", st.Muted.Render("Local directory (non-git)"))
				}
			})
		},
	}
}

func newRepoCloneCmd(appCtx *AppContext) *cobra.Command {
	var (
		targetDir string
		depth     int
		ref       string
		authMode  string
		sshKey    string
	)
	cmd := &cobra.Command{
		Use:   "clone [url]",
		Short: "Clone a remote configuration repository and link it to anx",
		Long: `Clones a Git configuration repository using automatic SSH discovery with HTTPS fallback.
Defaults to 'https://github.com/arturonavax/env' (or ANX_CONFIG_REPO) into '~/.local/share/anx/repo'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			explicit := ""
			if len(args) == 1 {
				explicit = args[0]
			}
			repoURL := resolveConfigRepoURL(appCtx, explicit)
			chosenRef := resolveConfigRef(appCtx, ref)
			chosenSSHKey := resolveConfigSSHKey(appCtx, sshKey)
			mode, err := gittransport.ParseAuthMode(authMode)
			if err != nil {
				return domain.NewExitError(domain.ExitUsageError, err)
			}

			dest := targetDir
			if dest == "" {
				dest = filepath.Join(appCtx.Platform.HomeDir(), ".local", "share", "anx", "repo")
			}
			dest = appCtx.Platform.ExpandPath(dest)

			if isDir(filepath.Join(dest, "manifests")) || isDir(filepath.Join(dest, "files")) {
				fmt.Fprintf(cmd.OutOrStdout(), "Repository already exists at %s.\n", dest)
				return linkRepoRoot(appCtx, dest, repoURL, chosenRef, cmd.OutOrStdout())
			}

			if appCtx.Flags.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would clone %s (ref=%q, auth=%s) into %s (depth: %d)\n", repoURL, chosenRef, mode, dest, depth)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Cloning configuration repository from %s into %s...\n", gittransport.NormalizeURL(repoURL).Redacted, dest)
			adapter := newGitAdapter()
			probe, err := adapter.Clone(cmd.Context(), gittransport.CloneOptions{
				URL:    repoURL,
				Ref:    chosenRef,
				Dest:   dest,
				Depth:  depth,
				Auth:   mode,
				SSHKey: chosenSSHKey,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Transport selected: %s (%s)\n", probe.Transport, probe.Normalized.Redacted)
			if chosenSSHKey != "" && appCtx.Platform != nil {
				_ = persistRepoSSHKey(appCtx.Platform.ConfigDir(), chosenSSHKey)
			}
			return linkRepoRoot(appCtx, dest, probe.Normalized.Redacted, chosenRef, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&targetDir, "dir", "", "Target destination directory for cloned repository (default: ~/.local/share/anx/repo)")
	cmd.Flags().IntVar(&depth, "depth", 1, "Shallow clone depth (0 for full history)")
	cmd.Flags().StringVar(&ref, "ref", "", "Branch, tag, or commit to checkout")
	cmd.Flags().StringVar(&authMode, "auth", "auto", "Git authentication mode (auto|ssh|https)")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key for private Git repositories")
	return cmd
}

func newRepoSetCmd(appCtx *AppContext) *cobra.Command {
	var (
		force    bool
		ref      string
		authMode string
		sshKey   string
	)
	cmd := &cobra.Command{
		Use:   "set <path-or-url>",
		Short: "Switch the active configuration repository to a local directory or Git URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(args[0])
			chosenRef := resolveConfigRef(appCtx, ref)
			chosenSSHKey := resolveConfigSSHKey(appCtx, sshKey)
			norm := gittransport.NormalizeURL(raw)

			if !norm.IsLocal {
				mode, err := gittransport.ParseAuthMode(authMode)
				if err != nil {
					return domain.NewExitError(domain.ExitUsageError, err)
				}
				dest := filepath.Join(appCtx.Platform.HomeDir(), ".local", "share", "anx", "repo")
				if appCtx.Flags.DryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would switch config repo to %s (dest: %s)\n", norm.Redacted, dest)
					return nil
				}
				// Clone into a fresh target directory if switching URLs
				if isDir(dest) {
					if err := os.RemoveAll(dest); err != nil {
						return fmt.Errorf("clearing previous repo cache %s: %w", dest, err)
					}
				}
				adapter := newGitAdapter()
				probe, err := adapter.Clone(cmd.Context(), gittransport.CloneOptions{
					URL:    raw,
					Ref:    chosenRef,
					Dest:   dest,
					Depth:  1,
					Auth:   mode,
					SSHKey: chosenSSHKey,
				})
				if err != nil {
					return err
				}
				if chosenSSHKey != "" && appCtx.Platform != nil {
					_ = persistRepoSSHKey(appCtx.Platform.ConfigDir(), chosenSSHKey)
				}
				return linkRepoRoot(appCtx, dest, probe.Normalized.Redacted, chosenRef, cmd.OutOrStdout())
			}

			target := appCtx.Platform.ExpandPath(raw)
			absPath, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolving path %q: %w", target, err)
			}
			if !isDir(absPath) {
				return domain.NewExitError(domain.ExitUsageError, fmt.Errorf("directory %s does not exist", absPath))
			}

			hasManifests := isDir(filepath.Join(absPath, "manifests"))
			hasFiles := isDir(filepath.Join(absPath, "files"))
			if !hasManifests && !hasFiles && !force {
				return domain.NewExitError(
					domain.ExitUsageError,
					fmt.Errorf("%s contains neither 'manifests/' nor 'files/'; use --force to link anyway", absPath),
				)
			}

			return linkRepoRoot(appCtx, absPath, absPath, chosenRef, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Link directory even if manifests/ or files/ are not detected")
	cmd.Flags().StringVar(&ref, "ref", "", "Git branch, tag, or commit to pin")
	cmd.Flags().StringVar(&authMode, "auth", "auto", "Git authentication mode (auto|ssh|https)")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key for private Git repositories")
	return cmd
}

func newRepoUpdateCmd(appCtx *AppContext) *cobra.Command {
	var (
		ref    string
		sshKey string
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Fetch and fast-forward updates for the linked Git configuration repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := appCtx.RepoRoot
			if !isDir(filepath.Join(root, ".git")) {
				return domain.NewExitError(domain.ExitPreflightFailed, fmt.Errorf("%s is not a git repository; nothing to update", root))
			}
			chosenRef := resolveConfigRef(appCtx, ref)
			chosenSSHKey := resolveConfigSSHKey(appCtx, sshKey)
			if appCtx.Flags.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would update git repository at %s (ref=%q)\n", root, chosenRef)
				return nil
			}
			adapter := newGitAdapter()
			if err := adapter.UpdateWithOptions(cmd.Context(), gittransport.UpdateOptions{
				Dir:    root,
				Ref:    chosenRef,
				SSHKey: chosenSSHKey,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated configuration repository at %s\n", root)
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Optional branch, tag, or commit to checkout")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key for private Git repositories")
	return cmd
}

type repoVerifyReport struct {
	RepoRoot     string   `json:"repo_root"`
	Valid        bool     `json:"valid"`
	PackageCount int      `json:"package_count"`
	ConfigCount  int      `json:"config_count"`
	ProfileCount int      `json:"profile_count"`
	Errors       []string `json:"errors,omitempty"`
}

func newRepoVerifyCmd(appCtx *AppContext) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Validate external manifests, profiles, and referenced dotfile assets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := appCtx.RepoRoot
			report := repoVerifyReport{RepoRoot: root, Valid: true}

			manifestsDir := filepath.Join(root, "manifests")
			if !isDir(manifestsDir) {
				report.Valid = false
				report.Errors = append(report.Errors, fmt.Sprintf("manifests directory missing at %s", manifestsDir))
			} else {
				cat, err := catalog.NewRegistryWithOverlays(manifestsDir, "")
				if err != nil {
					report.Valid = false
					report.Errors = append(report.Errors, fmt.Sprintf("manifest schema error: %v", err))
				} else {
					report.PackageCount = len(cat.Catalog().ListPackageSpecs())
					configs := cat.Catalog().ListConfigurations()
					report.ConfigCount = len(configs)
					profiles := cat.ListProfiles()
					report.ProfileCount = len(profiles)

					for _, p := range profiles {
						if _, err := cat.ResolveProfilePackages(p.Name); err != nil {
							report.Valid = false
							report.Errors = append(report.Errors, fmt.Sprintf("profile %q: %v", p.Name, err))
						}
					}
					for _, cfg := range configs {
						srcPath := filepath.Join(root, filepath.FromSlash(cfg.Source))
						if _, err := os.Stat(srcPath); err != nil {
							report.Valid = false
							report.Errors = append(report.Errors, fmt.Sprintf("config %q missing source %q", cfg.ID, cfg.Source))
						}
					}
				}
			}

			err := emit(cmd, appCtx, report, func(w io.Writer) {
				st := styles(appCtx)
				fmt.Fprintln(w, st.Header.Render("ANX Configuration Repository Verification"))
				fmt.Fprintf(w, "  • Root:      %s\n", report.RepoRoot)
				fmt.Fprintf(w, "  • Packages:  %d\n", report.PackageCount)
				fmt.Fprintf(w, "  • Configs:   %d\n", report.ConfigCount)
				fmt.Fprintf(w, "  • Profiles:  %d\n", report.ProfileCount)
				if report.Valid {
					fmt.Fprintf(w, "  • Result:    %s\n", st.Success.Render("[VALID]"))
				} else {
					fmt.Fprintf(w, "  • Result:    %s\n", st.Error.Render("[INVALID]"))
					for _, e := range report.Errors {
						fmt.Fprintf(w, "    - %s\n", e)
					}
				}
			})
			if err != nil {
				return err
			}
			if !report.Valid {
				return domain.NewExitError(domain.ExitPreflightFailed, errors.New("configuration repository verification failed"))
			}
			return nil
		},
	}
}

func linkRepoRoot(appCtx *AppContext, path, sourceURL, ref string, out io.Writer) error {
	appCtx.RepoRoot = path
	if appCtx.Store != nil {
		if err := appCtx.Store.SetRepoRoot(path); err != nil {
			return fmt.Errorf("persisting repo root to state: %w", err)
		}
	}
	if appCtx.SyncSvc != nil {
		appCtx.SyncSvc.SetRepoRoot(path)
	}
	if appCtx.Platform != nil {
		_ = persistRepoConfig(appCtx.Platform.ConfigDir(), path, sourceURL, ref)
	}

	fmt.Fprintf(out, "Linked configuration repository: %s\n", path)
	return nil
}

func persistRepoConfig(configDir, repoPath, sourceURL, ref string) error {
	cfgPath := filepath.Join(configDir, "config.yaml")
	data := map[string]any{}
	if raw, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(raw, &data)
	}
	data["repo"] = repoPath
	if sourceURL != "" {
		data["config_repo"] = sourceURL
	}
	if ref != "" {
		data["config_ref"] = ref
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

func persistRepoSSHKey(configDir, sshKey string) error {
	cfgPath := filepath.Join(configDir, "config.yaml")
	data := map[string]any{}
	if raw, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(raw, &data)
	}
	data["config_ssh_key"] = sshKey
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

func inspectRepoStatus(ctx context.Context, root string) repoStatusInfo {
	info := repoStatusInfo{
		RepoRoot:     root,
		Exists:       isDir(root),
		HasFiles:     isDir(filepath.Join(root, "files")),
		HasManifests: isDir(filepath.Join(root, "manifests")),
		IsGit:        isDir(filepath.Join(root, ".git")),
	}

	if info.IsGit {
		if out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output(); err == nil {
			info.RemoteURL = strings.TrimSpace(string(out))
		}
		if out, err := exec.CommandContext(ctx, "git", "-C", root, "branch", "--show-current").Output(); err == nil {
			info.Branch = strings.TrimSpace(string(out))
		}
		if out, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").Output(); err == nil {
			info.Clean = len(strings.TrimSpace(string(out))) == 0
		}
	}

	return info
}

func ensureDotfilesRepo(cmd *cobra.Command, appCtx *AppContext) error {
	if isDir(filepath.Join(appCtx.RepoRoot, "files")) {
		return nil
	}

	targetDir := filepath.Join(appCtx.Platform.HomeDir(), ".local", "share", "anx", "repo")
	defaultURL := resolveConfigRepoURL(appCtx, "")
	chosenRef := resolveConfigRef(appCtx, "")

	if appCtx.Flags.Offline {
		return domain.NewExitError(
			domain.ExitPreflightFailed,
			fmt.Errorf("configuration repository missing at %q and offline mode is enabled; run 'anx repo clone' when online", appCtx.RepoRoot),
		)
	}

	shouldClone := appCtx.Flags.Yes
	if !shouldClone && isTerminal(cmd) {
		fmt.Fprintf(cmd.OutOrStdout(), "\nNo local configuration repository found for synchronization.\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  Target directory: %s\n", targetDir)
		fmt.Fprintf(cmd.OutOrStdout(), "  Remote URL:       %s\n", defaultURL)
		fmt.Print("Would you like to clone the configuration repository now? [Y/n]: ")

		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.TrimSpace(strings.ToLower(response))
		shouldClone = response == "" || response == "y" || response == "yes"
	}

	if !shouldClone {
		return domain.NewExitError(
			domain.ExitUsageError,
			errors.New("configuration repository is required for synchronization; configure it with 'anx repo clone' or '--config-repo'"),
		)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Cloning configuration repository into %s...\n", targetDir)
	adapter := newGitAdapter()
	probe, err := adapter.Clone(cmd.Context(), gittransport.CloneOptions{
		URL:    defaultURL,
		Ref:    chosenRef,
		Dest:   targetDir,
		Depth:  1,
		Auth:   gittransport.AuthAuto,
		SSHKey: resolveConfigSSHKey(appCtx, ""),
	})
	if err != nil {
		return fmt.Errorf("failed to clone configuration repository: %w", err)
	}

	return linkRepoRoot(appCtx, targetDir, probe.Normalized.Redacted, chosenRef, cmd.OutOrStdout())
}
