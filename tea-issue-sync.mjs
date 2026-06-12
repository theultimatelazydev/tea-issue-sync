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
//   node tea-issue-sync.mjs pull [--dry-run] [--config <path>]
//   node tea-issue-sync.mjs --help
//
// Config resolution: --config <path> if given, else the nearest config.json
// walking up from the cwd (stopping at the git root). A config.local.json
// next to the chosen config overrides it per top-level section. Paths in the
// config (output.dir) are relative to the config file's directory.
//
// Token resolution (never committed): $GITEA_TOKEN, else the matching login's
// token in tea's config.yml (~/Library/Application Support/tea/config.yml or
// ~/.config/tea/config.yml).

import { readFileSync, rmSync, mkdirSync, writeFileSync, existsSync } from "node:fs";
import { dirname, join, resolve, relative } from "node:path";
import { homedir } from "node:os";

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
  const rel = relative(process.cwd(), absPath);
  return rel && !rel.startsWith("..") ? rel : absPath;
}

async function cmdPull({ dryRun, configPath }) {
  const cfgPath = findConfig(configPath);
  const cfg = loadConfig(cfgPath);
  const token = resolveToken(cfg.gitea.url);
  // output.dir is relative to the config file, so the mirror lands in the
  // same place no matter how deep in the repo the tool is invoked from.
  const outDir = resolve(dirname(cfgPath), cfg.output?.dir || ".issues-tea");

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
  tea-issue-sync pull [--dry-run] [--config <path>]   Pull all issues + PR-mirror items into the output folder
  tea-issue-sync --help

Config: --config <path>, else the nearest config.json from the cwd up to the git root
        (config.local.json next to it overrides per section; output.dir is relative to the config file)
Token:  $GITEA_TOKEN or tea's config.yml
Roadmap (TBD): push (local → Gitea), status, diff. See README.md.`;

const [cmd, ...rest] = process.argv.slice(2);
const dryRun = rest.includes("--dry-run");
const configFlag = rest.indexOf("--config");
const configPath = configFlag !== -1 ? rest[configFlag + 1] : null;
if (configFlag !== -1 && !configPath) fail("--config requires a path");
if (cmd === "pull") {
  cmdPull({ dryRun, configPath }).catch((e) => fail(e?.message || String(e)));
} else if (cmd === "--help" || cmd === "-h" || !cmd) {
  process.stdout.write(HELP + "\n");
} else {
  fail(`unknown command '${cmd}' (try --help)`);
}
