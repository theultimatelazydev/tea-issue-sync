package teasync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrDrift is returned by Status and Diff when the mirror differs from the
// remote, so the CLI can exit non-zero without printing an error.
var ErrDrift = errors.New("drift")

// PullOpts configures a pull.
type PullOpts struct {
	DryRun      bool
	Incremental bool
}

// PushOpts configures a push.
type PushOpts struct {
	DryRun bool
}

// Pull mirrors Gitea into the output folder. A full pull wipes and rewrites
// the managed subfolders; an incremental pull only rewrites issues updated
// since the last sync.
func Pull(env *Env, opts PullOpts) error {
	if opts.Incremental {
		return pullIncremental(env, opts.DryRun)
	}
	cfg := env.Cfg
	fmt.Printf("Fetching issues from %s (%s/%s) …\n", cfg.Gitea.URL, cfg.Gitea.Owner, cfg.Gitea.Repo)
	items, err := env.Client.fetchIssues(cfg)
	if err != nil {
		return err
	}

	// One paginated call for the whole repo's comments, grouped by issue.
	commentsByIssue := map[int64][]Comment{}
	if env.Comments {
		fmt.Println("Fetching comments …")
		all, err := getPaged[Comment](env.Client, env.Client.repoPath(cfg)+"/issues/comments")
		if err != nil {
			return err
		}
		for _, c := range all {
			num := issueNumFromURL(c.IssueURL)
			if num > 0 {
				commentsByIssue[num] = append(commentsByIssue[num], c)
			}
		}
	}

	type planItem struct{ dir, file, content string }
	var plan []planItem
	var nIssue, nPull, nComments int
	var syncedAt string
	for _, it := range items {
		syncedAt = newerStamp(it.UpdatedAt, syncedAt)
		pull, dir, file := routeFor(it, cfg)
		plan = append(plan, planItem{dir, file, RenderMarkdown(it)})
		if pull {
			nPull++
		} else {
			nIssue++
		}
		if cs := commentsByIssue[it.Number]; len(cs) > 0 {
			plan = append(plan, planItem{dir, commentsFileFor(file), RenderComments(it, cs)})
			nComments += len(cs)
		}
	}
	commentNote := ""
	if env.Comments {
		commentNote = fmt.Sprintf(" (+%d comments)", nComments)
	}

	if opts.DryRun {
		fmt.Printf("[dry-run] %d items → %d issues + %d pulls%s into %s/\n", len(items), nIssue, nPull, commentNote, cfg.Output.Dir)
		var sample []string
		for i := 0; i < len(plan) && i < 3; i++ {
			sample = append(sample, "  "+plan[i].dir+"/"+plan[i].file)
		}
		fmt.Printf("sample:\n%s\n", strings.Join(sample, "\n"))
		return nil
	}

	for _, sub := range managedDirs {
		if err := os.RemoveAll(filepath.Join(env.OutDir, sub)); err != nil {
			return err
		}
	}
	for _, sub := range managedDirs {
		if err := os.MkdirAll(filepath.Join(env.OutDir, sub), 0o755); err != nil {
			return err
		}
	}
	for _, p := range plan {
		if err := os.WriteFile(filepath.Join(env.OutDir, p.dir, p.file), []byte(p.content), 0o644); err != nil {
			return err
		}
	}
	if syncedAt == "" {
		syncedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := writeSyncMarker(env.OutDir, cfg, syncedAt); err != nil {
		return err
	}
	fmt.Printf("Wrote %d issues + %d pulls%s to %s/\n", nIssue, nPull, commentNote, relForLog(env.OutDir))
	return nil
}

// pullIncremental rewrites only issues updated since the marker's high-water
// mark. Renames/moves are handled by dropping every file for the issue
// number first; remote deletions are invisible to the since-feed, so a
// periodic full pull is still needed to reap those.
func pullIncremental(env *Env, dryRun bool) error {
	cfg := env.Cfg
	if !pathExists(filepath.Join(env.OutDir, ".gitea-sync.json")) {
		return fmt.Errorf("no sync marker in %s/ — run a full pull first", relForLog(env.OutDir))
	}
	marker, err := readSyncMarker(env.OutDir)
	if err != nil {
		return err
	}
	if marker.SyncedAt == "" {
		return errors.New("sync marker predates incremental support — run a full pull first")
	}

	fmt.Printf("Fetching issues updated since %s …\n", marker.SyncedAt)
	path := fmt.Sprintf("%s/issues?state=all&type=issues&since=%s", env.Client.repoPath(cfg), url.QueryEscape(marker.SyncedAt))
	items, err := getPaged[Issue](env.Client, path)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("up to date")
		return nil
	}
	if dryRun {
		for _, it := range items {
			_, dir, file := routeFor(it, cfg)
			fmt.Printf("[dry-run] would update %s/%s\n", dir, file)
		}
		return nil
	}

	syncedAt := marker.SyncedAt
	for _, it := range items {
		syncedAt = newerStamp(it.UpdatedAt, syncedAt)
		_, dir, file := routeFor(it, cfg)
		prefix := strconv.FormatInt(it.Number, 10) + "-"
		for _, sub := range managedDirs {
			entries, err := os.ReadDir(filepath.Join(env.OutDir, sub))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), prefix) {
					if err := os.Remove(filepath.Join(env.OutDir, sub, e.Name())); err != nil {
						return err
					}
				}
			}
		}
		if err := os.MkdirAll(filepath.Join(env.OutDir, dir), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(env.OutDir, dir, file), []byte(RenderMarkdown(it)), 0o644); err != nil {
			return err
		}
		if env.Comments && it.Comments > 0 {
			cs, err := getOnce[Comment](env.Client, env.Client.repoPath(cfg)+"/issues/"+strconv.FormatInt(it.Number, 10)+"/comments")
			if err != nil {
				return err
			}
			if len(cs) > 0 {
				if err := os.WriteFile(filepath.Join(env.OutDir, dir, commentsFileFor(file)), []byte(RenderComments(it, cs)), 0o644); err != nil {
					return err
				}
			}
		}
	}
	if err := writeSyncMarker(env.OutDir, cfg, syncedAt); err != nil {
		return err
	}
	fmt.Printf("Updated %d item(s) in %s/ (remote deletions need a full pull)\n", len(items), relForLog(env.OutDir))
	return nil
}

