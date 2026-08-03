# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.1] - 2026-06-14

### Added
- Friendly error when run in a repo whose remote isn't Gitea: GitHub, GitLab,
  and Bitbucket origins now get a clear pointer (GitHub → `gh-issue-sync`)
  instead of an opaque API failure.
- This `CHANGELOG.md`; the GitHub release notes are now taken from it.

## [1.2.0] - 2026-06-14

### Added
- Ergonomic local verbs `new`, `close`, `reopen`, and `list` — file-only
  operations on the mirror (no network); `push` stays the single sync point.
- Embedded agent skill: `tea-issue-sync skill install [--project] [--dir]`
  writes `SKILL.md` into an agent's skills directory (default
  `~/.claude/skills/tea-issue-sync/`), and `skill print` emits it to stdout.
- `AGENTS.md` / `CLAUDE.md` documenting the contributor workflow.

### Changed
- **Config is now optional.** `gitea.url` / `owner` / `repo` are inferred from
  the git `origin` remote when a config file doesn't supply them, so a repo
  whose origin is the Gitea repo needs no `config.json`. Config becomes
  override-only (`gitea.remote`, a different repo, `output.dir`, comments).
- Removed the tracked placeholder `config.json` (its values would have
  overridden inference and broken the zero-config path).
- `install.sh` installs to `~/.local/bin` by default (no `sudo`) and warns
  when that directory isn't on `PATH`.

## [1.1.0] - 2026-06-13

First release of the Go rewrite — a single native binary (no runtime), for
parity with `gh-issue-sync`. Replaces the earlier JavaScript/bun prototype
(the removed `1.0.0`). The on-disk Markdown format is byte-for-byte identical.

### Added
- `pull` (full and `--incremental`), `status`, `diff`, `push`.
- Optional comment mirroring into read-only `<index>-<slug>.comments.md`
  sidecars (`output.comments`).
- Config discovery (nearest `config.json` from the cwd to the git root) with a
  `config.local.json` overlay; token from `$GITEA_TOKEN` or the `tea` CLI.
- Native binaries via `make dist`, `go install`, an `install.sh`, and a GitHub
  Actions release workflow.

[Unreleased]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/theultimatelazydev/tea-issue-sync/releases/tag/v1.1.0
