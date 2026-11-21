package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arturonavax/anx/internal/catalog"
)

// RichCatalog returns a catalog.Registry overlaid with a self-contained synthetic
// test manifest fixture (multi-OS packages, profiles, and configs) so ANX engine
// unit tests run completely independently of any external env repository.
func RichCatalog(t *testing.T) *catalog.Registry {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(syntheticFixtureYAML), 0o644); err != nil {
		t.Fatalf("writing synthetic fixture: %v", err)
	}
	reg, err := catalog.NewRegistryWithOverlays(dir, "")
	if err != nil {
		t.Fatalf("loading synthetic fixture catalog: %v", err)
	}
	return reg
}

const syntheticFixtureYAML = `
packages:
  - id: build-essential
    name: Build Essential
    category: core-build
    os: [linux]
    strategies:
      - type: native
        packages:
          apt: [build-essential]
          dnf: [gcc, make]
          pacman: [base-devel]
          zypper: [patterns-devel-base-devel_basis]
          apk: [build-base]

  - id: ssl-dev
    name: OpenSSL Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libssl-dev], brew: [openssl@3] }
  - id: zlib-dev
    name: Zlib Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [zlib1g-dev], brew: [zlib] }
  - id: sqlite-dev
    name: SQLite Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libsqlite3-dev], brew: [sqlite] }
  - id: readline-dev
    name: Readline Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libreadline-dev], brew: [readline] }
  - id: ffi-dev
    name: LibFFI Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libffi-dev], brew: [libffi] }
  - id: bz2-dev
    name: Bzip2 Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libbz2-dev], brew: [bzip2] }
  - id: xz-dev
    name: XZ Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [liblzma-dev], brew: [xz] }
  - id: ncurses-dev
    name: Ncurses Dev
    category: core-libs
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { apt: [libncurses-dev], brew: [ncurses] }

  - id: golang
    name: Go
    category: toolchains
    binary_name: go
    strategies:
      - type: native
        when: { os: [windows] }
        packages: { winget: [GoLang.Go] }
      - type: script
        when: { os: [linux, darwin] }
        depends_on: [curl]
        script: { path: scripts/install_golang.sh, interpreter: bash }

  - id: make
    name: Make
    category: core-build
    strategies: [{ type: native, packages: { apt: [make], brew: [make] } }]
  - id: wget
    name: Wget
    category: core
    strategies: [{ type: native, packages: { apt: [wget], brew: [wget] } }]
  - id: ca-certificates
    name: CA Certificates
    category: core
    strategies: [{ type: native, packages: { apt: [ca-certificates], brew: [ca-certificates] } }]
  - id: cmake
    name: CMake
    category: core-build
    strategies: [{ type: native, packages: { apt: [cmake], brew: [cmake] } }]
  - id: tar
    name: Tar
    category: core
    strategies: [{ type: native, packages: { apt: [tar], brew: [gnu-tar] } }]
  - id: unzip
    name: Unzip
    category: core
    strategies: [{ type: native, packages: { apt: [unzip], brew: [unzip] } }]
  - id: htop
    name: htop
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [htop], brew: [htop] } }]
  - id: jq
    name: jq
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [jq], brew: [jq] } }]
  - id: fzf
    name: fzf
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [fzf], brew: [fzf] } }]
  - id: bat
    name: bat
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [bat], brew: [bat] } }]
  - id: eza
    name: eza
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [eza], brew: [eza] } }]
  - id: tree
    name: tree
    category: cli-tools
    strategies: [{ type: native, packages: { apt: [tree], brew: [tree] } }]

  - id: ripgrep
    name: Ripgrep
    category: cli-tools
    binary_name: rg
    strategies:
      - type: native
        packages: { apt: [ripgrep], dnf: [ripgrep], pacman: [ripgrep], zypper: [ripgrep], apk: [ripgrep], brew: [ripgrep], winget: [BurntSushi.ripgrep.MSVC] }

  - id: fd
    name: fd
    category: cli-tools
    binary_name: fd
    binary_aliases: [fdfind]
    strategies:
      - type: native
        packages: { apt: [fd-find], dnf: [fd-find], pacman: [fd], zypper: [fd], apk: [fd], brew: [fd], winget: [sharkdp.fd] }

  - id: rtk
    name: RTK
    category: ai
    binary_name: rtk
    strategies:
      - type: github_release
        when: { os: [linux, darwin] }
        github:
          repo: rtk-ai/rtk
          asset_pattern: "rtk-{os}-{arch}.tar.gz"
          binary: rtk
      - type: native
        packages: { brew: [rtk] }

  - id: herdr
    name: Herdr
    category: terminal
    binary_name: herdr
    os: [linux, darwin]
    strategies:
      - type: script
        depends_on: [curl]
        script: { url: "https://herdr.dev/install.sh", interpreter: sh }

  - id: starship
    name: Starship
    category: terminal
    binary_name: starship
    strategies:
      - type: script
        when: { os: [linux] }
        depends_on: [curl]
        script: { url: "https://starship.rs/install.sh", interpreter: sh }
      - type: native
        packages: { brew: [starship], winget: [Starship.Starship] }

  - id: rust
    name: Rust
    category: toolchains
    binary_name: cargo
    strategies:
      - type: script
        when: { os: [linux] }
        depends_on: [curl, build-essential]
        script: { path: scripts/install_rust.sh, interpreter: bash }
      - type: script
        when: { os: [darwin] }
        depends_on: [curl]
        script: { path: scripts/install_rust.sh, interpreter: bash }
      - type: native
        when: { os: [windows] }
        packages: { winget: [Rustlang.Rustup] }

  - id: zig
    name: Zig
    category: toolchains
    binary_name: zig
    strategies:
      - type: native
        when: { os: [windows] }
        packages: { winget: [zig.zig] }
      - type: script
        when: { os: [linux, darwin] }
        depends_on: [curl]
        script: { path: scripts/install_zig.sh, interpreter: bash }

  - id: pnpm
    name: pnpm
    category: toolchains
    binary_name: pnpm
    strategies:
      - type: native
        packages: { brew: [pnpm], winget: [pnpm.pnpm] }
      - type: script
        depends_on: [curl]
        script: { url: "https://get.pnpm.io/install.sh", interpreter: sh }

  - id: tree-sitter-cli
    name: Tree-sitter CLI
    category: editor
    binary_name: tree-sitter
    strategies:
      - type: native
        when: { distros: [alpine] }
        packages: { apk: [tree-sitter-cli] }
      - type: language_pkg
        when: { requires_binary: pnpm }
        depends_on: [pnpm]
        language_pkg: { manager: pnpm, package: tree-sitter-cli }
      - type: language_pkg
        depends_on: [rust]
        language_pkg: { manager: cargo, package: tree-sitter-cli }

  - id: pyenv
    name: Pyenv
    category: toolchains
    binary_name: pyenv
    os: [linux, darwin]
    strategies:
      - type: script
        when: { os: [linux] }
        depends_on: [git, curl, build-essential, bz2-dev, ssl-dev, zlib-dev, readline-dev, sqlite-dev, ffi-dev, xz-dev, ncurses-dev]
        script: { path: scripts/install_pyenv.sh, interpreter: bash }
      - type: native
        when: { os: [darwin] }
        packages: { brew: [pyenv] }

  - id: fnm
    name: fnm
    category: toolchains
    binary_name: fnm
    strategies:
      - type: script
        when: { os: [linux, darwin] }
        depends_on: [curl]
        script: { path: scripts/install_node.sh, interpreter: bash }
      - type: native
        when: { os: [windows] }
        packages: { winget: [Schniz.fnm] }

  - id: nodejs
    name: Node.js
    category: toolchains
    binary_name: node
    depends_on: [fnm]
    strategies:
      - type: script
        script: { path: scripts/install_node.sh, interpreter: bash }

  - id: claude-code
    name: Claude Code
    category: ai
    binary_name: claude
    strategies:
      - type: native
        when: { os: [darwin] }
        packages: { brew: ["--cask", "claude-code"] }
      - type: script
        when: { os: [linux] }
        depends_on: [curl]
        script: { url: "https://claude.ai/install.sh", interpreter: sh }
      - type: native
        when: { os: [windows] }
        packages: { winget: [Anthropic.ClaudeCode] }

  - id: opencode
    name: OpenCode
    category: ai
    binary_name: opencode
    strategies:
      - type: native
        when: { os: [windows] }
        packages: { winget: [SST.OpenCode] }
      - type: script
        when: { os: [linux, darwin] }
        depends_on: [curl]
        script: { url: "https://opencode.ai/install", interpreter: sh }

  - id: codex-cli
    name: Codex CLI
    category: ai
    binary_name: codex
    strategies:
      - type: script
        when: { os: [linux, darwin] }
        depends_on: [curl]
        script: { path: scripts/install_codex.sh, interpreter: bash }
      - type: native
        when: { os: [windows] }
        packages: { winget: [OpenAI.Codex] }

  - id: xclip
    name: xclip
    category: desktop
    binary_name: xclip
    os: [linux]
    display_servers: [x11]
    strategies:
      - type: native
        packages: { apt: [xclip], dnf: [xclip], pacman: [xclip], zypper: [xclip] }

  - id: wmctrl
    name: wmctrl
    category: desktop
    binary_name: wmctrl
    os: [linux]
    display_servers: [x11]
    strategies:
      - type: native
        packages: { apt: [wmctrl], dnf: [wmctrl], pacman: [wmctrl], zypper: [wmctrl] }

  - id: xdotool
    name: xdotool
    category: desktop
    binary_name: xdotool
    os: [linux]
    display_servers: [x11]
    strategies:
      - type: native
        packages: { apt: [xdotool], dnf: [xdotool], pacman: [xdotool], zypper: [xdotool] }

  - id: amethyst
    name: Amethyst
    category: desktop
    os: [darwin]
    strategies:
      - type: native
        packages: { brew: ["--cask", "amethyst"] }

  - id: karabiner-elements
    name: Karabiner-Elements
    category: desktop
    os: [darwin]
    strategies:
      - type: native
        packages: { brew: ["--cask", "karabiner-elements"] }

  - id: pam-reattach
    name: pam-reattach
    category: osconfig
    os: [darwin]
    strategies:
      - type: native
        packages: { brew: [pam-reattach] }

  - id: ghostty
    name: Ghostty
    category: terminal
    binary_name: ghostty
    os: [linux, darwin]
    strategies:
      - type: native
        packages: { brew: ["--cask", "ghostty"], apt: [ghostty] }

configurations:
  - id: zshrc-loader
    group: terminal
    source: files/zsh/.zshrc
    target: ~/.zshrc
    strategy: marker_block
    marker_start: "# >>> arturonavax/env >>>"
    marker_end: "# <<< arturonavax/env <<<"

  - id: nvim-config
    group: editor
    source: files/nvim
    target: ~/.config/nvim
    strategy: directory_sync

  - id: starship-config
    group: terminal
    source: files/starship/starship.toml
    target: ~/.config/starship.toml
    strategy: three_way

  - id: ssh-config
    group: ssh
    source: files/ssh/config
    target: ~/.ssh/config
    os: [linux, darwin]
    strategy: marker_block
    backup: true
    dir_mode: "0700"
    file_mode: "0600"
    skip_capture: true
    host_specific: true
    marker_start: "# >>> arturonavax/env >>>"
    marker_end: "# <<< arturonavax/env <<<"

  - id: ssh-agent-service
    group: ssh
    source: files/ssh/ssh-agent.service
    target: ~/.config/systemd/user/ssh-agent.service
    strategy: copy
    os: [linux]

  - id: ssh-agent-env
    group: ssh
    source: files/ssh/ssh-agent.conf
    target: ~/.config/environment.d/10-ssh-agent.conf
    strategy: copy
    os: [linux]

  - id: ssh-agent-zshenv
    group: ssh
    source: files/ssh/zshenv
    target: ~/.zshenv
    os: [linux, darwin]
    strategy: marker_block
    marker_start: "# >>> arturonavax/env >>>"
    marker_end: "# <<< arturonavax/env <<<"

  - id: ssh-agent-profile
    group: ssh
    source: files/ssh/profile
    target: ~/.profile
    os: [linux, darwin]
    strategy: marker_block
    marker_start: "# >>> arturonavax/env >>>"
    marker_end: "# <<< arturonavax/env <<<"

  - id: macos-osconfig-script
    group: osconfig
    source: scripts/osconfig/macos.sh
    target: ~/.local/bin/macos-osconfig
    strategy: replace
    os: [darwin]
    file_mode: "0755"
    backup: false
    post_sync_cmds:
      - ["sh", "-c", "sudo -n true && bash ~/.local/bin/macos-osconfig"]

profiles:
  - name: minimal
    description: Minimal profile
    packages: [git, curl, ripgrep, fd, starship, htop, jq, fzf, bat, eza, tree]
    config_groups: [terminal, ssh]

  - name: headless
    description: Headless server profile
    extends: minimal
    packages: [rust, pyenv, make, wget, ca-certificates, cmake, tar, unzip, golang]

  - name: developer
    description: Developer profile
    extends: headless
    packages: [herdr, rtk, tree-sitter-cli, nodejs, claude-code, codex-cli]
    config_groups: [editor]

  - name: workstation
    description: Full workstation profile
    extends: developer
    packages: [ghostty, xclip, wmctrl, xdotool, amethyst, pam-reattach]
    config_groups: [osconfig]
`
