package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// binding is one key the TUI answers to. This table is the only list of them:
// the keys overlay prints it and the rail takes its line out of it.
type binding struct {
	mode string // the screen it belongs to, printed as a heading when it changes
	keys string
	does string
	rail string // its short form on the rail, empty when it doesn't fit
}

var bindings = []binding{
	{"selection", "space · x", "turn a skill, profile, source or target on or off", "space toggle"},
	{"selection", "n", "create a new profile: name, description, and a checklist of skills", "n new profile"},
	{"selection", "e", "edit or rename the profile under the cursor", "e edit profile"},
	{"selection", "d", "delete the profile under the cursor (press twice; applied on save)", "d delete"},
	{"selection", "↑ ↓ · k j", "move inside a column", ""},
	{"selection", "← → · h l", "on the skills step, between a source or profile and its skills", "←→ columns"},
	{"selection", "enter", "read the skill's SKILL.md", ""},
	{"selection", "tab", "the next step: targets, then the review", "tab next step"},
	{"selection", "shift+tab", "the step before", "shift+tab back"},
	{"selection", "/", "filter the skills by name", "/ filter"},
	{"selection", "?", "the settings and this list", "? keys"},
	{"selection", "s", "review what a save would change", "s review"},
	{"selection", "p", "pull the repo", "p pull"},
	{"selection", "u", "sync the source under the cursor from its upstream", "u sync"},
	{"selection", "U", "sync every source from its upstream", ""},
	{"selection", "esc · q", "go back one: the filter, then the step, then quit (asking once when something is unsaved)", "esc back"},
	{"selection", "ctrl+c", "quit at once", ""},
	{"selection", "mouse", "click a row or a tab to go to it; the wheel scrolls the column under the pointer", ""},

	{"filtering", "space", "toggle the row under the cursor", "space toggles"},
	{"filtering", "↑ ↓", "move the cursor without leaving the filter", "↑↓ move"},
	{"filtering", "enter", "keep the filter and give the keys back to the list", "enter keeps the filter"},
	{"filtering", "esc", "give the keys back, keeping the filter; press it again to clear", "esc unfocuses"},
	{"filtering", "letters", "text, q included, so a skill with a q in it is searchable", ""},

	{"review", "enter", "apply the plan", "enter applies this"},
	{"review", "esc · q · shift+tab", "go back", "esc goes back"},

	{"overlay", "↑ ↓ · tab", "move between the settings and the key list, which scrolls", "↑↓ move"},
	// not on the rail: the settings carry their own keys, and a SKILL.md shares
	// this rail line without having any of them
	{"overlay", "space · ←→", "change the setting under the cursor, enter too", ""},
	{"overlay", "1 … 9", "pick that theme straight off, written as you press it", ""},
	{"overlay", "k j · wheel", "scroll a SKILL.md, where letters are not a search", ""},
	{"overlay", "esc · q", "clear the search, then close it where you were; in this list q is text, so only esc", "esc goes back"},
}

// railLine is a screen's one line of keys, taken from the table so the rail and
// the overlay cannot drift apart.
func railLine(mode string) string {
	var out []string
	for _, b := range bindings {
		if b.mode == mode && b.rail != "" {
			out = append(out, b.rail)
		}
	}
	return strings.Join(out, " · ")
}

// popKey is the overlay's own keys. Both popups scroll the same way; only the
// keys one takes typing and carries the settings, since a SKILL.md has nothing
// to search, and only there does esc have a search to take back first.
func (m *model) popKey(msg tea.KeyMsg) {
	key := msg.String()
	if m.pop != "keys" {
		switch key {
		case "esc":
			m.pop, m.popBody, m.popTop = "", nil, 0
		case "up", "k": // a SKILL.md has nothing to search, so letters scroll it too
			m.popTop--
		case "down", "j":
			m.popTop++
		}
		return
	}
	switch {
	// esc takes back one thing per press, so a search goes before the popup does
	case key == "esc" && m.query != "":
		m.query, m.popTop = "", 0
	case key == "esc":
		m.pop, m.popBody, m.popTop, m.query = "", nil, 0, ""
		return
	case key == "tab":
		m.setAt = (m.setAt + 1) % (len(settings) + 1)
	case key == "up":
		if m.setAt == len(settings) && m.popTop > 0 {
			m.popTop--
		} else {
			m.setAt = max(m.setAt-1, 0)
		}
	case key == "down":
		if m.setAt == len(settings) {
			m.popTop++
		} else {
			m.setAt++
		}
	case key == "left":
		m.stepSetting(-1)
	// enter and the arrows are never text; space is, once you are searching or
	// the cursor has left the settings for the list
	case key == "right", key == "enter", key == " " && m.query == "" && m.setAt < len(settings):
		m.stepSetting(1)
	// no key in the table is written with a digit, so digits stay free for the
	// themes. Every letter, q included, is a character in the search box.
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		if names := themeNames(); int(key[0]-'1') < len(names) {
			m.setTheme(names[key[0]-'1'])
		}
	case key == "backspace":
		if m.query != "" {
			m.query = m.query[:len(m.query)-1]
		}
		m.popTop = 0
	case len(msg.Runes) > 0:
		m.query += string(msg.Runes)
		m.popTop = 0
	}
	if m.setAt < len(settings) {
		m.popTop = 0 // a cursor you cannot see is worse than losing your place
	}
	m.popBody = m.keysBody()
}

