package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// apply makes the agent folders match the selection: enabled skills linked into enabled
// targets, our links for everything else removed. Entries we didn't create are never touched.
// A dry run writes nothing and reports what it would do, which is what the TUI reviews.
func apply(cfg *Config, out io.Writer, dry bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	if _, err := os.Stat(skillsDir); err != nil {
		return fmt.Errorf("no skills folder in %s: %w", cfg.Repo, err)
	}
	deps, err := loadDeps(cfg.Repo)
	if err != nil {
		return err
	}
	agentsDir := filepath.Join(cfg.Repo, "agents")
	isSkillIn := func(name string) bool { return isSkill(skillsDir, name) }
	isAgentIn := func(name string) bool { return isAgent(agentsDir, name) }

	// a hook config we can't parse must not leave one target changed and the other not
	for _, t := range targets {
		if err := syncHooks(filepath.Join(home, t.hooks), cfg.Repo, enabledFor(cfg, t), io.Discard, true); err != nil {
			return err
		}
	}
	for _, d := range legacyDirs {
		prune(filepath.Join(home, d), skillsDir, nil, isSkillIn, out, dry)
	}
	for _, t := range targets {
		dir := filepath.Join(home, t.dir)
		on := slices.Contains(cfg.Targets, t.name)
		enabled := enabledFor(cfg, t)
		agents := agentsFor(enabled, deps)
		prune(dir, skillsDir, enabled, isSkillIn, out, dry)
		if t.agents != "" {
			prune(filepath.Join(home, t.agents), agentsDir, agents, isAgentIn, out, dry)
		}
		if err := syncHooks(filepath.Join(home, t.hooks), cfg.Repo, enabled, out, dry); err != nil {
			return err
		}
		if !on {
			continue
		}
		if !dry {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		for _, name := range cfg.Enabled {
			if !isSkill(skillsDir, name) {
				fmt.Fprintf(out, "skip %s: not in the repo\n", name)
				continue
			}
			if err := link(dir, skillsDir, name, out, dry); err != nil {
				return err
			}
		}
		if t.agents == "" || len(agents) == 0 {
			continue
		}
		if !dry {
			if err := os.MkdirAll(filepath.Join(home, t.agents), 0o755); err != nil {
				return err
			}
		}
		for _, name := range agents {
			if !isAgent(agentsDir, name) {
				fmt.Fprintf(out, "skip agent %s: not in the repo\n", name)
				continue
			}
			// an agent is one file, which Windows can't junction; a failed link loses only that agent
			if err := link(filepath.Join(home, t.agents), agentsDir, name, out, dry); err != nil {
				fmt.Fprintf(out, "skip agent %s: %v\n", name, err)
			}
		}
	}
	return nil
}

// enabledFor is what a target should hold: nothing at all when the target is off.
func enabledFor(cfg *Config, t target) []string {
	if !slices.Contains(cfg.Targets, t.name) {
		return nil
	}
	return cfg.Enabled
}

// agentsFor is the agent files the enabled skills ship, as <name>.md in the repo's agents folder.
func agentsFor(enabled []string, deps map[string]dep) []string {
	var files []string
	for _, s := range enabled {
		for _, a := range deps[s].Agents {
			if f := a + ".md"; !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
	}
	return files
}

// prune removes links in dir that point into srcDir unless they are a correct link
// for a kept name that still exists. Links pointing anywhere else, and real files, are left alone.
func prune(dir, srcDir string, keep []string, exists func(string) bool, out io.Writer, dry bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		// ours point one level into the repo's folder; compare the way the
		// platform does, since Windows and macOS both spell one folder several ways
		dest := linkDest(path)
		if dest == "" || !sameDir(filepath.Dir(dest), srcDir) {
			continue
		}
		if slices.Contains(keep, name) && samePath(dest, filepath.Join(srcDir, name)) && exists(name) {
			continue
		}
		if dry {
			fmt.Fprintf(out, "unlink %s\n", path)
			continue
		}
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(out, "could not remove %s: %v\n", path, err)
			continue
		}
		fmt.Fprintf(out, "removed %s\n", path)
	}
}

// link makes dir/name point at srcDir/name, which the caller has checked exists.
func link(dir, srcDir, name string, out io.Writer, dry bool) error {
	dest := filepath.Join(srcDir, name)
	path := filepath.Join(dir, name)
	if samePath(linkDest(path), dest) {
		return nil
	}
	stale := false
	if _, err := os.Lstat(path); err == nil {
		// a link whose target has gone is one of ours from before the clone moved
		_, live := os.Stat(path)
		if linkDest(path) == "" || live == nil {
			fmt.Fprintf(out, "skip %s: exists and isn't from this repo\n", path)
			return nil
		}
		stale = true
	}
	if dry {
		fmt.Fprintf(out, "link %s\n", path)
		return nil
	}
	if stale {
		if err := os.Remove(path); err != nil {
			return err
		}
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
