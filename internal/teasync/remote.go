package teasync

import (
	"os/exec"
	"regexp"
	"strings"
)

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
