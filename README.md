# ANX — Universal Cross-Platform Environment Orchestration Engine

`anx` is a standalone, cross-platform environment orchestration engine written in Go. It manages package provisioning, dotfile 3-way synchronization, hardware optimizations, transactional SQLite state, security auditing, and a multi-source developer tools engine — completely decoupled from any specific personal dotfiles repository.

## Architecture

- **Engine (`arturonavax/anx`)**: Standalone Go CLI/TUI, installers, Cloudflare edge router, and universal engine defaults (`defaults/`).
- **Configuration Repository (default: `arturonavax/env`)**: Pure declarative YAML manifests, dotfiles, profiles, and scripts. Replaceable via `--config-repo`, `ANX_CONFIG_REPO`, or `anx repo set <path-or-url>`.
- **Multi-Source Tools Engine**: Discovers and installs tools from `builtin` (`tools/bundled/`), standalone single-tool repos (`tool.yaml`), and multi-tool collection repos (`tools.yaml` / `tools/*/tool.yaml`) across public, private (SSH/HTTPS), and local sources.

## Installation

### Linux / macOS

Quick install:

```sh
curl -fsSL https://sh.arturonavax.dev | sh
```

This runs the installer served by the Cloudflare Worker. If you want to review
the code before running it, use a trusted checkout instead:

```sh
git clone https://github.com/arturonavax/anx.git
cd anx
# Review install.sh and the checked-out revision before continuing.
sh ./install.sh --install-only
```

Configuration repositories can run scripts too; use only repositories you trust.

To clone and link an environment, run `anx repo clone <url>`. The default
checkout is `~/.local/share/anx/env`, regardless of the remote repository name.
The installer keeps ANX source separate at `~/.local/share/anx/engine` on
Linux/macOS or `%LOCALAPPDATA%\anx\engine` on Windows. Existing linked
environments stay where they are. To switch remotes without replacing a checkout,
use `anx repo clone <url> --dir <new-path>`.

When the installer launches the setup wizard or an unattended apply, ANX
enrolls the environment before it loads the plan. Use the installer's
`--config-repo` and `--config-ref` options (or `ANX_CONFIG_REPO` and
`ANX_CONFIG_REF`) to select a source. ANX does not replace a different linked
checkout or fetch one for an offline run or a dry run. Use `--install-only`
on Linux/macOS to install just the ANX binary.

```sh
sh ./install.sh wizard --yes --config-repo git@github.com:YOUR-ORG/YOUR-ENV.git
```

The bootstrap does not install GVM or Homebrew automatically. When Go is
missing, it can use an existing GVM or native package manager. To avoid
automatic toolchain setup, install Go first and build from source.

### Windows

Install ANX for the current user on x64 or ARM64 from PowerShell or Command Prompt:

```cmd
powershell.exe -NoProfile -Command "irm https://sh.arturonavax.dev/install.ps1 | iex"
```

The installer tries a checksum-verified release first. If none is available, it
can build ANX using an existing Go toolchain or a verified portable Go archive.
It installs to `%LOCALAPPDATA%\Programs\anx\bin` and updates only the user
`PATH`; it does not request elevation or change PowerShell execution policy.
Open a new terminal and run `anx version` to verify the installation. Individual
packages you later select for environment setup may still require elevation.
Set `ANX_NO_PATH=1` before running the installer to leave your persistent user
`PATH` unchanged.

The quick command executes the downloaded script immediately. To inspect it first,
download it from Command Prompt, review it, and then run it if your policy allows:

```cmd
curl.exe -fsSLo anx-install.ps1 https://sh.arturonavax.dev/install.ps1
notepad.exe anx-install.ps1
powershell.exe -NoProfile -File .\anx-install.ps1
```

If your organization blocks the installer, use an approved source build or ask
your administrator. Do not bypass endpoint protection or execution policy.

### From source (including Windows)

[Install Go](https://go.dev/dl/) and run this from the reviewed checkout:

```sh
go install ./cmd/anx
```

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

## Environment manifests

Use ordered package strategies for platform-specific installation methods.
Declare package and strategy dependencies with `depends_on`; the scheduler
honors dependency order and serializes access to shared package managers.
Local scripts, OS configuration, and fonts belong in the environment repo.

A profile can cap its workers with `max_parallel: 1` through `32`; omit it or
use `0` for ANX's CPU-based default. The limit belongs to the selected
profile and does not inherit through `extends`.

Configuration entries can combine `os`, `distros`, `desktops` and
`display_servers`. ANX applies these rules to both plans and synchronization.
For example, `distros: [debian, ubuntu]` matches those distribution IDs and
`display_servers: [wayland]` requires Wayland. Omitted filters are unrestricted.

## Key Commands

```sh
# Configuration repository management
anx repo status
anx repo clone [url] [--ref <ref>] [--auth auto|ssh|https]
anx repo set <path-or-url> [--ref <ref>]
anx repo update --check
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

To publish captured dotfiles, opt in with `anx capture --all --commit --push`.
The push uses the environment checkout's configured `origin`; automatic
enrollment prefers SSH when available. For a private SSH origin, configure
`ANX_SSH_KEY` or the key saved by `anx repo clone --ssh-key`, and establish
the server's host key before pushing. The capture commit excludes unrelated
staged changes. Offline and dry-run modes do not push.

`anx repo status` reports the checked-out Git revision and cached upstream
without fetching. `anx repo update --check` refreshes the current branch's
tracking refs and lists incoming manifest, dotfile, and script paths without
changing the branch, index, or working tree. It requires a tracking branch;
a pinned detached checkout still reports its revision in `repo status`.
Use `anx repo update` to update explicitly, then `anx plan` to review what ANX
would apply. Updating Git does not apply packages or dotfiles automatically.

## Security & Git Authentication

- Release checksums detect a missing or mismatched download. A checksum from
  the same release is not a signature or independent proof of publisher identity.
- Automatic non-interactive SSH discovery (`GIT_TERMINAL_PROMPT=0`, `BatchMode=yes`) with HTTPS credential-helper fallback when `auth: auto`.
- Never disables SSH host-key verification (`StrictHostKeyChecking=no` is strictly prohibited).
- Never persists credentials in repository URLs or logs.
- Atomic tool updates preserve the last known working executable if a build or install fails.
