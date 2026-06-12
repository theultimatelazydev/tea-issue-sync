// Tests for tea-issue-sync's pure helpers. Zero dependencies: node:test.
//
//   node --test tea-issue-sync.test.mjs

import { test } from "node:test";
import assert from "node:assert/strict";

process.env.TEA_ISSUE_SYNC_AS_LIB = "1";
const {
  slugify,
  yamlSingleQuote,
  renderMarkdown,
  parseIssueFile,
  isPull,
  diffFields,
  localAsItem,
  unifiedDiff,
  normBody,
  renderComments,
} = await import(new URL("./tea-issue-sync.mjs", import.meta.url).href);

// --- slugify ---------------------------------------------------------------

test("slugify drops cross-ref and MERGED prefixes", () => {
  assert.equal(slugify("[GH-ISSUE #2] B-03 Smart Collections"), "b-03-smart-collections");
  assert.equal(slugify("[PR #5] [MERGED] Fix the thing"), "fix-the-thing");
});

test("slugify collapses non-alphanumerics and trims dashes", () => {
  assert.equal(slugify("  Héllo,   wörld!  "), "h-llo-w-rld");
  assert.equal(slugify("---"), "untitled");
  assert.equal(slugify(""), "untitled");
});

test("slugify caps length at 90 without a trailing dash", () => {
  const slug = slugify("x".repeat(50) + " " + "y".repeat(50));
  assert.ok(slug.length <= 90);
  assert.ok(!slug.endsWith("-"));
});

// --- YAML helpers ----------------------------------------------------------

test("yamlSingleQuote escapes single quotes", () => {
  assert.equal(yamlSingleQuote("it's"), "'it''s'");
  assert.equal(yamlSingleQuote("plain"), "'plain'");
});

// --- render / parse round-trip ----------------------------------------------

test("renderMarkdown ↔ parseIssueFile round-trips an issue", () => {
  const item = {
    title: "It's a [test] title",
    labels: [{ name: "alpha" }, { name: "p2" }],
    state: "closed",
    state_reason: "completed",
    body: "Line one.\r\n\r\nLine two with trailing spaces.   \n\n",
  };
  const { meta, body } = parseIssueFile(renderMarkdown(item), "test");
  assert.equal(meta.title, item.title);
  assert.deepEqual(meta.labels, ["alpha", "p2"]);
  assert.equal(meta.state, "closed");
  assert.equal(meta.state_reason, "completed");
  assert.equal(normBody(body), "Line one.\n\nLine two with trailing spaces.");
});

test("renderMarkdown writes labels: [] for unlabelled issues", () => {
  const md = renderMarkdown({ title: "t", labels: [], state: "open", body: "" });
  assert.match(md, /labels: \[\]/);
  const { meta } = parseIssueFile(md, "test");
  assert.deepEqual(meta.labels, []);
});

test("parseIssueFile tolerates hand-edited variants", () => {
  const md = '---\ntitle: unquoted plain title\nlabels: []\nstate: open\nstate_reason: null\n---\nNo separator blank line.\n';
  const { meta, body } = parseIssueFile(md, "test");
  assert.equal(meta.title, "unquoted plain title");
  assert.equal(meta.state, "open");
  assert.equal(meta.state_reason, null);
  assert.equal(normBody(body), "No separator blank line.");
});

test("parseIssueFile throws on missing frontmatter or title", () => {
  assert.throws(() => parseIssueFile("just a body", "x"), /missing frontmatter/);
  assert.throws(() => parseIssueFile("---\nlabels: []\n---\nbody\n", "x"), /missing title/);
});

test("parseIssueFile keeps intentional extra leading blank lines", () => {
  // one blank line is the renderer's separator; a second one is content
  const md = "---\ntitle: 't'\nlabels: []\nstate: open\nstate_reason: null\n---\n\n\nbody\n";
  const { body } = parseIssueFile(md, "test");
  assert.equal(body, "\nbody\n");
});