// Status lists local-vs-remote drift. Returns ErrDrift when anything drifts.
func Status(env *Env) error {
	drift, err := computeDrift(env)
	if err != nil {
		return err
	}
	fmt.Printf("local ↔ %s/%s\n", env.Cfg.Gitea.Owner, env.Cfg.Gitea.Repo)
	for _, m := range drift.Modified {
		fmt.Printf("  ~ %s/%s  (%s)\n", m.Local.Dir, m.Local.File, strings.Join(m.Fields, ", "))
	}
	for _, l := range drift.Created {
		fmt.Printf("  + %s/%s  (new — push will create)\n", l.Dir, l.File)
	}
	for _, l := range drift.Orphaned {
		fmt.Printf("  ! %s/%s  (#%d not on remote — pull will remove)\n", l.Dir, l.File, l.Number)
	}
	for _, r := range drift.Missing {
		fmt.Printf("  ? #%d '%s' missing locally — run pull\n", r.Number, r.Title)
	}
	for _, pc := range drift.Pending {
		fmt.Printf("  » comment on #%d  (%s/%s — push will post)\n", pc.Number, pc.Dir, pc.File)
	}
	parts := []string{fmt.Sprintf("%d in sync", drift.Clean)}
	if len(drift.Modified) > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", len(drift.Modified)))
	}
	if len(drift.Created) > 0 {
		parts = append(parts, fmt.Sprintf("%d new", len(drift.Created)))
	}
	if len(drift.Orphaned) > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned", len(drift.Orphaned)))
	}
	if len(drift.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing locally", len(drift.Missing)))
	}
	if len(drift.Pending) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending comment(s)", len(drift.Pending)))
	}
	fmt.Println(strings.Join(parts, ", "))
	if drift.any() {
		return ErrDrift
	}
	return nil
}

