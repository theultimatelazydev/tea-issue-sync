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
[ROADMAP.md](ROADMAP.md). Ships as a self-contained binary (no runtime
needed) or runs from source on Node 18+ with zero dependencies.

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

Download a binary from the repo's **releases** page — it's self-contained,
no Node required:

```bash
curl -fL -o /usr/local/bin/tea-issue-sync \
  "https://<your-gitea>/TheUltimateLazyDev/tea-issue-sync/releases/download/v1.0.0/tea-issue-sync-$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/x64/;s/aarch64/arm64/')"
chmod +x /usr/local/bin/tea-issue-sync
```

Or skip installing and run from source with Node 18+ (zero dependencies):
`node tea-issue-sync.mjs …`. To build binaries yourself you need
[bun](https://bun.sh): `./build.sh` for the current platform, or
`./build.sh --all` for darwin/linux × arm64/x64 into `dist/`.

## Usage

```bash
node tea-issue-sync.mjs pull                 # mirror Gitea → <output>/ (full rewrite)
node tea-issue-sync.mjs pull --incremental   # only issues updated since the last sync
node tea-issue-sync.mjs pull --dry-run       # counts + a sample, writes nothing
node tea-issue-sync.mjs status               # list local-vs-remote drift (exit 1 if any)
node tea-issue-sync.mjs diff                 # unified diff of the drift, remote → local
node tea-issue-sync.mjs push                 # push local edits / new files to Gitea
node tea-issue-sync.mjs push --dry-run       # show what push would do
node tea-issue-sync.mjs --help               # also: --version
```

Every command accepts `--config <path>` (see below).

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

`config.json` defines the target and output. It is resolved per *invocation*,
not per install: `--config <path>` if given, otherwise the nearest
`config.json` walking up from the current directory (stopping at the git
root). A `config.local.json` next to the chosen config overrides it per
top-level section — useful for per-machine URLs without touching the tracked
file. `output.dir` is relative to the config file's directory, so the mirror
lands in the same place no matter where in the repo you invoke the tool from.

The `config.json` tracked in this repo holds placeholder values: either edit
it in your own checkout, or leave it and put your real instance in a
(gitignored) `config.local.json` next to it.

```json
{
  "gitea":  { "url": "https://gitea.example.com", "owner": "me", "repo": "my-repo" },
  "output": { "dir": ".issues-tea", "comments": false },
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

## Development

```bash
node --test tea-issue-sync.test.mjs
```

Zero dependencies — the suite uses Node's built-in `node:test`. CI runs it
via [`.gitea/workflows/test.yml`](.gitea/workflows/test.yml) (needs a
registered Gitea Actions runner).

## Roadmap

See [ROADMAP.md](ROADMAP.md). Done: phases 0–2 (pull incl. incremental and
comments, status, diff, push, tests + CI). Next: standalone packaging.

## Credit

Conceptually modeled on [`gh-issue-sync`](https://github.com/mitsuhiko/gh-issue-sync).
The "issues as a folder of Markdown files" idea, and the frontmatter format, are
its design — this project carries them over to Gitea.

## License

Apache-2.0. Like `gh-issue-sync`: this code is entirely LLM generated. It is
unclear if LLM generated code can be copyrighted.
