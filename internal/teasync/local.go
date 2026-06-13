package teasync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// These commands operate only on the local mirror — no network, no token.
// They are convenience wrappers over the same "edit the Markdown, then push"
// model: new writes a number-less file, close/reopen flip frontmatter state
// and move the file, list reads the mirror. `push` remains the only command
// that writes to Gitea.

// NewOpts configures the `new` command.
type NewOpts struct {
	Title  string
	Labels []string
	Body   string
	State  string // "open" (default) or "closed"
	Edit   bool   // open $EDITOR on the created file
}

// New creates a number-less local issue file that `push` will create on Gitea
// (and rename to its canonical <index>-<slug>.md).
func New(env *Env, opts NewOpts) error {
	if strings.TrimSpace(opts.Title) == "" {
		return errors.New("new: a title is required (tea-issue-sync new \"Title\")")
	}
	state := "open"
	if opts.State == "closed" {
		state = "closed"
	}
	it := Issue{Title: opts.Title, State: state, Body: opts.Body}
	for _, name := range opts.Labels {
		it.Labels = append(it.Labels, Label{Name: name})
	}

	base := Slugify(opts.Title)
	// A leading-number slug (e.g. "2026-roadmap") would be misread as an
	// existing issue #2026, so guard against it.
	if reLeadingNum.MatchString(base) {
		base = "new-" + base
	}
	dirAbs := filepath.Join(env.OutDir, state)
	if err := os.MkdirAll(dirAbs, 0o755); err != nil {
		return err
	}
	name := base + ".md"
	for i := 2; pathExists(filepath.Join(dirAbs, name)); i++ {
		name = fmt.Sprintf("%s-%d.md", base, i)
	}
	full := filepath.Join(dirAbs, name)
	if err := os.WriteFile(full, []byte(RenderMarkdown(it)), 0o644); err != nil {
		return err
	}
	fmt.Printf("created %s/%s (local — run push to create it on Gitea)\n", state, name)
	if opts.Edit {
		return runEditor(full)
	}
	return nil
}

// Close marks issue #number closed locally (optionally with a reason) and
// moves its file to closed/. Run push to sync.
func Close(env *Env, number int64, reason string) error {
	return setState(env, number, "closed", reason)
}

// Reopen marks issue #number open locally and moves its file to open/.
func Reopen(env *Env, number int64) error {
	return setState(env, number, "open", "")
}

func setState(env *Env, number int64, state, reason string) error {
	dir, file, ok := findIssueFile(env.OutDir, number)
	if !ok {
		return fmt.Errorf("issue #%d not found in the local mirror — run pull first", number)
	}
	content, err := os.ReadFile(filepath.Join(env.OutDir, dir, file))
	if err != nil {
		return err
	}
	meta, body, err := ParseIssueFile(string(content), dir+"/"+file)
	if err != nil {
		return err
	}
	meta.State = state
	if state == "closed" {
		if reason != "" {
			meta.StateReason = &reason
		}
	} else {
		meta.StateReason = nil
	}
	rendered := RenderMarkdown(LocalAsItem(Local{Meta: meta, Body: body}))

	if dir != state {
		if err := os.MkdirAll(filepath.Join(env.OutDir, state), 0o755); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(env.OutDir, dir, file)); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(env.OutDir, state, file), []byte(rendered), 0o644); err != nil {
		return err
	}
	verb := "closed"
	if state == "open" {
		verb = "reopened"
	}
	fmt.Printf("%s #%d → %s/%s (local — run push to sync)\n", verb, number, state, file)
	return nil
}

// ListOpts configures the `list` command.
type ListOpts struct {
	State  string // open | closed | all (default all)
	Label  string
	Search string
}

// List prints issues from the local mirror, optionally filtered.
func List(env *Env, opts ListOpts) error {
	if !pathExists(env.OutDir) {
		return fmt.Errorf("no local mirror at %s — run pull first", relForLog(env.OutDir))
	}
	locals, err := readLocalIssues(env.OutDir)
	if err != nil {
		return err
	}
	state := opts.State
	if state == "" {
		state = "all"
	}
	search := strings.ToLower(opts.Search)
	shown := 0
	for _, l := range locals {
		if state != "all" && l.Meta.State != state {
			continue
		}
		if opts.Label != "" && !containsFold(l.Meta.Labels, opts.Label) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(l.Meta.Title+"\n"+l.Body), search) {
			continue
		}
		id := "#" + strconv.FormatInt(l.Number, 10)
		if !l.HasNumber {
			id = "#new"
		}
		labels := ""
		if len(l.Meta.Labels) > 0 {
			labels = "  [" + strings.Join(l.Meta.Labels, ", ") + "]"
		}
		fmt.Printf("%-7s %-6s %s%s\n", id, l.Meta.State, l.Meta.Title, labels)
		shown++
	}
	if shown == 0 {
		fmt.Println("no issues match")
	}
	return nil
}

// findIssueFile locates the file for issue #number in open/ or closed/.
func findIssueFile(outDir string, number int64) (dir, file string, ok bool) {
	prefix := strconv.FormatInt(number, 10) + "-"
	for _, d := range []string{"open", "closed"} {
		entries, err := os.ReadDir(filepath.Join(outDir, d))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".comments.md") {
				return d, name, true
			}
		}
	}
	return "", "", false
}

func containsFold(items []string, want string) bool {
	for _, s := range items {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func runEditor(path string) error {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		fmt.Printf("(set $EDITOR or $VISUAL to edit; file at %s)\n", path)
		return nil
	}
	cmd := exec.Command(ed, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
