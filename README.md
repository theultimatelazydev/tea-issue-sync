# tea-issue-sync

Mirror a [Gitea](https://about.gitea.com/) repository's issues to local
Markdown files — one file per issue, with YAML frontmatter — so you can read,
search, grep, and edit your issues from your editor instead of a browser, and
push the edits back.

> **Inspired by [`gh-issue-sync`](https://github.com/mitsuhiko/gh-issue-sync)**
> by Armin Ronacher (mitsuhiko), which does the same thing for GitHub.
> `tea-issue-sync` is the Gitea-side counterpart and keeps the **same on-disk
> Markdown format**, so tooling written against `gh-issue-sync`'s `.issues/`
> folder works against a `tea-issue-sync` folder unchanged.

`pull` (Gitea → local, full or incremental), `status` / `diff` (drift
inspection) and `push` (local → Gitea) are implemented; see
[ROADMAP.md](ROADMAP.md). A single native binary written in Go — no runtime
to install, no dependencies beyond the standard library.

## What it does

`pull` walks every issue in the repo via the Gitea API and writes one Markdown
file per issue into an output folder:

```
<output>/
  open/        <index>-<slug>.md      open issues
  closed/      <index>-<slug>.md      closed issues
  pulls/open|closed/                  pull requests (see below)
  .gitea-sync.json                    provenance + counts (generated)
```

Each run wipes and rewrites those managed subfolders — **Gitea is the source of
truth**, so renames and deletions there propagate cleanly without leaving stale
local files. The tool and the data it produces live in separate folders so the
tool can move independently of any particular repo's mirror.

> The output folder is typically a local read-cache (e.g. `.gitignore`d) since
> the issues already live on the server — the Markdown just saves you a round
> trip. Track it only if you want issues-as-code (diffable, offline, in CI).

## Install

Any one of:

```bash
# 1. Prebuilt binary (no toolchain needed)
curl -fsSL https://raw.githubusercontent.com/theultimatelazydev/tea-issue-sync/main/install.sh | sh

# 2. With the Go toolchain
go install github.com/theultimatelazydev/tea-issue-sync/cmd/tea-issue-sync@latest

# 3. From a clone
make install        # → $GOBIN, or: make build → ./dist/tea-issue-sync
```

`install.sh` grabs the right binary for your OS/arch from the latest GitHub
release; point it elsewhere with `TEA_ISSUE_SYNC_BASE_URL` or pass a target
path as its first argument.

## Usage

```bash
# Sync (talk to Gitea)
tea-issue-sync pull                 # mirror Gitea → <output>/ (full rewrite)
tea-issue-sync pull --incremental   # only issues updated since the last sync
tea-issue-sync pull --dry-run       # counts + a sample, writes nothing
tea-issue-sync status               # list local-vs-remote drift (exit 1 if any)
tea-issue-sync diff                 # unified diff of the drift, remote → local
tea-issue-sync push                 # push local edits / new files to Gitea
tea-issue-sync push --dry-run       # show what push would do

# Edit locally (no network; run push to sync)
tea-issue-sync new "Title" --label bug --body "…"   # create an issue file
tea-issue-sync close 42 --reason completed          # mark closed, move to closed/
tea-issue-sync reopen 42                             # mark open, move to open/
tea-issue-sync list --search login --state open      # list from the local mirror

tea-issue-sync --help               # also: --version
```

Every command accepts `--config <path>` (see below). Without installing, run
from a clone with `go run ./cmd/tea-issue-sync <command>`.

The `new`/`close`/`reopen`/`list` verbs are pure local-file convenience
wrappers — they never touch Gitea. `new` writes a number-less file,
`close`/`reopen` flip the frontmatter `state` and move the file; `push`
remains the one command that syncs everything to Gitea.

An incremental pull resumes from the `syncedAt` high-water mark in
`.gitea-sync.json` and rewrites only what changed (handling renames and
state moves). Remote *deletions* are invisible to the since-feed, so run a
full `pull` periodically to reap those.

### Comments

Set `"comments": true` in the `output` section to mirror each issue's
comments into a read-only sidecar next to the issue file —
`<index>-<slug>.comments.md` — one `## @author — timestamp` section per
comment. The issue file itself stays in the `gh-issue-sync`-compatible
format, and `status` / `diff` / `push` ignore the sidecars.

## Editing issues locally

After a `pull`, the Markdown files are yours to edit — `title`, `labels`,
`state` in the frontmatter, and the body below it. `status` shows what
drifted (per file, per field), `diff` shows it as a unified diff, and `push`
applies it to Gitea, updating only the fields that actually differ.

Conventions:

- **The leading number in the filename is the issue's identity.** A file
  *without* one (e.g. `open/my-idea.md` with just frontmatter + body) is a
  new issue: `push` creates it on Gitea and renames the file to its canonical
  `<index>-<slug>.md` name.
- **Frontmatter `state` wins over folder placement.** Flip `state: open` to
  `closed` to close an issue on push; the file moves to the right folder on
  the next `pull`.
- **`push` never deletes.** Deleting a local file does nothing remotely
  (Gitea stays the source of truth for existence; the next `pull` simply
  restores the mirror). Labels that don't exist on the remote are skipped
  with a warning, not created.
- **`pulls/` is read-only.** The PR mirror is never compared or pushed.

## Configuration & auth

**Config is optional.** Run `tea-issue-sync` inside a git repo whose `origin`
remote points at the Gitea repo and it infers everything — the instance URL,
owner, and repo — from that remote. No config file required.

Add a `.tea-issue-sync.json` only to override a default (the name is
tool-specific on purpose, so it never collides with a project's own
`config.json`):

```json
{
  "gitea":  { "url": "https://gitea.example.com", "owner": "me", "repo": "my-repo", "remote": "origin" },
  "output": { "dir": ".issues-tea", "comments": false },
  "mirror": { "pullLabel": "pull-request", "pullTitlePrefix": "[PR #" }
}
```

Every field is optional and only overrides what's otherwise inferred or
defaulted:

- `gitea.url` / `owner` / `repo` — point at a different repo than the origin
  remote (explicit values win over inference).
- `gitea.remote` — infer from a remote other than `origin`.
- `output.dir` — where the mirror lands (default `.issues-tea`).
- `output.comments` — mirror comments into `<index>-<slug>.comments.md`.
- `mirror.*` — how PR-mirror items are recognized.

Config is resolved per *invocation*: `--config <path>` if given, else the
nearest `.tea-issue-sync.json` from the cwd up to the git root, with an
optional `.tea-issue-sync.local.json` overlay next to it (per-field, e.g. for
a per-machine URL you don't want tracked). `output.dir` is relative to the
config file, or the git root when there's no config.

The API token is resolved at runtime and **never stored in config or printed**:
`$GITEA_TOKEN` first, otherwise the matching login's token from the
[`tea`](https://gitea.com/gitea/tea) CLI's `config.yml`
(`~/Library/Application Support/tea/config.yml` or `~/.config/tea/config.yml`).

## Pull requests, and the `gitea-mirror` pattern

In Gitea's API, pull requests surface alongside issues. `tea-issue-sync` detects
them (by the non-null `pull_request` field, a `pull-request` label, or a
configurable title prefix) and routes them to `pulls/` so they don't mix in with
real issues.

This also handles repos created with
[RayLabsHQ/gitea-mirror](https://github.com/RayLabsHQ/gitea-mirror), which
imports a GitHub repo's pull requests **as Gitea issues** — labelled
`pull-request`, titled `[PR #N] [MERGED] …`. Those are recognized by the same
rules and routed to `pulls/` as well.

When a repo was mirrored from GitHub, the original GitHub number is typically
preserved in the title prefix (`[GH-ISSUE #N]` / `[PR #N]`) and the issue body;
`tea-issue-sync` uses the **Gitea index** for the filename (the live ID on the
current platform) and leaves those references intact.

## File format

```markdown
---
title: '[Alpha] Example issue title'
labels:
    - alpha
    - p2
state: open
state_reason: null
---

Issue body, verbatim.
```

Single-quoted `title`, a YAML `labels` list (4-space indent), `state`, and
`state_reason` — identical to `gh-issue-sync`, so reading tools are portable
between the two.

## Development

```bash
make test     # go test ./...   (pure-helper suite, stdlib only)
make vet
make build    # → dist/tea-issue-sync
make dist     # cross-compile darwin/linux × amd64/arm64 into dist/
```

The logic lives in `internal/teasync` (unit-tested); `cmd/tea-issue-sync` is
a thin CLI. CI runs vet + tests on both [Gitea](.gitea/workflows/test.yml)
(needs a registered Actions runner) and [GitHub](.github/workflows/test.yml);
tagging `v*` triggers [a GitHub release](.github/workflows/release.yml) that
builds and attaches the binaries.

## Roadmap

See [ROADMAP.md](ROADMAP.md). Done: phases 0–3 (full + incremental pull,
comments, status, diff, push, tests + CI, native binary distribution).

## Credit

Conceptually modeled on [`gh-issue-sync`](https://github.com/mitsuhiko/gh-issue-sync).
The "issues as a folder of Markdown files" idea, and the frontmatter format, are
its design — this project carries them over to Gitea.

## License

Apache-2.0.

Like `gh-issue-sync`: this code is entirely LLM generated. It is
unclear if LLM generated code can be copyrighted.
