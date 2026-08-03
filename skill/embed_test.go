package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentHasFrontmatter(t *testing.T) {
	if !strings.HasPrefix(Content, "---\n") {
		t.Fatal("SKILL.md should start with YAML frontmatter")
	}
	if !strings.Contains(Content, "name: tea-issue-sync") {
		t.Error("missing name in frontmatter")
	}
	if !strings.Contains(Content, "description:") {
		t.Error("missing description in frontmatter")
	}
}

func TestInstallWritesSkill(t *testing.T) {
	base := t.TempDir()
	path, err := Install(base)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "tea-issue-sync", "SKILL.md")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != Content {
		t.Error("installed content differs from embedded content")
	}
}
