package teasync

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

// --- slugify ---------------------------------------------------------------

func TestSlugifyDropsPrefixes(t *testing.T) {
	if got := Slugify("[GH-ISSUE #2] B-03 Smart Collections"); got != "b-03-smart-collections" {
		t.Errorf("got %q", got)
	}
	if got := Slugify("[PR #5] [MERGED] Fix the thing"); got != "fix-the-thing" {
		t.Errorf("got %q", got)
	}
}

func TestSlugifyCollapsesAndTrims(t *testing.T) {
	if got := Slugify("  Héllo,   wörld!  "); got != "h-llo-w-rld" {
		t.Errorf("got %q", got)
	}
	if got := Slugify("---"); got != "untitled" {
		t.Errorf("got %q", got)
	}
	if got := Slugify(""); got != "untitled" {
		t.Errorf("got %q", got)
	}
}

func TestSlugifyCapsLength(t *testing.T) {
	slug := Slugify(strings.Repeat("x", 50) + " " + strings.Repeat("y", 50))
	if len(slug) > 90 {
		t.Errorf("len %d > 90", len(slug))
	}
	if strings.HasSuffix(slug, "-") {
		t.Errorf("trailing dash: %q", slug)
	}
}

// --- YAML helpers ----------------------------------------------------------

func TestYAMLSingleQuote(t *testing.T) {
	if got := YAMLSingleQuote("it's"); got != "'it''s'" {
		t.Errorf("got %q", got)
	}
	if got := YAMLSingleQuote("plain"); got != "'plain'" {
		t.Errorf("got %q", got)
	}
}

// --- render / parse round-trip ----------------------------------------------

func TestRenderParseRoundTrip(t *testing.T) {
	item := Issue{
		Title:       "It's a [test] title",
		Labels:      []Label{{Name: "alpha"}, {Name: "p2"}},
		State:       "closed",
		StateReason: ptr("completed"),
		Body:        "Line one.\r\n\r\nLine two with trailing spaces.   \n\n",
	}
	meta, body, err := ParseIssueFile(RenderMarkdown(item), "test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != item.Title {
		t.Errorf("title %q", meta.Title)
	}
	if !reflect.DeepEqual(meta.Labels, []string{"alpha", "p2"}) {
		t.Errorf("labels %v", meta.Labels)
	}
	if meta.State != "closed" {
		t.Errorf("state %q", meta.State)
	}
	if meta.StateReason == nil || *meta.StateReason != "completed" {
		t.Errorf("state_reason %v", meta.StateReason)
	}
	if NormBody(body) != "Line one.\n\nLine two with trailing spaces." {
		t.Errorf("body %q", NormBody(body))
	}
}

func TestRenderEmptyLabels(t *testing.T) {
	md := RenderMarkdown(Issue{Title: "t", State: "open"})
	if !strings.Contains(md, "labels: []") {
		t.Errorf("expected labels: [] in %q", md)
	}
	meta, _, err := ParseIssueFile(md, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Labels) != 0 {
		t.Errorf("labels %v", meta.Labels)
	}
}

func TestParseToleratesHandEdits(t *testing.T) {
	md := "---\ntitle: unquoted plain title\nlabels: []\nstate: open\nstate_reason: null\n---\nNo separator blank line.\n"
	meta, body, err := ParseIssueFile(md, "test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "unquoted plain title" {
		t.Errorf("title %q", meta.Title)
	}
	if meta.State != "open" {
		t.Errorf("state %q", meta.State)
	}
	if meta.StateReason != nil {
		t.Errorf("state_reason %v", meta.StateReason)
	}
	if NormBody(body) != "No separator blank line." {
		t.Errorf("body %q", NormBody(body))
	}
}

func TestParseThrowsOnMissing(t *testing.T) {
	if _, _, err := ParseIssueFile("just a body", "x"); err == nil || !strings.Contains(err.Error(), "missing frontmatter") {
		t.Errorf("err %v", err)
	}
	if _, _, err := ParseIssueFile("---\nlabels: []\n---\nbody\n", "x"); err == nil || !strings.Contains(err.Error(), "missing title") {
		t.Errorf("err %v", err)
	}
}

func TestParseKeepsExtraLeadingBlankLines(t *testing.T) {
	md := "---\ntitle: 't'\nlabels: []\nstate: open\nstate_reason: null\n---\n\n\nbody\n"
	_, body, err := ParseIssueFile(md, "test")
	if err != nil {
		t.Fatal(err)
	}
	if body != "\nbody\n" {
		t.Errorf("body %q", body)
	}
}

// --- PR routing --------------------------------------------------------------

func TestIsPullSignals(t *testing.T) {
	cfg := &Config{}
	if !IsPull(Issue{PullRequest: json.RawMessage(`{"merged":false}`)}, cfg) {
		t.Error("pull_request not detected")
	}
	if !IsPull(Issue{Labels: []Label{{Name: "Pull-Request"}}}, cfg) {
		t.Error("label not detected")
	}
	if !IsPull(Issue{Title: "[PR #7] something"}, cfg) {
		t.Error("prefix not detected")
	}
	if IsPull(Issue{Title: "ordinary issue", Labels: []Label{{Name: "bug"}}}, cfg) {
		t.Error("false positive")
	}
	if IsPull(Issue{PullRequest: json.RawMessage(`null`)}, cfg) {
		t.Error("null pull_request treated as pull")
	}
}

