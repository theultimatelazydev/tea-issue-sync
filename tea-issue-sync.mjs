#!/usr/bin/env node
// tea-issue-sync — mirror Gitea issues (and gitea-mirror's PR-as-issue items)
// into a local Markdown folder, in the same format as gh-issue-sync's issues.
//
// Inspired by gh-issue-sync (https://github.com/mitsuhiko/gh-issue-sync), the
// GitHub-side tool with the same idea. pull, status, diff and push are
// implemented — see ROADMAP.md for what's next. Zero runtime dependencies
// (Node 18+ global fetch).
//
// The tool is intentionally separate from the issue DATA it produces (the
// output folder, default `.issues-tea/`), so it can be extracted to its own
// repo/binary later without dragging any particular repo's mirror along.
//
// Usage:
//   node tea-issue-sync.mjs pull [--incremental] [--dry-run] [--config <path>]
//   node tea-issue-sync.mjs status|diff [--config <path>]
//   node tea-issue-sync.mjs push [--dry-run] [--config <path>]
//   node tea-issue-sync.mjs --help | --version
//
// The pure helpers are exported for the test suite; setting
// $TEA_ISSUE_SYNC_AS_LIB skips the CLI dispatch on import.
//
// Config resolution: --config <path> if given, else the nearest config.json
// walking up from the cwd (stopping at the git root). A config.local.json
// next to the chosen config overrides it per top-level section. Paths in the
// config (output.dir) are relative to the config file's directory.
//
// Token resolution (never committed): $GITEA_TOKEN, else the matching login's
// token in tea's config.yml (~/Library/Application Support/tea/config.yml or
// ~/.config/tea/config.yml).

import { readFileSync, rmSync, mkdirSync, writeFileSync, existsSync, readdirSync, renameSync } from "node:fs";
import { dirname, join, resolve, relative } from "node:path";
import { homedir } from "node:os";

export const VERSION = "0.9.0";

const MANAGED_DIRS = ["open", "closed", "pulls/open", "pulls/closed"];

function findConfig(explicitPath) {
  if (explicitPath) {
    const p = resolve(explicitPath);
    if (!existsSync(p)) fail(`config not found: ${explicitPath}`);
    return p;
  }
  let dir = process.cwd();
  for (;;) {
    const candidate = join(dir, "config.json");
    if (existsSync(candidate)) return candidate;
    const parent = dirname(dir);
    if (existsSync(join(dir, ".git")) || parent === dir) break; // git root / fs root
    dir = parent;
  }
  fail("no config.json found between the cwd and the git root; pass --config <path>");
}

function loadConfig(configPath) {
  const cfg = JSON.parse(readFileSync(configPath, "utf8"));
  // Per-machine overrides, merged per top-level section (gitea/output/mirror).
  const localPath = join(dirname(configPath), "config.local.json");
  if (existsSync(localPath)) {
    const local = JSON.parse(readFileSync(localPath, "utf8"));
    for (const [key, val] of Object.entries(local)) {
      cfg[key] = val && typeof val === "object" && !Array.isArray(val) ? { ...cfg[key], ...val } : val;
    }
  }
  if (!cfg.gitea?.url || !cfg.gitea?.owner || !cfg.gitea?.repo) {
    fail(`${configPath} missing gitea.url / gitea.owner / gitea.repo`);
  }
  return cfg;
}

