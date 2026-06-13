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

// findConfig resolves the config file: an explicit path, else the nearest
// config.json walking up from the cwd, stopping at the git root.
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
			break
		}
		dir = parent
	}
	return "", errors.New("no config.json found between the cwd and the git root; pass --config <path>")
}

// loadConfig reads config.json, overlays config.local.json (per-field), then
// applies defaults and validates.
func loadConfig(configPath string) (*Config, error) {
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
		// Unmarshaling onto the populated struct overrides only the fields
		// present in the local file (the per-section override behaviour).
		if err := json.Unmarshal(ld, cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", localPath, err)
		}
	}
	if cfg.Output.Dir == "" {
		cfg.Output.Dir = ".issues-tea"
	}
	if cfg.Mirror.PullLabel == "" {
		cfg.Mirror.PullLabel = "pull-request"
	}
	if cfg.Mirror.PullTitlePrefix == "" {
		cfg.Mirror.PullTitlePrefix = "[PR #"
	}
	if cfg.Gitea.URL == "" || cfg.Gitea.Owner == "" || cfg.Gitea.Repo == "" {
		return nil, fmt.Errorf("%s missing gitea.url / gitea.owner / gitea.repo", configPath)
	}
	return cfg, nil
}

// outputDir resolves output.dir relative to the config file's directory, so
// the mirror lands in the same place regardless of the invocation cwd.
func outputDir(configPath string, cfg *Config) (string, error) {
	return filepath.Abs(filepath.Join(filepath.Dir(configPath), cfg.Output.Dir))
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
		// Light line-scan: pair each url: with the nearest token:; prefer the
		// login whose url matches our host.
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
