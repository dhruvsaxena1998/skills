package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// profile is a named set of skills in the repo's optional profiles.json.
type profile struct {
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

func loadProfiles(repo string) (map[string]profile, error) {
	data, err := os.ReadFile(filepath.Join(repo, "profiles.json"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles map[string]profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("profiles.json: %w", err)
	}
	if profiles == nil {
		profiles = map[string]profile{}
	}
	return profiles, nil
}

// saveProfiles writes the repo's profiles.json, keeping the same hand-editable
// layout: sorted names, and each profile's sorted skills. Nothing writes until the
// caller has validated what it is saving.
func saveProfiles(repo string, profiles map[string]profile) error {
	sorted := make(map[string]profile, len(profiles))
	for _, name := range slices.Sorted(maps.Keys(profiles)) {
		p := profiles[name]
		p.Skills = slices.Compact(slices.Sorted(slices.Values(p.Skills)))
		sorted[name] = p
	}
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo, "profiles.json"), append(data, '\n'), 0o644)
}

// profileNames returns the profile names in stable order.
func profileNames(profiles map[string]profile) []string {
	return slices.Sorted(maps.Keys(profiles))
}

// enableProfile merges a profile into the current selection. Validation finishes
// before any state changes, so one missing member or requirement rejects the set.
func enableProfile(cfg *Config, name string, p profile, deps map[string]dep, skillsDir string) (added []string, activated bool, err error) {
	var want []string
	for _, member := range p.Skills {
		if !isSkill(skillsDir, member) {
			return nil, false, fmt.Errorf("profile %q includes %q, which isn't in the repo", name, member)
		}
		if err := withRequires(member, deps, skillsDir, &want); err != nil {
			return nil, false, fmt.Errorf("cannot enable profile %s: %w", name, err)
		}
	}

	if !slices.Contains(cfg.Profiles, name) {
		cfg.Profiles = append(cfg.Profiles, name)
		activated = true
	}
	for _, member := range want {
		if !slices.Contains(cfg.Enabled, member) {
			cfg.Enabled = append(cfg.Enabled, member)
			added = append(added, member)
		}
		if !slices.Contains(cfg.Seen, member) {
			cfg.Seen = append(cfg.Seen, member)
		}
	}
	return added, activated, nil
}

// disableProfile removes only the profile's explicit members. A member survives
// when it belongs to another enabled profile or is needed by a skill that remains
// enabled. Requirements pulled in for the profile but not
// listed as members are left alone, matching ordinary skill disable behavior.
func disableProfile(cfg *Config, name string, p profile, profiles map[string]profile, deps map[string]dep) (removed []string, deactivated bool) {
	at := slices.Index(cfg.Profiles, name)
	if at < 0 {
		return nil, false
	}
	cfg.Profiles = slices.Delete(cfg.Profiles, at, at+1)
	deactivated = true

	candidates := map[string]bool{}
	for _, member := range p.Skills {
		candidates[member] = true
	}
	protected := map[string]bool{}
	for _, other := range cfg.Profiles {
		for _, member := range profiles[other].Skills {
			if candidates[member] {
				protected[member] = true
			}
		}
	}

	// Every non-candidate stays enabled. Walk requirements to a fixed point so a
	// protected member also protects any of its requirements in the removed set.
	for changed := true; changed; {
		changed = false
		for _, enabled := range cfg.Enabled {
			if candidates[enabled] && !protected[enabled] {
				continue
			}
			for _, required := range deps[enabled].Requires {
				if candidates[required] && !protected[required] {
					protected[required] = true
					changed = true
				}
			}
		}
	}
	for _, member := range slices.Sorted(slices.Values(p.Skills)) {
		if protected[member] {
			continue
		}
		if at := slices.Index(cfg.Enabled, member); at >= 0 {
			cfg.Enabled = slices.Delete(cfg.Enabled, at, at+1)
			removed = append(removed, member)
		}
	}
	return removed, true
}

// removeProfile turns a profile off with the usual disable rules, then drops its
// definition, so deleting one never leaves skills that no profile can turn off.
func removeProfile(cfg *Config, name string, profiles map[string]profile, deps map[string]dep) []string {
	removed, _ := disableProfile(cfg, name, profiles[name], profiles, deps)
	delete(profiles, name)
	return removed
}

// resyncProfile keeps this machine in step when an enabled profile's members
// change: new members come on with their requirements, and dropped ones go off
// under the disable rules. profiles must already hold the updated definition. It
// works on a copy, so a failed enable leaves cfg untouched.
func resyncProfile(cfg *Config, name string, old, updated profile, profiles map[string]profile, deps map[string]dep, skillsDir string) (added, removed []string, err error) {
	if !slices.Contains(cfg.Profiles, name) {
		return nil, nil, nil
	}
	var gone []string
	for _, member := range old.Skills {
		if !slices.Contains(updated.Skills, member) {
			gone = append(gone, member)
		}
	}
	c := *cfg
	c.Enabled = slices.Clone(cfg.Enabled)
	c.Profiles, c.Seen = slices.Clone(cfg.Profiles), slices.Clone(cfg.Seen)
	removed, _ = disableProfile(&c, name, profile{Skills: gone}, profiles, deps)
	if added, _, err = enableProfile(&c, name, updated, deps, skillsDir); err != nil {
		return nil, nil, err
	}
	*cfg = c
	// a dropped member that a new one requires went off and came back on
	both := func(s string) bool { return slices.Contains(added, s) && slices.Contains(removed, s) }
	return slices.DeleteFunc(slices.Clone(added), both), slices.DeleteFunc(removed, both), nil
}
