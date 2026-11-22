package gittransport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TrackingInfo describes divergence from the locally cached upstream branch.
type TrackingInfo struct {
	UpstreamRevision   string   `json:"upstream_revision" yaml:"upstream_revision"`
	Ahead              int      `json:"ahead" yaml:"ahead"`
	Behind             int      `json:"behind" yaml:"behind"`
	IncomingPaths      []string `json:"incoming_paths,omitempty" yaml:"incoming_paths,omitempty"`
	IncomingPathsKnown bool     `json:"incoming_paths_known,omitempty" yaml:"incoming_paths_known,omitempty"`
}

// TrackingRemote returns the configured remote of the current branch.
func (a *Adapter) TrackingRemote(ctx context.Context, dir string) (string, error) {
	_, remote, err := a.trackingBranch(ctx, dir)
	return remote, err
}

func (a *Adapter) trackingBranch(ctx context.Context, dir string) (string, string, error) {
	branch, err := a.readLocalValue(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		return "", "", errors.New("no current Git branch")
	}
	remote, err := a.readLocalValue(ctx, dir, "config", "--get", "branch."+branch+".remote")
	if err != nil || remote == "" {
		return "", "", errors.New("the current Git branch has no configured upstream remote")
	}
	return branch, remote, nil
}

// RefreshTracking fetches only the named upstream into its remote-tracking ref.
// A custom refspec cannot refresh other branches, tags or submodules during a check.
func (a *Adapter) RefreshTracking(ctx context.Context, opts UpdateOptions) error {
	branch, remote, err := a.trackingBranch(ctx, opts.Dir)
	if err != nil || remote == "." {
		return err
	}
	source, sourceErr := a.readLocalValue(ctx, opts.Dir, "config", "--get", "branch."+branch+".merge")
	dest, destErr := a.readLocalValue(ctx, opts.Dir, "rev-parse", "--symbolic-full-name", "@{upstream}")
	if sourceErr != nil || destErr != nil || !strings.HasPrefix(source, "refs/heads/") ||
		!strings.HasPrefix(dest, "refs/remotes/") || strings.ContainsAny(source+dest, "\r\n") {
		return errors.New("the upstream is not backed by a local remote-tracking branch")
	}
	if _, err := a.readLocalValue(ctx, opts.Dir, "check-ref-format", source); err != nil {
		return fmt.Errorf("invalid upstream ref: %w", err)
	}
	if _, err := a.readLocalValue(ctx, opts.Dir, "check-ref-format", dest); err != nil {
		return fmt.Errorf("invalid local tracking ref: %w", err)
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	env, err := NonInteractiveEnvWithKey(opts.SSHKey)
	if err != nil {
		return err
	}
	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	args := []string{
		"fetch", "--no-tags", "--no-prune", "--no-recurse-submodules", "--refmap=", "--",
		remote, "+" + source + ":" + dest,
	}
	if _, stderr, err := runner.Run(fetchCtx, env, opts.Dir, "git", args...); err != nil {
		return classifyGitError(opts.Dir, source, "git", stderr, err)
	}
	return nil
}

// LocalTracking reads cached upstream information without contacting a remote.
func (a *Adapter) LocalTracking(ctx context.Context, dir, head string, withPaths bool) (*TrackingInfo, error) {
	upstream, err := a.LocalRevision(ctx, dir, "@{upstream}")
	if err != nil {
		return nil, err
	}
	counts, err := a.readLocalValue(ctx, dir, "rev-list", "--left-right", "--count", head+"..."+upstream)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(counts)
	if len(fields) != 2 {
		return nil, errors.New("Git did not report both divergence counts")
	}
	ahead, aheadErr := strconv.Atoi(fields[0])
	behind, behindErr := strconv.Atoi(fields[1])
	if aheadErr != nil || behindErr != nil || ahead < 0 || behind < 0 {
		return nil, errors.New("Git reported invalid divergence counts")
	}
	info := &TrackingInfo{UpstreamRevision: upstream, Ahead: ahead, Behind: behind}
	if withPaths && behind > 0 {
		base, err := a.readLocalValue(ctx, dir, "merge-base", head, upstream)
		if err == nil && base != "" {
			info.IncomingPaths, err = a.ChangedEnvironmentPaths(ctx, dir, base, upstream)
			info.IncomingPathsKnown = err == nil
		}
	}
	return info, nil
}

// ChangedEnvironmentPaths lists changed manifest, dotfile and script paths between two commit IDs.
func (a *Adapter) ChangedEnvironmentPaths(ctx context.Context, dir, before, after string) ([]string, error) {
	args := []string{
		"diff", "--name-only", "-z", "--no-ext-diff", "--no-renames", before, after, "--", "manifests", "files", "scripts",
	}
	out, err := a.readLocalOutput(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
}
