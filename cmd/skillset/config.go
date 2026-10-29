package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// Config is this machine's selection. It lives outside the repo and is never committed.
type Config struct {
	Repo    string   `json:"repo"`
	Targets []string `json:"targets"`
	Enabled []string `json:"enabled"`
	// Direct names skills enabled on their own, rather than by a profile. Enabled
	// remains the flattened selection apply consumes; Direct preserves ownership
	// when one of several overlapping profiles is disabled.
	Direct   []string `json:"direct"`
	Profiles []string `json:"profiles,omitempty"`
	Seen     []string `json:"seen"`
	// Theme picks the screen's look by name; empty, or a name that no longer
	// exists, falls back to the default.
	Theme string `json:"theme,omitempty"`
}

var errNotSetUp = errors.New("not set up on this machine yet: run `skillset init` inside your skills clone, or `skillset init <path>`")

// configPath is ~/.config/skillset/config.json on both OSes, %USERPROFILE%\.config on Windows.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "skillset", "config.json"), nil
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNotSetUp
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ensureDirect adopts every skill in an old config as directly selected. Before
// profiles existed that was the only meaning Enabled had, and keeping that meaning
// makes the first profile disable non-destructive. The non-nil empty slice records
// the migration even when nothing is selected.
func (c *Config) ensureDirect() {
	if c.Direct == nil {
		c.Direct = append([]string{}, c.Enabled...)
	}
}

func (c *Config) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	c.ensureDirect()
	slices.Sort(c.Targets)
	slices.Sort(c.Enabled)
	slices.Sort(c.Direct)
	slices.Sort(c.Profiles)
	slices.Sort(c.Seen)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
