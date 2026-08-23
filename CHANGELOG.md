# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.4.1] - 2026-06-14

### Fixed
- A full `pull` no longer destroys unpushed local-only work. It still refreshes
  remote-sourced files and reaps remote deletions, but **preserves** drafts
  (files without a `<number>-` prefix) and pending `.comment.md` files, and
  prints a note listing what it kept with a nudge to `push`. Incremental pull
  likewise no longer removes a pending comment when refreshing its issue.

### Added
- `version` subcommand (alongside the existing `--version` / `-v`, plus `--v`).

## [1.4.0] - 2026-06-14

### Added
- **Post comments**, closing the loop on comment sync (pull already mirrored
  them read-only). A singular `<n>.comment.md` (or `<index>-<slug>.comment.md`)
  file is a pending comment; `push` posts it on issue `#n` and deletes the
  file. New `comment <n> [--body TEXT] [--edit]` verb writes the file, and
  `status`/`diff` list pending comments. The plural read-only
  `<index>-<slug>.comments.md` mirror is unchanged.

## [1.3.0] - 2026-06-14

### Changed
- The optional config file is now **`.tea-issue-sync.json`** (with a
  `.tea-issue-sync.local.json` overlay), not the generic `config.json`, so it
  never collides with a project's own application config — and the upward
  search no longer risks picking up an unrelated `config.json`. `config.json`
  is no longer recognized; rename any existing one. Config remains optional
  (inferred from the git remote), so most setups need no change.

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

[Unreleased]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.4.1...HEAD
[1.4.1]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.2.1...v1.3.0
[1.2.1]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/theultimatelazydev/tea-issue-sync/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/theultimatelazydev/tea-issue-sync/releases/tag/v1.1.0
