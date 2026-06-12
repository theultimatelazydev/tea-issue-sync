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

## Phase 2 — quality of life

- [ ] Incremental pull (`since` timestamp) instead of a full wipe-and-rewrite
- [ ] Mirror issue comments (optional, behind a config flag)
- [ ] Basic test suite (frontmatter round-trip, slug edge cases, PR routing,
      drift detection) and a CI job on the Gitea instance

## Phase 3 — standalone distribution

- [ ] Package as a self-contained binary so Node is no longer required.
      Cheapest path that keeps the current zero-dependency source:
      `bun build --compile` or `deno compile` (both produce a single static
      executable from the existing `.mjs` with little or no change).
      A Go/Rust rewrite is the fallback if binary size or startup time ever
      matters — not before.
- [ ] Versioned releases on Gitea + an install one-liner (and optionally a
      Homebrew tap)
