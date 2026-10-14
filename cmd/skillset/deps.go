package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// dep is one skill's entry in the repo's dependency map: skills it can't work
// without, and skills that only improve it.
type dep struct {
	Requires []string `json:"requires"`
	Optional []string `json:"optional"`
}

// loadDeps reads sources.json and merges every source's map, since dependencies cross sources.
func loadDeps(repo string) (map[string]dep, error) {
	data, err := os.ReadFile(filepath.Join(repo, "sources.json"))
	if err != nil {
		return nil, err
	}
	var sources map[string]struct {
		Deps map[string]dep `json:"deps"`
	}
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("sources.json: %w", err)
	}
	all := map[string]dep{}
	for _, s := range sources {
		maps.Copy(all, s.Deps)
	}
	return all, nil
}

// withRequires collects name and everything it requires, transitively. It fails if a
// required skill is missing from the repo, which makes the whole set unusable.
func withRequires(name string, deps map[string]dep, skillsDir string, got *[]string) error {
	if slices.Contains(*got, name) {
		return nil
	}
	*got = append(*got, name)
	for _, r := range deps[name].Requires {
		if !isSkill(skillsDir, r) {
			return fmt.Errorf("%s requires %s, which isn't in the repo", name, r)
		}
		if err := withRequires(r, deps, skillsDir, got); err != nil {
			return err
		}
	}
	return nil
}

// withDependents collects name and every skill that requires it, transitively.
func withDependents(name string, deps map[string]dep, got *[]string) {
	if slices.Contains(*got, name) {
		return
	}
	*got = append(*got, name)
	for _, s := range slices.Sorted(maps.Keys(deps)) {
		if slices.Contains(deps[s].Requires, name) {
			withDependents(s, deps, got)
		}
	}
}

// warnings lists each enabled skill that is running without an optional dependency.
func warnings(cfg *Config, deps map[string]dep) []string {
	var out []string
	for _, name := range slices.Sorted(slices.Values(cfg.Enabled)) {
		var missing []string
		for _, o := range deps[name].Optional {
			if !slices.Contains(cfg.Enabled, o) {
				missing = append(missing, o)
			}
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("warning: %s is enabled without %s (optional)", name, strings.Join(missing, ", ")))
		}
	}
	return out
}
