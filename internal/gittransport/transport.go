package gittransport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AuthMode controls Git transport selection.
type AuthMode string

const (
	AuthAuto  AuthMode = "auto"
	AuthSSH   AuthMode = "ssh"
	AuthHTTPS AuthMode = "https"
)

// ErrorKind classifies Git transport failures for actionable diagnostics.
type ErrorKind string

const (
	ErrKindAuth       ErrorKind = "auth_failed"
	ErrKindNotFound   ErrorKind = "repo_not_found"
	ErrKindNetwork    ErrorKind = "network_failed"
	ErrKindInvalidRef ErrorKind = "invalid_ref"
	ErrKindGitMissing ErrorKind = "git_missing"
	ErrKindUnknown    ErrorKind = "unknown"
)

// TransportError wraps a classified Git transport failure without leaking credentials.
type TransportError struct {
	Kind        ErrorKind
	URL         string
	Transport   string
	Detail      string
	Remediation string
	Cause       error
}

func (e *TransportError) Error() string {
	if e.Remediation != "" {
		return fmt.Sprintf("git transport [%s] for %s (%s): %s — %s", e.Kind, e.URL, e.Transport, e.Detail, e.Remediation)
	}
	return fmt.Sprintf("git transport [%s] for %s (%s): %s", e.Kind, e.URL, e.Transport, e.Detail)
}

func (e *TransportError) Unwrap() error { return e.Cause }

// NormalizedRepo describes a parsed and normalized repository locator.
type NormalizedRepo struct {
	Raw          string
	Canonical    string // Normalized URL or absolute local path
	Redacted     string // Credential-stripped URL safe for logs and persistence
	Host         string
	OwnerRepo    string // e.g. "arturonavax/env"
	SSHCandidate string // e.g. "git@github.com:arturonavax/env.git"
	HTTPSURL     string // e.g. "https://github.com/arturonavax/env.git"
	IsLocal      bool
	IsSSH        bool
	IsHTTPS      bool
}

// ProbeResult records the verified transport selected for a repository.
type ProbeResult struct {
	Normalized  NormalizedRepo
	ResolvedURL string
	Transport   string // "local", "ssh", "https"
	Ref         string
	CommitSHA   string
}

// CommandRunner executes a command with bounded environment and returns stdout/stderr.
type CommandRunner interface {
	Run(ctx context.Context, env []string, dir string, name string, args ...string) (stdout, stderr string, err error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	if dir != "" {
		cmd.Dir = dir
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// Adapter performs non-interactive Git probing, cloning, and updating.
type Adapter struct {
	Runner  CommandRunner
	Timeout time.Duration
}

// NewAdapter creates a default Git transport adapter with safe non-interactive timeouts.
func NewAdapter() *Adapter {
	return &Adapter{
		Runner:  execRunner{},
		Timeout: 15 * time.Second,
	}
}

// ParseAuthMode validates and normalizes an authentication mode string.
func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return AuthAuto, nil
	case "ssh":
		return AuthSSH, nil
	case "https":
		return AuthHTTPS, nil
	default:
		return "", fmt.Errorf("unsupported auth mode %q (expected auto, ssh, or https)", s)
	}
}

var sshCapableHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
	"codeberg.org":  true,
}

