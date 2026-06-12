#!/usr/bin/env node
// tea-issue-sync — mirror Gitea issues (and gitea-mirror's PR-as-issue items)
// into a local Markdown folder, in the same format as gh-issue-sync's issues.
//
// Inspired by gh-issue-sync (https://github.com/mitsuhiko/gh-issue-sync), the
// GitHub-side tool with the same idea. This is the START of the Gitea
// counterpart: `pull` (Gitea → local Markdown) is implemented; push/status/diff
// are TBD — see README.md. Zero runtime dependencies (Node 18+ global fetch).
//
// The tool is intentionally separate from the issue DATA it produces (the
// output folder, default `.issues-tea/`), so it can be extracted to its own
// repo/binary later without dragging any particular repo's mirror along.
//
// Usage:
//   node scripts/tea-issue-sync/tea-issue-sync.mjs pull [--dry-run]
//   node scripts/tea-issue-sync/tea-issue-sync.mjs --help
//
// Token resolution (never committed): $GITEA_TOKEN, else the matching login's
// token in tea's config.yml (~/Library/Application Support/tea/config.yml or
// ~/.config/tea/config.yml).

import { readFileSync, rmSync, mkdirSync, writeFileSync, existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { homedir } from "node:os";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(HERE, "..", "..");

function loadConfig() {
  const cfg = JSON.parse(readFileSync(join(HERE, "config.json"), "utf8"));
  if (!cfg.gitea?.url || !cfg.gitea?.owner || !cfg.gitea?.repo) {
    fail("config.json missing gitea.url / gitea.owner / gitea.repo");
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

async function apiGetAll(cfg, token) {
  const base = `${cfg.gitea.url.replace(/\/+$/, "")}/api/v1/repos/${cfg.gitea.owner}/${cfg.gitea.repo}/issues`;
  const out = [];
  for (let page = 1; page < 200; page++) {
    const url = `${base}?state=all&type=issues&limit=50&page=${page}`;
    const res = await fetch(url, { headers: { Authorization: `token ${token}` } });
    if (!res.ok) fail(`Gitea API ${res.status} ${res.statusText} on page ${page}`);
    const batch = await res.json();
    if (!Array.isArray(batch) || batch.length === 0) break;
    out.push(...batch);
  }
  return out;
}

// gitea-mirror brings GitHub PRs across as issues; also real Gitea PRs surface
// in the issues API with a non-null pull_request field. Treat all as "pulls".
function isPull(item, cfg) {
  if (item.pull_request) return true;
  const label = (cfg.mirror?.pullLabel || "pull-request").toLowerCase();
  if ((item.labels || []).some((l) => (l.name || "").toLowerCase() === label)) return true;
  const prefix = cfg.mirror?.pullTitlePrefix || "[PR #";
  return typeof item.title === "string" && item.title.startsWith(prefix);
}

function slugify(title) {
  return String(title)
    .replace(/^\[(GH-ISSUE|PR)\s*#\d+\]\s*/i, "") // drop the cross-ref prefix
    .replace(/^\[MERGED\]\s*/i, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 90)
    .replace(/-+$/g, "") || "untitled";
}

function yamlSingleQuote(s) {
  return `'${String(s).replace(/'/g, "''")}'`;
}

function renderMarkdown(item) {
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

function relForLog(absPath) {
  return absPath.startsWith(REPO_ROOT) ? absPath.slice(REPO_ROOT.length + 1) : absPath;
}

async function cmdPull({ dryRun }) {
  const cfg = loadConfig();
  const token = resolveToken(cfg.gitea.url);
  const outDir = resolve(REPO_ROOT, cfg.output.dir);

  process.stdout.write(`Fetching issues from ${cfg.gitea.url} (${cfg.gitea.owner}/${cfg.gitea.repo}) …\n`);
  const items = await apiGetAll(cfg, token);

  // Managed subfolders — wiped and rewritten each pull (Gitea is the source of
  // truth; this keeps deletions/renames from leaving stale files behind).
  const managed = ["open", "closed", "pulls/open", "pulls/closed"];
  const plan = []; // { dir, file, content }
  let nIssue = 0, nPull = 0;
  for (const item of items) {
    const pull = isPull(item, cfg);
    const state = item.state === "closed" ? "closed" : "open";
    const dir = pull ? `pulls/${state}` : state;
    const file = `${item.number}-${slugify(item.title)}.md`;
    plan.push({ dir, file, content: renderMarkdown(item) });
    if (pull) nPull++; else nIssue++;
  }

  if (dryRun) {
    process.stdout.write(`[dry-run] ${items.length} items → ${nIssue} issues + ${nPull} pulls into ${cfg.output.dir}/\n`);
    const sample = plan.slice(0, 3).map((p) => `  ${p.dir}/${p.file}`).join("\n");
    process.stdout.write(`sample:\n${sample}\n`);
    return;
  }

  for (const sub of managed) rmSync(join(outDir, sub), { recursive: true, force: true });
  for (const sub of managed) mkdirSync(join(outDir, sub), { recursive: true });
  for (const { dir, file, content } of plan) {
    writeFileSync(join(outDir, dir, file), content, "utf8");
  }
  // .gitea-sync marker so the folder's provenance is obvious.
  writeFileSync(
    join(outDir, ".gitea-sync.json"),
    JSON.stringify({ source: `${cfg.gitea.owner}/${cfg.gitea.repo}`, url: cfg.gitea.url, issues: nIssue, pulls: nPull }, null, 2) + "\n",
    "utf8",
  );
  process.stdout.write(`Wrote ${nIssue} issues + ${nPull} pulls to ${relForLog(outDir)}/\n`);
}

function fail(msg) {
  process.stderr.write(`tea-issue-sync: ${msg}\n`);
  process.exit(1);
}

const HELP = `tea-issue-sync — mirror Gitea issues to local Markdown (start of a gh-issue-sync counterpart)

Usage:
  tea-issue-sync pull [--dry-run]   Pull all issues + PR-mirror items into the output folder
  tea-issue-sync --help

Config: scripts/tea-issue-sync/config.json   Token: $GITEA_TOKEN or tea's config.yml
Roadmap (TBD): push (local → Gitea), status, diff. See README.md.`;

const [cmd, ...rest] = process.argv.slice(2);
const dryRun = rest.includes("--dry-run");
if (cmd === "pull") {
  cmdPull({ dryRun }).catch((e) => fail(e?.message || String(e)));
} else if (cmd === "--help" || cmd === "-h" || !cmd) {
  process.stdout.write(HELP + "\n");
} else {
  fail(`unknown command '${cmd}' (try --help)`);
}
