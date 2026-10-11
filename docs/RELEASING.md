# Publishing ais

The release workflow builds macOS and Linux binaries for ARM64 and AMD64,
packages the usage guide and Bash/Zsh/Fish completions, and generates SHA-256
checksums. It also creates a Homebrew formula using those archives, verifies
their hashes, and attaches `ais.rb` to the GitHub release.

## First publication

The Homebrew tap and first tagged release have not been published as part of the
packaging implementation. The advertised `brew install caanmert/tap/ais`
command starts working only after the following steps are complete:

1. Create `caanmert/homebrew-tap` as a public GitHub repository with an initial
   commit and default branch. A short README is sufficient.
2. Add an Actions secret named `HOMEBREW_TAP_TOKEN` to `ai-session-tool`.
   Use a fine-grained token restricted to `homebrew-tap`, with repository
   contents read/write permission. The standard `GITHUB_TOKEN` cannot write to
   a different repository. Do not place a token in source files or documentation.
3. Merge the implementation and ensure CI passes, including the packaging job.
4. Create and push a release tag such as `v0.1.0` on the reviewed commit.
   Pushing a `v*` tag triggers `.github/workflows/release.yml` and publishes
   the release. Use `vMAJOR.MINOR.PATCH` or a prerelease such as `v0.1.0-rc.1`.
5. The workflow commits `Formula/ais.rb` to the tap for stable tags. Prereleases
   get a formula artifact but do not update the stable tap.
6. On a clean installation, run `brew install caanmert/tap/ais`,
   `brew test caanmert/tap/ais`, `ais setup`, and `ais`. Check the package on
   both macOS and Linux before claiming both have been installation-tested.
7. Remove the initial-publication notices in README and USAGE once installation
   has been verified.

No release is published just by building or testing locally. The GoReleaser
configuration is intentionally independent of tap credentials: when
`HOMEBREW_TAP_TOKEN` is absent, the release still succeeds and the formula remains
available as a release attachment and Actions artifact. Copy that formula to
`Formula/ais.rb` in the tap manually if preferred.

## Verify packaging locally

Requires Go, GoReleaser v2, Python 3, and Ruby for the syntax check. No agent CLI
or tmux is needed to package the program.

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -v
goreleaser check
goreleaser release --snapshot --clean
```

Each `dist/ais_*.tar.gz` must contain `ais`, `README.md`, `docs/USAGE.md`, and
`completions/ais.bash`, `completions/ais.zsh`, `completions/ais.fish`. CI checks
the archive contents. A snapshot builds locally without publishing.

For an actual release's artifacts, generate and inspect the formula with:

```sh
python3 scripts/homebrew.py --tag v0.1.0 --dist dist
ruby -c dist/homebrew/ais.rb
```

The tag must match the archive versions. The generator requires all four
platform archives and rejects missing, duplicate, malformed, or mismatched
checksums. It does not publish anything itself. The custom formula keeps
macOS and Linux binary distribution in one package without relying on
GoReleaser's deprecated `brews` integration.

## What installation changes

The formula installs only the binary, documentation and shell completions.
It does not install agents, tmux or Git, run setup, alter shell configuration,
or remove user data on uninstall. Optional dependencies and their remedies
appear in `ais setup` and `ais doctor`.

## Updates and recovery

Publish another stable tag to refresh the formula. Users run `brew update`
and then `brew upgrade ais`.

If the release succeeds but the tap update fails, fix the tap permission or
branch issue and copy the attached `ais.rb` into the tap. There is no need to
rebuild or overwrite already published release archives. Confirm that the
formula points to the intended release before committing it.

Implementation references: [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
and [GoReleaser archive configuration](https://goreleaser.com/customization/archives/).
