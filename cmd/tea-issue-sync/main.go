// Command tea-issue-sync mirrors Gitea issues to a local folder of Markdown
// files and pushes local edits back. See ROADMAP.md and the README.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	tea "github.com/theultimatelazydev/tea-issue-sync/internal/teasync"
	"github.com/theultimatelazydev/tea-issue-sync/skill"
)

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "tea-issue-sync: %s\n", msg)
	os.Exit(1)
}

// flagValue returns the argument after index i, or fatals if it's missing.
func flagValue(args []string, i int, flag string) string {
	if i+1 >= len(args) {
		fatal(flag + " requires a value")
	}
	return args[i+1]
}

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	rest := args
	if len(args) > 0 {
		rest = args[1:]
	}

	// --config is global to every command.
	configPath := ""
	for i, a := range rest {
		if a == "--config" {
			configPath = flagValue(rest, i, "--config")
		}
	}

	switch cmd {
	case "pull", "status", "diff", "push":
		dryRun, incremental := false, false
		for _, a := range rest {
			switch a {
			case "--dry-run":
				dryRun = true
			case "--incremental":
				incremental = true
			}
		}
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
		exitOn(err)

	case "new":
		opts := tea.NewOpts{}
		for i := 0; i < len(rest); i++ {
			switch a := rest[i]; a {
			case "--label":
				opts.Labels = append(opts.Labels, flagValue(rest, i, a))
				i++
			case "--body":
				opts.Body = flagValue(rest, i, a)
				i++
			case "--state":
				opts.State = flagValue(rest, i, a)
				i++
			case "--config":
				i++ // value handled globally
			case "--edit":
				opts.Edit = true
			default:
				if len(a) > 0 && a[0] != '-' && opts.Title == "" {
					opts.Title = a
				}
			}
		}
		env, err := tea.LoadLocal(configPath)
		if err != nil {
			fatal(err.Error())
		}
		exitOn(tea.New(env, opts))

	case "close", "reopen":
		number, reason := int64(0), ""
		for i := 0; i < len(rest); i++ {
			switch a := rest[i]; a {
			case "--reason":
				reason = flagValue(rest, i, a)
				i++
			case "--config":
				i++
			default:
				if len(a) > 0 && a[0] != '-' && number == 0 {
					n, err := strconv.ParseInt(a, 10, 64)
					if err != nil {
						fatal(fmt.Sprintf("%s: expected an issue number, got %q", cmd, a))
					}
					number = n
				}
			}
		}
		if number == 0 {
			fatal(cmd + " requires an issue number (e.g. tea-issue-sync " + cmd + " 42)")
		}
		env, err := tea.LoadLocal(configPath)
		if err != nil {
			fatal(err.Error())
		}
		if cmd == "close" {
			exitOn(tea.Close(env, number, reason))
		} else {
			exitOn(tea.Reopen(env, number))
		}

	case "comment":
		number, body := int64(0), ""
		edit := false
		for i := 0; i < len(rest); i++ {
			switch a := rest[i]; a {
			case "--body":
				body = flagValue(rest, i, a)
				i++
			case "--edit":
				edit = true
			case "--config":
				i++
			default:
				if len(a) > 0 && a[0] != '-' && number == 0 {
					n, err := strconv.ParseInt(a, 10, 64)
					if err != nil {
						fatal(fmt.Sprintf("comment: expected an issue number, got %q", a))
					}
					number = n
				}
			}
		}
		if number == 0 {
			fatal("comment requires an issue number (e.g. tea-issue-sync comment 42 --body \"...\")")
		}
		env, err := tea.LoadLocal(configPath)
		if err != nil {
			fatal(err.Error())
		}
		exitOn(tea.DraftComment(env, number, body, edit))

	case "list":
		opts := tea.ListOpts{}
		for i := 0; i < len(rest); i++ {
			switch a := rest[i]; a {
			case "--state":
				opts.State = flagValue(rest, i, a)
				i++
			case "--label":
				opts.Label = flagValue(rest, i, a)
				i++
			case "--search":
				opts.Search = flagValue(rest, i, a)
				i++
			case "--config":
				i++
			}
		}
		env, err := tea.LoadLocal(configPath)
		if err != nil {
			fatal(err.Error())
		}
		exitOn(tea.List(env, opts))

	case "skill":
		sub := ""
		if len(rest) > 0 {
			sub = rest[0]
		}
		switch sub {
		case "print":
			fmt.Print(skill.Content)
		case "install", "":
			project := false
			dir := ""
			for i := 0; i < len(rest); i++ {
				switch rest[i] {
				case "--project":
					project = true
				case "--dir":
					dir = flagValue(rest, i, "--dir")
					i++
				}
			}
			base := dir
			if base == "" {
				if project {
					base = filepath.Join(".claude", "skills")
				} else {
					home, err := os.UserHomeDir()
					if err != nil {
						fatal(err.Error())
					}
					base = filepath.Join(home, ".claude", "skills")
				}
			}
			path, err := skill.Install(base)
			if err != nil {
				fatal(err.Error())
			}
			fmt.Printf("installed skill → %s\n", path)
		default:
			fatal("skill: unknown subcommand '" + sub + "' (use: install [--project] [--dir <path>] | print)")
		}

	case "version", "--version", "-v", "--v":
		fmt.Printf("tea-issue-sync %s\n", tea.Version)
	case "--help", "-h", "":
		fmt.Println(tea.Help())
	default:
		fatal(fmt.Sprintf("unknown command '%s' (try --help)", cmd))
	}
}

func exitOn(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, tea.ErrDrift) {
		os.Exit(1)
	}
	fatal(err.Error())
}
