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

func press(m *model, key string) tea.Cmd {
	switch key {
	case " ":
		return m.key(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	case "esc":
		return m.key(tea.KeyMsg{Type: tea.KeyEsc})
	case "tab":
		return m.key(tea.KeyMsg{Type: tea.KeyTab})
	case "shift+tab":
		return m.key(tea.KeyMsg{Type: tea.KeyShiftTab})
	case "enter":
		return m.key(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

// quits is whether that key asked the program to end.
func quits(m *model, key string) bool {
	cmd := press(m, key)
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
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

	m.setStep(tabTargets) // they live in the target folders, so that is where they are listed
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
		"SKILLS", "example.test/s", "[x] base", "[ ] app", // the skills of the source we are on
		"DETAIL", "requires   core", "optional   nice", // the detail of the skill we are on
		"2/4 on", "1 target", "up to date with origin",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the skills step is missing %q", want)
		}
	}

	m.setStep(tabTargets)
	targets := m.View()
	for _, want := range []string{
		"TARGETS", "[x] claude", "[ ] agents",
		"NOT OURS", "synced",
	} {
		if !strings.Contains(targets, want) {
			t.Errorf("the targets step is missing %q", want)
		}
	}
	if strings.Contains(targets, "SOURCES") {
		t.Error("the sources belong to the step before this one")
	}
	m.setStep(tabSkills)

	// the new badge rides on the skill itself, so it has to be where that skill is
	goTo(t, m, "nice")
	if !strings.Contains(m.View(), "new") {
		t.Error("an unseen skill should be badged on screen")
	}
	t.Log("\n" + view + "\n" + targets)
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

// TestTUIStepsThroughTheFlow: the tab bar is the steps, and tab walks them
// forward while esc walks them back.
func TestTUIStepsThroughTheFlow(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")
	if m.tab() != tabSkills {
		t.Fatal("the flow starts on the skills")
	}

	press(m, "tab")
	if m.tab() != tabTargets {
		t.Fatal("tab should step onto the targets")
	}
	if r, _ := m.at(); r.kind != rowTarget {
		t.Fatalf("the cursor is on %+v, want a target row", r)
	}

	press(m, "tab")
	if m.tab() != tabChanges || m.review == nil {
		t.Fatal("the step after the targets is the review")
	}
	press(m, "tab") // the review is the last step, with nowhere forward to go
	if m.review == nil {
		t.Fatal("tab should not have left the review")
	}

	press(m, "esc")
	if m.tab() != tabTargets {
		t.Fatal("esc should come back out of the review")
	}
	press(m, "esc")
	if m.tab() != tabSkills {
		t.Fatal("esc should step back to the skills")
	}
	if r, _ := m.at(); r.kind != rowSource || r.name != "s" {
		t.Fatalf("stepping back landed on %+v, want the source we were reading", r)
	}

	press(m, "s") // and s still jumps straight to the review from anywhere
	if m.review == nil || m.tab() != tabChanges {
		t.Fatal("s opens the review")
	}
	press(m, "q") // q is esc everywhere outside a text field
	if m.review != nil {
		t.Fatal("q should leave the review")
	}
}

// TestTUIQuitsOnlyFromTheFirstStep: both keys dismiss one thing at a time, and
// quit only when there is nothing left to dismiss.
func TestTUIQuitsOnlyFromTheFirstStep(t *testing.T) {
	m, _, _ := tuiModel(t)
	m.setStep(tabTargets)

	if quits(m, "q") {
		t.Fatal("from a later step, q steps back rather than quitting")
	}
	if m.tab() != tabSkills {
		t.Fatal("q should have stepped back to the skills")
	}
	if !quits(m, "q") || !quits(m, "esc") {
		t.Fatal("on the first step, with nothing left to dismiss, both keys quit")
	}

	// a filter is one more thing to dismiss, and it goes before the step does
	press(m, "/")
	press(m, "c")
	press(m, "esc") // the focus first, keeping what is typed
	if m.typing || m.filter != "c" {
		t.Fatalf("typing = %v, filter = %q, want the focus dropped and the text kept", m.typing, m.filter)
	}
	if quits(m, "esc") || m.filter != "" {
		t.Fatalf("the next esc clears the filter, not the app: filter = %q", m.filter)
	}
	if !quits(m, "esc") {
		t.Fatal("with the filter gone there is nothing left to dismiss")
	}
}

// TestTUICountsForeignEntriesOnTheTargetsTab covers #13: the entries are a step
// away now, so the count has to be on screen while you are not looking at them.
func TestTUICountsForeignEntriesOnTheTargetsTab(t *testing.T) {
	m, home, _ := tuiModel(t)
	for _, name := range []string{"synced", "trash"} {
		write(t, filepath.Join(home, ".claude", "skills", name, "SKILL.md"), "# not ours")
	}
	m.build()

	view := m.View()

	if strings.Contains(view, "synced") || strings.Contains(view, "trash") {
		t.Fatal("the skills step is meant not to list them; the test proves nothing")
	}
	if !strings.Contains(view, "2 not ours") {
		t.Errorf("the count of foreign entries is missing from:\n%s", view)
	}
}

// TestTUIListsWhatIsInsideAForeignEntry: the middle column is what the entry
// under the cursor holds, not the skills of whichever source was last selected.
func TestTUIListsWhatIsInsideAForeignEntry(t *testing.T) {
	m, home, _ := tuiModel(t)
	write(t, filepath.Join(home, ".claude", "skills", "synced", "SKILL.md"), "# not ours")
	write(t, filepath.Join(home, ".claude", "skills", "synced", "notes.md"), "notes")
	m.build()
	goTo(t, m, "app") // the source whose skills used to be stuck in that column

	m.setStep(tabTargets)
	goTo(t, m, filepath.Join("~", ".claude", "skills", "synced"))
	view := m.View()

	for _, want := range []string{"CONTENTS", "SKILL.md", "notes.md"} {
		if !strings.Contains(view, want) {
			t.Errorf("the middle column is missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[ ] base") {
		t.Error("a foreign entry does not hold this repo's skills")
	}
}

// TestTUIFilterTogglesWithSpace covers #12: space is the way from a filter to a
// toggle, and the filter survives it.
func TestTUIFilterTogglesWithSpace(t *testing.T) {
	m, _, _ := tuiModel(t)

	press(m, "/")
	for _, r := range "cor" {
		press(m, string(r))
	}
	press(m, " ")

	if m.filter != "cor" {
		t.Fatalf("filter = %q, want the space to toggle rather than land in it", m.filter)
	}
	if !slices.Contains(m.cfg.Enabled, "core") {
		t.Fatalf("enabled = %v, want the match under the cursor turned on", m.cfg.Enabled)
	}
	if !m.typing {
		t.Fatal("a toggle shouldn't drop you out of the filter")
	}
}

// TestTUIFilterTakesQAsText: in a text field letters are text, or you could
// never search for a skill with a q in its name.
func TestTUIFilterTakesQAsText(t *testing.T) {
	m, _, _ := tuiModel(t)

	press(m, "/")
	if quits(m, "q") {
		t.Fatal("q in the filter must type a q, not quit")
	}
	if m.filter != "q" || !m.typing {
		t.Fatalf("filter = %q, typing = %v, want the letter in the filter", m.filter, m.typing)
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

// TestTUIShiftTabStepsBack covers the flow going backwards: tab walks forward
// without wrapping, and shift+tab is the way out of every step, the review too.
func TestTUIShiftTabStepsBack(t *testing.T) {
	m, _, _ := tuiModel(t)

	press(m, "tab")
	press(m, "tab")
	if m.tab() != tabChanges {
		t.Fatal("two tabs should have reached the review")
	}

	press(m, "shift+tab")
	if m.tab() != tabTargets {
		t.Fatalf("shift+tab left the review on %v, want the targets", m.tab())
	}
	press(m, "shift+tab")
	if m.tab() != tabSkills {
		t.Fatalf("shift+tab landed on %v, want the first step", m.tab())
	}
	press(m, "shift+tab") // the first step has nowhere back to go, and must not wrap
	if m.tab() != tabSkills {
		t.Fatalf("shift+tab wrapped round to %v", m.tab())
	}
}

// TestTUIClickSelectsARowWithoutTogglingIt: the pointer moves the cursor, and
// space stays the only way to change what a save would write.
func TestTUIClickSelectsARowWithoutTogglingIt(t *testing.T) {
	m, _, _ := tuiModel(t)
	m.width, m.height = 120, 30
	screen := strings.Split(plain(m.View()), "\n") // a click is mapped against the last draw

	y := bodyTop + 2 // the heading, the first skill, then this one
	if !strings.Contains(screen[y], "base") {
		t.Fatalf("line %d of the screen is %q, want the row this test clicks", y, screen[y])
	}
	m.mouse(tea.MouseMsg{X: m.hit.x2 + 4, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})

	if r, _ := m.at(); r.name != "base" {
		t.Fatalf("the click landed on %q, want the row under the pointer", r.name)
	}
	if len(m.cfg.Enabled) != 0 || m.dirty {
		t.Fatalf("enabled = %v, dirty = %v: a click must never toggle", m.cfg.Enabled, m.dirty)
	}

	m.mouse(tea.MouseMsg{X: m.hit.tabs[0] + 1, Y: tabLine, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.tab() != tabTargets {
		t.Fatalf("clicking the second tab went to %v, want the targets", m.tab())
	}
}

// TestTUIClickFindsTheRowInAScrolledColumn: the hit list is cut the way the
// column is, or every click past the fold would land a few rows out.
func TestTUIClickFindsTheRowInAScrolledColumn(t *testing.T) {
	m, _, repo := tuiModel(t)
	for _, s := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9"} {
		write(t, filepath.Join(repo, "skills", s, "SKILL.md"), "# "+s)
	}
	m.build()
	m.width, m.height = 120, 15
	goTo(t, m, "unlisted") // the loose skills, more of them than the screen has room for
	press(m, "l")
	for range 8 {
		press(m, "j") // far enough down that the column has scrolled
	}
	screen := strings.Split(plain(m.View()), "\n")

	y := bodyTop + 2
	line := screen[y]
	m.mouse(tea.MouseMsg{X: m.hit.x2 + 4, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})

	r, _ := m.at()
	if r.name == "" || !strings.Contains(line, r.name) {
		t.Fatalf("the click landed on %q, and line %d of the screen reads %q", r.name, y, line)
	}
}

// TestTUICascadeIsThePendingStateAndClears covers the rail line: it says what a
// save would do rather than what the last key did, and goes when the cursor does.
func TestTUICascadeIsThePendingStateAndClears(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")

	press(m, " ") // app drags core and base in with it
	if !strings.Contains(m.cascade, "3 skills on") ||
		!strings.Contains(m.cascade, "also on, it requires them: base, core") {
		t.Fatalf("cascade = %q, want the pending state and what the toggle dragged in", m.cascade)
	}

	press(m, " ") // app off again: base and core stay on, so the line has to change
	if !strings.Contains(m.cascade, "2 skills on") || strings.Contains(m.cascade, "app") {
		t.Fatalf("cascade = %q, want what a save would do now, not the last toggle alone", m.cascade)
	}

	press(m, "j")
	if m.cascade != "" {
		t.Fatalf("cascade = %q, want it gone once the cursor left the row", m.cascade)
	}
	if !m.dirty {
		t.Fatal("moving the cursor clears the line, not the changes behind it")
	}
}

// TestTUIDetailLoadsPatchesOffTheDraw covers the stall the owner felt: the log
// behind the detail pane is a command, so no draw ever waits on a subprocess.
func TestTUIDetailLoadsPatchesOffTheDraw(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "app")

	view := plain(m.View())
	if len(m.patches) != 0 {
		t.Fatalf("the draw went off to the repo: patches = %v", m.patches)
	}
	if !strings.Contains(view, "patches") {
		t.Errorf("a field still loading should say so:\n%s", view)
	}

	m.ticking = true // a tick is already in flight, so this is the one command alone
	cmd := m.load()
	if cmd == nil {
		t.Fatal("the skill under the cursor should have its patches on the way")
	}
	msg, ok := cmd().(patchMsg)
	if !ok || msg.name != "app" {
		t.Fatalf("the command sent %#v, want the patches of the row under the cursor", msg)
	}

	m.Update(patchMsg{name: "app", list: []string{"abc1234 patch(app): a local fix"}})
	if view = plain(m.View()); !strings.Contains(view, "1 local") || !strings.Contains(view, "abc1234") {
		t.Errorf("the pane should fill in when the answer arrives:\n%s", view)
	}
	if m.load() != nil {
		t.Fatal("a loaded skill must not ask again")
	}
}
