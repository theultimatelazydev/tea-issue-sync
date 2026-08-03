// Package skill embeds the agent SKILL.md so the binary can install it into
// an agent's skills directory (e.g. ~/.claude/skills/tea-issue-sync/).
package skill

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed SKILL.md
var Content string

// Install writes the embedded SKILL.md into <baseDir>/tea-issue-sync/SKILL.md
// and returns the written path.
func Install(baseDir string) (string, error) {
	dir := filepath.Join(baseDir, "tea-issue-sync")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(Content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
