package teasync

import (
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

// nonGiteaHint returns a helpful message when the resolved instance URL is a
// well-known host that isn't Gitea, so users pointed at the wrong tool (most
// often a GitHub repo) get a clear pointer instead of an opaque API error.
// Empty string means "looks fine, proceed" — self-hosted Gitea on any domain
// passes through untouched.
func nonGiteaHint(giteaURL string) string {
	host := ""
	if u, err := url.Parse(giteaURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return "this looks like a GitHub repo (" + giteaURL + "). tea-issue-sync syncs Gitea issues — for GitHub use gh-issue-sync (github.com/mitsuhiko/gh-issue-sync). If your Gitea repo is elsewhere, set gitea.url/owner/repo in a .tea-issue-sync.json."
	case host == "gitlab.com":
		return "this looks like a GitLab repo (" + giteaURL + "). tea-issue-sync only speaks the Gitea API — point it at a Gitea repo, or set gitea.url/owner/repo in a .tea-issue-sync.json."
	case host == "bitbucket.org":
		return "this looks like a Bitbucket repo (" + giteaURL + "). tea-issue-sync only speaks the Gitea API — point it at a Gitea repo, or set gitea.url/owner/repo in a .tea-issue-sync.json."
	}
	return ""
}

// Parse the two remote URL shapes Gitea hands out. HTTP(S) is exact; for SSH
// remotes the web/API host is assumed to be https://<host> (the port and
// scheme of the HTTP endpoint aren't encoded in an SSH URL).
var (
	reHTTPRemote = regexp.MustCompile(`^(https?://[^/]+)/([^/]+)/([^/]+?)(?:\.git)?/?$`)
	reSSHRemote  = regexp.MustCompile(`^(?:ssh://)?[^@]+@([^/:]+)(?::\d+)?[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// inferGiteaFromRemote reads the git remote (default "origin") of the repo at
// gitDir and extracts the Gitea base URL, owner, and repo. Empty strings when
// there is no such remote or it doesn't parse.
func inferGiteaFromRemote(gitDir, remote string) (url, owner, repo string) {
	if remote == "" {
		remote = "origin"
	}
	out, err := exec.Command("git", "-C", gitDir, "remote", "get-url", remote).Output()
	if err != nil {
		return "", "", ""
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

func parseRemoteURL(remote string) (url, owner, repo string) {
	if m := reHTTPRemote.FindStringSubmatch(remote); m != nil {
		return m[1], m[2], m[3]
	}
	if m := reSSHRemote.FindStringSubmatch(remote); m != nil {
		return "https://" + m[1], m[2], m[3]
	}
	return "", "", ""
}
