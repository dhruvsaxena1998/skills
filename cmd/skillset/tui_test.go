package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// tuiModel builds a screen over a two-source repo: app -> core -> base in "s",
// with "nice" optional for app and living in the other source.
func tuiModel(t *testing.T) (*model, string, string) {
	t.Helper()
	home, repo := setup(t, "app", "core", "base", "nice")
	write(t, filepath.Join(repo, "sources.json"), `{
		"s": {"repo": "https://example.test/s", "ref": "main", "path": "skills",
		      "skills": ["app", "base", "core"],
		      "deps": {"app": {"requires": ["core"], "optional": ["nice"]},
		               "core": {"requires": ["base"]}, "base": {}}},
		"t": {"repo": "https://example.test/t", "ref": "main", "path": "skills",
		      "skills": ["nice"], "deps": {"nice": {}}}
	}`)
	cfg := &Config{Repo: repo, Targets: []string{"claude"}}
	save(t, cfg)
	m, err := newModel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m, home, repo
}

// goTo puts the cursor on the row with that name.
func goTo(t *testing.T, m *model, name string) {
	t.Helper()
	for i, idx := range m.shown {
		if m.rows[idx].name == name {
			m.cursor = i
			return
		}
	}
	t.Fatalf("no row named %q", name)
}

func press(m *model, key string) {
	if key == " " {
		m.key(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
		return
	}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func TestTUITogglingASkillPullsInWhatItRequires(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")

	press(m, " ")

	if !slices.Equal(slices.Sorted(slices.Values(m.cfg.Enabled)), []string{"app", "base", "core"}) {
		t.Fatalf("enabled = %v, want the whole chain", m.cfg.Enabled)
	}
	if !strings.Contains(m.cascade, "also on, it requires them: base, core") {
		t.Fatalf("cascade = %q, want the chain explained", m.cascade)
	}
	if !strings.Contains(m.View(), "app is enabled without nice") {
		t.Fatal("the optional warning should be on screen")
	}

	// off again: what app required stays, since nothing else requires app
	press(m, " ")
	if got := slices.Sorted(slices.Values(m.cfg.Enabled)); !slices.Equal(got, []string{"base", "core"}) {
		t.Fatalf("enabled = %v, want app off and its requirements left alone", got)
	}
}

func TestTUITogglingASourceTogglesItsSkills(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "s")

	press(m, " ")

	if got := slices.Sorted(slices.Values(m.cfg.Enabled)); !slices.Equal(got, []string{"app", "base", "core"}) {
		t.Fatalf("enabled = %v, want every skill of the source", got)
	}
	goTo(t, m, "s")
	press(m, " ")
	if len(m.cfg.Enabled) != 0 {
		t.Fatalf("enabled = %v, want the source off", m.cfg.Enabled)
	}
}

func TestTUIFilterKeepsOnlyMatchingSkills(t *testing.T) {
	m, _, _ := tuiModel(t)

	press(m, "/")
	for _, r := range "cor" {
		press(m, string(r))
	}

	var names []string
	for _, i := range m.shown {
		names = append(names, m.rows[i].name)
	}
	if !slices.Equal(names, []string{"core"}) {
		t.Fatalf("shown = %v, want only the match", names)
	}
}

func TestTUIMarksSkillsYouHaveNotSeenAsNew(t *testing.T) {
	m, _, _ := tuiModel(t)
	m.cfg.Seen = []string{"app", "base", "core"}
	m.build()

	goTo(t, m, "nice")
	if m.rows[m.shown[m.cursor]].note != "new" {
		t.Fatal("an unseen skill should be marked new")
	}
	if slices.Contains(m.cfg.Enabled, "nice") {
		t.Fatal("a new skill must stay off until it is turned on")
	}
}

func TestTUIShowsForeignEntriesAndNeverChangesThem(t *testing.T) {
	m, home, _ := tuiModel(t)
	foreign := filepath.Join(home, ".claude", "skills", "synced")
	write(t, filepath.Join(foreign, "SKILL.md"), "# not ours")
	m.build()

	goTo(t, m, filepath.Join("~", ".claude", "skills", "synced"))
	press(m, " ") // toggling it must do nothing at all
	if m.dirty {
		t.Fatal("a foreign entry is not ours to change")
	}

	m.cfg.Enabled = []string{"base"}
	m.save()
	if _, err := os.Stat(filepath.Join(foreign, "SKILL.md")); err != nil {
		t.Fatalf("the foreign entry should survive a save: %v", err)
	}
}

func TestTUIReviewsThePlanBeforeTouchingAnything(t *testing.T) {
	m, home, repo := tuiModel(t)
	goTo(t, m, "base")
	press(m, " ")

	press(m, "s")

	link := filepath.Join(home, ".claude", "skills", "base")
	if len(m.review) == 0 || !strings.Contains(strings.Join(m.review, "\n"), "link "+link) {
		t.Fatalf("review = %v, want the link it would make", m.review)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("review must not write anything")
	}

	m.save()

	if got := linkDest(link); !samePath(got, filepath.Join(repo, "skills", "base")) {
		t.Fatalf("link -> %q, want the skill in the repo", got)
	}
	if m.dirty || m.review != nil {
		t.Fatal("a save clears the review and the unsaved mark")
	}
	cfg, err := loadConfig()
	if err != nil || !slices.Contains(cfg.Seen, "nice") {
		t.Fatalf("saving should record every skill as seen: %v, %v", cfg, err)
	}
}

// TestTUIRenders is the smoke test for the screen itself: run it with -v to look at it.
func TestTUIRenders(t *testing.T) {
	m, home, _ := tuiModel(t)
	write(t, filepath.Join(home, ".claude", "skills", "synced", "SKILL.md"), "# from claude.ai")
	m.cfg.Enabled = []string{"base", "core"}
	m.cfg.Seen = []string{"app", "base", "core"}
	m.status = "up to date with origin"
	m.build()
	goTo(t, m, "app")

	view := m.View()

	for _, want := range []string{"AGENT TARGETS", "claude", "[x] base", "[ ] app", "new", "NOT MANAGED HERE", "requires: core"} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen is missing %q", want)
		}
	}
	t.Log("\n" + view)
}
