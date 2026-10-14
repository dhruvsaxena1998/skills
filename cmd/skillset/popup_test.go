package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func esc(m *model) { m.key(tea.KeyMsg{Type: tea.KeyEsc}) }

// tap presses a key that carries no text, which press() cannot spell.
func tap(m *model, k tea.KeyType) { m.key(tea.KeyMsg{Type: k}) }

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain is the text without its colours: glamour styles word by word, so a
// sentence on screen is never one run of bytes.
func plain(s string) string { return sgr.ReplaceAllString(s, "") }

// TestPopupReadsASkillAndEscGoesBack: the point of the overlay is reading the
// file without losing your place in the list.
func TestPopupReadsASkillAndEscGoesBack(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "skills", "core", "SKILL.md"), "# core\n\nthe engine room.\n")
	goTo(t, m, "core")
	where := m.cursor

	m.key(tea.KeyMsg{Type: tea.KeyEnter})

	if m.pop != "skill:core" || !strings.Contains(plain(strings.Join(m.popBody, "\n")), "the engine room") {
		t.Fatalf("the overlay is %q with %v, want the rendered file", m.pop, m.popBody)
	}
	if !strings.Contains(plain(m.View()), "the engine room") {
		t.Fatal("the overlay should be on screen over the columns")
	}

	m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.popTop != 1 {
		t.Fatalf("popTop = %d, want the overlay scrolled", m.popTop)
	}

	esc(m)
	if m.pop != "" || m.cursor != where {
		t.Fatalf("esc left pop=%q cursor=%d, want the list back exactly where it was", m.pop, m.cursor)
	}
}