// NormalizeURL parses a local path, GitHub shorthand (owner/repo), HTTPS URL, or SSH URL.
// Credentials in userinfo are stripped from Redacted and never persisted.
func NormalizeURL(raw string) NormalizedRepo {
	trimmed := strings.TrimSpace(raw)
	res := NormalizedRepo{Raw: trimmed}
	if trimmed == "" {
		return res
	}

	// File URL or local filesystem path
	if strings.HasPrefix(trimmed, "file://") {
		p := strings.TrimPrefix(trimmed, "file://")
		res.IsLocal = true
		res.Canonical = filepath.Clean(p)
		res.Redacted = res.Canonical
		return res
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "~/") {
		res.IsLocal = true
		res.Canonical = trimmed
		res.Redacted = trimmed
		return res
	}
	if info, err := os.Stat(trimmed); err == nil && info.IsDir() {
		abs, aErr := filepath.Abs(trimmed)
		if aErr == nil {
			res.Canonical = abs
		} else {
			res.Canonical = trimmed
		}
		res.IsLocal = true
		res.Redacted = res.Canonical
		return res
	}

	// SCP-style SSH: git@github.com:owner/repo.git
	if !strings.Contains(trimmed, "://") && strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") {
		atIdx := strings.Index(trimmed, "@")
		colonIdx := strings.Index(trimmed[atIdx:], ":")
		if colonIdx > 0 {
			host := strings.ToLower(trimmed[atIdx+1 : atIdx+colonIdx])
			pathPart := strings.TrimPrefix(trimmed[atIdx+colonIdx+1:], "/")
			ownerRepo := strings.TrimSuffix(pathPart, ".git")
			res.IsSSH = true
			res.Host = host
			res.OwnerRepo = ownerRepo
			res.SSHCandidate = trimmed
			if sshCapableHosts[host] && ownerRepo != "" {
				res.HTTPSURL = fmt.Sprintf("https://%s/%s.git", host, ownerRepo)
			}
			res.Canonical = trimmed
			res.Redacted = trimmed
			return res
		}
	}

	// ssh:// URL
	if strings.HasPrefix(trimmed, "ssh://") {
		u, err := url.Parse(trimmed)
		if err == nil {
			host := strings.ToLower(u.Hostname())
			ownerRepo := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
			res.IsSSH = true
			res.Host = host
			res.OwnerRepo = ownerRepo
			res.SSHCandidate = trimmed
			if sshCapableHosts[host] && ownerRepo != "" {
				res.HTTPSURL = fmt.Sprintf("https://%s/%s.git", host, ownerRepo)
			}
			res.Canonical = trimmed
			res.Redacted = redactURL(u)
			return res
		}
	}

	// GitHub shorthand: owner/repo (no scheme, single slash, no spaces)
	if !strings.Contains(trimmed, "://") && strings.Count(trimmed, "/") == 1 && !strings.ContainsAny(trimmed, " \t\\") {
		ownerRepo := strings.TrimSuffix(trimmed, ".git")
		httpsURL := fmt.Sprintf("https://github.com/%s.git", ownerRepo)
		res.IsHTTPS = true
		res.Host = "github.com"
		res.OwnerRepo = ownerRepo
		res.HTTPSURL = httpsURL
		res.SSHCandidate = fmt.Sprintf("git@github.com:%s.git", ownerRepo)
		res.Canonical = httpsURL
		res.Redacted = httpsURL
		return res
	}

	// HTTP(S) URL
	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		u, err := url.Parse(trimmed)
		if err == nil {
			host := strings.ToLower(u.Hostname())
			pathPart := strings.Trim(u.Path, "/")
			ownerRepo := strings.TrimSuffix(pathPart, ".git")
			redacted := redactURL(u)
			res.IsHTTPS = true
			res.Host = host
			res.OwnerRepo = ownerRepo
			res.HTTPSURL = redacted
			res.Canonical = redacted
			res.Redacted = redacted
			if sshCapableHosts[host] && strings.Count(ownerRepo, "/") >= 1 {
				res.SSHCandidate = fmt.Sprintf("git@%s:%s.git", host, ownerRepo)
			}
			return res
		}
	}

	res.Canonical = trimmed
	res.Redacted = trimmed
	return res
}

func redactURL(u *url.URL) string {
	cp := *u
	cp.User = nil
	return cp.String()
}

