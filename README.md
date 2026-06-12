# tea-issue-sync

Mirror a [Gitea](https://about.gitea.com/) repository's issues to local
Markdown files — one file per issue, with YAML frontmatter — so you can read,
search, grep, and (eventually) edit your issues from your editor instead of a
browser.

> **Inspired by [`gh-issue-sync`](https://github.com/mitsuhiko/gh-issue-sync)**
> by Armin Ronacher (mitsuhiko), which does the same thing for GitHub.
> `tea-issue-sync` is the Gitea-side counterpart and keeps the **same on-disk
> Markdown format**, so tooling written against `gh-issue-sync`'s `.issues/`
> folder works against a `tea-issue-sync` folder unchanged.

This is an early-stage tool: `pull` (Gitea → local Markdown) is implemented;
`push` / `status` / `diff` are on the [roadmap](#roadmap). Zero runtime
dependencies — just Node 18+ (uses the global `fetch`).

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

## Usage

```bash
node tea-issue-sync.mjs pull            # mirror Gitea → <output>/
node tea-issue-sync.mjs pull --dry-run  # counts + a sample, writes nothing
node tea-issue-sync.mjs --help
```

## Configuration & auth

`config.json` (next to the script) defines the target and output:

```json
{
  "gitea":  { "url": "https://gitea.example.com", "owner": "me", "repo": "my-repo" },
  "output": { "dir": ".issues-tea" },
  "mirror": { "pullLabel": "pull-request", "pullTitlePrefix": "[PR #" }
}
```

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

## Roadmap

- [x] `pull` — Gitea → local Markdown
- [ ] `push` — local → Gitea (create/update issues from edited Markdown)
- [ ] `status` / `diff` — show local-vs-remote drift
- [ ] incremental pull (since-timestamp) instead of a full rewrite
- [ ] packaging as an installable CLI / standalone binary

## Credit

Conceptually modeled on [`gh-issue-sync`](https://github.com/mitsuhiko/gh-issue-sync).
The "issues as a folder of Markdown files" idea, and the frontmatter format, are
its design — this project carries them over to Gitea.

## License

TBD.
