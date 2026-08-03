# Roadmap

## Phase 0 — housekeeping & portability fixes ✅

- [x] `pull` — Gitea → local Markdown
- [x] `.gitignore` (output mirror, secrets, OS/editor noise)
- [x] **Fix output-path resolution.** `output.dir` resolves against the
      config file's directory, not the script location.
- [x] **Config discovery + `--config` flag.** The nearest `config.json` from
      the cwd up to the git root, with `config.local.json` per-section
      overrides — the prerequisite for one installed binary serving many
      repos.
- [x] License: Apache-2.0.

## Phase 1 — the write path ✅

- [x] `status` — local-vs-remote drift, per file and per field (title,
      labels, state, body). Exits 1 when anything drifts, so it can gate
      scripts/CI.
- [x] `diff` — unified diff of the drift, remote → local (the direction
      `push` would apply). Both sides are rendered canonically, so
      formatting-only edits in the frontmatter don't count as drift.
- [x] `push` — local → Gitea. Updates only the fields that differ; creates
      issues from files without a `<number>-` prefix and renames them to the
      canonical filename afterwards. Never deletes anything remotely, and
      ignores `pulls/` (read-only mirror).

## Phase 2 — quality of life ✅

- [x] Incremental pull: `pull --incremental` resumes from the `syncedAt`
      high-water mark in `.gitea-sync.json` and rewrites only updated issues
      (renames/moves handled by number; remote deletions still need a full
      pull)
- [x] Mirror issue comments behind `output.comments` — read-only
      `<index>-<slug>.comments.md` sidecars, ignored by status/diff/push
- [x] Test suite (18 tests on the pure helpers: slugs, YAML round-trip, PR
      routing, drift detection, unified diff, comment rendering), plus a
      Gitea Actions workflow (needs a registered runner)

## Phase 3 — standalone distribution ✅

Originally prototyped as a `bun build --compile` of the JavaScript source.
Superseded by a full **Go rewrite** for true parity with gh-issue-sync (also
Go): a ~6 MB native binary instead of a ~70 MB embedded-runtime one, plus
`go install`.

- [x] Rewrite in Go — `cmd/tea-issue-sync` + `internal/teasync`, standard
      library only; identical on-disk Markdown format (verified by a
      byte-for-byte diff of a full pull against the JS implementation)
- [x] Native binaries via `make dist` (darwin/linux × amd64/arm64), no
      runtime required
- [x] `go install …/cmd/tea-issue-sync@latest`, an `install.sh`, and a
      GitHub Actions release workflow that builds + attaches binaries on tag
      (so the binaries reach the GitHub mirror, which push-mirroring alone
      does not carry)
- [ ] Homebrew tap — deferred until the repo settles on its public host

## Phase 4 — agent ergonomics ✅

- [x] Local convenience verbs `new` / `close` / `reopen` / `list` (no network)
- [x] Embedded agent skill (`skill/SKILL.md` via `go:embed`) + `skill install`
      / `skill print`; `AGENTS.md` / `CLAUDE.md` for contributors
- [x] Zero-config: `gitea.url` / `owner` / `repo` are inferred from the git
      `origin` remote, so config is optional (override-only). The optional
      file is `.tea-issue-sync.json` (tool-specific, never a generic
      `config.json`)
- [x] `install.sh` installs to `~/.local/bin` (no sudo) and warns if it's not
      on PATH

## Later / ideas

- Conflict-aware push (compare against a pulled base snapshot to detect
  remote changes since the last pull before overwriting)
- `pull --prune` using the repo's issue count to detect deletions without a
  full rewrite
