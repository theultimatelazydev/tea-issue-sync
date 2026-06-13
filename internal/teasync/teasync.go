// Package teasync mirrors a Gitea repository's issues to a local folder of
// Markdown files (one file per issue, with YAML frontmatter) in the same
// on-disk format as gh-issue-sync, and pushes local edits back.
//
// It is the Gitea-side counterpart to mitsuhiko/gh-issue-sync. The Markdown
// format is identical, so tooling written against a gh-issue-sync ".issues/"
// folder works against a tea-issue-sync folder unchanged.
package teasync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Version is overridable at build time via
// -ldflags "-X github.com/theultimatelazydev/tea-issue-sync/internal/teasync.Version=...".
var Version = "1.1.0"

// MANAGED_DIRS are wiped and rewritten on every full pull (Gitea is the
// source of truth), so renames and deletions there don't leave stale files.
var managedDirs = []string{"open", "closed", "pulls/open", "pulls/closed"}

// Config mirrors config.json. Unmarshaling config.local.json over an
// already-populated value gives per-field overrides for free.
type Config struct {
	Gitea struct {
		URL   string `json:"url"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	} `json:"gitea"`
	Output struct {
		Dir      string `json:"dir"`
		Comments bool   `json:"comments"`
	} `json:"output"`
	Mirror struct {
		PullLabel       string `json:"pullLabel"`
		PullTitlePrefix string `json:"pullTitlePrefix"`
	} `json:"mirror"`
}

// Issue is the subset of Gitea's issue payload this tool reads or writes.
type Issue struct {
	Number      int64           `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	StateReason *string         `json:"state_reason"`
	Labels      []Label         `json:"labels"`
	PullRequest json.RawMessage `json:"pull_request"`
	UpdatedAt   string          `json:"updated_at"`
	Comments    int             `json:"comments"`
}

// Label is a Gitea label; the id is needed because the label APIs take ids.
type Label struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Comment is the subset of a Gitea issue comment used for the sidecar mirror.
type Comment struct {
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	IssueURL  string `json:"issue_url"`
	User      struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"user"`
}

// Meta is the parsed frontmatter of a local issue file.
type Meta struct {
	Title       string
	Labels      []string
	State       string
	StateReason *string
}

// Local is one issue file read back from the mirror. Number==0 (HasNumber
// false) marks a file with no <n>- prefix: a new issue to create on push.
type Local struct {
	Number    int64
	HasNumber bool
	Dir       string
	File      string
	Meta      Meta
	Body      string
}

// Env bundles the resolved config, an authenticated client and the absolute
// output directory — everything the commands need.
type Env struct {
	Cfg      *Config
	Client   *Client
	OutDir   string
	Comments bool
}

// LoadLocal resolves the config and output directory but does NOT resolve a
// token or build a client — for commands that only touch the local mirror
// (new/close/reopen/list), which must work offline.
func LoadLocal(configPath string) (*Env, error) {
	cfgPath, err := findConfig(configPath)
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	outDir, err := outputDir(cfgPath, cfg)
	if err != nil {
		return nil, err
	}
	return &Env{Cfg: cfg, OutDir: outDir, Comments: cfg.Output.Comments}, nil
}

// Load is LoadLocal plus a resolved token and an authenticated client — for
// commands that hit the Gitea API (pull/status/diff/push).
func Load(configPath string) (*Env, error) {
	env, err := LoadLocal(configPath)
	if err != nil {
		return nil, err
	}
	token, err := resolveToken(env.Cfg.Gitea.URL)
	if err != nil {
		return nil, err
	}
	env.Client = NewClient(env.Cfg.Gitea.URL, token)
	return env, nil
}

// Client is a thin authenticated Gitea API client (stdlib only).
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient builds a client for the given Gitea base URL and token.
func NewClient(giteaURL, token string) *Client {
	base := strings.TrimRight(giteaURL, "/") + "/api/v1"
	return &Client{base: base, token: token, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) repoPath(cfg *Config) string {
	return fmt.Sprintf("repos/%s/%s", cfg.Gitea.Owner, cfg.Gitea.Repo)
}

func (c *Client) get(url string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Gitea API %s on GET %s", res.Status, trimBase(url, c.base))
	}
	return json.NewDecoder(res.Body).Decode(dest)
}

// Send performs a write request (PATCH/PUT/POST). 204 returns nil bytes.
func (c *Client) Send(method, path string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.base+"/"+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Gitea API %s on %s %s", res.Status, method, path)
	}
	if res.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	return io.ReadAll(res.Body)
}

func trimBase(url, base string) string {
	return strings.TrimPrefix(url, base+"/")
}

// getPaged walks limit/page pagination until a short/empty page.
func getPaged[T any](c *Client, path string) ([]T, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var out []T
	for page := 1; page < 200; page++ {
		url := fmt.Sprintf("%s/%s%slimit=50&page=%d", c.base, path, sep, page)
		var batch []T
		if err := c.get(url, &batch); err != nil {
			return nil, fmt.Errorf("%w (page %d)", err, page)
		}
		if len(batch) == 0 {
			break
		}
		out = append(out, batch...)
		if len(batch) < 50 {
			break
		}
	}
	return out, nil
}

// getOnce fetches a single unpaginated list — for endpoints (like the
// per-issue comments list) that ignore limit/page and would otherwise make
// getPaged spin to the page cap.
func getOnce[T any](c *Client, path string) ([]T, error) {
	var out []T
	err := c.get(c.base+"/"+path, &out)
	return out, err
}

func (c *Client) fetchIssues(cfg *Config) ([]Issue, error) {
	// type=issues excludes real Gitea PRs; gitea-mirror's PR-as-issue items
	// still come through and are filtered by IsPull.
	return getPaged[Issue](c, c.repoPath(cfg)+"/issues?state=all&type=issues")
}

// IsPull reports whether an issues-API item is really a pull request:
// a non-null pull_request field, the configured pull label, or the
// configured title prefix (gitea-mirror imports GitHub PRs as such issues).
func IsPull(it Issue, cfg *Config) bool {
	if len(it.PullRequest) > 0 && string(it.PullRequest) != "null" {
		return true
	}
	label := cfg.Mirror.PullLabel
	if label == "" {
		label = "pull-request"
	}
	for _, l := range it.Labels {
		if strings.EqualFold(l.Name, label) {
			return true
		}
	}
	prefix := cfg.Mirror.PullTitlePrefix
	if prefix == "" {
		prefix = "[PR #"
	}
	return strings.HasPrefix(it.Title, prefix)
}

func newerStamp(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, ea := time.Parse(time.RFC3339, a)
	tb, eb := time.Parse(time.RFC3339, b)
	if ea != nil {
		return b
	}
	if eb != nil {
		return a
	}
	if ta.After(tb) {
		return a
	}
	return b
}
