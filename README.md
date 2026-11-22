# ANX — Universal Cross-Platform Environment Orchestration Engine

`anx` is a standalone, cross-platform environment orchestration engine written in Go. It manages package provisioning, dotfile 3-way synchronization, hardware optimizations, transactional SQLite state, security auditing, and a multi-source developer tools engine — completely decoupled from any specific personal dotfiles repository.

## Architecture

- **Engine (`arturonavax/anx`)**: Standalone Go CLI/TUI, installers, Cloudflare edge router, and universal engine defaults (`defaults/`).
- **Configuration Repository (default: `arturonavax/env`)**: Pure declarative YAML manifests, dotfiles, profiles, and scripts. Replaceable via `--config-repo`, `ANX_CONFIG_REPO`, or `anx repo set <path-or-url>`.
- **Multi-Source Tools Engine**: Discovers and installs tools from `builtin` (`tools/bundled/`), standalone single-tool repos (`tool.yaml`), and multi-tool collection repos (`tools.yaml` / `tools/*/tool.yaml`) across public, private (SSH/HTTPS), and local sources.

## Installation

Start from a trusted checkout. Review the ref and install script before running
it; do not pipe a mutable remote script directly into a shell. Configuration
repositories can run scripts too: use only repositories you trust.

```sh
git clone https://github.com/arturonavax/anx.git
cd anx
```

### Linux / macOS

```sh
sh ./install.sh --install-only
```

The bootstrap does not install GVM or Homebrew automatically. When Go is
missing, it can use an existing GVM or native package manager. To avoid
automatic toolchain setup, install Go first and build from source.

### From source (including Windows)

[Install Go](https://go.dev/dl/) and run this from the reviewed checkout:

```sh
go install ./cmd/anx
```

The standalone Windows PowerShell installer is experimental.

## Versioning

`make version` reports the reproducible development version. `make version-next`
prints the proposed [SemVer](https://semver.org/) stable number without creating
a tag or release. The calculator starts at the checked-in 1.0.0 checkpoint
and replays first-parent commits in order. Each `fix` or `perf` increments patch,
`feat` increments minor, and a conventional `!` or `BREAKING CHANGE:` footer
increments major. Other commits still advance the development commit count.
Existing tags do not change the calculation.

`make build` and both source bootstraps use the same calculator. Development
builds carry a commit and mark dirty worktrees. Source archives and incomplete
histories use a development fallback; they cannot compute a stable version.
Plain local `go build`/`go install` may report `dev`; versioned Go module installs
retain Go's module metadata. Use complete history to plan a release. Before
publishing a new major version, verify Go module path and installation
compatibility. Planning a version never performs a release action.

## Key Commands

```sh
# Configuration repository management
anx repo status
anx repo clone [url] [--ref <ref>] [--auth auto|ssh|https]
anx repo set <path-or-url> [--ref <ref>]
anx repo update
anx repo verify

# Environment orchestration
anx plan [--profile <name>]
anx apply [--profile <name>] [--include-hacks] [-y]
anx sync [--dry-run]
anx doctor
anx audit [--profile workstation|server|minimal]

# Multi-source tools engine
anx tools list
anx tools info <tool>
anx tools install <tool>
anx tools run <tool> [args...]
anx tools update [tool|--all]
anx tools doctor
anx tools source list
anx tools source add <url-or-path> [--id <id>] [--type single|collection] [--ref <ref>] [--auth auto|ssh|https]
anx tools source remove <id>
anx tools source update <id>
anx tools source verify <id>
```

## Security & Git Authentication

- Release checksums detect a missing or mismatched download. A checksum from
  the same release is not a signature or independent proof of publisher identity.
- Automatic non-interactive SSH discovery (`GIT_TERMINAL_PROMPT=0`, `BatchMode=yes`) with HTTPS credential-helper fallback when `auth: auto`.
- Never disables SSH host-key verification (`StrictHostKeyChecking=no` is strictly prohibited).
- Never persists credentials in repository URLs or logs.
- Atomic tool updates preserve the last known working executable if a build or install fails.
