package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// hooksRepo is a clone with two skills that each register one SessionStart hook.
func hooksRepo(t *testing.T) (home, repo string) {
	t.Helper()
	home, repo = setup(t, "ponytail", "other")
	write(t, filepath.Join(repo, "hooks", "hooks.json"), `{
		"SessionStart": [
			{"skill": "ponytail", "matcher": "startup", "hooks": [{"type": "command", "command": "node \"{{ROOT}}/hooks/ponytail-activate.js\""}]},
			{"skill": "other", "hooks": [{"type": "command", "command": "node \"{{ROOT}}/hooks/other.js\""}]}
		],
		"UserPromptSubmit": [
			{"skill": "ponytail", "hooks": [{"type": "command", "command": "node \"{{ROOT}}/hooks/ponytail-mode-tracker.js\""}]}
		]
	}`)
	return home, repo
}

// foreign is a hook config holding two entries of another tool, in this order.
const foreign = `{
	"model": "opus",
	"hooks": {
		"SessionStart": [
			{"matcher": "startup", "hooks": [{"type": "command", "command": "first-tool"}]},
			{"hooks": [{"type": "command", "command": "second-tool"}]}
		]
	}
}`

// commands lists the command of every hook entry of a config file, per event, in order.
func commands(t *testing.T, path string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	got := map[string][]string{}
	for event, entries := range doc.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				got[event] = append(got[event], h.Command)
			}
		}
	}
	return got
}

func wantCommands(t *testing.T, path string, want map[string][]string) {
	t.Helper()
	got := commands(t, path)
	if len(got) != len(want) {
		t.Fatalf("%s hooks = %v, want %v", path, got, want)
	}
	for event, cmds := range want {
		if !slices.Equal(got[event], cmds) {
			t.Fatalf("%s %s = %v, want %v", path, event, got[event], cmds)
		}
	}
}

func TestApplyRegistersHooksOfEnabledSkillsOnly(t *testing.T) {
	home, repo := hooksRepo(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	write(t, claude, foreign)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil { // Codex is installed here
		t.Fatal(err)
	}
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"ponytail"}})

	skillset(t, "apply")

	activate := "node \"" + filepath.ToSlash(repo) + "/hooks/ponytail-activate.js\""
	tracker := "node \"" + filepath.ToSlash(repo) + "/hooks/ponytail-mode-tracker.js\""
	// other tools' entries keep their content and their order, ours come last
	wantCommands(t, claude, map[string][]string{
		"SessionStart":     {"first-tool", "second-tool", activate},
		"UserPromptSubmit": {tracker},
	})
	wantCommands(t, filepath.Join(home, ".codex", "hooks.json"), map[string][]string{
		"SessionStart":     {activate},
		"UserPromptSubmit": {tracker},
	})

	var doc map[string]any
	data, err := os.ReadFile(claude)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "opus" {
		t.Fatalf("settings = %v, want the other settings kept", doc)
	}
	if strings.Contains(string(data), `"skill"`) {
		t.Fatalf("settings = %s, want the skill field dropped", data)
	}
	if backup, err := os.ReadFile(claude + ".bak"); err != nil || string(backup) != foreign {
		t.Fatalf("backup = %q (%v), want the file as it was", backup, err)
	}
	if out := skillset(t, "apply"); strings.Contains(out, "hooks updated") {
		t.Fatalf("second apply said %q, want nothing to do", out)
	}
}

func TestApplyRemovesHooksOfADisabledSkill(t *testing.T) {
	home, repo := hooksRepo(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	write(t, claude, foreign)
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"ponytail"}})
	skillset(t, "apply")

	skillset(t, "disable", "ponytail")
	skillset(t, "apply")

	wantCommands(t, claude, map[string][]string{"SessionStart": {"first-tool", "second-tool"}})
}

func TestApplyRemovesHooksOfATargetThatIsOff(t *testing.T) {
	home, repo := hooksRepo(t)
	codex := filepath.Join(home, ".codex", "hooks.json")
	write(t, codex, `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "first-tool"}]}]}}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"ponytail"}})
	skillset(t, "apply")

	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"ponytail"}})
	skillset(t, "apply")

	wantCommands(t, codex, map[string][]string{"SessionStart": {"first-tool"}})
}

func TestApplyDoesNotDuplicateTheHooksLinkShWrote(t *testing.T) {
	home, repo := hooksRepo(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	root := filepath.ToSlash(repo)
	// exactly what scripts/link.sh registered on this machine, without a skill field
	write(t, claude, `{"hooks": {
		"SessionStart": [
			{"hooks": [{"type": "command", "command": "first-tool"}]},
			{"matcher": "startup", "hooks": [{"type": "command", "command": "node \"`+root+`/hooks/ponytail-activate.js\""}]}
		],
		"UserPromptSubmit": [
			{"hooks": [{"type": "command", "command": "node \"`+root+`/hooks/ponytail-mode-tracker.js\""}]}
		]
	}}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"ponytail"}})

	skillset(t, "apply")

	wantCommands(t, claude, map[string][]string{
		"SessionStart":     {"first-tool", "node \"" + root + "/hooks/ponytail-activate.js\""},
		"UserPromptSubmit": {"node \"" + root + "/hooks/ponytail-mode-tracker.js\""},
	})
}

func TestApplyWritesNoHookConfigWhenThereAreNoHooks(t *testing.T) {
	home, repo := hooksRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}})

	skillset(t, "apply")

	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err == nil {
		t.Fatal("want no hook config created for an agent that has none")
	}
}

func TestHookBackupKeepsThePermissionsOfTheOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows has no permission bits to keep: Go reports 0666 for anything that
		// isn't read-only, so there is nothing here to assert. Access is an ACL
		// question there, which os.Chmod can't reach either way.
		t.Skip("no Unix permission bits on Windows")
	}
	home, repo := hooksRepo(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	write(t, claude, foreign)
	if err := os.Chmod(claude, 0o600); err != nil { // as a config holding an API key would be
		t.Fatal(err)
	}
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"ponytail"}})

	skillset(t, "apply")

	for _, path := range []string{claude, claude + ".bak"} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s is %v, want the original 0600", filepath.Base(path), st.Mode().Perm())
		}
	}
}

func TestAHookConfigItCannotParseChangesNothingAtAll(t *testing.T) {
	home, repo := hooksRepo(t)
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"hooks": []}`) // a shape we don't expect
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"ponytail"}})

	if err := run([]string{"apply"}, io.Discard, strings.NewReader("")); err == nil {
		t.Fatal("want an error for a hooks block we can't read")
	}

	for _, dir := range []string{".claude", ".agents"} {
		if got := entries(t, filepath.Join(home, dir, "skills")); len(got) != 0 {
			t.Fatalf("%s = %v, want nothing linked before the configs were checked", dir, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err == nil {
		t.Fatal("the second target should not have been written either")
	}
}

func TestNoHookFileForAnAgentThatIsNotInstalled(t *testing.T) {
	home, repo := hooksRepo(t)
	write(t, filepath.Join(home, ".claude", "settings.json"), foreign)
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"ponytail"}})

	out := skillset(t, "apply")

	if _, err := os.Stat(filepath.Join(home, ".codex")); err == nil {
		t.Fatal("skillset must not create an agent folder of its own")
	}
	if !strings.Contains(out, "skip hooks: no") {
		t.Fatalf("output = %q, want it to say the agent isn't here", out)
	}
}
