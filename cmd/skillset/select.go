package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// enableCmd turns a skill on for this machine, along with everything it requires.
func enableCmd(args []string, out io.Writer) error {
	cfg, deps, name, err := selection(args, "enable")
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	if !isSkill(skillsDir, name) {
		return fmt.Errorf("no skill named %q in %s", name, skillsDir)
	}
	var want []string
	if err := withRequires(name, deps, skillsDir, &want); err != nil {
		return fmt.Errorf("cannot enable %s: %w", name, err)
	}

	var added []string
	for _, n := range want {
		if !slices.Contains(cfg.Enabled, n) {
			cfg.Enabled = append(cfg.Enabled, n)
			added = append(added, n)
		}
		if !slices.Contains(cfg.Seen, n) {
			cfg.Seen = append(cfg.Seen, n)
		}
	}
	if err := cfg.save(); err != nil {
		return err
	}
	report(out, cfg, deps, "enabled", name, added, "also enabled (required)")
	return nil
}

// disableCmd turns a skill off, along with every skill that can't work without it.
func disableCmd(args []string, out io.Writer) error {
	cfg, deps, name, err := selection(args, "disable")
	if err != nil {
		return err
	}
	var drop []string
	withDependents(name, deps, &drop)

	var removed []string
	for _, n := range drop {
		if i := slices.Index(cfg.Enabled, n); i >= 0 {
			cfg.Enabled = slices.Delete(cfg.Enabled, i, i+1)
			removed = append(removed, n)
		}
	}
	if err := cfg.save(); err != nil {
		return err
	}
	report(out, cfg, deps, "disabled", name, removed, "also disabled (they require it)")
	return nil
}

func selection(args []string, verb string) (*Config, map[string]dep, string, error) {
	if len(args) != 1 {
		return nil, nil, "", fmt.Errorf("usage: skillset %s <skill>", verb)
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, "", err
	}
	deps, err := loadDeps(cfg.Repo)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, deps, args[0], nil
}

// report says what changed and what is now running without an optional dependency.
func report(out io.Writer, cfg *Config, deps map[string]dep, verb, name string, changed []string, alsoLabel string) {
	if !slices.Contains(changed, name) {
		fmt.Fprintf(out, "already %s: %s\n", verb, name)
	} else {
		fmt.Fprintf(out, "%s %s\n", verb, name)
	}
	if also := slices.DeleteFunc(slices.Clone(changed), func(n string) bool { return n == name }); len(also) > 0 {
		slices.Sort(also)
		fmt.Fprintf(out, "%s: %s\n", alsoLabel, strings.Join(also, ", "))
	}
	for _, w := range warnings(cfg, deps) {
		fmt.Fprintln(out, w)
	}
	if len(changed) > 0 {
		fmt.Fprintln(out, "run `skillset apply` to link")
	}
}