// Diff prints a unified diff of the drift, remote → local (the push
// direction). Both sides are rendered canonically. Returns ErrDrift when
// anything drifts.
func Diff(env *Env) error {
	drift, err := computeDrift(env)
	if err != nil {
		return err
	}
	printed := 0
	emit := func(d string) {
		if printed > 0 {
			fmt.Println()
		}
		fmt.Println(d)
		printed++
	}
	for _, m := range drift.Modified {
		d := UnifiedDiff(RenderMarkdown(m.Remote), RenderMarkdown(LocalAsItem(m.Local)),
			fmt.Sprintf("remote/#%d", m.Local.Number), "local/"+m.Local.Dir+"/"+m.Local.File)
		if d == "" {
			continue
		}
		emit(d)
	}
	for _, l := range drift.Created {
		emit(UnifiedDiff("", RenderMarkdown(LocalAsItem(l)), "remote/(none)", "local/"+l.Dir+"/"+l.File))
	}
	for _, pc := range drift.Pending {
		emit(UnifiedDiff("", NormBody(pc.Body)+"\n",
			fmt.Sprintf("remote/#%d/(no comment yet)", pc.Number), "local/"+pc.Dir+"/"+pc.File))
	}
	if printed == 0 {
		fmt.Println("no content drift")
	}
	if drift.any() {
		return ErrDrift
	}
	return nil
}

