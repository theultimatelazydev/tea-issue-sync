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