func TestIsPullHonoursConfig(t *testing.T) {
	cfg := &Config{}
	cfg.Mirror.PullLabel = "mirrored-pr"
	cfg.Mirror.PullTitlePrefix = "<<PR "
	if !IsPull(Issue{Labels: []Label{{Name: "mirrored-pr"}}}, cfg) {
		t.Error("configured label not detected")
	}
	if !IsPull(Issue{Title: "<<PR 9>> thing"}, cfg) {
		t.Error("configured prefix not detected")
	}
	if IsPull(Issue{Title: "[PR #7] not the configured prefix"}, cfg) {
		t.Error("default prefix should not match")
	}
}

// --- drift detection ---------------------------------------------------------

func localFixture() Local {
	return Local{Number: 1, HasNumber: true, Dir: "open", File: "1-t.md",
		Meta: Meta{Title: "t", Labels: []string{"a", "b"}, State: "open"}, Body: "body\n"}
}

func remoteFixture() Issue {
	return Issue{Number: 1, Title: "t", Labels: []Label{{Name: "b"}, {Name: "a"}}, State: "open", Body: "body"}
}

func TestDiffFieldsClean(t *testing.T) {
	if got := DiffFields(localFixture(), remoteFixture()); len(got) != 0 {
		t.Errorf("expected clean, got %v", got)
	}
}

func TestDiffFieldsFlagsChanges(t *testing.T) {
	l := localFixture()
	l.Meta.Title = "T2"
	if got := DiffFields(l, remoteFixture()); !reflect.DeepEqual(got, []string{"title"}) {
		t.Errorf("got %v", got)
	}
	l = localFixture()
	l.Meta.Labels = []string{"a"}
	if got := DiffFields(l, remoteFixture()); !reflect.DeepEqual(got, []string{"labels"}) {
		t.Errorf("got %v", got)
	}
	l = localFixture()
	l.Meta.State = "closed"
	if got := DiffFields(l, remoteFixture()); !reflect.DeepEqual(got, []string{"state"}) {
		t.Errorf("got %v", got)
	}
	l = localFixture()
	l.Body = "other\n"
	if got := DiffFields(l, remoteFixture()); !reflect.DeepEqual(got, []string{"body"}) {
		t.Errorf("got %v", got)
	}
}

func TestLocalAsItemRendersIdentically(t *testing.T) {
	// RenderMarkdown preserves label order; only drift detection is order-insensitive.
	sameOrder := remoteFixture()
	sameOrder.Labels = []Label{{Name: "a"}, {Name: "b"}}
	if RenderMarkdown(LocalAsItem(localFixture())) != RenderMarkdown(sameOrder) {
		t.Error("renderings differ")
	}
}

// --- unified diff -------------------------------------------------------------

func TestUnifiedDiffIdentical(t *testing.T) {
	if got := UnifiedDiff("a\nb\n", "a\nb\n", "x", "y"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestUnifiedDiffHeaders(t *testing.T) {
	a := strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8"}, "\n")
	b := strings.Join([]string{"1", "2", "3", "four", "5", "6", "7", "8"}, "\n")
	d := UnifiedDiff(a, b, "a", "b")
	if !strings.HasPrefix(d, "--- a\n+++ b\n@@ -1,7 +1,7 @@\n") {
		t.Errorf("headers wrong:\n%s", d)
	}
	if !strings.Contains(d, "\n-4\n+four\n") {
		t.Errorf("change wrong:\n%s", d)
	}
}

func TestUnifiedDiffCreation(t *testing.T) {
	d := UnifiedDiff("", "new\n", "none", "created")
	if !strings.Contains(d, "+new") {
		t.Errorf("got %q", d)
	}
}

// --- comments sidecar ----------------------------------------------------------

func TestRenderComments(t *testing.T) {
	var c1, c2 Comment
	c1.User.Login = "rene"
	c1.CreatedAt = "2026-01-02T03:04:05Z"
	c1.Body = "first\r\n"
	c2.CreatedAt = "2026-01-03T00:00:00Z"
	c2.Body = "second"
	out := RenderComments(Issue{Number: 12, Title: "T"}, []Comment{c1, c2})
	if !strings.HasPrefix(out, "# Comments — #12 T\n") {
		t.Errorf("header wrong:\n%s", out)
	}
	if !strings.Contains(out, "## @rene — 2026-01-02T03:04:05Z\n\nfirst\n") {
		t.Errorf("c1 wrong:\n%s", out)
	}
	if !strings.HasSuffix(out, "## @unknown — 2026-01-03T00:00:00Z\n\nsecond\n") {
		t.Errorf("c2 wrong:\n%s", out)
	}
}
