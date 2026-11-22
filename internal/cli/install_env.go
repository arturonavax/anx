package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arturonavax/anx/internal/bootstrap"
	"github.com/arturonavax/anx/internal/domain"
	"github.com/arturonavax/anx/internal/gittransport"
)

type installerEnvironmentSelection struct {
	repo string
	ref  string
}

func enrollInstallerEnvironment(cmd *cobra.Command, appCtx *AppContext, selection installerEnvironmentSelection) error {
	flags := appCtx.Flags
	if os.Getenv("ANX_INSTALL_ENV") != "1" || os.Getenv("ANX_BOOTSTRAP_ONLY") == "1" ||
		flags.DryRun || flags.Offline || flags.ScriptMode {
		return nil
	}
	applies := cmd == cmd.Root() && flags.Yes ||
		cmd.Parent() == cmd.Root() && (cmd.Name() == "apply" || cmd.Name() == "wizard")
	if !applies {
		return nil
	}

	if selection.repo == "" {
		selection.repo = os.Getenv("ANX_CONFIG_REPO")
	}
	if selection.ref == "" {
		selection.ref = os.Getenv("ANX_CONFIG_REF")
	}
	requested := gittransport.NormalizeURL(selection.repo)
	ref := resolveConfigRef(appCtx, "")
	adapter := newGitAdapter()
	ctx := cmd.Context()
	dest := bootstrap.ManagedEnvironmentDir(appCtx.Platform.HomeDir())
	existingRoot := appCtx.RepoRoot
	custom := os.Getenv("ANX_CONFIG_REPO_DIR")
	if custom != "" {
		if strings.Contains(custom, "://") {
			return domain.NewExitError(domain.ExitUsageError, fmt.Errorf(
				"ANX_CONFIG_REPO_DIR needs a directory path; use --config-repo to select a Git URL",
			))
		}
		var err error
		dest, err = filepath.Abs(appCtx.Platform.ExpandPath(custom))
		if err != nil {
			return err
		}
		existingRoot = dest
	}
	if requested.IsLocal {
		root, err := filepath.Abs(appCtx.Platform.ExpandPath(requested.Canonical))
		if err != nil {
			return err
		}
		if custom != "" && dest != root {
			return domain.NewExitError(domain.ExitUsageError, fmt.Errorf(
				"a local --config-repo is already a checkout; use that same path or select a Git URL",
			))
		}
		if err := checkInstallerEnvironmentRef(ctx, adapter, root, selection.ref); err != nil {
			return err
		}
		if root == appCtx.RepoRoot {
			return nil
		}
		return activateInstallerEnvironment(cmd, appCtx, root, root, ref)
	}
	if isDir(filepath.Join(existingRoot, "manifests")) {
		source := existingRoot
		if requested.Canonical != "" {
			origin, err := adapter.LocalOrigin(ctx, existingRoot)
			if err != nil || !sameInstallerEnvironmentOrigin(requested, gittransport.NormalizeURL(origin)) {
				return domain.NewExitError(domain.ExitPreflightFailed, fmt.Errorf(
					"linked environment %q does not match %s; link the requested environment before installing",
					existingRoot, requested.Redacted,
				))
			}
			source = requested.Redacted
		} else if custom != "" && isGitEnvironment(existingRoot) {
			if origin, err := adapter.LocalOrigin(ctx, existingRoot); err == nil && origin != "" {
				source = gittransport.NormalizeURL(origin).Redacted
			}
		}
		if err := checkInstallerEnvironmentRef(ctx, adapter, existingRoot, selection.ref); err != nil {
			return err
		}
		if existingRoot != appCtx.RepoRoot {
			return activateInstallerEnvironment(cmd, appCtx, existingRoot, source, strings.TrimSpace(selection.ref))
		}
		return nil
	}

	source := resolveConfigRepoURL(appCtx, "")
	if custom != "" && gittransport.NormalizeURL(source).IsLocal {
		return domain.NewExitError(domain.ExitUsageError, fmt.Errorf(
			"choose an existing environment checkout or supply --config-repo <git-url> to clone into a new path",
		))
	}
	if err := checkCheckoutDestination(dest); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"Cloning configuration repository from %s into %s...\n", gittransport.NormalizeURL(source).Redacted, dest)
	probe, err := adapter.Clone(ctx, gittransport.CloneOptions{
		URL:    source,
		Ref:    ref,
		Dest:   dest,
		Depth:  1,
		Auth:   gittransport.AuthAuto,
		SSHKey: resolveConfigSSHKey(appCtx, ""),
	})
	if err != nil {
		return err
	}
	return activateInstallerEnvironment(cmd, appCtx, dest, probe.Normalized.Redacted, ref)
}

func sameInstallerEnvironmentOrigin(requested, actual gittransport.NormalizedRepo) bool {
	if actual.Redacted == "" {
		return false
	}
	if requested.Redacted == actual.Redacted {
		return true
	}
	return requested.SSHCandidate != "" && requested.SSHCandidate == actual.SSHCandidate
}

func checkInstallerEnvironmentRef(ctx context.Context, adapter *gittransport.Adapter, root, ref string) error {
	if ref = strings.TrimSpace(ref); ref == "" {
		return nil
	}
	head, headErr := adapter.LocalRevision(ctx, root, "HEAD")
	pinned, refErr := adapter.LocalRevision(ctx, root, ref)
	if headErr != nil || refErr != nil || head == "" || head != pinned {
		return domain.NewExitError(domain.ExitPreflightFailed, fmt.Errorf(
			"linked environment %q is not at ref %q; first run 'anx repo update --ref <ref>' or link a matching checkout",
			root, ref,
		))
	}
	return nil
}

func activateInstallerEnvironment(cmd *cobra.Command, appCtx *AppContext, root, source, ref string) error {
	if !isDir(filepath.Join(root, "manifests")) {
		return domain.NewExitError(
			domain.ExitPreflightFailed, fmt.Errorf("environment %q does not contain a manifests directory", root))
	}
	if err := appCtx.Close(); err != nil {
		return fmt.Errorf("closing the initial ANX runtime: %w", err)
	}
	next, err := buildAppContext(appCtx.Version, appCtx.Commit, appCtx.Date, root)
	if err != nil {
		return err
	}
	next.Flags = appCtx.Flags
	*appCtx = *next
	return linkRepoRoot(appCtx, root, source, ref, cmd.OutOrStdout())
}
