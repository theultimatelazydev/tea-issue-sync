package teasync

import (
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		remote, url, owner, repo string
	}{
		// HTTP with a custom port (the common Gitea case)
		{"http://gitea.example.com:3000/acme/note-app", "http://gitea.example.com:3000", "acme", "note-app"},
		{"https://gitea.example.com/acme/roadmap.git", "https://gitea.example.com", "acme", "roadmap"},
		{"https://gitea.example.com/acme/roadmap/", "https://gitea.example.com", "acme", "roadmap"},
		// SSH forms — host only, HTTPS assumed for the API
		{"git@gitea.example.com:acme/roadmap.git", "https://gitea.example.com", "acme", "roadmap"},
		{"ssh://git@gitea.example.com:22/acme/roadmap.git", "https://gitea.example.com", "acme", "roadmap"},
		// Unparseable
		{"not a url", "", "", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		url, owner, repo := parseRemoteURL(c.remote)
		if url != c.url || owner != c.owner || repo != c.repo {
			t.Errorf("parseRemoteURL(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.remote, url, owner, repo, c.url, c.owner, c.repo)
		}
	}
}

func TestNonGiteaHint(t *testing.T) {
	// Known non-Gitea hosts produce a pointer to the right tool.
	for _, u := range []string{"https://github.com", "https://gitlab.com", "https://bitbucket.org"} {
		if nonGiteaHint(u) == "" {
			t.Errorf("expected a hint for %q", u)
		}
	}
	if !strings.Contains(nonGiteaHint("https://github.com"), "gh-issue-sync") {
		t.Error("GitHub hint should mention gh-issue-sync")
	}
	// Self-hosted Gitea on any domain (incl. custom ports) passes through.
	for _, u := range []string{"http://gitea.example.com:3000", "https://git.mycompany.dev", ""} {
		if h := nonGiteaHint(u); h != "" {
			t.Errorf("nonGiteaHint(%q) = %q, want empty", u, h)
		}
	}
}
