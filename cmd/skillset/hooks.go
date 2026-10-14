package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// hookEvents is the `hooks` map of an agent's config: event name -> entries. An entry
// is kept as a plain map so fields we know nothing about survive a rewrite.
type hookEvents map[string][]map[string]any

// repoHooks reads hooks/hooks.json and returns the entries whose skill is enabled,
// with {{ROOT}} expanded. Each entry names its skill in a "skill" field, which is
// dropped here: the agent configs never see it.
func repoHooks(repo string, enabled []string) (hookEvents, error) {
	data, err := os.ReadFile(filepath.Join(repo, "hooks", "hooks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Forward slashes on purpose: node on Windows can't open /c/... paths, and this is
	// the form scripts/link.sh wrote, so its entries are recognised as ours.
	data = bytes.ReplaceAll(data, []byte("{{ROOT}}"), []byte(filepath.ToSlash(repo)))
	var all hookEvents
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("hooks/hooks.json: %w", err)
	}
	ours := hookEvents{}
	for event, list := range all {
		for _, entry := range list {
			skill, _ := entry["skill"].(string)
			if skill == "" {
				return nil, fmt.Errorf("hooks/hooks.json: a %s entry names no skill", event)
			}
			if !slices.Contains(enabled, skill) {
				continue
			}
			delete(entry, "skill")
			ours[event] = append(ours[event], entry)
		}
	}
	return ours, nil
}

// syncHooks makes one agent's hook config match the selection. Our entries go after the
// other tools' ones (Codex keys hook trust by position) and are found again by their path
// into the repo. The file is backed up before it is written, and only written when it changes.
func syncHooks(path, repo string, enabled []string, out io.Writer, dry bool) error {
	ours, err := repoHooks(repo, enabled)
	if err != nil {
		return err
	}
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if len(ours) == 0 {
			return nil // don't create a config for an agent that has none
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	events := hookEvents{}
	if raw, ok := doc["hooks"]; ok {
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &events); err != nil {
			return fmt.Errorf("%s: unexpected hooks shape: %w", path, err)
		}
	}
	before, _ := json.Marshal(doc)

	mine := filepath.ToSlash(repo) + "/hooks/"
	for event, list := range events {
		kept := []map[string]any{}
		for _, entry := range list {
			if !ourEntry(entry, mine) {
				kept = append(kept, entry)
			}
		}
		events[event] = kept
	}
	for event, list := range ours {
		events[event] = append(events[event], list...)
	}
	for event, list := range events {
		if len(list) == 0 {
			delete(events, event)
		}
	}
	if _, had := doc["hooks"]; had || len(events) > 0 {
		doc["hooks"] = events
	}

	after, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}
	if dry {
		fmt.Fprintf(out, "update hooks in %s\n", path)
		return nil
	}
	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if data != nil {
		if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, append(pretty, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "hooks updated in %s\n", path)
	return nil
}

// ourEntry reports whether a hook entry runs a command out of this repo's hooks folder.
func ourEntry(entry map[string]any, prefix string) bool {
	hooks, _ := entry["hooks"].([]any)
	for _, h := range hooks {
		hook, _ := h.(map[string]any)
		cmd, _ := hook["command"].(string)
		if strings.Contains(cmd, prefix) {
			return true
		}
	}
	return false
}
