---
name: tea-issue-sync
description: "Manage Gitea issues locally as Markdown files. Use for triaging, searching, editing, creating, and closing issues without leaving your editor or terminal."
---

# tea-issue-sync

Mirrors Gitea issues to local Markdown files under `<output>/open/` and
`<output>/closed/` (default output `.issues-tea/`). Edit the files, then push.
**Gitea is the source of truth.**

## Commands

Sync (talk to Gitea):

```
tea-issue-sync pull [--incremental]   # mirror Gitea → local (full wipes+rewrites; --incremental = only changed)
tea-issue-sync status                 # local-vs-remote drift, per file/field (exit 1 if any)
tea-issue-sync diff                   # unified diff of the drift, remote → local
tea-issue-sync push [--dry-run]       # push local edits / new files to Gitea
```

Edit locally (no network, no token; run `push` to sync):

```
tea-issue-sync new "Title" [--label L]... [--body TEXT] [--state open|closed]
tea-issue-sync close <n> [--reason completed|not_planned]
tea-issue-sync reopen <n>
tea-issue-sync comment <n> [--body TEXT] [--edit]
tea-issue-sync list [--state open|closed|all] [--label L] [--search TEXT]
```

Every command accepts `--config <path>`.

## File format

`<output>/open/42-fix-login-bug.md`:

```markdown
---
title: Fix login bug
labels:
    - bug
    - p1
state: open
state_reason: null
---

Issue body in Markdown.
```

The **leading number in the filename is the issue's identity** — it is not
stored in frontmatter. `labels: []` means none. Titles may be single-quoted.

## Creating issues

A file with **no** leading `<number>-` is a new issue. Use `new`, or just
write a file like `open/my-idea.md` with frontmatter + body. On `push` it is
created on Gitea and renamed to `<index>-<slug>.md`.

## Closing / reopening

Flip `state:` in the frontmatter (or use `close` / `reopen`). `push` updates
the remote state. The frontmatter `state` wins over which folder the file is
in; folders are reorganized on the next `pull`.

## Comments

- **Read**: when `output.comments` is true, each pull mirrors an issue's
  comments into a read-only sidecar `<index>-<slug>.comments.md` (plural).
  Don't edit these — read with `cat`.
- **Post**: a singular `<n>.comment.md` (or `<index>-<slug>.comment.md`) file
  is a pending comment; `push` posts it on issue `#n` and deletes the file.
  Use `comment <n> --body "…"` (or `--edit`) to write it. `status` lists
  pending comments.

## Notes

- `push` never deletes on Gitea. Deleting a local file does nothing remotely;
  the next `pull` restores it.
- Labels must already exist on the remote — unknown labels are skipped with a
  warning, never created.
- `pulls/` mirrors pull requests and is read-only; never edit or push it.
- Token for the sync commands: `$GITEA_TOKEN`, else the `tea` CLI's
  `config.yml`. The local-only verbs (`new`/`close`/`reopen`/`list`) need no
  token.
- Config is optional: in a git repo whose `origin` points at the Gitea repo,
  the URL/owner/repo are inferred from the remote. A `.tea-issue-sync.json` is
  only needed to override (different repo/remote, custom `output.dir`,
  comments).
