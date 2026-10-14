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
			m.mark()
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

	for _, want := range []string{
		"skillset", "skills", "targets", "changes", // the tab bar
		"SOURCES", "▌ s", "2/3", // the sources column, with its rule and tally
		"TARGETS", "◉ claude", "○ agents",
		"NOT OURS", "synced",
		"SKILLS", "example.test/s", "◉ base", "○ app", // the skills of the source we are on
		"DETAIL", "requires   core", "optional   nice", // the detail of the skill we are on
		"2/4 on", "1 target", "up to date with origin",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen is missing %q", want)
		}
	}

	// the new badge rides on the skill itself, so it has to be where that skill is
	goTo(t, m, "nice")
	if !strings.Contains(m.View(), "new") {
		t.Error("an unseen skill should be badged on screen")
	}
	t.Log("\n" + view)
}

// TestTUIMovesBetweenColumns covers the one thing the flat list didn't have: a
// skill belongs to the source left of it, and only its siblings are a step away.
func TestTUIMovesBetweenColumns(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")

	press(m, "h") // out of the skills column, onto the source it belongs to
	if r, _ := m.at(); r.kind != rowSource || r.name != "s" {
		t.Fatalf("h landed on %+v, want the source s", r)
	}

	press(m, "j") // the first column moves source to source, never into a skill
	if r, _ := m.at(); r.name != "t" {
		t.Fatalf("j landed on %q, want the next source", r.name)
	}
	if m.source != "t" {
		t.Fatalf("the skills column is showing %q, want the source under the cursor", m.source)
	}

	press(m, "l") // into that source's skills
	if r, _ := m.at(); r.kind != rowSkill || r.name != "nice" {
		t.Fatalf("l landed on %+v, want the first skill of t", r)
	}
	press(m, "j") // t has one skill, so down must stay put rather than fall into s
	if r, _ := m.at(); r.name != "nice" {
		t.Fatalf("j left the column, landing on %q", r.name)
	}
}

// TestTUITabBarGoesSomewhere: a tab that doesn't respond to a key is decoration.
func TestTUITabBarGoesSomewhere(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")
	if m.tab() != tabSkills {
		t.Fatal("a skill under the cursor is the skills tab")
	}

	m.key(tea.KeyMsg{Type: tea.KeyTab})
	if r, _ := m.at(); r.kind != rowTarget || m.tab() != tabTargets {
		t.Fatalf("tab landed on %+v, want the first target", r)
	}

	m.key(tea.KeyMsg{Type: tea.KeyTab})
	if m.review == nil || m.tab() != tabChanges {
		t.Fatal("tab from the targets should open the changes")
	}

	m.key(tea.KeyMsg{Type: tea.KeyTab})
	if r, _ := m.at(); m.review != nil || r.kind != rowSkill {
		t.Fatalf("tab should come back round to the skills, got %+v", r)
	}
}

func TestTUIReadsADescriptionWrittenAsAYAMLBlock(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "skills", "base", "SKILL.md"), `---
name: base
description: >
  Lazy senior dev mode: the smallest change that fully solves the task.
  Levels: lite, full, ultra.
---

# base
`)

	if got := m.descriptionOf("base"); !strings.HasPrefix(got, "Lazy senior dev mode") || !strings.Contains(got, "Levels:") {
		t.Fatalf("description = %q, want both lines of the block", got)
	}
}