// NonInteractiveEnv builds a safe process environment for unattended Git operations.
// It never disables SSH host-key verification and never uses StrictHostKeyChecking=no.
func NonInteractiveEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+4)
	var existingSSH string
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "GIT_TERMINAL_PROMPT="):
			continue
		case strings.HasPrefix(kv, "GIT_SSH_COMMAND="):
			existingSSH = strings.TrimPrefix(kv, "GIT_SSH_COMMAND=")
			continue
		default:
			out = append(out, kv)
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	if existingSSH != "" {
		if !strings.Contains(existingSSH, "BatchMode=") {
			existingSSH += " -o BatchMode=yes"
		}
		out = append(out, "GIT_SSH_COMMAND="+existingSSH)
	} else {
		out = append(out, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=8")
	}
	return out
}

// NonInteractiveEnvWithKey builds a safe process environment for unattended Git operations.
// When sshKey is non-empty, it expands ~/ via os.UserHomeDir, validates that the key file
// exists, and configures GIT_SSH_COMMAND with -i <key> -o IdentitiesOnly=yes while preserving
// BatchMode=yes, ConnectTimeout=8, and strict SSH host-key verification.
func NonInteractiveEnvWithKey(sshKey string) ([]string, error) {
	trimmedKey := strings.TrimSpace(sshKey)
	if trimmedKey == "" {
		return NonInteractiveEnv(), nil
	}

	expanded, err := expandSSHKeyPath(trimmedKey)
	if err != nil {
		return nil, &TransportError{
			Kind:        ErrKindAuth,
			Transport:   "ssh",
			Detail:      fmt.Sprintf("cannot resolve SSH key path %q: %v", trimmedKey, err),
			Remediation: "Provide an explicit path to an existing SSH private key file",
			Cause:       err,
		}
	}

	info, err := os.Stat(expanded)
	if err != nil {
		return nil, &TransportError{
			Kind:        ErrKindAuth,
			Transport:   "ssh",
			Detail:      fmt.Sprintf("SSH identity key not found: %s", expanded),
			Remediation: "Verify the SSH private key path exists and has readable permissions (chmod 600)",
			Cause:       err,
		}
	}
	if info.IsDir() {
		return nil, &TransportError{
			Kind:        ErrKindAuth,
			Transport:   "ssh",
			Detail:      fmt.Sprintf("SSH identity key path is a directory: %s", expanded),
			Remediation: "Provide the path to an SSH private key file rather than a directory",
		}
	}

	base := os.Environ()
	out := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GIT_TERMINAL_PROMPT=") || strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	sshCmd := fmt.Sprintf(
		"ssh -i %s -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=8",
		shellQuote(filepath.ToSlash(expanded)),
	)
	out = append(out, "GIT_SSH_COMMAND="+sshCmd)
	return out, nil
}

func expandSSHKeyPath(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
	}
	if key == "~" || strings.HasPrefix(key, "~/") || strings.HasPrefix(key, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if key == "~" {
			return filepath.Clean(home), nil
		}
		return filepath.Clean(filepath.Join(home, key[2:])), nil
	}
	return filepath.Clean(key), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ProbeOptions configures a non-interactive repository probe.
type ProbeOptions struct {
	Auth   AuthMode
	Ref    string
	SSHKey string
}

// Probe verifies repository accessibility and selects the working transport.
func (a *Adapter) Probe(ctx context.Context, rawURL, ref string, mode AuthMode) (ProbeResult, error) {
	return a.ProbeWithOptions(ctx, rawURL, ProbeOptions{
		Auth: mode,
		Ref:  ref,
	})
}

