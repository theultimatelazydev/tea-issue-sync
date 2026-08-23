package teasync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tmpEnv(t *testing.T) *Env {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"open", "closed"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &Env{Cfg: &Config{}, OutDir: dir}
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewCreatesNumberlessFile(t *testing.T) {
	env := tmpEnv(t)
	if err := New(env, NewOpts{Title: "Fix the login bug", Labels: []string{"bug"}, Body: "Steps."}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(env.OutDir, "open", "fix-the-login-bug.md")
	content := read(t, p)
	meta, body, err := ParseIssueFile(content, "x")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Fix the login bug" || meta.State != "open" {
		t.Errorf("meta %+v", meta)
	}
	if len(meta.Labels) != 1 || meta.Labels[0] != "bug" {
		t.Errorf("labels %v", meta.Labels)
	}
	if NormBody(body) != "Steps." {
		t.Errorf("body %q", NormBody(body))
	}
	// readLocalIssues should treat it as a new (number-less) issue.
	locals, _ := readLocalIssues(env.OutDir)
	if len(locals) != 1 || locals[0].HasNumber {
		t.Errorf("expected one number-less local, got %+v", locals)
	}
}

func TestNewAvoidsDigitLeadingSlug(t *testing.T) {
	env := tmpEnv(t)
	if err := New(env, NewOpts{Title: "2026 roadmap"}); err != nil {
		t.Fatal(err)
	}
	// "2026-roadmap" would be misread as issue #2026, so it gets a new- prefix.
	if !pathExists(filepath.Join(env.OutDir, "open", "new-2026-roadmap.md")) {
		entries, _ := os.ReadDir(filepath.Join(env.OutDir, "open"))
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected new-2026-roadmap.md, found %v", names)
	}
}

func TestNewCollisionSuffix(t *testing.T) {
	env := tmpEnv(t)
	for _, want := range []string{"dup.md", "dup-2.md", "dup-3.md"} {
		if err := New(env, NewOpts{Title: "dup"}); err != nil {
			t.Fatal(err)
		}
		if !pathExists(filepath.Join(env.OutDir, "open", want)) {
			t.Errorf("expected %s", want)
		}
	}
}

func TestCloseAndReopenMoveAndSetState(t *testing.T) {
	env := tmpEnv(t)
	// Seed an open issue #42.
	seed := RenderMarkdown(Issue{Number: 42, Title: "Something", State: "open"})
	if err := os.WriteFile(filepath.Join(env.OutDir, "open", "42-something.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Close(env, 42, "completed"); err != nil {
		t.Fatal(err)
	}
	if pathExists(filepath.Join(env.OutDir, "open", "42-something.md")) {
		t.Error("file still in open/ after close")
	}
	meta, _, err := ParseIssueFile(read(t, env.OutDir, "closed", "42-something.md"), "x")
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != "closed" || meta.StateReason == nil || *meta.StateReason != "completed" {
		t.Errorf("meta %+v reason %v", meta, meta.StateReason)
	}

	if err := Reopen(env, 42); err != nil {
		t.Fatal(err)
	}
	if pathExists(filepath.Join(env.OutDir, "closed", "42-something.md")) {
		t.Error("file still in closed/ after reopen")
	}
	meta, _, _ = ParseIssueFile(read(t, env.OutDir, "open", "42-something.md"), "x")
	if meta.State != "open" || meta.StateReason != nil {
		t.Errorf("meta %+v", meta)
	}
}

func TestCloseUnknownIssue(t *testing.T) {
	env := tmpEnv(t)
	if err := Close(env, 999, ""); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err %v", err)
	}
}

func TestDraftCommentNextToIssue(t *testing.T) {
	env := tmpEnv(t)
	// seed issue #42
	seed := RenderMarkdown(Issue{Number: 42, Title: "Some Thing", State: "open"})
	if err := os.WriteFile(filepath.Join(env.OutDir, "open", "42-some-thing.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DraftComment(env, 42, "Looks good to me.", false); err != nil {
		t.Fatal(err)
	}
	// written next to the issue file, singular .comment.md
	if !pathExists(filepath.Join(env.OutDir, "open", "42-some-thing.comment.md")) {
		t.Fatal("expected 42-some-thing.comment.md")
	}
	// readLocalIssues must NOT treat the pending comment as an issue
	locals, err := readLocalIssues(env.OutDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(locals) != 1 || locals[0].File != "42-some-thing.md" {
		t.Errorf("readLocalIssues picked up the comment file: %+v", locals)
	}
	// readPendingComments must find it, with the right number and body
	pend, err := readPendingComments(env.OutDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].Number != 42 || NormBody(pend[0].Body) != "Looks good to me." {
		t.Errorf("pending = %+v", pend)
	}
}

func TestDraftCommentWithoutMirroredIssue(t *testing.T) {
	env := tmpEnv(t)
	if err := DraftComment(env, 7, "note", false); err != nil {
		t.Fatal(err)
	}
	// falls back to open/<n>.comment.md
	if !pathExists(filepath.Join(env.OutDir, "open", "7.comment.md")) {
		t.Fatal("expected open/7.comment.md")
	}
	pend, _ := readPendingComments(env.OutDir)
	if len(pend) != 1 || pend[0].Number != 7 {
		t.Errorf("pending = %+v", pend)
	}
}

func TestReadPendingCommentsSkipsSidecar(t *testing.T) {
	env := tmpEnv(t)
	// a plural read-only sidecar must be ignored by pending scan
	if err := os.WriteFile(filepath.Join(env.OutDir, "open", "5-x.comments.md"), []byte("# Comments\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pend, err := readPendingComments(env.OutDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 0 {
		t.Errorf("sidecar treated as pending: %+v", pend)
	}
}

func TestWipeRemoteMirrorPreservesLocalOnly(t *testing.T) {
	env := tmpEnv(t)
	write := func(sub, name string) {
		if err := os.WriteFile(filepath.Join(env.OutDir, sub, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// remote-sourced (must be wiped)
	write("open", "42-foo.md")          // issue mirror
	write("open", "42-foo.comments.md") // read-only sidecar
	write("closed", "9-bar.md")         // issue mirror
	// local-only (must be preserved)
	write("open", "my-idea.md")        // number-less draft
	write("open", "Ta1f0-phase0.md")   // T-prefixed draft
	write("open", "42-foo.comment.md") // pending comment (numbered)
	write("open", "7.comment.md")      // pending comment (no dash)

	kept, err := wipeRemoteMirror(env.OutDir)
	if err != nil {
		t.Fatal(err)
	}
	gone := []string{"open/42-foo.md", "open/42-foo.comments.md", "closed/9-bar.md"}
	for _, g := range gone {
		if pathExists(filepath.Join(env.OutDir, g)) {
			t.Errorf("expected %s wiped", g)
		}
	}
	stay := []string{"open/my-idea.md", "open/Ta1f0-phase0.md", "open/42-foo.comment.md", "open/7.comment.md"}
	for _, s := range stay {
		if !pathExists(filepath.Join(env.OutDir, s)) {
			t.Errorf("expected %s preserved", s)
		}
	}
	if len(kept.Drafts) != 2 {
		t.Errorf("drafts kept = %v", kept.Drafts)
	}
	if len(kept.Comments) != 2 {
		t.Errorf("comments kept = %v", kept.Comments)
	}
}
