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
      routing, drift detection, unified diff, comment rendering) via
      `node --test`, plus a Gitea Actions workflow (needs a registered
      runner)

## Phase 3 — standalone distribution ✅

- [x] Self-contained binaries via `bun build --compile` (`./build.sh`,
      `--all` for darwin/linux × arm64/x64) — Node is no longer required to
      run the tool; bun is a build-time dependency only
- [x] Versioned releases on Gitea (binaries attached) + an install
      one-liner in the README
- [ ] Homebrew tap — deferred until the repo lands on its public host

## Later / ideas

- Conflict-aware push (compare against a pulled base snapshot to detect
  remote changes since the last pull before overwriting)
- `pull --prune` using the repo's issue count to detect deletions without a
  full rewrite
