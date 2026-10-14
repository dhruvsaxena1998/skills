package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// apply makes the agent folders match the selection: enabled skills linked into enabled
// targets, our links for everything else removed. Entries we didn't create are never touched.
func apply(cfg *Config, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	if _, err := os.Stat(skillsDir); err != nil {
		return fmt.Errorf("no skills folder in %s: %w", cfg.Repo, err)
	}

	for _, d := range legacyDirs {
		prune(filepath.Join(home, d), skillsDir, nil, out)
	}
	for _, t := range targets {
		dir := filepath.Join(home, t.dir)
		on := slices.Contains(cfg.Targets, t.name)
		enabled := cfg.Enabled
		if !on {
			enabled = nil // a target that is off keeps neither links nor hooks of ours
		}
		prune(dir, skillsDir, enabled, out)
		if err := syncHooks(filepath.Join(home, t.hooks), cfg.Repo, enabled, out); err != nil {
			return err
		}
		if !on {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, name := range cfg.Enabled {
			if err := link(dir, skillsDir, name, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// prune removes links in dir that point into skillsDir unless they are a correct link
// for a kept skill. Links pointing anywhere else, and real files, are left alone.
func prune(dir, skillsDir string, keep []string, out io.Writer) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		dest := linkDest(path)
		if dest == "" || !strings.HasPrefix(dest, skillsDir+string(filepath.Separator)) {
			continue
		}
		if slices.Contains(keep, name) && samePath(dest, filepath.Join(skillsDir, name)) && isSkill(skillsDir, name) {
			continue
		}
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(out, "could not remove %s: %v\n", path, err)
			continue
		}
		fmt.Fprintf(out, "removed %s\n", path)
	}
}

func link(dir, skillsDir, name string, out io.Writer) error {
	dest := filepath.Join(skillsDir, name)
	if !isSkill(skillsDir, name) {
		fmt.Fprintf(out, "skip %s: not in the repo\n", name)
		return nil
	}
	path := filepath.Join(dir, name)
	if samePath(linkDest(path), dest) {
		return nil
	}
	if _, err := os.Lstat(path); err == nil {
		fmt.Fprintf(out, "skip %s: exists and isn't from this repo\n", path)
		return nil
	}
	kind, err := createLink(dest, path)
	if err != nil {
		return fmt.Errorf("link %s: %w", path, err)
	}
	if kind == "symlink" {
		fmt.Fprintf(out, "linked %s\n", path)
	} else {
		fmt.Fprintf(out, "linked %s (%s)\n", path, kind)
	}
	return nil
}
