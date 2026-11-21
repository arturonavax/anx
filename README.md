# ANX — Universal Cross-Platform Environment Orchestration Engine

`anx` is a standalone, cross-platform environment orchestration engine written in Go. It manages package provisioning, dotfile 3-way synchronization, hardware optimizations, transactional SQLite state, security auditing, and a multi-source developer tools engine — completely decoupled from any specific personal dotfiles repository.

## Architecture

- **Engine (`arturonavax/anx`)**: Standalone Go CLI/TUI, installers, Cloudflare edge router, and universal engine defaults (`defaults/`).
- **Configuration Repository (default: `arturonavax/env`)**: Pure declarative YAML manifests, dotfiles, profiles, and scripts. Replaceable via `--config-repo`, `ANX_CONFIG_REPO`, or `anx repo set <path-or-url>`.
- **Multi-Source Tools Engine**: Discovers and installs tools from `builtin` (`tools/bundled/`), standalone single-tool repos (`tool.yaml`), and multi-tool collection repos (`tools.yaml` / `tools/*/tool.yaml`) across public, private (SSH/HTTPS), and local sources.

## Installation

### Linux / macOS
```sh
curl -fsSL https://sh.arturonavax.dev/install.sh | sh
# Or with a custom configuration repository:
curl -fsSL https://sh.arturonavax.dev/install.sh \
  | sh -s -- --config-repo git@github.com:someone/env.git --profile developer
```

### Windows (PowerShell)
```powershell
irm https://sh.arturonavax.dev/install.ps1 | iex
# Or with a custom configuration repository:
& ([scriptblock]::Create((irm https://sh.arturonavax.dev/install.ps1))) -ConfigRepo "https://github.com/someone/env.git"
```

### From Source
```sh
go install github.com/arturonavax/anx/cmd/anx@latest
```

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

- Automatic non-interactive SSH discovery (`GIT_TERMINAL_PROMPT=0`, `BatchMode=yes`) with HTTPS credential-helper fallback when `auth: auto`.
- Never disables SSH host-key verification (`StrictHostKeyChecking=no` is strictly prohibited).
- Never persists credentials in repository URLs or logs.
- Atomic tool updates preserve the last known working executable if a build or install fails.