// Push applies local edits to Gitea: it PATCHes only the fields that differ,
// and creates issues from files without a <n>- prefix, renaming them to the
// canonical filename. It never deletes anything remotely.
func Push(env *Env, opts PushOpts) error {
	drift, err := computeDrift(env)
	if err != nil {
		return err
	}
	cfg := env.Cfg
	rp := env.Client.repoPath(cfg)

	for _, l := range drift.Orphaned {
		fmt.Fprintf(os.Stderr, "warning: %s/%s (#%d) not on remote — skipped (pull will remove it)\n", l.Dir, l.File, l.Number)
	}
	if len(drift.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d remote issue(s) missing locally — run pull\n", len(drift.Missing))
	}
	if len(drift.Modified) == 0 && len(drift.Created) == 0 && len(drift.Pending) == 0 {
		fmt.Println("nothing to push")
		return nil
	}

	// Gitea's label APIs take ids, so resolve names → ids up front: repo
	// labels, then org labels merged in when the owner is an org.
	labelIDs := map[string]int64{}
	needLabels := false
	for _, l := range drift.Created {
		if len(l.Meta.Labels) > 0 {
			needLabels = true
		}
	}
	for _, m := range drift.Modified {
		if contains(m.Fields, "labels") {
			needLabels = true
		}
	}
	if needLabels {
		repoLabels, err := getPaged[Label](env.Client, rp+"/labels")
		if err != nil {
			return err
		}
		for _, l := range repoLabels {
			labelIDs[strings.ToLower(l.Name)] = l.ID
		}
		if orgLabels, err := getPaged[Label](env.Client, "orgs/"+cfg.Gitea.Owner+"/labels"); err == nil {
			for _, l := range orgLabels {
				if _, ok := labelIDs[strings.ToLower(l.Name)]; !ok {
					labelIDs[strings.ToLower(l.Name)] = l.ID
				}
			}
		}
	}
	resolveLabels := func(names []string, where string) []int64 {
		ids := []int64{}
		for _, name := range names {
			if id, ok := labelIDs[strings.ToLower(name)]; ok {
				ids = append(ids, id)
			} else {
				fmt.Fprintf(os.Stderr, "warning: label '%s' not on remote — skipped (%s)\n", name, where)
			}
		}
		return ids
	}

	for _, m := range drift.Modified {
		l := m.Local
		desc := fmt.Sprintf("%s/%s (%s)", l.Dir, l.File, strings.Join(m.Fields, ", "))
		if opts.DryRun {
			fmt.Printf("[dry-run] would update #%d: %s\n", l.Number, desc)
			continue
		}
		patch := map[string]any{}
		if contains(m.Fields, "title") {
			patch["title"] = l.Meta.Title
		}
		if contains(m.Fields, "body") {
			patch["body"] = NormBody(l.Body)
		}
		if contains(m.Fields, "state") {
			patch["state"] = l.Meta.State
		}
		if len(patch) > 0 {
			if _, err := env.Client.Send("PATCH", fmt.Sprintf("%s/issues/%d", rp, l.Number), patch); err != nil {
				return err
			}
		}
		if contains(m.Fields, "labels") {
			if _, err := env.Client.Send("PUT", fmt.Sprintf("%s/issues/%d/labels", rp, l.Number),
				map[string]any{"labels": resolveLabels(l.Meta.Labels, desc)}); err != nil {
				return err
			}
		}
		fmt.Printf("updated #%d: %s\n", l.Number, desc)
	}

	for _, l := range drift.Created {
		if opts.DryRun {
			fmt.Printf("[dry-run] would create issue from %s/%s\n", l.Dir, l.File)
			continue
		}
		payload := map[string]any{
			"title":  l.Meta.Title,
			"body":   NormBody(l.Body),
			"closed": l.Meta.State == "closed",
		}
		if len(l.Meta.Labels) > 0 {
			payload["labels"] = resolveLabels(l.Meta.Labels, l.Dir+"/"+l.File)
		}
		raw, err := env.Client.Send("POST", rp+"/issues", payload)
		if err != nil {
			return err
		}
		var created Issue
		if err := json.Unmarshal(raw, &created); err != nil {
			return err
		}
		newFile := fmt.Sprintf("%d-%s.md", created.Number, Slugify(created.Title))
		if err := os.Rename(filepath.Join(env.OutDir, l.Dir, l.File), filepath.Join(env.OutDir, l.Dir, newFile)); err != nil {
			return err
		}
		fmt.Printf("created #%d from %s/%s → %s/%s\n", created.Number, l.Dir, l.File, l.Dir, newFile)
	}

	// Post locally-authored comments (<n>.comment.md), then remove the file.
	for _, pc := range drift.Pending {
		rel := pc.Dir + "/" + pc.File
		body := NormBody(pc.Body)
		if body == "" {
			fmt.Fprintf(os.Stderr, "warning: %s is empty — skipped (add text or delete it)\n", rel)
			continue
		}
		if opts.DryRun {
			fmt.Printf("[dry-run] would post comment on #%d (%s)\n", pc.Number, rel)
			continue
		}
		if _, err := env.Client.Send("POST", fmt.Sprintf("%s/issues/%d/comments", rp, pc.Number),
			map[string]any{"body": body}); err != nil {
			return fmt.Errorf("posting comment on #%d (%s): %w", pc.Number, rel, err)
		}
		if err := os.Remove(filepath.Join(env.OutDir, pc.Dir, pc.File)); err != nil {
			return err
		}
		fmt.Printf("commented on #%d (posted, removed %s)\n", pc.Number, rel)
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func issueNumFromURL(u string) int64 {
	parts := strings.Split(u, "/")
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// Help returns the CLI help text.
func Help() string {
	return "tea-issue-sync " + Version + ` — mirror Gitea issues to local Markdown (a gh-issue-sync counterpart)

Sync (talk to Gitea):
  tea-issue-sync pull [--incremental] [--dry-run]   Mirror Gitea → output folder. Full pull wipes the
                                                    managed subfolders; --incremental only rewrites
                                                    issues updated since the last sync (deletions
                                                    still need a full pull)
  tea-issue-sync status                             List local-vs-remote drift (exit 1 when drifted)
  tea-issue-sync diff                               Unified diff of the drift, remote → local
  tea-issue-sync push [--dry-run]                   Push local edits; create issues from number-less files

Edit locally (no network; run push to sync):
  tea-issue-sync new "Title" [--label L]... [--body TEXT] [--state open|closed] [--edit]
                                                    Create a new issue file (push creates it on Gitea)
  tea-issue-sync close <n> [--reason completed|not_planned]   Mark #n closed and move it to closed/
  tea-issue-sync reopen <n>                         Mark #n open and move it to open/
  tea-issue-sync comment <n> [--body TEXT] [--edit] Draft a comment on #n (push posts it to Gitea)
  tea-issue-sync list [--state open|closed|all] [--label L] [--search TEXT]
                                                    List issues from the local mirror

Agent setup:
  tea-issue-sync skill install [--project] [--dir <path>]   Install the agent skill (default ~/.claude/skills/)
  tea-issue-sync skill print                                Print the embedded SKILL.md to stdout

  tea-issue-sync --help | --version

Every command accepts --config <path>.

Config: OPTIONAL. In a git repo whose origin points at the Gitea repo, the URL,
        owner, and repo are inferred from the remote — no config file needed.
        Add a .tea-issue-sync.json (nearest one from the cwd up to the git root,
        with an optional .tea-issue-sync.local.json overlay) only to override the
        inferred repo, pick a different remote (gitea.remote), change output.dir,
        or set output.comments=true (mirror comments into <index>-<slug>.comments.md).
Token:  $GITEA_TOKEN or tea's config.yml
Roadmap: see ROADMAP.md.`
}