// TestPopupSaysSoWhenTheFileIsUnreadable: a skill with no SKILL.md must not give
// a crash or an empty box.
func TestPopupSaysSoWhenTheFileIsUnreadable(t *testing.T) {
	m, _, repo := tuiModel(t)
	if err := os.Remove(filepath.Join(repo, "skills", "base", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "base")

	m.key(tea.KeyMsg{Type: tea.KeyEnter})

	if body := plain(strings.Join(m.popBody, "\n")); !strings.Contains(body, "can't read") {
		t.Fatalf("overlay = %q, want it to say the file could not be read", body)
	}
	if !strings.Contains(plain(m.View()), "can't read") {
		t.Fatal("the screen should carry the message too")
	}
}

// TestPopupOnlyOpensOnASkill: a source or a target has no SKILL.md behind it.
func TestPopupOnlyOpensOnASkill(t *testing.T) {
	m, _, _ := tuiModel(t)
	goTo(t, m, "s")
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pop != "" {
		t.Fatalf("enter on a source opened %q", m.pop)
	}
}

// TestKeysOverlayListsEveryBoundKeyAndSearches covers the reason the table
// exists: the rail is one line of it, and nothing is listed in only one place.
func TestKeysOverlayListsEveryBoundKey(t *testing.T) {
	m, _, _ := tuiModel(t)

	press(m, "?")

	body := plain(strings.Join(m.popBody, "\n"))
	for _, want := range []string{"space", "x", "h l", "enter", "/", "?", "s", "p", "q", "esc", "SETTINGS"} {
		if !strings.Contains(body, want) {
			t.Errorf("the keys overlay never mentions %q", want)
		}
	}
	for _, b := range bindings {
		if b.rail != "" && !strings.Contains(railLine(b.mode), b.rail) {
			t.Errorf("the %s rail is missing %q, which the table puts on it", b.mode, b.rail)
		}
	}
	// tab steps through the flow again, so the selection rail must carry it and the
	// rails for a filter and the review, which tab does not reach, must not
	if !strings.Contains(railLine("selection"), "tab") {
		t.Error("tab steps through the flow but the selection rail never says so")
	}
	for _, mode := range []string{"filtering", "review"} {
		if strings.Contains(railLine(mode), "tab") {
			t.Errorf("tab is not a %s key and must not be on its rail", mode)
		}
	}

	for _, r := range "pull" {
		press(m, string(r))
	}
	body = plain(strings.Join(m.popBody, "\n"))
	if !strings.Contains(body, "pull the repo") || strings.Contains(body, "apply the plan") {
		t.Fatalf("searching for pull gave:\n%s", body)
	}

	m.key(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.query != "pul" {
		t.Fatalf("query = %q, want backspace to take one off", m.query)
	}
	press(m, "zzz")
	if !strings.Contains(plain(strings.Join(m.popBody, "\n")), "no key matches") {
		t.Fatal("a search that matches nothing should say so")
	}

	// esc undoes one thing per press: the search first, the popup only once there
	// is no search left to take back
	esc(m)
	if m.pop != "keys" || m.query != "" {
		t.Fatalf("the first esc left pop=%q query=%q, want the search cleared and the popup up", m.pop, m.query)
	}
	esc(m)
	if m.pop != "" {
		t.Fatalf("the second esc left pop=%q, want the popup closed", m.pop)
	}
}

// TestKeysOverlayTakesQAsALetter: q quits the list, but inside the search box it
// is a character like any other.
func TestKeysOverlayTakesQAsALetter(t *testing.T) {
	m, _, _ := tuiModel(t)
	press(m, "?")

	press(m, "q")

	if m.pop != "keys" || m.query != "q" {
		t.Fatalf("q left pop=%q query=%q, want it typed into the search", m.pop, m.query)
	}
}

// TestKeysOverlayPicksAThemeAndWritesIt: a theme is this machine's look, so it
// lands at once rather than riding the unsaved-changes flow.
func TestKeysOverlayPicksAThemeAndWritesIt(t *testing.T) {
	defer useTheme(defaultTheme)
	m, _, _ := tuiModel(t)
	press(m, "?")

	want := themeNames()[1] // 1 is the default; 2 is the next one along
	press(m, "2")

	if m.cfg.Theme != want || ui.theme.Accent != themes[want].Accent {
		t.Fatalf("theme = %q and the screen is %v, want %q on both", m.cfg.Theme, ui.theme.Accent, want)
	}
	if m.dirty {
		t.Fatal("a theme is not an unsaved selection change")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Theme != want {
		t.Fatalf("the config on disk says %q (%v), want it written as it was pressed", cfg.Theme, err)
	}
	if !strings.Contains(plain(strings.Join(m.popBody, "\n")), want) {
		t.Fatal("the picker should still list the themes after a pick")
	}

	press(m, "9") // no ninth theme: nothing moves rather than a panic
	if m.cfg.Theme != want {
		t.Fatalf("theme = %q after a digit with no theme behind it", m.cfg.Theme)
	}
}

// TestSettingsRowChangesUnderTheCursor: the overlay used to be the one screen
// with its own interaction model. The setting under the cursor now moves on
// space, enter and the arrows, like every other row in the app.
func TestSettingsRowChangesUnderTheCursor(t *testing.T) {
	defer useTheme(defaultTheme)
	m, _, _ := tuiModel(t)
	press(m, "?")
	if settingAt != 0 {
		t.Fatalf("the overlay opened with the cursor at %d, want it on the first setting", settingAt)
	}

	names := themeNames()
	press(m, " ")
	if m.cfg.Theme != names[1] || ui.theme.Accent != themes[names[1]].Accent {
		t.Fatalf("space gave theme %q on a %v screen, want %q on both", m.cfg.Theme, ui.theme.Accent, names[1])
	}
	tap(m, tea.KeyRight)
	if m.cfg.Theme != names[2] {
		t.Fatalf("theme = %q, want → to step to %q", m.cfg.Theme, names[2])
	}
	tap(m, tea.KeyLeft)
	if m.cfg.Theme != names[1] {
		t.Fatalf("theme = %q, want ← to step back to %q", m.cfg.Theme, names[1])
	}
	press(m, "enter")
	if m.cfg.Theme != names[2] {
		t.Fatalf("theme = %q, want enter to step to %q", m.cfg.Theme, names[2])
	}

	for range names { // round the end and back to where it was
		tap(m, tea.KeyRight)
	}
	if m.cfg.Theme != names[2] {
		t.Fatalf("theme = %q after a full lap, want %q", m.cfg.Theme, names[2])
	}
	if m.dirty {
		t.Fatal("a theme is not an unsaved selection change")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Theme != names[2] {
		t.Fatalf("the config on disk says %q (%v), want every step written as it was pressed", cfg.Theme, err)
	}
	if body := plain(strings.Join(m.popBody, "\n")); !strings.Contains(body, "SETTINGS") || !strings.Contains(body, names[2]) {
		t.Fatalf("the settings list should still show the row it just changed:\n%s", body)
	}
}

// TestSettingsCursorStepsIntoTheKeyList: the cursor stops past the last setting,
// on the list, which is the only thing in the overlay long enough to scroll.
func TestSettingsCursorStepsIntoTheKeyList(t *testing.T) {
	defer useTheme(defaultTheme)
	m, _, _ := tuiModel(t)
	press(m, "?")

	tap(m, tea.KeyDown)
	if settingAt != len(settings) {
		t.Fatalf("settingAt = %d after one ↓, want the cursor off the settings and on the list", settingAt)
	}
	tap(m, tea.KeyDown)
	if m.popTop != 1 {
		t.Fatalf("popTop = %d, want the list to scroll once the cursor is on it", m.popTop)
	}
	tap(m, tea.KeyUp)
	tap(m, tea.KeyUp)
	if settingAt != 0 || m.popTop != 0 {
		t.Fatalf("settingAt = %d popTop = %d, want ↑ back to the top of the list and then onto the setting", settingAt, m.popTop)
	}

	press(m, "tab")
	if settingAt != len(settings) {
		t.Fatalf("tab left the cursor at %d, want it on the list", settingAt)
	}
	press(m, "tab")
	if settingAt != 0 {
		t.Fatalf("tab left the cursor at %d, want it round to the first setting", settingAt)
	}
	press(m, " ") // nothing to change while the cursor is on the list is not a panic
	tap(m, tea.KeyDown)
	press(m, " ")
	press(m, "enter")
}

// TestSettingsSearchKeepsSpaceAsText: typing still filters the key list, so a
// space mid-search is a character and only the keys that are never text reach
// the setting.
func TestSettingsSearchKeepsSpaceAsText(t *testing.T) {
	defer useTheme(defaultTheme)
	m, _, _ := tuiModel(t)
	press(m, "?")
	before := m.themeName()

	press(m, "the")
	press(m, " ")
	press(m, "repo")

	if m.query != "the repo" {
		t.Fatalf("query = %q, want the space typed into the search", m.query)
	}
	if m.themeName() != before {
		t.Fatalf("the search changed the theme to %q", m.themeName())
	}
	body := plain(strings.Join(m.popBody, "\n"))
	if !strings.Contains(body, "pull the repo") || strings.Contains(body, "apply the plan") {
		t.Fatalf("searching for %q gave:\n%s", m.query, body)
	}

	tap(m, tea.KeyRight)
	if m.themeName() == before {
		t.Fatal("the arrows are not text, so they should still reach the setting while a search is up")
	}
	press(m, "2")
	if m.themeName() != themeNames()[1] {
		t.Fatalf("theme = %q, want the digit to stay a shortcut straight to it", m.themeName())
	}
}

// TestPopupTakesTheWholeWidthAndRewraps: the owner's SKILL.md sat in 80 columns
// on a wide screen, and stayed at that width when the terminal changed.
func TestPopupTakesTheWholeWidthAndRewraps(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "skills", "core", "SKILL.md"),
		"# core\n\n"+strings.Repeat("the engine room runs everything else. ", 20)+"\n")
	m.width, m.height = 200, 50
	goTo(t, m, "core")
	m.key(tea.KeyMsg{Type: tea.KeyEnter})

	if w := m.popWidth(); w < 190 {
		t.Fatalf("popWidth = %d on a 200-column screen, want the box to use it", w)
	}
	wide := widest(m.popBody)
	if wide <= 140 {
		t.Fatalf("the widest line is %d columns on a 200-column screen, want the prose uncapped", wide)
	}

	m.popTop = 3
	m.width = 60
	m.resizePop()

	if narrow := widest(m.popBody); narrow >= wide {
		t.Fatalf("the lines are %d columns after shrinking to 60, were %d", narrow, wide)
	}
	if m.popTop != 3 {
		t.Fatalf("popTop = %d, want a resize to leave your place alone", m.popTop)
	}
	if !strings.Contains(plain(strings.Join(m.popBody, "\n")), "engine room") {
		t.Fatal("the re-wrapped body should still be the same file")
	}
}

// widest is the longest line once the colours are off them.
func widest(body []string) int {
	n := 0
	for _, line := range body {
		n = max(n, len([]rune(strings.TrimRight(plain(line), " "))))
	}
	return n
}

// TestEveryThemeFillsEveryField: a half-filled palette renders black on black.
func TestEveryThemeFillsEveryField(t *testing.T) {
	defer useTheme(defaultTheme)
	for name, th := range themes {
		useTheme(name)
		for field, c := range map[string]string{"Accent": string(th.Accent), "AccentAlt": string(th.AccentAlt),
			"OK": string(th.OK), "Warn": string(th.Warn), "Muted": string(th.Muted),
			"Text": string(th.Text), "Inverse": string(th.Inverse), "BarInk": string(th.BarInk)} {
			if len(c) != 7 || c[0] != '#' {
				t.Errorf("theme %q has %s = %q, want a hex colour", name, field, c)
			}
		}
		if len(th.SourceRules) < 2 {
			t.Errorf("theme %q has %d source rules, want enough to tell two sources apart", name, len(th.SourceRules))
		}
	}
	if !slices.Contains(themeNames(), defaultTheme) {
		t.Fatal("the picker must list the default")
	}
}
