package teasync

import (
	"fmt"
	"sort"
	"strings"
)

func labelNames(labels []Label) []string {
	var out []string
	for _, l := range labels {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out
}

func sortedJoin(items []string) string {
	cp := append([]string(nil), items...)
	sort.Strings(cp)
	return strings.Join(cp, "\n")
}

// DiffFields lists which fields differ between a local file and its remote
// issue. Label order is ignored; frontmatter state wins over folder
// placement (pull reorganizes folders).
func DiffFields(l Local, r Issue) []string {
	var fields []string
	if l.Meta.Title != r.Title {
		fields = append(fields, "title")
	}
	if sortedJoin(l.Meta.Labels) != sortedJoin(labelNames(r.Labels)) {
		fields = append(fields, "labels")
	}
	rState := "open"
	if r.State == "closed" {
		rState = "closed"
	}
	if l.Meta.State != rState {
		fields = append(fields, "state")
	}
	if NormBody(l.Body) != NormBody(r.Body) {
		fields = append(fields, "body")
	}
	return fields
}

type diffOp struct {
	tag  byte // ' ', '-', '+'
	line string
}

// UnifiedDiff is a minimal LCS-based unified diff — enough for issue-sized
// texts, no dependencies. It returns "" when the texts are identical.
func UnifiedDiff(aText, bText, aLabel, bLabel string) string {
	if aText == bText {
		return ""
	}
	const context = 3
	a := strings.Split(aText, "\n")
	b := strings.Split(bText, "\n")
	n, m := len(a), len(b)

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}

	type pos struct{ a, b int }
	positions := make([]pos, len(ops))
	ai, bi := 1, 1
	for k, o := range ops {
		positions[k] = pos{ai, bi}
		if o.tag != '+' {
			ai++
		}
		if o.tag != '-' {
			bi++
		}
	}

	lines := []string{"--- " + aLabel, "+++ " + bLabel}
	k := 0
	for k < len(ops) {
		if ops[k].tag == ' ' {
			k++
			continue
		}
		start := k - context
		if start < 0 {
			start = 0
		}
		end := k + 1
		lastChange := k
		for end < len(ops) && end-lastChange <= context*2+1 {
			if ops[end].tag != ' ' {
				lastChange = end
			}
			end++
		}
		end = min(len(ops), lastChange+context+1)
		hunk := ops[start:end]
		aCount, bCount := 0, 0
		for _, o := range hunk {
			if o.tag != '+' {
				aCount++
			}
			if o.tag != '-' {
				bCount++
			}
		}
		lines = append(lines, fmt.Sprintf("@@ -%d,%d +%d,%d @@", positions[start].a, aCount, positions[start].b, bCount))
		for _, o := range hunk {
			lines = append(lines, string(o.tag)+o.line)
		}
		k = end
	}
	return strings.Join(lines, "\n")
}
