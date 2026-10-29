package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// enableCmd turns a skill or profile on for this machine. Skills bring everything
// they require; profiles merge their members without displacing earlier choices.
func enableCmd(args []string, out io.Writer) error {
	cfg, deps, profiles, name, err := selection(args, "enable")
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	profileName, forced := strings.CutPrefix(name, "profile:")
	p, profileExists := profiles[profileName]
	if forced || (!isSkill(skillsDir, name) && profileExists) {
		if !profileExists {
			return fmt.Errorf("no profile named %q", profileName)
		}
		added, activated, err := enableProfile(cfg, profileName, p, deps, skillsDir)
		if err != nil {
			return err
		}
		if err := cfg.save(); err != nil {
			return err
		}
		reportProfile(out, cfg, deps, "enabled", profileName, added, activated)
		return nil
	}
	if !isSkill(skillsDir, name) {
		return fmt.Errorf("no skill or profile named %q in %s", name, skillsDir)
	}
	var want []string
	if err := withRequires(name, deps, skillsDir, &want); err != nil {
		return fmt.Errorf("cannot enable %s: %w", name, err)
	}

	var added []string
	if !slices.Contains(cfg.Direct, name) {
		cfg.Direct = append(cfg.Direct, name)
	}
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
	collisionHint(out, profiles, name)
	return nil
}

// disableCmd turns a skill off with every skill that can't work without it, or a
// profile off while preserving skills still supplied by another selection.
func disableCmd(args []string, out io.Writer) error {
	cfg, deps, profiles, name, err := selection(args, "disable")
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	skillKnown := isSkill(skillsDir, name) || slices.Contains(cfg.Enabled, name)
	profileName, forced := strings.CutPrefix(name, "profile:")
	p, profileExists := profiles[profileName]
	profileActive := slices.Contains(cfg.Profiles, profileName)
	if forced || (!skillKnown && (profileExists || profileActive)) {
		if !profileExists && !profileActive {
			return fmt.Errorf("no profile named %q", profileName)
		}
		removed, deactivated := disableProfile(cfg, profileName, p, profiles, deps)
		if err := cfg.save(); err != nil {
			return err
		}
		reportProfile(out, cfg, deps, "disabled", profileName, removed, deactivated)
		return nil
	}
	if !skillKnown {
		return fmt.Errorf("no skill named %q, no profile by that name, and nothing enabled", name)
	}
	var drop []string
	withDependents(name, deps, &drop)

	var removed []string
	for _, n := range drop {
		if i := slices.Index(cfg.Direct, n); i >= 0 {
			cfg.Direct = slices.Delete(cfg.Direct, i, i+1)
		}
		if i := slices.Index(cfg.Enabled, n); i >= 0 {
			cfg.Enabled = slices.Delete(cfg.Enabled, i, i+1)
			removed = append(removed, n)
		}
	}
	if err := cfg.save(); err != nil {
		return err
	}
	report(out, cfg, deps, "disabled", name, removed, "also disabled (they require it)")
	collisionHint(out, profiles, name)
	return nil
}

// collisionHint says how to reach a profile that a same-named skill shadows.
func collisionHint(out io.Writer, profiles map[string]profile, name string) {
	if _, ok := profiles[name]; ok {
		fmt.Fprintf(out, "%s is also a profile; use profile:%s for it\n", name, name)
	}
}

func selection(args []string, verb string) (*Config, map[string]dep, map[string]profile, string, error) {
	if len(args) != 1 {
		return nil, nil, nil, "", fmt.Errorf("usage: skillset %s <skill|profile>", verb)
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, nil, "", err
	}
	cfg.ensureDirect()
	deps, err := loadDeps(cfg.Repo)
	if err != nil {
		return nil, nil, nil, "", err
	}
	profiles, err := loadProfiles(cfg.Repo)
	if err != nil {
		return nil, nil, nil, "", err
	}
	return cfg, deps, profiles, args[0], nil
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

func reportProfile(out io.Writer, cfg *Config, deps map[string]dep, verb, name string, changed []string, stateChanged bool) {
	if !stateChanged && len(changed) == 0 {
		fmt.Fprintf(out, "already %s: %s\n", verb, name)
	} else {
		fmt.Fprintf(out, "%s profile %s\n", verb, name)
	}
	if len(changed) > 0 {
		slices.Sort(changed)
		fmt.Fprintf(out, "skills %s: %s\n", verb, strings.Join(changed, ", "))
	}
	for _, w := range warnings(cfg, deps) {
		fmt.Fprintln(out, w)
	}
	if len(changed) > 0 {
		fmt.Fprintln(out, "run `skillset apply` to link")
	}
}