// --- PR routing --------------------------------------------------------------

test("isPull detects the three routing signals", () => {
  const cfg = { mirror: {} };
  assert.ok(isPull({ pull_request: { merged: false } }, cfg));
  assert.ok(isPull({ labels: [{ name: "Pull-Request" }] }, cfg));
  assert.ok(isPull({ title: "[PR #7] something" }, cfg));
  assert.ok(!isPull({ title: "ordinary issue", labels: [{ name: "bug" }] }, cfg));
});

test("isPull honours configured label and prefix", () => {
  const cfg = { mirror: { pullLabel: "mirrored-pr", pullTitlePrefix: "<<PR " } };
  assert.ok(isPull({ labels: [{ name: "mirrored-pr" }] }, cfg));
  assert.ok(isPull({ title: "<<PR 9>> thing" }, cfg));
  assert.ok(!isPull({ title: "[PR #7] not the configured prefix" }, cfg));
});

// --- drift detection ---------------------------------------------------------

function localFixture(overrides = {}) {
  return {
    number: 1,
    dir: "open",
    file: "1-t.md",
    meta: { title: "t", labels: ["a", "b"], state: "open", state_reason: null, ...overrides.meta },
    body: overrides.body ?? "body\n",
  };
}

const remoteFixture = {
  number: 1,
  title: "t",
  labels: [{ name: "b" }, { name: "a" }],
  state: "open",
  state_reason: null,
  body: "body",
};

test("diffFields: clean when equal (label order ignored)", () => {
  assert.deepEqual(diffFields(localFixture(), remoteFixture), []);
});

test("diffFields flags each changed field", () => {
  assert.deepEqual(diffFields(localFixture({ meta: { title: "T2" } }), remoteFixture), ["title"]);
  assert.deepEqual(diffFields(localFixture({ meta: { labels: ["a"] } }), remoteFixture), ["labels"]);
  assert.deepEqual(diffFields(localFixture({ meta: { state: "closed" } }), remoteFixture), ["state"]);
  assert.deepEqual(diffFields(localFixture({ body: "other\n" }), remoteFixture), ["body"]);
});

test("localAsItem renders identically to a same-order remote item", () => {
  // renderMarkdown preserves label order; only drift detection is order-insensitive
  const sameOrder = { ...remoteFixture, labels: [{ name: "a" }, { name: "b" }] };
  assert.equal(renderMarkdown(localAsItem(localFixture())), renderMarkdown(sameOrder));
});

// --- unified diff -------------------------------------------------------------

test("unifiedDiff returns null for identical texts", () => {
  assert.equal(unifiedDiff("a\nb\n", "a\nb\n", "x", "y"), null);
});

test("unifiedDiff produces hunks with correct headers", () => {
  const a = ["1", "2", "3", "4", "5", "6", "7", "8"].join("\n");
  const b = ["1", "2", "3", "four", "5", "6", "7", "8"].join("\n");
  const d = unifiedDiff(a, b, "a", "b");
  assert.match(d, /^--- a\n\+\+\+ b\n@@ -1,7 \+1,7 @@\n/);
  assert.match(d, /\n-4\n\+four\n/);
});

test("unifiedDiff handles creation from empty", () => {
  const d = unifiedDiff("", "new\n", "none", "created");
  assert.match(d, /\+new/);
});

// --- comments sidecar ----------------------------------------------------------

test("renderComments formats one section per comment", () => {
  const out = renderComments(
    { number: 12, title: "T" },
    [
      { user: { login: "rene" }, created_at: "2026-01-02T03:04:05Z", body: "first\r\n" },
      { user: {}, created_at: "2026-01-03T00:00:00Z", body: "second" },
    ],
  );
  assert.match(out, /^# Comments — #12 T\n/);
  assert.match(out, /## @rene — 2026-01-02T03:04:05Z\n\nfirst\n/);
  assert.match(out, /## @unknown — 2026-01-03T00:00:00Z\n\nsecond\n$/);
});