// ProbeWithOptions verifies repository accessibility using explicit auth mode, ref, and optional SSH key.
func (a *Adapter) ProbeWithOptions(ctx context.Context, rawURL string, opts ProbeOptions) (ProbeResult, error) {
	mode := opts.Auth
	if mode == "" {
		mode = AuthAuto
	}
	ref := opts.Ref
	norm := NormalizeURL(rawURL)
	if norm.Canonical == "" {
		return ProbeResult{}, &TransportError{
			Kind:   ErrKindNotFound,
			URL:    rawURL,
			Detail: "empty repository URL or path",
		}
	}

	if norm.IsLocal {
		info, err := os.Stat(norm.Canonical)
		if err != nil || !info.IsDir() {
			return ProbeResult{}, &TransportError{
				Kind:        ErrKindNotFound,
				URL:         norm.Redacted,
				Transport:   "local",
				Detail:      "local directory does not exist",
				Remediation: "Verify the path exists and is readable",
				Cause:       err,
			}
		}
		return ProbeResult{
			Normalized:  norm,
			ResolvedURL: norm.Canonical,
			Transport:   "local",
			Ref:         ref,
		}, nil
	}

	// Validate an explicitly supplied SSH key up front so a bad path is never masked by fallback.
	if strings.TrimSpace(opts.SSHKey) != "" && mode != AuthHTTPS {
		if _, err := NonInteractiveEnvWithKey(opts.SSHKey); err != nil {
			var tErr *TransportError
			if errors.As(err, &tErr) && tErr.URL == "" {
				tErr.URL = norm.Redacted
			}
			return ProbeResult{}, err
		}
	}

	type candidate struct {
		url       string
		transport string
	}
	var candidates []candidate

	switch mode {
	case AuthSSH:
		if norm.SSHCandidate == "" {
			return ProbeResult{}, &TransportError{
				Kind:        ErrKindAuth,
				URL:         norm.Redacted,
				Transport:   "ssh",
				Detail:      "cannot derive SSH URL for this host",
				Remediation: "Provide an explicit SSH URL (git@host:owner/repo.git) or use --auth https",
			}
		}
		candidates = append(candidates, candidate{url: norm.SSHCandidate, transport: "ssh"})
	case AuthHTTPS:
		target := norm.HTTPSURL
		if target == "" {
			target = norm.Canonical
		}
		candidates = append(candidates, candidate{url: target, transport: "https"})
	default: // AuthAuto
		if norm.IsSSH {
			candidates = append(candidates, candidate{url: norm.SSHCandidate, transport: "ssh"})
		} else {
			if norm.SSHCandidate != "" {
				candidates = append(candidates, candidate{url: norm.SSHCandidate, transport: "ssh"})
			}
			httpsTarget := norm.HTTPSURL
			if httpsTarget == "" {
				httpsTarget = norm.Canonical
			}
			candidates = append(candidates, candidate{url: httpsTarget, transport: "https"})
		}
	}

	var lastErr error
	for _, cand := range candidates {
		sshKey := ""
		if cand.transport == "ssh" {
			sshKey = opts.SSHKey
		}
		sha, err := a.lsRemote(ctx, cand.url, ref, cand.transport, sshKey)
		if err == nil {
			return ProbeResult{
				Normalized:  norm,
				ResolvedURL: cand.url,
				Transport:   cand.transport,
				Ref:         ref,
				CommitSHA:   sha,
			}, nil
		}
		lastErr = err
		// If ref itself is invalid on a reachable repo, don't mask it by falling back unless auth failed
		var tErr *TransportError
		if errors.As(err, &tErr) && tErr.Kind == ErrKindInvalidRef {
			return ProbeResult{}, err
		}
	}

	return ProbeResult{}, lastErr
}

// VerifySSHAccess forces SSH transport and verifies that a remote repository (such as a private repo)
// can be queried/cloned via SSH using the configured key (or default ssh-agent when sshKey is empty).
func (a *Adapter) VerifySSHAccess(ctx context.Context, rawURL string, sshKey string) (ProbeResult, error) {
	return a.ProbeWithOptions(ctx, rawURL, ProbeOptions{
		Auth:   AuthSSH,
		SSHKey: sshKey,
	})
}

