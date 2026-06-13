package teasync

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	reCrossRef    = regexp.MustCompile(`(?i)^\[(GH-ISSUE|PR)\s*#\d+\]\s*`)
	reMerged      = regexp.MustCompile(`(?i)^\[MERGED\]\s*`)
	reNonAlnum    = regexp.MustCompile(`[^a-z0-9]+`)
	reFrontmatter = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?`)
	reLabelItem   = regexp.MustCompile(`^\s+-\s+(.+?)\s*$`)
	reKV          = regexp.MustCompile(`^(\w+):\s*(.*)$`)
)

// Slugify turns a title into a filename slug, dropping gitea-mirror's
// cross-reference and [MERGED] prefixes and capping length at 90.
func Slugify(title string) string {
	s := reCrossRef.ReplaceAllString(title, "")
	s = reMerged.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = reNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 90 {
		s = s[:90]
	}
	s = strings.TrimRight(s, "-")
	if s == "" {
		return "untitled"
	}
	return s
}

// YAMLSingleQuote wraps a scalar in single quotes, escaping inner quotes.
func YAMLSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func trimTrailingSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

// NormBody normalizes a body for comparison/writing: LF line endings, no
// trailing whitespace.
func NormBody(s string) string {
	return trimTrailingSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// RenderMarkdown produces the canonical gh-issue-sync-compatible file:
// single-quoted title, a 4-space labels list (or labels: []), state and
// state_reason, then the body.
func RenderMarkdown(it Issue) string {
	var labels []string
	for _, l := range it.Labels {
		if l.Name != "" {
			labels = append(labels, l.Name)
		}
	}
	fm := []string{"---", "title: " + YAMLSingleQuote(it.Title)}
	if len(labels) > 0 {
		fm = append(fm, "labels:")
		for _, l := range labels {
			fm = append(fm, "    - "+l)
		}
	} else {
		fm = append(fm, "labels: []")
	}
	state := it.State
	if state == "" {
		state = "open"
	}
	fm = append(fm, "state: "+state)
	if it.StateReason != nil {
		fm = append(fm, "state_reason: "+YAMLSingleQuote(*it.StateReason))
	} else {
		fm = append(fm, "state_reason: null")
	}
	fm = append(fm, "---", "")
	return strings.Join(fm, "\n") + "\n" + NormBody(it.Body) + "\n"
}

// RenderComments renders the read-only comments sidecar: one section per
// comment, headed by author and timestamp.
func RenderComments(it Issue, comments []Comment) string {
	parts := []string{trimTrailingSpace("# Comments — #" + strconv.FormatInt(it.Number, 10) + " " + it.Title), ""}
	for _, c := range comments {
		who := c.User.Login
		if who == "" {
			who = c.User.Username
		}
		if who == "" {
			who = "unknown"
		}
		created := c.CreatedAt
		if created == "" {
			created = "?"
		}
		parts = append(parts, "## @"+who+" — "+created, "", NormBody(c.Body), "")
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n") + "\n"
}

func yamlUnquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return s
}

// ParseIssueFile parses the frontmatter subset this tool writes (title,
// labels, state, state_reason), tolerating hand edits: unquoted or
// double-quoted scalars, labels: [], varying list indentation.
func ParseIssueFile(content, where string) (Meta, string, error) {
	loc := reFrontmatter.FindStringSubmatchIndex(content)
	if loc == nil {
		return Meta{}, "", fmt.Errorf("%s: missing frontmatter", where)
	}
	fmText := content[loc[2]:loc[3]]
	meta := Meta{State: "open"}
	inLabels := false
	for _, line := range strings.Split(fmText, "\n") {
		if inLabels {
			if m := reLabelItem.FindStringSubmatch(line); m != nil {
				meta.Labels = append(meta.Labels, yamlUnquote(m[1]))
				continue
			}
		}
		inLabels = false
		kv := reKV.FindStringSubmatch(line)
		if kv == nil {
			continue
		}
		key, raw := kv[1], kv[2]
		switch key {
		case "labels":
			inLabels = raw == ""
		case "title":
			if raw == "" || raw == "null" {
				meta.Title = ""
			} else {
				meta.Title = yamlUnquote(raw)
			}
		case "state":
			if raw == "" || raw == "null" {
				meta.State = ""
			} else {
				meta.State = yamlUnquote(raw)
			}
		case "state_reason":
			if raw == "" || raw == "null" {
				meta.StateReason = nil
			} else {
				v := yamlUnquote(raw)
				meta.StateReason = &v
			}
		}
	}
	if meta.Title == "" {
		return Meta{}, "", fmt.Errorf("%s: missing title in frontmatter", where)
	}
	if meta.State != "closed" {
		meta.State = "open"
	}
	// Drop the single blank separator line RenderMarkdown writes after the
	// frontmatter; it's formatting, not body content.
	body := strings.TrimPrefix(content[loc[1]:], "\n")
	return meta, body, nil
}

// LocalAsItem shapes a parsed local file like an API item so RenderMarkdown
// produces the canonical form of both sides (formatting-only edits don't
// register as drift).
func LocalAsItem(l Local) Issue {
	var labels []Label
	for _, n := range l.Meta.Labels {
		labels = append(labels, Label{Name: n})
	}
	return Issue{Title: l.Meta.Title, Labels: labels, State: l.Meta.State, StateReason: l.Meta.StateReason, Body: l.Body}
}