// settings are the overlay's rows, in the order they are drawn. The cursor also
// stops one past the end, on the key list, which is where it scrolls.
var settings = []string{"theme"}

// stepSetting moves the focused setting n places along its options, wrapping,
// and writes it: a setting is this machine's look, not part of the selection
// that the review flow saves.
func (m *model) stepSetting(n int) {
	if m.setAt >= len(settings) {
		return // the cursor is on the key list, which has nothing to change
	}
	switch settings[m.setAt] {
	case "theme":
		names := themeNames()
		m.setTheme(names[(slices.Index(names, m.themeName())+n+len(names))%len(names)])
	}
}

func themeNames() []string { return slices.Sorted(maps.Keys(themes)) }

// pick is one option in a picker row, filled in when it is the one in force.
func pick(label string, on bool) string {
	if on {
		return ui.bar.Render(" "+label+" ") + " "
	}
	return ui.dim.Render(" "+label+" ") + " "
}

// setTheme takes effect and writes the theme alone at once: the theme is this
// machine's look, not part of the selection that the review flow saves, so the
// disk keeps the selection from the last save and unsaved toggles stay pending.
func (m *model) setTheme(name string) {
	m.cfg.Theme = name
	useTheme(name)
	m.writeSaved()
}

// writeSaved writes the config with the selection as of the last save, for the
// settings that take effect at once without saving the pending toggles.
func (m *model) writeSaved() {
	c := *m.cfg
	// clones, because save sorts in place and must not reorder the saved selection
	c.Enabled = slices.Clone(m.savedOn)
	c.Profiles, c.Targets = slices.Clone(m.savedProfiles), slices.Clone(m.savedTargets)
	if err := c.save(); err != nil {
		m.status = err.Error()
	}
}

// themeName is the theme actually on screen, which an unknown name in the config
// is not.
func (m *model) themeName() string {
	if _, ok := themes[m.cfg.Theme]; ok {
		return m.cfg.Theme
	}
	return defaultTheme
}

func (m *model) openKeys() {
	m.pop, m.popTop, m.query, m.setAt = "keys", 0, "", 0
	m.popBody = m.keysBody()
}

// keysBody is the keys overlay: the settings you can change, then every binding
// the table knows, narrowed by what you have typed. The settings are first
// because they are the half you act on, and a long key list would push them off
// a short screen.
func (m *model) keysBody() []string {
	head := ui.text.Bold(true).Render("keys") + ui.dim.Render("  ·  type to search  ·  esc goes back")
	if m.query != "" {
		head = ui.text.Bold(true).Render("keys") + ui.dim.Render("  matching ") + ui.text.Render(m.query) + ui.label.Render("\u258c")
	}
	out := []string{head, "", ui.dim.Render("SETTINGS")}
	for i := range settings {
		out = append(out, m.settingRow(i))
	}

	mode, found := "", false
	for _, b := range bindings {
		if !strings.Contains(strings.ToLower(b.mode+" "+b.keys+" "+b.does), strings.ToLower(m.query)) {
			continue
		}
		if b.mode != mode {
			out, mode = append(out, "", ui.dim.Render(strings.ToUpper(b.mode))), b.mode
		}
		out, found = append(out, "  "+ui.label.Render(fit(b.keys, 12))+ui.text.Render(b.does)), true
	}
	if !found {
		out = append(out, "", ui.dim.Render("no key matches "+m.query))
	}
	return out
}

// settingRow is one setting: the selection bar as a cursor when it has focus,
// its name, then its options with the one in force filled in.
func (m *model) settingRow(i int) string {
	cursor := "  "
	if i == m.setAt {
		cursor = ui.bar.Render(" ") + " "
	}
	opts := ""
	switch settings[i] {
	case "theme":
		for n, name := range themeNames() {
			opts += pick(fmt.Sprintf("%d %s", n+1, name), name == m.themeName())
		}
	}
	return cursor + ui.text.Render(fit(settings[i], 8)) + opts
}

// openSkill puts a skill's SKILL.md over the screen, rendered.
func (m *model) openSkill(name string) {
	path := filepath.Join(m.cfg.Repo, "skills", name, "SKILL.md")
	// the name rides in pop, so a resize can re-wrap the file without the model
	// growing a field for it
	m.pop, m.popTop = "skill:"+name, 0
	head := ui.text.Bold(true).Render(name) + ui.dim.Render("  ·  esc goes back")
	body, err := os.ReadFile(path)
	if err != nil {
		m.popBody = []string{head, "", ui.warn.Render("can't read " + path), ui.dim.Render(err.Error())}
		return
	}
	// the prose takes the whole box: a cap on the measure is what left the file
	// sitting in 80 columns on a wide terminal
	m.popBody = append([]string{head, ""}, lines(markdown(skipFrontmatter(string(body)), m.popWidth()-4))...)
}

// resizePop re-wraps a SKILL.md after the terminal changes size, keeping your
// place. The keys overlay is rebuilt from the table on every keystroke already.
func (m *model) resizePop() {
	name, ok := strings.CutPrefix(m.pop, "skill:")
	if !ok {
		return
	}
	top := m.popTop
	m.openSkill(name)
	m.popTop = top
}

// skipFrontmatter drops the YAML header, which the renderer would otherwise draw
// as a stray heading; the detail column already shows what is in it.
func skipFrontmatter(src string) string {
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return src
	}
	if _, after, ok := strings.Cut(rest, "\n---"); ok {
		return strings.TrimLeft(after, "\n")
	}
	return src
}