// Resolve a Gitea API token without ever printing or persisting it.
function resolveToken(giteaUrl) {
  if (process.env.GITEA_TOKEN) return process.env.GITEA_TOKEN.trim();
  const candidates = [
    join(homedir(), "Library", "Application Support", "tea", "config.yml"),
    join(homedir(), ".config", "tea", "config.yml"),
  ];
  for (const path of candidates) {
    if (!existsSync(path)) continue;
    // Light line-scan of tea's YAML: pair each `url:` with the nearest `token:`
    // in the same login block; prefer the login whose url matches our host.
    const lines = readFileSync(path, "utf8").split(/\r?\n/);
    let curUrl = null;
    let firstToken = null;
    let matchToken = null;
    for (const line of lines) {
      const u = line.match(/^\s*url:\s*(.+?)\s*$/);
      if (u) curUrl = u[1].replace(/['"]/g, "");
      const t = line.match(/^\s*token:\s*(.+?)\s*$/);
      if (t) {
        const tok = t[1].replace(/['"]/g, "");
        if (!firstToken) firstToken = tok;
        if (curUrl && giteaUrl.startsWith(curUrl.replace(/\/+$/, ""))) matchToken = tok;
      }
    }
    if (matchToken) return matchToken;
    if (firstToken) return firstToken;
  }
  fail("no token: set $GITEA_TOKEN or log in with `tea login add`");
}

function apiBase(cfg) {
  return `${cfg.gitea.url.replace(/\/+$/, "")}/api/v1`;
}

function repoPath(cfg) {
  return `repos/${cfg.gitea.owner}/${cfg.gitea.repo}`;
}

async function apiGetPaged(cfg, token, path) {
  const sep = path.includes("?") ? "&" : "?";
  const out = [];
  for (let page = 1; page < 200; page++) {
    const res = await fetch(`${apiBase(cfg)}/${path}${sep}limit=50&page=${page}`, {
      headers: { Authorization: `token ${token}` },
    });
    if (!res.ok) throw new Error(`Gitea API ${res.status} ${res.statusText} on GET ${path} (page ${page})`);
    const batch = await res.json();
    if (!Array.isArray(batch) || batch.length === 0) break;
    out.push(...batch);
  }
  return out;
}

// Single unpaginated GET — for endpoints that ignore limit/page (e.g. the
// per-issue comments list) and would make apiGetPaged spin to the page cap.
async function apiGetOnce(cfg, token, path) {
  const res = await fetch(`${apiBase(cfg)}/${path}`, {
    headers: { Authorization: `token ${token}` },
  });
  if (!res.ok) throw new Error(`Gitea API ${res.status} ${res.statusText} on GET ${path}`);
  return res.json();
}

async function apiSend(cfg, token, method, path, body) {
  const res = await fetch(`${apiBase(cfg)}/${path}`, {
    method,
    headers: { Authorization: `token ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`Gitea API ${res.status} ${res.statusText} on ${method} ${path}`);
  return res.status === 204 ? null : res.json();
}

function fetchIssues(cfg, token) {
  // type=issues excludes real Gitea PRs; gitea-mirror's PR-as-issue items
  // still come through and are filtered by isPull().
  return apiGetPaged(cfg, token, `${repoPath(cfg)}/issues?state=all&type=issues`);
}

// gitea-mirror brings GitHub PRs across as issues; also real Gitea PRs surface
// in the issues API with a non-null pull_request field. Treat all as "pulls".
export function isPull(item, cfg) {
  if (item.pull_request) return true;
  const label = (cfg.mirror?.pullLabel || "pull-request").toLowerCase();
  if ((item.labels || []).some((l) => (l.name || "").toLowerCase() === label)) return true;
  const prefix = cfg.mirror?.pullTitlePrefix || "[PR #";
  return typeof item.title === "string" && item.title.startsWith(prefix);
}

export function slugify(title) {
  return String(title)
    .replace(/^\[(GH-ISSUE|PR)\s*#\d+\]\s*/i, "") // drop the cross-ref prefix
    .replace(/^\[MERGED\]\s*/i, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 90)
    .replace(/-+$/g, "") || "untitled";
}

export function yamlSingleQuote(s) {
  return `'${String(s).replace(/'/g, "''")}'`;
}

export function renderMarkdown(item) {
  const labels = (item.labels || []).map((l) => l.name).filter(Boolean);
  const fm = ["---", `title: ${yamlSingleQuote(item.title)}`];
  if (labels.length) {
    fm.push("labels:");
    for (const l of labels) fm.push(`    - ${l}`);
  } else {
    fm.push("labels: []");
  }
  fm.push(`state: ${item.state || "open"}`);
  fm.push(`state_reason: ${item.state_reason ? yamlSingleQuote(item.state_reason) : "null"}`);
  fm.push("---", "");
  const body = (item.body || "").replace(/\r\n/g, "\n").replace(/\s*$/, "");
  return `${fm.join("\n")}\n${body}\n`;
}

// Comments are mirrored (when output.comments is true) into a read-only
// sidecar next to each issue file — <index>-<slug>.comments.md — so the
// issue file itself stays in the gh-issue-sync-compatible format.
export function renderComments(item, comments) {
  const parts = [`# Comments — #${item.number} ${item.title}`.trimEnd(), ""];
  for (const c of comments) {
    const who = c.user?.login || c.user?.username || "unknown";
    parts.push(`## @${who} — ${c.created_at || "?"}`, "", normBody(c.body), "");
  }
  return parts.join("\n").replace(/\n*$/, "\n");
}

function commentsFileFor(issueFile) {
  return issueFile.replace(/\.md$/, ".comments.md");
}

function countMirror(outDir) {
  const count = (subs) =>
    subs.reduce((n, s) => {
      const d = join(outDir, s);
      if (!existsSync(d)) return n;
      return n + readdirSync(d).filter((f) => f.endsWith(".md") && !f.endsWith(".comments.md")).length;
    }, 0);
  return { issues: count(["open", "closed"]), pulls: count(["pulls/open", "pulls/closed"]) };
}

// ---------------------------------------------------------------------------
// Local mirror read-back (status / diff / push)
// ---------------------------------------------------------------------------

export function normBody(s) {
  return String(s || "").replace(/\r\n/g, "\n").replace(/\s*$/, "");
}

function yamlUnquote(s) {
  s = s.trim();
  if (s.startsWith("'") && s.endsWith("'") && s.length >= 2) return s.slice(1, -1).replace(/''/g, "'");
  if (s.startsWith('"') && s.endsWith('"') && s.length >= 2) return s.slice(1, -1);
  return s;
}

// Parse the frontmatter subset this tool writes (title, labels, state,
// state_reason). Tolerates hand edits: unquoted/double-quoted scalars,
// `labels: []`, varying list indentation.
export function parseIssueFile(content, where) {
  const m = content.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!m) throw new Error(`${where}: missing frontmatter`);
  const meta = { title: null, labels: [], state: "open", state_reason: null };
  let inLabels = false;
  for (const line of m[1].split("\n")) {
    const item = line.match(/^\s+-\s+(.+?)\s*$/);
    if (inLabels && item) {
      meta.labels.push(yamlUnquote(item[1]));
      continue;
    }
    inLabels = false;
    const kv = line.match(/^(\w+):\s*(.*)$/);
    if (!kv) continue;
    const [, key, raw] = kv;
    if (key === "labels") {
      inLabels = raw === "";
    } else if (key in meta) {
      meta[key] = raw === "" || raw === "null" ? null : yamlUnquote(raw);
    }
  }
  if (!meta.title) throw new Error(`${where}: missing title in frontmatter`);
  if (meta.state !== "closed") meta.state = "open";
  // Drop the single blank separator line renderMarkdown writes after the
  // frontmatter; it's formatting, not body content.
  return { meta, body: content.slice(m[0].length).replace(/^\n/, "") };
}

// Scan open/ and closed/ for issue files. The leading number in the filename
// is the issue's identity; files without one are new issues to create on
// push. pulls/ is a read-only mirror and is never scanned.
function readLocalIssues(outDir) {
  const out = [];
  for (const dir of ["open", "closed"]) {
    const d = join(outDir, dir);
    if (!existsSync(d)) continue;
    for (const file of readdirSync(d).sort()) {
      if (!file.endsWith(".md") || file.endsWith(".comments.md")) continue;
      const { meta, body } = parseIssueFile(readFileSync(join(d, file), "utf8"), `${dir}/${file}`);
      const num = file.match(/^(\d+)-/);
      out.push({ number: num ? Number(num[1]) : null, dir, file, meta, body });
    }
  }
  return out;
}

function remoteLabelNames(item) {
  return (item.labels || []).map((l) => l.name).filter(Boolean);
}

// Which fields differ between a local file and its remote issue. The
// frontmatter `state` wins over folder placement (pull reorganizes folders).
export function diffFields(local, remote) {
  const fields = [];
  if (local.meta.title !== remote.title) fields.push("title");
  const ll = [...local.meta.labels].sort().join("\n");
  const rl = remoteLabelNames(remote).sort().join("\n");
  if (ll !== rl) fields.push("labels");
  if (local.meta.state !== (remote.state === "closed" ? "closed" : "open")) fields.push("state");
  if (normBody(local.body) !== normBody(remote.body)) fields.push("body");
  return fields;
}

// A local entry shaped like an API item, so renderMarkdown() can produce the
// canonical form of both sides (formatting-only edits don't count as drift).
export function localAsItem(l) {
  return {
    title: l.meta.title,
    labels: l.meta.labels.map((name) => ({ name })),
    state: l.meta.state,
    state_reason: l.meta.state_reason,
    body: l.body,
  };
}

async function computeDrift(configPath) {
  const cfgPath = findConfig(configPath);
  const cfg = loadConfig(cfgPath);
  const token = resolveToken(cfg.gitea.url);
  const outDir = resolve(dirname(cfgPath), cfg.output?.dir || ".issues-tea");
  if (!existsSync(outDir)) fail(`no local mirror at ${relForLog(outDir)} — run pull first`);
  const remote = new Map();
  for (const item of await fetchIssues(cfg, token)) {
    if (!isPull(item, cfg)) remote.set(item.number, item);
  }
  const drift = { modified: [], created: [], orphaned: [], missing: [], clean: 0 };
  const seen = new Set();
  for (const l of readLocalIssues(outDir)) {
    if (l.number == null) {
      drift.created.push(l);
      continue;
    }
    seen.add(l.number);
    const r = remote.get(l.number);
    if (!r) {
      drift.orphaned.push(l);
      continue;
    }
    const fields = diffFields(l, r);
    if (fields.length) drift.modified.push({ local: l, remote: r, fields });
    else drift.clean++;
  }
  for (const [num, r] of remote) if (!seen.has(num)) drift.missing.push(r);
  return { cfg, token, outDir, drift };
}

function hasDrift(drift) {
  return drift.modified.length + drift.created.length + drift.orphaned.length + drift.missing.length > 0;
}

// Minimal LCS-based unified diff — enough for issue-sized texts, keeps the
// zero-dependency promise. Returns null when the texts are identical.
export function unifiedDiff(aText, bText, aLabel, bLabel, context = 3) {
  if (aText === bText) return null;
  const a = aText.split("\n");
  const b = bText.split("\n");
  const n = a.length;
  const m = b.length;
  const dp = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const ops = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      ops.push([" ", a[i]]); i++; j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      ops.push(["-", a[i]]); i++;
    } else {
      ops.push(["+", b[j]]); j++;
    }
  }
  while (i < n) ops.push(["-", a[i++]]);
  while (j < m) ops.push(["+", b[j++]]);
  let ai = 1;
  let bi = 1;
  const pos = ops.map(([t]) => {
    const p = [ai, bi];
    if (t !== "+") ai++;
    if (t !== "-") bi++;
    return p;
  });
  const lines = [`--- ${aLabel}`, `+++ ${bLabel}`];
  let k = 0;
  while (k < ops.length) {
    if (ops[k][0] === " ") {
      k++;
      continue;
    }
    const start = Math.max(0, k - context);
    let end = k + 1;
    let lastChange = k;
    while (end < ops.length && end - lastChange <= context * 2 + 1) {
      if (ops[end][0] !== " ") lastChange = end;
      end++;
    }
    end = Math.min(ops.length, lastChange + context + 1);
    const hunk = ops.slice(start, end);
    const aCount = hunk.filter(([t]) => t !== "+").length;
    const bCount = hunk.filter(([t]) => t !== "-").length;
    lines.push(`@@ -${pos[start][0]},${aCount} +${pos[start][1]},${bCount} @@`);
    for (const [t, line] of hunk) lines.push(t + line);
    k = end;
  }
  return lines.join("\n");
}

function relForLog(absPath) {
  const rel = relative(process.cwd(), absPath);
  return rel && !rel.startsWith("..") ? rel : absPath;
}

function routeFor(item, cfg) {
  const pull = isPull(item, cfg);
  const state = item.state === "closed" ? "closed" : "open";
  return { pull, dir: pull ? `pulls/${state}` : state, file: `${item.number}-${slugify(item.title)}.md` };
}

// The marker records provenance and the high-water timestamp incremental
// pulls resume from. It lives inside the output dir, which is gitignored,
// so the real instance URL never reaches tracked files.
function writeSyncMarker(outDir, cfg, syncedAt) {
  const counts = countMirror(outDir);
  writeFileSync(
    join(outDir, ".gitea-sync.json"),
    JSON.stringify(
      { source: `${cfg.gitea.owner}/${cfg.gitea.repo}`, url: cfg.gitea.url, issues: counts.issues, pulls: counts.pulls, syncedAt },
      null,
      2,
    ) + "\n",
    "utf8",
  );
}

// Timestamps come back RFC3339 but not always in the same zone notation, so
// compare instants, not strings.
function newerStamp(a, b) {
  if (!a) return b;
  if (!b) return a;
  return Date.parse(a) > Date.parse(b) ? a : b;
}

async function cmdPull({ dryRun, configPath, incremental }) {
  const cfgPath = findConfig(configPath);
  const cfg = loadConfig(cfgPath);
  const token = resolveToken(cfg.gitea.url);
  // output.dir is relative to the config file, so the mirror lands in the
  // same place no matter how deep in the repo the tool is invoked from.
  const outDir = resolve(dirname(cfgPath), cfg.output?.dir || ".issues-tea");
  const withComments = cfg.output?.comments === true;
  if (incremental) return pullIncremental(cfg, token, outDir, { dryRun, withComments });

  process.stdout.write(`Fetching issues from ${cfg.gitea.url} (${cfg.gitea.owner}/${cfg.gitea.repo}) …\n`);
  const items = await fetchIssues(cfg, token);

  // One paginated call for the whole repo's comments, grouped by issue.
  const commentsByIssue = new Map();
  if (withComments) {
    process.stdout.write("Fetching comments …\n");
    for (const c of await apiGetPaged(cfg, token, `${repoPath(cfg)}/issues/comments`)) {
      const num = Number((c.issue_url || "").split("/").pop());
      if (!Number.isFinite(num) || num <= 0) continue;
      if (!commentsByIssue.has(num)) commentsByIssue.set(num, []);
      commentsByIssue.get(num).push(c);
    }
  }

  const plan = []; // { dir, file, content }
  let nIssue = 0, nPull = 0, nComments = 0, syncedAt = null;
  for (const item of items) {
    syncedAt = newerStamp(item.updated_at, syncedAt);
    const { pull, dir, file } = routeFor(item, cfg);
    plan.push({ dir, file, content: renderMarkdown(item) });
    if (pull) nPull++; else nIssue++;
    const comments = commentsByIssue.get(item.number);
    if (comments?.length) {
      plan.push({ dir, file: commentsFileFor(file), content: renderComments(item, comments) });
      nComments += comments.length;
    }
  }
  const commentNote = withComments ? ` (+${nComments} comments)` : "";

  if (dryRun) {
    process.stdout.write(`[dry-run] ${items.length} items → ${nIssue} issues + ${nPull} pulls${commentNote} into ${cfg.output.dir}/\n`);
    const sample = plan.slice(0, 3).map((p) => `  ${p.dir}/${p.file}`).join("\n");
    process.stdout.write(`sample:\n${sample}\n`);
    return;
  }

  // Managed subfolders — wiped and rewritten each full pull (Gitea is the
  // source of truth; this keeps deletions/renames from leaving stale files).
  for (const sub of MANAGED_DIRS) rmSync(join(outDir, sub), { recursive: true, force: true });
  for (const sub of MANAGED_DIRS) mkdirSync(join(outDir, sub), { recursive: true });
  for (const { dir, file, content } of plan) {
    writeFileSync(join(outDir, dir, file), content, "utf8");
  }
  writeSyncMarker(outDir, cfg, syncedAt || new Date().toISOString());
  process.stdout.write(`Wrote ${nIssue} issues + ${nPull} pulls${commentNote} to ${relForLog(outDir)}/\n`);
}

// Incremental pull: only issues updated since the marker's high-water mark.
// Renames/moves are handled by dropping every file for the issue number
// first; remote DELETIONS are invisible to the since-feed, so a periodic
// full pull is still the way to reap those.
async function pullIncremental(cfg, token, outDir, { dryRun, withComments }) {
  const markerPath = join(outDir, ".gitea-sync.json");
  if (!existsSync(markerPath)) fail(`no sync marker in ${relForLog(outDir)}/ — run a full pull first`);
  const marker = JSON.parse(readFileSync(markerPath, "utf8"));
  if (!marker.syncedAt) fail("sync marker predates incremental support — run a full pull first");

  process.stdout.write(`Fetching issues updated since ${marker.syncedAt} …\n`);
  const items = await apiGetPaged(
    cfg,
    token,
    `${repoPath(cfg)}/issues?state=all&type=issues&since=${encodeURIComponent(marker.syncedAt)}`,
  );
  if (!items.length) {
    process.stdout.write("up to date\n");
    return;
  }
  if (dryRun) {
    for (const item of items) {
      const { dir, file } = routeFor(item, cfg);
      process.stdout.write(`[dry-run] would update ${dir}/${file}\n`);
    }
    return;
  }

  let syncedAt = marker.syncedAt;
  for (const item of items) {
    syncedAt = newerStamp(item.updated_at, syncedAt);
    const { dir, file } = routeFor(item, cfg);
    for (const sub of MANAGED_DIRS) {
      const d = join(outDir, sub);
      if (!existsSync(d)) continue;
      for (const f of readdirSync(d)) {
        if (f.startsWith(`${item.number}-`)) rmSync(join(d, f));
      }
    }
    mkdirSync(join(outDir, dir), { recursive: true });
    writeFileSync(join(outDir, dir, file), renderMarkdown(item), "utf8");
    if (withComments && item.comments > 0) {
      const comments = await apiGetOnce(cfg, token, `${repoPath(cfg)}/issues/${item.number}/comments`);
      if (Array.isArray(comments) && comments.length) {
        writeFileSync(join(outDir, dir, commentsFileFor(file)), renderComments(item, comments), "utf8");
      }
    }
  }
  writeSyncMarker(outDir, cfg, syncedAt);
  process.stdout.write(`Updated ${items.length} item(s) in ${relForLog(outDir)}/ (remote deletions need a full pull)\n`);
}

// status — list local-vs-remote drift. Exits 1 when anything drifts, so it
// can gate scripts/CI.
async function cmdStatus({ configPath }) {
  const { cfg, drift } = await computeDrift(configPath);
  process.stdout.write(`local ↔ ${cfg.gitea.owner}/${cfg.gitea.repo}\n`);
  for (const { local: l, fields } of drift.modified) {
    process.stdout.write(`  ~ ${l.dir}/${l.file}  (${fields.join(", ")})\n`);
  }
  for (const l of drift.created) {
    process.stdout.write(`  + ${l.dir}/${l.file}  (new — push will create)\n`);
  }
  for (const l of drift.orphaned) {
    process.stdout.write(`  ! ${l.dir}/${l.file}  (#${l.number} not on remote — pull will remove)\n`);
  }
  for (const r of drift.missing) {
    process.stdout.write(`  ? #${r.number} '${r.title}' missing locally — run pull\n`);
  }
  const parts = [`${drift.clean} in sync`];
  if (drift.modified.length) parts.push(`${drift.modified.length} modified`);
  if (drift.created.length) parts.push(`${drift.created.length} new`);
  if (drift.orphaned.length) parts.push(`${drift.orphaned.length} orphaned`);
  if (drift.missing.length) parts.push(`${drift.missing.length} missing locally`);
  process.stdout.write(parts.join(", ") + "\n");
  if (hasDrift(drift)) process.exitCode = 1;
}

// diff — unified diff of the drift, remote → local (push direction). Both
// sides are rendered canonically so formatting-only edits don't show up.
async function cmdDiff({ configPath }) {
  const { drift } = await computeDrift(configPath);
  let printed = 0;
  for (const { local: l, remote: r } of drift.modified) {
    const d = unifiedDiff(
      renderMarkdown(r),
      renderMarkdown(localAsItem(l)),
      `remote/#${l.number}`,
      `local/${l.dir}/${l.file}`,
    );
    if (!d) continue;
    process.stdout.write((printed ? "\n" : "") + d + "\n");
    printed++;
  }
  for (const l of drift.created) {
    const d = unifiedDiff("", renderMarkdown(localAsItem(l)), "remote/(none)", `local/${l.dir}/${l.file}`);
    process.stdout.write((printed ? "\n" : "") + d + "\n");
    printed++;
  }
  if (!printed) process.stdout.write("no content drift\n");
  if (hasDrift(drift)) process.exitCode = 1;
}

// push — local → Gitea. Updates only the fields that differ; creates issues
// from number-less files and renames them to their canonical filename.
// Never deletes anything remotely.
async function cmdPush({ dryRun, configPath }) {
  const { cfg, token, outDir, drift } = await computeDrift(configPath);
  const rp = repoPath(cfg);

  for (const l of drift.orphaned) {
    process.stderr.write(`warning: ${l.dir}/${l.file} (#${l.number}) not on remote — skipped (pull will remove it)\n`);
  }
  if (drift.missing.length) {
    process.stderr.write(`warning: ${drift.missing.length} remote issue(s) missing locally — run pull\n`);
  }
  if (!drift.modified.length && !drift.created.length) {
    process.stdout.write("nothing to push\n");
    return;
  }

  // Gitea's label APIs take ids, so resolve names → ids up front. Repo
  // labels first; org labels merged in when the owner is an org.
  const labelIds = new Map();
  const needLabels =
    drift.created.some((l) => l.meta.labels.length) ||
    drift.modified.some((m) => m.fields.includes("labels"));
  if (needLabels) {
    for (const l of await apiGetPaged(cfg, token, `${rp}/labels`)) labelIds.set(l.name.toLowerCase(), l.id);
    try {
      for (const l of await apiGetPaged(cfg, token, `orgs/${cfg.gitea.owner}/labels`)) {
        if (!labelIds.has(l.name.toLowerCase())) labelIds.set(l.name.toLowerCase(), l.id);
      }
    } catch {
      // owner is a user, not an org — repo labels are all there is
    }
  }
  const resolveLabels = (names, where) => {
    const ids = [];
    for (const name of names) {
      const id = labelIds.get(name.toLowerCase());
      if (id == null) process.stderr.write(`warning: label '${name}' not on remote — skipped (${where})\n`);
      else ids.push(id);
    }
    return ids;
  };

  for (const { local: l, fields } of drift.modified) {
    const desc = `${l.dir}/${l.file} (${fields.join(", ")})`;
    if (dryRun) {
      process.stdout.write(`[dry-run] would update #${l.number}: ${desc}\n`);
      continue;
    }
    const patch = {};
    if (fields.includes("title")) patch.title = l.meta.title;
    if (fields.includes("body")) patch.body = normBody(l.body);
    if (fields.includes("state")) patch.state = l.meta.state;
    if (Object.keys(patch).length) {
      await apiSend(cfg, token, "PATCH", `${rp}/issues/${l.number}`, patch);
    }
    if (fields.includes("labels")) {
      await apiSend(cfg, token, "PUT", `${rp}/issues/${l.number}/labels`, {
        labels: resolveLabels(l.meta.labels, desc),
      });
    }
    process.stdout.write(`updated #${l.number}: ${desc}\n`);
  }

  for (const l of drift.created) {
    if (dryRun) {
      process.stdout.write(`[dry-run] would create issue from ${l.dir}/${l.file}\n`);
      continue;
    }
    const payload = {
      title: l.meta.title,
      body: normBody(l.body),
      closed: l.meta.state === "closed",
    };
    if (l.meta.labels.length) payload.labels = resolveLabels(l.meta.labels, `${l.dir}/${l.file}`);
    const created = await apiSend(cfg, token, "POST", `${rp}/issues`, payload);
    const newFile = `${created.number}-${slugify(created.title)}.md`;
    renameSync(join(outDir, l.dir, l.file), join(outDir, l.dir, newFile));
    process.stdout.write(`created #${created.number} from ${l.dir}/${l.file} → ${l.dir}/${newFile}\n`);
  }
}

function fail(msg) {
  process.stderr.write(`tea-issue-sync: ${msg}\n`);
  process.exit(1);
}

const HELP = `tea-issue-sync ${VERSION} — mirror Gitea issues to local Markdown (a gh-issue-sync counterpart)

Usage:
  tea-issue-sync pull [--incremental] [--dry-run] [--config <path>]
                                                      Mirror Gitea → output folder. Full pull wipes the
                                                      managed subfolders; --incremental only rewrites
                                                      issues updated since the last sync (deletions
                                                      still need a full pull)
  tea-issue-sync status [--config <path>]             List local-vs-remote drift (exit 1 when drifted)
  tea-issue-sync diff [--config <path>]               Unified diff of the drift, remote → local
  tea-issue-sync push [--dry-run] [--config <path>]   Push local edits; create issues from number-less files
  tea-issue-sync --help | --version

Config: --config <path>, else the nearest config.json from the cwd up to the git root
        (config.local.json next to it overrides per section; output.dir is relative to the config file).
        Set output.comments=true to mirror comments into <index>-<slug>.comments.md sidecars.
Token:  $GITEA_TOKEN or tea's config.yml
Roadmap: see ROADMAP.md.`;

if (!process.env.TEA_ISSUE_SYNC_AS_LIB) {
  const [cmd, ...rest] = process.argv.slice(2);
  const dryRun = rest.includes("--dry-run");
  const incremental = rest.includes("--incremental");
  const configFlag = rest.indexOf("--config");
  const configPath = configFlag !== -1 ? rest[configFlag + 1] : null;
  if (configFlag !== -1 && !configPath) fail("--config requires a path");
  const commands = { pull: cmdPull, status: cmdStatus, diff: cmdDiff, push: cmdPush };
  if (commands[cmd]) {
    commands[cmd]({ dryRun, configPath, incremental }).catch((e) => fail(e?.message || String(e)));
  } else if (cmd === "--version" || cmd === "-v") {
    process.stdout.write(`tea-issue-sync ${VERSION}\n`);
  } else if (cmd === "--help" || cmd === "-h" || !cmd) {
    process.stdout.write(HELP + "\n");
  } else {
    fail(`unknown command '${cmd}' (try --help)`);
  }
}
