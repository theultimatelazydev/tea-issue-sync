// Command tea-issue-sync mirrors Gitea issues to a local folder of Markdown
// files and pushes local edits back. See ROADMAP.md and the README.
package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/theultimatelazydev/tea-issue-sync/internal/teasync"
)

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "tea-issue-sync: %s\n", msg)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	dryRun := false
	incremental := false
	configPath := ""
	for i, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--incremental":
			incremental = true
		case "--config":
			if i+1 >= len(args) {
				fatal("--config requires a path")
			}
			configPath = args[i+1]
		}
	}

	switch cmd {
	case "pull", "status", "diff", "push":
		env, err := tea.Load(configPath)
		if err != nil {
			fatal(err.Error())
		}
		switch cmd {
		case "pull":
			err = tea.Pull(env, tea.PullOpts{DryRun: dryRun, Incremental: incremental})
		case "status":
			err = tea.Status(env)
		case "diff":
			err = tea.Diff(env)
		case "push":
			err = tea.Push(env, tea.PushOpts{DryRun: dryRun})
		}
		if err != nil {
			if errors.Is(err, tea.ErrDrift) {
				os.Exit(1)
			}
			fatal(err.Error())
		}
	case "--version", "-v":
		fmt.Printf("tea-issue-sync %s\n", tea.Version)
	case "--help", "-h", "":
		fmt.Println(tea.Help())
	default:
		fatal(fmt.Sprintf("unknown command '%s' (try --help)", cmd))
	}
}
