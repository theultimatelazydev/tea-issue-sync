package teasync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	reLeadingNum = regexp.MustCompile(`^(\d+)-`)
	reCommentNum = regexp.MustCompile(`^(\d+)`)
)

func commentsFileFor(issueFile string) string {
	return strings.TrimSuffix(issueFile, ".md") + ".comments.md"
}

// isCommentSidecar reports the read-only pulled comment mirror
// (<index>-<slug>.comments.md, plural).
func isCommentSidecar(name string) bool {
	return strings.HasSuffix(name, ".comments.md")
}

// isPendingComment reports a comment authored locally and waiting to be posted
// (<n>.comment.md or <n>-<slug>.comment.md, singular).
func isPendingComment(name string) bool {
	return strings.HasSuffix(name, ".comment.md")
}

// routeFor decides where an item's file goes and what it's called.
func routeFor(it Issue, cfg *Config) (pull bool, dir, file string) {
	pull = IsPull(it, cfg)
	state := "open"
	if it.State == "closed" {
		state = "closed"
	}
	dir = state
	if pull {
		dir = "pulls/" + state
	}
	return pull, dir, fmt.Sprintf("%d-%s.md", it.Number, Slugify(it.Title))
}

// readLocalIssues scans open/ and closed/ for issue files. The leading
// number in the filename is the issue's identity; files without one are new
// issues to create on push. pulls/ is a read-only mirror, never scanned.
func readLocalIssues(outDir string) ([]Local, error) {
	var out []Local
	for _, dir := range []string{"open", "closed"} {
		d := filepath.Join(outDir, dir)
		entries, err := os.ReadDir(d)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			file := e.Name()
			if !strings.HasSuffix(file, ".md") || isCommentSidecar(file) || isPendingComment(file) {
				continue
			}
			content, err := os.ReadFile(filepath.Join(d, file))
			if err != nil {
				return nil, err
			}
			meta, body, err := ParseIssueFile(string(content), dir+"/"+file)
			if err != nil {
				return nil, err
			}
			l := Local{Dir: dir, File: file, Meta: meta, Body: body}
			if m := reLeadingNum.FindStringSubmatch(file); m != nil {
				n, _ := parseInt64(m[1])
				l.Number = n
				l.HasNumber = true
			}
			out = append(out, l)
		}
	}
	return out, nil
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscan(s, &n)
	return n, err
}

func countMirror(outDir string) (issues, pulls int) {
	count := func(subs []string) int {
		total := 0
		for _, s := range subs {
			entries, err := os.ReadDir(filepath.Join(outDir, s))
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if strings.HasSuffix(name, ".md") && !isCommentSidecar(name) && !isPendingComment(name) {
					total++
				}
			}
		}
		return total
	}
	return count([]string{"open", "closed"}), count([]string{"pulls/open", "pulls/closed"})
}

type syncMarker struct {
	Source   string `json:"source"`
	URL      string `json:"url"`
	Issues   int    `json:"issues"`
	Pulls    int    `json:"pulls"`
	SyncedAt string `json:"syncedAt"`
}

// writeSyncMarker records provenance and the high-water timestamp that
// incremental pulls resume from. It lives inside the gitignored output dir,
// so the instance URL never reaches tracked files.
func writeSyncMarker(outDir string, cfg *Config, syncedAt string) error {
	issues, pulls := countMirror(outDir)
	marker := syncMarker{
		Source:   cfg.Gitea.Owner + "/" + cfg.Gitea.Repo,
		URL:      cfg.Gitea.URL,
		Issues:   issues,
		Pulls:    pulls,
		SyncedAt: syncedAt,
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, ".gitea-sync.json"), append(data, '\n'), 0o644)
}

func readSyncMarker(outDir string) (*syncMarker, error) {
	data, err := os.ReadFile(filepath.Join(outDir, ".gitea-sync.json"))
	if err != nil {
		return nil, err
	}
	var m syncMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func relForLog(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return rel
}

// ModItem is a modified local file with its remote counterpart and the
// fields that differ.
type ModItem struct {
	Local  Local
	Remote Issue
	Fields []string
}

// PendingComment is a comment authored locally (a <n>.comment.md file) that
// push will post to issue #Number and then delete.
type PendingComment struct {
	Dir    string
	File   string
	Number int64
	Body   string
}

// readPendingComments collects *.comment.md files from open/ and closed/. The
// leading number in the filename identifies the issue to comment on.
func readPendingComments(outDir string) ([]PendingComment, error) {
	var out []PendingComment
	for _, dir := range []string{"open", "closed"} {
		entries, err := os.ReadDir(filepath.Join(outDir, dir))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if !isPendingComment(name) {
				continue
			}
			m := reCommentNum.FindStringSubmatch(name)
			if m == nil {
				continue
			}
			n, _ := parseInt64(m[1])
			b, err := os.ReadFile(filepath.Join(outDir, dir, name))
			if err != nil {
				return nil, err
			}
			out = append(out, PendingComment{Dir: dir, File: name, Number: n, Body: string(b)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Number != out[j].Number {
			return out[i].Number < out[j].Number
		}
		return out[i].File < out[j].File
	})
	return out, nil
}

// Drift classifies every local file against the remote.
type Drift struct {
	Modified []ModItem
	Created  []Local
	Orphaned []Local
	Missing  []Issue
	Pending  []PendingComment
	Clean    int
}

func (d Drift) any() bool {
	return len(d.Modified)+len(d.Created)+len(d.Orphaned)+len(d.Missing)+len(d.Pending) > 0
}

func computeDrift(env *Env) (Drift, error) {
	if !pathExists(env.OutDir) {
		return Drift{}, fmt.Errorf("no local mirror at %s — run pull first", relForLog(env.OutDir))
	}
	issues, err := env.Client.fetchIssues(env.Cfg)
	if err != nil {
		return Drift{}, err
	}
	remote := make(map[int64]Issue)
	for _, it := range issues {
		if !IsPull(it, env.Cfg) {
			remote[it.Number] = it
		}
	}
	locals, err := readLocalIssues(env.OutDir)
	if err != nil {
		return Drift{}, err
	}
	var drift Drift
	seen := make(map[int64]bool)
	for _, l := range locals {
		if !l.HasNumber {
			drift.Created = append(drift.Created, l)
			continue
		}
		seen[l.Number] = true
		r, ok := remote[l.Number]
		if !ok {
			drift.Orphaned = append(drift.Orphaned, l)
			continue
		}
		if fields := DiffFields(l, r); len(fields) > 0 {
			drift.Modified = append(drift.Modified, ModItem{Local: l, Remote: r, Fields: fields})
		} else {
			drift.Clean++
		}
	}
	for num, r := range remote {
		if !seen[num] {
			drift.Missing = append(drift.Missing, r)
		}
	}
	// Map iteration is unordered; sort for stable output.
	sort.Slice(drift.Missing, func(i, j int) bool { return drift.Missing[i].Number < drift.Missing[j].Number })

	pending, err := readPendingComments(env.OutDir)
	if err != nil {
		return Drift{}, err
	}
	drift.Pending = pending
	return drift, nil
}
