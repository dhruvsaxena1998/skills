package main

import "github.com/charmbracelet/lipgloss"

// Theme names a colour by the job it does, never by the colour it is, so a new
// look is one entry in themes and no change at all to the layout code.
type Theme struct {
	Accent    lipgloss.Color // the app's own colour: title, cursor marker, field labels
	AccentAlt lipgloss.Color // the second voice: the new badge
	OK        lipgloss.Color // enabled, and an addition in the review
	Warn      lipgloss.Color // warnings
	Drop      lipgloss.Color // a removal: the unlink lines in the review
	Muted     lipgloss.Color // headings, counts, anything secondary
	Text      lipgloss.Color // what you are meant to be reading
	Inverse   lipgloss.Color // ink, for text sitting on a filled block

	SourceRules []lipgloss.Color // cycled, one rule per source
	FilledTabs  bool             // the active tab as a filled block, or underlined
	ColumnRules bool             // rules between the columns, or air
}

const defaultTheme = "atlas"

var themes = map[string]Theme{
	"atlas": {
		Accent: "#8B5CF6", AccentAlt: "#F25D94", OK: "#34D399", Warn: "#FBBF24",
		Drop:  "#F87171", // tailwind red-400, the scale OK and Warn come from
		Muted: "#64748B", Text: "#E2E8F0", Inverse: "#0F172A",
		SourceRules: []lipgloss.Color{"#8B5CF6", "#22D3EE", "#F25D94"},
		FilledTabs:  true,
	},
	// the published Catppuccin Mocha palette: mauve, pink, green, yellow, red, overlay1,
	// text and base
	"catppuccin-mocha": {
		Accent: "#CBA6F7", AccentAlt: "#F5C2E7", OK: "#A6E3A1", Warn: "#F9E2AF",
		Drop:  "#F38BA8",
		Muted: "#7F849C", Text: "#CDD6F4", Inverse: "#1E1E2E",
		SourceRules: []lipgloss.Color{"#CBA6F7", "#89DCEB", "#FAB387"},
		FilledTabs:  true,
		ColumnRules: true,
	},
	// gruvbox dark, bright variants over fg/bg0_h
	"gruvbox-dark": {
		Accent: "#FE8019", AccentAlt: "#D3869B", OK: "#B8BB26", Warn: "#FABD2F",
		Drop:  "#FB4934",
		Muted: "#928374", Text: "#EBDBB2", Inverse: "#1D2021",
		SourceRules: []lipgloss.Color{"#FE8019", "#8EC07C", "#D3869B"},
		ColumnRules: true,
	},
}

// styles are the active theme, already built. A lipgloss.Color anywhere outside
// this file is a bug.
type styles struct {
	theme                     Theme
	dim, text, ok, warn, drop lipgloss.Style
	title, label              lipgloss.Style
	badge, chip, bar          lipgloss.Style
	box                       lipgloss.Style
	tabOn, tabAlt, tabOff     lipgloss.Style
	rules                     []lipgloss.Style
}

var ui = newStyles(themes[defaultTheme])

// useTheme switches to the named theme, falling back to the default when the
// name is empty or unknown, so a typo in the config still gives a usable screen.
func useTheme(name string) {
	t, ok := themes[name]
	if !ok {
		t = themes[defaultTheme]
	}
	ui = newStyles(t)
}

func newStyles(t Theme) styles {
	base := lipgloss.NewStyle()
	u := styles{
		theme: t,
		dim:   base.Foreground(t.Muted),
		text:  base.Foreground(t.Text),
		ok:    base.Foreground(t.OK),
		warn:  base.Foreground(t.Warn),
		drop:  base.Foreground(t.Drop),
		title: base.Foreground(t.Accent).Bold(true),
		label: base.Foreground(t.Accent),
		badge: base.Background(t.AccentAlt).Foreground(t.Inverse),
		chip:  base.Background(t.Warn).Foreground(t.Inverse).Bold(true),
		// a solid accent block: one cell marks the row under the cursor, and a few
		// more make the chip under a label that is in force
		bar: base.Background(t.Accent).Foreground(t.Inverse).Bold(true),
		box: base.Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(0, 1),

		tabOn:  base.Background(t.Accent).Foreground(t.Inverse).Bold(true).Padding(0, 2),
		tabAlt: base.Foreground(t.Accent).Bold(true).Underline(true).Padding(0, 2),
		tabOff: base.Foreground(t.Muted).Padding(0, 2),
	}
	for _, c := range t.SourceRules {
		u.rules = append(u.rules, base.Foreground(c))
	}
	return u
}

// rule is the coloured bar that marks a source, cycled through the theme's list.
func (u styles) rule(i int) string {
	if len(u.rules) == 0 {
		return " "
	}
	return u.rules[i%len(u.rules)].Render("▌")
}

func (u styles) tab(name string, active bool) string {
	switch {
	case active && u.theme.FilledTabs:
		return u.tabOn.Render(name)
	case active:
		return u.tabAlt.Render(name)
	default:
		return u.tabOff.Render(name)
	}
}

// gutter is the space between two columns: air, or a rule if the theme wants one.
func (u styles) gutter() string {
	if u.theme.ColumnRules {
		return u.dim.Render(" │ ")
	}
	return "   "
}
