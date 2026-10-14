package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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
	{"selection", "space · x", "turn a skill, a source or a target on or off", "space toggle"},
	{"selection", "↑ ↓ · k j", "move inside a column", ""},
	{"selection", "← → · h l", "step between the sources and their skills", "←→ columns"},
	{"selection", "enter", "read the skill's SKILL.md", ""},
	{"selection", "/", "filter the skills by name", "/ filter"},
	{"selection", "?", "this list, and the theme", "? keys"},
	{"selection", "s", "review what a save would change", "s review"},
	{"selection", "p", "pull the repo", "p pull"},
	{"selection", "q · ctrl+c", "quit, asking once when something is unsaved", "q quit"},

	{"filtering", "space", "toggle the row under the cursor", "space toggles"},
	{"filtering", "↑ ↓", "move the cursor without leaving the filter", "↑↓ move"},
	{"filtering", "enter", "keep the filter and give the keys back to the list", "enter keeps the filter"},
	{"filtering", "esc", "clear the filter", "esc clears it"},

	{"review", "enter", "apply the plan", "enter applies this"},
	{"review", "esc · q", "go back", "esc goes back"},

	{"overlay", "↑ ↓", "scroll", "↑↓ scroll"},
	{"overlay", "k j", "scroll a SKILL.md, where letters are not a search", ""},
	// not on the rail: the picker already numbers the themes, and a SKILL.md shares
	// this rail line without having any
	{"overlay", "a digit", "pick that theme, written as you press it", ""},
	{"overlay", "esc", "close it, exactly where you were", "esc goes back"},
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

// popKey is the overlay's own keys. Both popups scroll and close the same way;
// only the keys one takes typing, since a SKILL.md has nothing to search.
func (m *model) popKey(msg tea.KeyMsg) {
	switch key := msg.String(); {
	case key == "esc":
		m.pop, m.popBody, m.popTop, m.query = "", nil, 0, ""
	case key == "up":
		m.popTop--
	case key == "down":
		m.popTop++
	case m.pop != "keys":
		if key == "k" { // a SKILL.md has nothing to search, so letters scroll it too
			m.popTop--
		}
		if key == "j" {
			m.popTop++
		}
	// no key in the table is written with a digit, so digits are free to pick a theme
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		if names := themeNames(); int(key[0]-'1') < len(names) {
			m.setTheme(names[key[0]-'1'])
			m.popBody = m.keysBody()
		}
	case key == "backspace":
		if m.query != "" {
			m.query = m.query[:len(m.query)-1]
		}
		m.popBody, m.popTop = m.keysBody(), 0
	case len(msg.Runes) > 0:
		m.query += string(msg.Runes)
		m.popBody, m.popTop = m.keysBody(), 0
	}
}

func themeNames() []string { return slices.Sorted(maps.Keys(themes)) }

// setTheme takes effect and is written at once: the theme is this machine's
// look, not part of the selection that the review flow saves.
func (m *model) setTheme(name string) {
	m.cfg.Theme = name
	useTheme(name)
	if err := m.cfg.save(); err != nil {
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
	m.pop, m.popTop, m.query = "keys", 0, ""
	m.popBody = m.keysBody()
}

// keysBody is the keys overlay: the theme picker, then every binding the table
// knows, narrowed by what you have typed. The picker is first because it is the
// half you act on, and a long key list would push it off a short screen.
func (m *model) keysBody() []string {
	head := ui.text.Bold(true).Render("keys") + ui.dim.Render("  ·  type to search  ·  esc goes back")
	if m.query != "" {
		head = ui.text.Bold(true).Render("keys") + ui.dim.Render("  matching ") + ui.text.Render(m.query) + ui.label.Render("\u258c")
	}
	row, here := "", m.themeName()
	for i, name := range themeNames() {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if name == here {
			row += ui.bar.Render(label) + " "
			continue
		}
		row += ui.dim.Render(label) + " "
	}
	out := []string{head, "", ui.dim.Render("THEME"), "  " + row}

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

// openSkill puts a skill's SKILL.md over the screen, rendered.
func (m *model) openSkill(name string) {
	path := filepath.Join(m.cfg.Repo, "skills", name, "SKILL.md")
	m.pop, m.popTop = "skill", 0
	head := ui.text.Bold(true).Render(name) + ui.dim.Render("  ·  esc goes back")
	body, err := os.ReadFile(path)
	if err != nil {
		m.popBody = []string{head, "", ui.warn.Render("can't read " + path), ui.dim.Render(err.Error())}
		return
	}
	// shortcut: the markdown is wrapped to the width it opened at; reopen it after
	// a resize, or re-render on tea.WindowSizeMsg if that ever grates
	m.popBody = append([]string{head, ""}, lines(markdown(skipFrontmatter(string(body)), m.popWidth()-4))...)
}

// skipFrontmatter drops the YAML header, which glamour would otherwise draw as a
// stray rule and heading; the detail column already shows what is in it.
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

// markdown renders with glamour's own dark style. Mapping the skillset theme onto
// a glamour StyleConfig would have to be redone for every theme added.
func markdown(src string, width int) string {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err == nil {
		if out, err := r.Render(src); err == nil {
			return out
		}
	}
	return src // the file itself beats an error about not being able to draw it
}