func (a *Adapter) lsRemote(ctx context.Context, targetURL, ref, transport, sshKey string) (string, error) {
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	pCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env, err := NonInteractiveEnvWithKey(sshKey)
	if err != nil {
		return "", err
	}

	args := []string{"ls-remote", "--exit-code", targetURL}
	if strings.TrimSpace(ref) != "" {
		args = append(args, strings.TrimSpace(ref))
	} else {
		args = append(args, "HEAD")
	}

	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	stdout, stderr, err := runner.Run(pCtx, env, "", "git", args...)
	if err != nil {
		return "", classifyGitError(targetURL, ref, transport, stderr, err)
	}
	fields := strings.Fields(strings.TrimSpace(stdout))
	if len(fields) > 0 {
		return fields[0], nil
	}
	return "", nil
}

// CloneOptions configures a non-interactive Git clone.
type CloneOptions struct {
	URL    string
	Ref    string
	Dest   string
	Depth  int
	Auth   AuthMode
	SSHKey string
}

// Clone probes and clones a repository into Dest using the selected transport.
func (a *Adapter) Clone(ctx context.Context, opts CloneOptions) (ProbeResult, error) {
	probe, err := a.ProbeWithOptions(ctx, opts.URL, ProbeOptions{
		Auth:   opts.Auth,
		Ref:    opts.Ref,
		SSHKey: opts.SSHKey,
	})
	if err != nil {
		return ProbeResult{}, err
	}
	if probe.Transport == "local" {
		return probe, nil
	}

	if err := os.MkdirAll(filepath.Dir(opts.Dest), 0o755); err != nil {
		return ProbeResult{}, fmt.Errorf("creating parent directory %s: %w", filepath.Dir(opts.Dest), err)
	}

	sshKey := ""
	if probe.Transport == "ssh" {
		sshKey = opts.SSHKey
	}
	env, err := NonInteractiveEnvWithKey(sshKey)
	if err != nil {
		return ProbeResult{}, err
	}

	args := []string{"clone"}
	if opts.Depth > 0 && opts.Ref == "" {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	args = append(args, probe.ResolvedURL, opts.Dest)

	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	_, stderr, err := runner.Run(ctx, env, "", "git", args...)
	if err != nil {
		return ProbeResult{}, classifyGitError(probe.Normalized.Redacted, opts.Ref, probe.Transport, stderr, err)
	}

	if ref := strings.TrimSpace(opts.Ref); ref != "" {
		_, coErrStr, coErr := runner.Run(ctx, env, opts.Dest, "git", "checkout", ref)
		if coErr != nil {
			return ProbeResult{}, &TransportError{
				Kind:        ErrKindInvalidRef,
				URL:         probe.Normalized.Redacted,
				Transport:   probe.Transport,
				Detail:      fmt.Sprintf("failed to checkout ref %q: %s", ref, strings.TrimSpace(coErrStr)),
				Remediation: "Verify the branch, tag, or commit exists in the repository",
				Cause:       coErr,
			}
		}
	}

	return probe, nil
}

// UpdateOptions configures a non-interactive Git update.
type UpdateOptions struct {
	Dir    string
	Ref    string
	SSHKey string
	Remote string
}

// LocalOrigin reads the configured origin without applying Git's URL rewrites.
func (a *Adapter) LocalOrigin(ctx context.Context, dir string) (string, error) {
	return a.readLocalValue(ctx, dir, "config", "--local", "--get", "remote.origin.url")
}

// OriginPushURL resolves the single push destination for origin.
func (a *Adapter) OriginPushURL(ctx context.Context, dir string) (string, error) {
	output, err := a.readLocalValue(ctx, dir, "remote", "get-url", "--push", "--all", "origin")
	if err != nil {
		return "", fmt.Errorf("reading the push URL for origin: %w", err)
	}
	if output == "" || strings.Contains(output, "\n") {
		return "", errors.New("origin must have exactly one push destination")
	}
	return output, nil
}

// LocalRevision resolves a local ref to the full ID of a commit without fetching.
func (a *Adapter) LocalRevision(ctx context.Context, dir, ref string) (string, error) {
	return a.readLocalValue(ctx, dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
}

func (a *Adapter) readLocalValue(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := a.readLocalOutput(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

func (a *Adapter) readLocalOutput(ctx context.Context, dir string, args ...string) (string, error) {
	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	out, _, err := runner.Run(ctx, NonInteractiveEnv(), dir, "git", args...)
	return out, err
}

// PushOptions binds a branch and verified origin URL to a push.
type PushOptions struct {
	Dir    string
	URL    string
	Branch string
	SSHKey string
}

// ValidateBranch rejects expressions and options that are not literal branch names.
func (a *Adapter) ValidateBranch(ctx context.Context, dir, branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	resolved, err := a.readLocalValue(ctx, dir, "check-ref-format", "--branch", branch)
	if err != nil {
		return fmt.Errorf("invalid branch name %q: %w", branch, err)
	}
	if resolved != branch {
		return fmt.Errorf("expected a literal branch name, got %q", branch)
	}
	return nil
}

// PushBranch publishes a branch to the configured origin without prompting or forcing.
func (a *Adapter) PushBranch(ctx context.Context, opts PushOptions) error {
	if err := a.ValidateBranch(ctx, opts.Dir, opts.Branch); err != nil {
		return err
	}
	target, err := a.OriginPushURL(ctx, opts.Dir)
	if err != nil {
		return err
	}
	if target != opts.URL {
		return errors.New("origin push destination changed after authorization")
	}
	normalized := NormalizeURL(target)
	sshKey := ""
	if normalized.IsSSH {
		sshKey = opts.SSHKey
	}
	env, err := NonInteractiveEnvWithKey(sshKey)
	if err != nil {
		return err
	}
	if normalized.IsSSH {
		for i, entry := range env {
			command, ok := strings.CutPrefix(entry, "GIT_SSH_COMMAND=")
			if !ok {
				continue
			}
			if sshKey == "" {
				command = "ssh -o BatchMode=yes -o ConnectTimeout=8"
			}
			env[i] = "GIT_SSH_COMMAND=" + command + " -o StrictHostKeyChecking=yes"
		}
	}
	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	ref := "refs/heads/" + opts.Branch
	args := []string{"push", "--set-upstream", "--no-follow-tags", "--recurse-submodules=no", "origin", ref + ":" + ref}
	_, stderr, err := runner.Run(ctx, env, opts.Dir, "git", args...)
	if err != nil {
		transport := "git"
		if normalized.IsSSH {
			transport = "ssh"
		}
		return classifyGitError(target, opts.Branch, transport, strings.ReplaceAll(stderr, target, normalized.Redacted), err)
	}
	return nil
}

// Update fetches and fast-forwards an existing Git checkout safely in non-interactive mode.
func (a *Adapter) Update(ctx context.Context, dir, ref string) error {
	return a.UpdateWithOptions(ctx, UpdateOptions{
		Dir: dir,
		Ref: ref,
	})
}

func (a *Adapter) fetchOrigin(ctx context.Context, opts UpdateOptions, env []string) error {
	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}
	if _, stderr, err := runner.Run(ctx, env, opts.Dir, "git", "fetch", "--prune", "--", remote); err != nil {
		return classifyGitError(opts.Dir, opts.Ref, "git", stderr, err)
	}
	return nil
}

// UpdateWithOptions fetches and fast-forwards an existing Git checkout using optional explicit SSH key.
func (a *Adapter) UpdateWithOptions(ctx context.Context, opts UpdateOptions) error {
	runner := a.Runner
	if runner == nil {
		runner = execRunner{}
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	uCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env, err := NonInteractiveEnvWithKey(opts.SSHKey)
	if err != nil {
		return err
	}

	if err := a.fetchOrigin(uCtx, opts, env); err != nil {
		return err
	}
	targetRef := strings.TrimSpace(opts.Ref)
	if targetRef != "" {
		if _, stderr, err := runner.Run(uCtx, env, opts.Dir, "git", "checkout", targetRef); err != nil {
			return &TransportError{
				Kind:   ErrKindInvalidRef,
				URL:    opts.Dir,
				Detail: strings.TrimSpace(stderr),
				Cause:  err,
			}
		}
	}
	if _, stderr, err := runner.Run(uCtx, env, opts.Dir, "git", "pull", "--ff-only"); err != nil {
		// Detached HEAD on tag/commit cannot pull --ff-only; treat as already at ref.
		if strings.Contains(stderr, "not currently on a branch") {
			return nil
		}
		return classifyGitError(opts.Dir, targetRef, "git", stderr, err)
	}
	return nil
}

func classifyGitError(rawURL, ref, transport, stderr string, cause error) *TransportError {
	lower := strings.ToLower(stderr)
	safeURL := NormalizeURL(rawURL).Redacted
	if safeURL == "" {
		safeURL = rawURL
	}

	if errors.Is(cause, exec.ErrNotFound) {
		return &TransportError{
			Kind:        ErrKindGitMissing,
			URL:         safeURL,
			Transport:   transport,
			Detail:      "git executable not found in PATH",
			Remediation: "Install git before cloning external repositories",
			Cause:       cause,
		}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return &TransportError{
			Kind:        ErrKindNetwork,
			URL:         safeURL,
			Transport:   transport,
			Detail:      "operation timed out",
			Remediation: "Check network connectivity and SSH/HTTPS reachability",
			Cause:       cause,
		}
	}

	switch {
	case strings.Contains(lower, "permission denied (publickey)") ||
		strings.Contains(lower, "could not read username") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "terminal prompts disabled") ||
		strings.Contains(lower, "host key verification failed") ||
		strings.Contains(lower, "no such identity") ||
		strings.Contains(lower, "invalid format"):
		rem := "Ensure your SSH key is loaded in ssh-agent (ssh-add -l) and known_hosts is populated, or configure a Git HTTPS credential helper"
		if transport == "ssh" {
			rem = "SSH authentication failed: verify ssh-agent has an authorized key (ssh-add -l) and host key is in ~/.ssh/known_hosts"
		}
		return &TransportError{
			Kind:        ErrKindAuth,
			URL:         safeURL,
			Transport:   transport,
			Detail:      firstLine(stderr),
			Remediation: rem,
			Cause:       cause,
		}
	case strings.Contains(lower, "repository not found") || strings.Contains(lower, "does not appear to be a git repository"):
		return &TransportError{
			Kind:        ErrKindNotFound,
			URL:         safeURL,
			Transport:   transport,
			Detail:      firstLine(stderr),
			Remediation: "Verify the repository URL exists and your identity has read permission",
			Cause:       cause,
		}
	case strings.Contains(lower, "could not resolve host") || strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "timed out"):
		return &TransportError{
			Kind:        ErrKindNetwork,
			URL:         safeURL,
			Transport:   transport,
			Detail:      firstLine(stderr),
			Remediation: "Check internet/VPN connectivity or use a local repository path",
			Cause:       cause,
		}
	case ref != "" && strings.TrimSpace(stderr) == "":
		// git ls-remote --exit-code exits 2 with empty stderr when the ref does not exist
		return &TransportError{
			Kind:        ErrKindInvalidRef,
			URL:         safeURL,
			Transport:   transport,
			Detail:      fmt.Sprintf("ref %q not found in remote repository", ref),
			Remediation: "Check branch/tag name with 'git ls-remote <url>'",
			Cause:       cause,
		}
	default:
		detail := firstLine(stderr)
		if detail == "" {
			detail = cause.Error()
		}
		return &TransportError{
			Kind:      ErrKindUnknown,
			URL:       safeURL,
			Transport: transport,
			Detail:    detail,
			Cause:     cause,
		}
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}
