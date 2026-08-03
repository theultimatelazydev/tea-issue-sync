# Agent guide

Rules for AI agents working on this repository.

## Workflow

- All changes go on a branch with a PR to `main`. **Never commit on `main`.**
- This repo lives on a Gitea instance and mirrors to GitHub. Use the `tea`
  CLI for PRs. From a git worktree, `tea pr create` fails on the
  `worktreeConfig` extension — create the PR from a shallow temp clone of the
  branch instead.
- Don't put the Gitea instance URL in tracked files. The tool needs no
  `config.json` here — it infers the repo from the `origin` remote. If you
  need an override, use a gitignored `config.local.json`.

## Issues — dogfood the tool

Read and edit issues locally instead of the web UI:

```
go run ./cmd/tea-issue-sync pull          # mirror Gitea → .issues-tea/
cat .issues-tea/open/<number>-*.md        # read an issue
go run ./cmd/tea-issue-sync new "Title"   # draft a new issue locally
go run ./cmd/tea-issue-sync push          # sync local edits/new files to Gitea
```

See `skill/SKILL.md` (or run `tea-issue-sync skill print`) for the full
command set and file format.

## Code

- **Standard library only** — no third-party Go dependencies (there is no
  `go.sum`, keep it that way).
- The on-disk Markdown format must stay byte-for-byte compatible with
  gh-issue-sync. If you touch rendering or parsing, the round-trip tests must
  still pass.
- `make vet && make test` before pushing; keep `gofmt` clean.
- Logic lives in `internal/teasync`; `cmd/tea-issue-sync` is a thin CLI.

## Releases

- Add a `## [X.Y.Z] - DATE` section to `CHANGELOG.md` and bump
  `teasync.Version` before tagging. The release workflow extracts that
  section for the GitHub release notes.
- Tag `vX.Y.Z` on `main`; the GitHub Actions release workflow builds and
  attaches the binaries. The push mirror is scheduled, so force a sync after
  tagging if you need it on GitHub immediately.
