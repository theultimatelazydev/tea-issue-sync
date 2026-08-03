package teasync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// findConfig locates an optional config file. An explicit path that doesn't
// exist is an error; otherwise it returns the nearest config.json walking up
// from the cwd to the git root, or "" (no error) when there is none.
func findConfig(explicit string) (string, error) {
	if explicit != "" {
		p, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if !pathExists(p) {
			return "", fmt.Errorf("config not found: %s", explicit)
		}
		return p, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		cand := filepath.Join(dir, "config.json")
		if pathExists(cand) {
			return cand, nil
		}
		parent := filepath.Dir(dir)
		if pathExists(filepath.Join(dir, ".git")) || parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// gitRoot returns the nearest ancestor of the cwd that contains a .git entry,
// or "" if the cwd is not inside a git repository.
func gitRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if pathExists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// loadConfigFile parses config.json and overlays config.local.json (per
// field). It does NOT apply defaults or validate — callers do that after
// merging in any values inferred from the git remote.
func loadConfigFile(configPath string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	localPath := filepath.Join(filepath.Dir(configPath), "config.local.json")
	if pathExists(localPath) {
		ld, err := os.ReadFile(localPath)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(ld, cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", localPath, err)
		}
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Output.Dir == "" {
		cfg.Output.Dir = ".issues-tea"
	}
	if cfg.Mirror.PullLabel == "" {
		cfg.Mirror.PullLabel = "pull-request"
	}
	if cfg.Mirror.PullTitlePrefix == "" {
		cfg.Mirror.PullTitlePrefix = "[PR #"
	}
}

var (
	reTeaURL   = regexp.MustCompile(`^\s*url:\s*(.+?)\s*$`)
	reTeaToken = regexp.MustCompile(`^\s*token:\s*(.+?)\s*$`)
)

// resolveToken finds a Gitea API token without ever printing or persisting
// it: $GITEA_TOKEN first, else the matching login's token in tea's config.yml.
func resolveToken(giteaURL string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("GITEA_TOKEN")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(home, "Library", "Application Support", "tea", "config.yml"),
		filepath.Join(home, ".config", "tea", "config.yml"),
	}
	stripQuotes := func(s string) string { return strings.Trim(s, `'"`) }
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var curURL, firstToken, matchToken string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, "\r")
			if m := reTeaURL.FindStringSubmatch(line); m != nil {
				curURL = stripQuotes(m[1])
			}
			if m := reTeaToken.FindStringSubmatch(line); m != nil {
				tok := stripQuotes(m[1])
				if firstToken == "" {
					firstToken = tok
				}
				if curURL != "" && strings.HasPrefix(giteaURL, strings.TrimRight(curURL, "/")) {
					matchToken = tok
				}
			}
		}
		if matchToken != "" {
			return matchToken, nil
		}
		if firstToken != "" {
			return firstToken, nil
		}
	}
	return "", errors.New("no token: set $GITEA_TOKEN or log in with `tea login add`")
}

// resolveConfig produces the final Config and the directory the output folder
// is anchored to. Precedence for gitea.url/owner/repo: explicit config values
// win; anything omitted is inferred from the git remote. config.json is
// optional — a git repo whose origin is the Gitea repo needs none.
func resolveConfig(configPath string) (*Config, string, error) {
	cfgPath, err := findConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	cfg := &Config{}
	anchor := ""
	if cfgPath != "" {
		cfg, err = loadConfigFile(cfgPath)
		if err != nil {
			return nil, "", err
		}
		anchor = filepath.Dir(cfgPath)
	}

	root := gitRoot()
	if cfg.Gitea.URL == "" || cfg.Gitea.Owner == "" || cfg.Gitea.Repo == "" {
		if root != "" {
			u, o, r := inferGiteaFromRemote(root, cfg.Gitea.Remote)
			if cfg.Gitea.URL == "" {
				cfg.Gitea.URL = u
			}
			if cfg.Gitea.Owner == "" {
				cfg.Gitea.Owner = o
			}
			if cfg.Gitea.Repo == "" {
				cfg.Gitea.Repo = r
			}
		}
	}
	applyDefaults(cfg)

	if cfg.Gitea.URL == "" || cfg.Gitea.Owner == "" || cfg.Gitea.Repo == "" {
		return nil, "", errors.New("could not determine the Gitea repo: no config.json with gitea.url/owner/repo, and no git remote 'origin' to infer from. Add a config.json or run inside a git repo whose origin points at the Gitea repo")
	}

	if anchor == "" {
		anchor = root
	}
	if anchor == "" {
		if anchor, err = os.Getwd(); err != nil {
			return nil, "", err
		}
	}
	return cfg, anchor, nil
}
