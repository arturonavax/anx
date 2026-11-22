# AI Agent Engineering Guide — anx

`arturonavax/anx` is the standalone Go environment orchestration engine.

## Architectural Invariants

1. **Zero coupling to personal `env`**: `anx` must build, start, display help/version, and run core diagnostics without an `env` checkout. Never embed personal dotfiles, profiles, or machine-specific hacks in `defaults/`.
2. **Public repository — 0% private/personal data**: Never commit credentials, private emails, Cloudflare/Wrangler account caches, or local user paths.
3. **Unified Git transport (`internal/gittransport`)**: All Git probing, cloning, and updating for config repos and tool sources must go through `internal/gittransport` with non-interactive env and strict host-key checking.
4. **Multi-source tools (`internal/tools`)**: Support `builtin`, `single`, and `collection` sources with deterministic collision handling and atomic binary replacement.

## Verification

```sh
go test -race ./...
sh -n install.sh
bash -n scripts/bootstrap_anx.sh
node --test deploy/cloudflare/src/index.test.js
```

## Versioning

- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/): `fix` and `perf` advance PATCH, `feat` advances MINOR, and `!` or a `BREAKING CHANGE:` footer advances MAJOR. Other types do not change the stable version.
- Do not edit version literals for ordinary changes. Use `make version` for a source build and `make version-next` for the proposed stable version. The calculator replays first-parent commits since its checked-in checkpoint, so classify squash and merge commits for the changes they deliver.
- Do not create tags, publish releases, or change the Go module path implicitly. See [Versioning in the README](README.md#versioning).
