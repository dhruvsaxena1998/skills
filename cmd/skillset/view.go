package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type tabID int

const (
	tabSkills tabID = iota
	tabTargets
	tabChanges
)

// the screen above the columns is fixed: a blank line, the top line, a blank, the
// tabs, a blank. A click arrives as a screen position, so the two have to agree.
const (
	tabLine = 3
	bodyTop = 5
)

// the checkbox: three cells wide wherever it is drawn, which the column widths
// and every plain string behind the selection bar have to allow for
const (
	boxOn  = "[x]"
	boxOff = "[ ]"
)

// layout is where the last draw put things, so a click can be mapped back to a
// row without the drawing and the pointer having to agree about it twice over.
type layout struct {
	tabs       []int // the x each tab ends at
	x2, x3     int   // where the middle and the detail column start
	first, mid []int // the cursor index on each drawn line of that column, or -1
}

// tab is the step on screen: the review while it is up, and otherwise the step
// the keys are moving you through.
func (m *model) tab() tabID {
	if m.review != nil {
		return tabChanges
	}
	return m.step
}

func (m *model) View() string {
	foot := m.rail()
	h := max(5, m.height-len(foot)-6)
	body := m.columns(h)
	if m.review != nil {
		body = m.reviewPane(h)
	}
	if m.pop != "" {
		body = m.popupPane(h)
	}
	return "\n" + m.topLine() + "\n\n  " + m.tabBar() + "\n\n" +
		body + "\n" + strings.Join(foot, "\n")
}

// topLine is who we are and where we stand: the repo, what is on, and origin.
func (m *model) topLine() string {
	on, total := 0, 0
	for _, r := range m.rows {
		if r.kind != rowSkill || isProfileGroup(r.src) {
			continue
		}
		total++
		if slices.Contains(m.cfg.Enabled, r.name) {
			on++
		}
	}
	stats := fmt.Sprintf("%d/%d on", on, total)
	if len(m.profileNames()) > 0 {
		stats += "  ·  " + plural(len(m.cfg.Profiles), "profile")
	}
	stats += "  ·  " + plural(len(m.cfg.Targets), "target")
	if m.status != "" {
		stats += "  ·  " + m.status
	}
	if m.loading["origin"] {
		stats += " " + m.spin() // the fetch is still out; the screen didn't wait for it
	}
	title, repo := ui.title.Render("skillset"), "  "+m.cfg.Repo
	if lipgloss.Width(title)+lipgloss.Width(repo)+lipgloss.Width(stats)+6 > m.width {
		repo = "" // the path is the first thing worth losing
	}
	return pair(title+ui.dim.Render(repo), ui.dim.Render(trunc(stats, max(10, m.width-4))), m.width)
}

func (m *model) tabBar() string {
	changes := "changes"
	if m.dirty {
		changes += " •" // there is something unsaved behind that tab
	}
	// the foreign entries are a step away now, so their count rides on the tab they
	// are behind: the screen must never read as though the folders hold only ours
	targets := "targets"
	if n := m.notOurs(); n > 0 {
		targets += fmt.Sprintf(" · %d not ours", n)
	}
	t := m.tab()
	bar := []string{ui.tab("skills", t == tabSkills), ui.tab(targets, t == tabTargets),
		ui.tab(changes, t == tabChanges)}
	m.hit.tabs = nil
	x := 2 // the two spaces the bar is printed after
	for _, s := range bar {
		x += lipgloss.Width(s)
		m.hit.tabs = append(m.hit.tabs, x)
	}
	return strings.Join(bar, "")
}

// columns draws the three of them: this step's rows, what the one under the cursor
// holds, and everything about it.
func (m *model) columns(h int) string {
	// below 90 the detail column has nothing but truncation to offer, so it goes and
	// the descriptions move inline instead; the description is never absent from both
	showDetail := m.width >= 90
	inline := !showDetail || m.width >= 120
	// the sources column holds the longest source name and its tally side by side;
	// the detail pane is where a choice is actually made, so it takes a quarter of a
	// wide screen, and the skills column is whatever the two of them leave
	sourceW, detailW := 20, max(36, m.width/4)
	skillW := max(24, m.width-sourceW-7)
	if showDetail {
		skillW = max(24, m.width-sourceW-detailW-10)
	}

	c1, sel1, hit1 := m.firstColumn(sourceW)
	c2, sel2, hit2 := m.skillColumn(skillW, inline)
	if m.step == tabTargets {
		c2, sel2, hit2 = m.insideColumn(skillW), -1, nil
	}
	panes := []string{
		pad(windowed(c1, &m.left, sel1, h), sourceW, h),
		pad(windowed(c2, &m.mid, sel2, h), skillW, h),
	}
	m.hit.x2, m.hit.x3 = 2+sourceW+3, m.width+1
	m.hit.first, m.hit.mid = drawn(hit1, m.left, h), drawn(hit2, m.mid, h)
	if showDetail {
		m.hit.x3 = m.hit.x2 + skillW + 3
		// shortcut: a detail longer than the screen is cut, not scrolled; it is the
		// one pane you can always read in the file itself
		panes = append(panes, pad(m.detail(detailW), detailW, h))
	}
	var joined []string
	for i, p := range panes {
		if i > 0 {
			joined = append(joined, ui.gutter())
		}
		joined = append(joined, p)
	}
	return lipgloss.NewStyle().MarginLeft(2).Render(lipgloss.JoinHorizontal(lipgloss.Top, joined...))
}

// firstColumn is the first column of the step you are on: every source with its
// own rule and tally, or the agent targets and the entries in those folders that
// aren't ours. It returns the lines, which of them the cursor is on, and the row
// each of them belongs to, which is what a click is mapped through.
func (m *model) firstColumn(width int) ([]string, int, []int) {
	here := -1
	if len(m.shown) > 0 {
		here = m.shown[m.cursor]
	}
	var out []string
	var hits []int // the cursor index on each line, -1 where there is no row on it
	noRow := func(line string) { out, hits = append(out, line), append(hits, -1) }
	sel, kind, nth := -1, rowKind(-1), 0
	for i, r := range m.rows {
		if r.kind == rowSkill || stepOf(r.kind) != m.step {
			continue
		}
		if r.kind != kind {
			if len(out) > 0 && out[len(out)-1] != "" {
				noRow("")
			}
			noRow(ui.dim.Render(headings[r.kind]))
			kind = r.kind
		}
		var plain, styled string
		switch r.kind {
		case rowSource:
			on, total := m.counts(r.name)
			name, tally := trunc(r.name, width-8), fmt.Sprintf("%d/%d", on, total)
			gap := strings.Repeat(" ", max(1, width-3-lipgloss.Width(name)-lipgloss.Width(tally)))
			plain = "▌ " + name + gap + tally
			shown := ui.dim.Render(name)
			if r.name == m.source {
				shown = ui.text.Bold(true).Render(name) // the source the middle column is on
			}
			styled, nth = ui.rule(nth)+" "+shown+gap+ui.dim.Render(tally), nth+1
		case rowProfile:
			on, total := m.profileCounts(r.name)
			tally := fmt.Sprintf("%d/%d", on, total)
			name := trunc(r.name, width-7-lipgloss.Width(tally))
			gap := strings.Repeat(" ", max(1, width-6-lipgloss.Width(name)-lipgloss.Width(tally)))
			glyph, box, shown := boxOff, ui.dim.Render(boxOff), ui.dim.Render(name)
			if slices.Contains(m.cfg.Profiles, r.name) {
				glyph, box, shown = boxOn, ui.ok.Render(boxOn), ui.text.Render(name)
			}
			plain = "  " + glyph + " " + name + gap + tally
			styled = "  " + box + " " + shown + gap + ui.dim.Render(tally)
		case rowTarget:
			name := trunc(r.name, width-6) // two in, the box, and the space after it
			glyph, box, shown := boxOff, ui.dim.Render(boxOff), ui.dim.Render(name)
			if slices.Contains(m.cfg.Targets, r.name) {
				glyph, box, shown = boxOn, ui.ok.Render(boxOn), ui.text.Render(name)
			}
			plain, styled = "  "+glyph+" "+name, "  "+box+" "+shown
		default:
			plain = "  " + trunc(filepath.Base(r.name), width-2)
			styled = ui.dim.Render(plain)
		}
		if i == here {
			sel, styled = len(out), selected(plain, width)
		}
		out, hits = append(out, styled), append(hits, slices.Index(m.shown, i))
	}
	return out, sel, hits
}

// insideColumn is the middle column on the targets step: what the folder under
// the cursor actually holds. None of it is ours to change, so none of it toggles.
func (m *model) insideColumn(width int) []string {
	r, ok := m.at()
	if !ok {
		return []string{ui.dim.Render("CONTENTS")}
	}
	out := []string{ui.dim.Render("CONTENTS  " + trunc(r.name, width-12))}
	names := inside(r.src)
	if len(names) == 0 {
		return append(out, "", ui.dim.Render(" nothing in it to list"))
	}
	for _, name := range names {
		out = append(out, ui.dim.Render(" "+trunc(name, width-2)))
	}
	return out
}

// notOurs is how many entries in the agent folders this repo didn't create.
func (m *model) notOurs() int {
	n := 0
	for _, r := range m.rows {
		if r.kind == rowOther {
			n++
		}
	}
	return n
}

// skillColumn is the middle column: the skills of the current source, or every
// match while a filter is on, with the description inline when there is room.
func (m *model) skillColumn(width int, inline bool) ([]string, int, []int) {
	head := ui.dim.Render("SKILLS")
	if s, ok := m.sources[m.source]; ok && s.Repo != "" && m.filter == "" {
		head += ui.dim.Render("  " + trunc(trimScheme(s.Repo), width-10))
	}
	if name, ok := profileName(m.source); ok && m.filter == "" {
		head += ui.dim.Render("  " + trunc(name, width-10))
	}
	if m.filter != "" {
		head = ui.dim.Render("SKILLS  matching ") + ui.text.Render(trunc(m.filter, width-18))
	}
	out, sel, hits := []string{head}, -1, []int{-1}

	// a row is a space, the three-cell box, a space, the name, then the description
	nameW := 24
	descW := width - nameW - 6
	if !inline || descW < 18 {
		nameW, descW = width-6, 0
	}
	for _, i := range m.midRows() {
		r := m.rows[m.shown[i]]
		name := trunc(r.name, nameW)
		box, glyph := ui.dim.Render(boxOff), boxOff
		if slices.Contains(m.cfg.Enabled, r.name) {
			box, glyph = ui.ok.Render(boxOn), boxOn
		}
		badge, badgePlain := "", ""
		if r.note == "new" {
			badge, badgePlain = ui.badge.Render(" new ")+" ", " new  "
		}
		plain, styled := " "+glyph+" "+name, " "+box+" "+name
		if descW > 0 {
			gap := strings.Repeat(" ", max(1, nameW+1-lipgloss.Width(name)))
			desc := trunc(m.descriptionOf(r.name), descW-lipgloss.Width(badgePlain))
			plain += gap + badgePlain + desc
			styled += gap + badge + ui.dim.Render(desc)
		} else {
			plain, styled = plain+badgePlain, styled+badge
		}
		if i == m.cursor {
			sel, styled = len(out), selected(plain, width)
		}
		out, hits = append(out, styled), append(hits, i)
	}
	if len(out) == 1 {
		empty := " no skills here"
		if m.filter != "" {
			empty = " nothing matches " + trunc(m.filter, width-18)
		}
		out, hits = append(out, "", ui.dim.Render(empty)), append(hits, -1, -1)
	}
	return out, sel, hits
}

// midRows lists the shown skills the middle column carries: one source's, or every
// match while a filter is on, since a search crosses sources.
func (m *model) midRows() []int {
	var out []int
	for i, idx := range m.shown {
		if r := m.rows[idx]; r.kind == rowSkill && (m.filter != "" || r.src == m.source) {
			out = append(out, i)
		}
	}
	return out
}

// counts is how many of a source's skills are on, out of how many it has here.
func (m *model) counts(src string) (on, total int) {
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	for _, s := range m.sourceSkills(src) {
		if !isSkill(skillsDir, s) {
			continue
		}
		total++
		if slices.Contains(m.cfg.Enabled, s) {
			on++
		}
	}
	return on, total
}

func (m *model) profileCounts(name string) (on, total int) {
	seen := map[string]bool{}
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	for _, skill := range m.profiles[name].Skills {
		if seen[skill] || !isSkill(skillsDir, skill) {
			continue // a member missing from the repo is listed in the detail pane instead
		}
		seen[skill] = true
		total++
		if slices.Contains(m.cfg.Enabled, skill) {
			on++
		}
	}
	return on, total
}

// rail is the bottom of the screen: what the last toggle dragged along, what is
// now inconsistent, and the keys.
func (m *model) rail() []string {
	var out []string
	warns := warnings(m.cfg, m.deps)
	chip := ui.chip.Render(fmt.Sprintf(" ⚠ %d ", len(warns)))
	cont := strings.Repeat(" ", lipgloss.Width(chip))
	for i, w := range warns {
		// shortcut: past three the rest are counted, not listed; raise it if a real
		// selection ever carries that many missing options
		if i == 3 {
			out = append(out, "  "+cont+" "+ui.warn.Render(fmt.Sprintf("… and %d more", len(warns)-3)))
			break
		}
		head := cont
		if i == 0 {
			head = chip
		}
		text := strings.TrimPrefix(w, "warning: ")
		out = append(out, "  "+head+" "+ui.warn.Render(trunc(text, max(10, m.width-lipgloss.Width(cont)-5))))
	}
	if m.cascade != "" {
		out = append(out, "  "+ui.text.Render(trunc(m.cascade, max(10, m.width-4))))
	}

	keys := railLine("selection")
	switch {
	case m.pop != "":
		keys = railLine("overlay")
	case m.review != nil:
		keys = railLine("review")
	case m.typing:
		keys = railLine("filtering")
	}
	left := ""
	if m.typing || m.filter != "" {
		left = ui.dim.Render("filter ") + ui.text.Render(m.filter)
		if m.typing {
			left += ui.label.Render("▌")
		}
	}
	return append(out, pair(left, ui.dim.Render(trunc(keys, max(10, m.width-4))), m.width))
}

// popupPane is the overlay both popups share: a box over the columns, scrolled by
// the same window the columns use.
func (m *model) popupPane(h int) string {
	w, sel := m.popWidth(), -1
	if m.pop == profileEdit && m.picking {
		sel = m.pickLine // keep the checklist's cursor on screen
	}
	return lipgloss.NewStyle().MarginLeft(2).
		Render(ui.box.Render(pad(windowed(m.popBody, &m.popTop, sel, h-2), w-4, h-2)))
}

// popWidth keeps the box off both edges and takes everything between them; it is
// the prose inside that stops widening, not the box.
func (m *model) popWidth() int { return max(34, m.width-6) }

// reviewPane is the changes tab: every link, unlink and hook file a save would write.
func (m *model) reviewPane(h int) string {
	body, more := m.review, ""
	if len(body) > h-2 {
		more = ui.dim.Render(fmt.Sprintf("  … and %d more", len(body)-h+2))
		body = body[:h-2]
	}
	out := []string{"  " + ui.text.Bold(true).Render("review") + "  " + ui.dim.Render(countPlan(m.review)), ""}
	for _, line := range body {
		sign, rest, style := reviewLine(line)
		out = append(out, "  "+style.Render(sign+" "+trunc(rest, max(10, m.width-6))))
	}
	if more != "" {
		out = append(out, more)
	}
	return strings.Join(out, "\n")
}

// reviewLine reads a plan line as a diff: a link adds a file, an unlink takes one
// away, and a hook edit changes a file that is there either way. The sign carries
// the verb, so the path gets the width instead.
func reviewLine(line string) (sign, rest string, style lipgloss.Style) {
	switch {
	case strings.HasPrefix(line, "link "):
		return "+", strings.TrimPrefix(line, "link "), ui.ok
	case strings.HasPrefix(line, "unlink "):
		return "-", strings.TrimPrefix(line, "unlink "), ui.drop
	case strings.HasPrefix(line, "update hooks"):
		return "~", strings.TrimPrefix(line, "update "), ui.label
	}
	// a skip, or nothing to change: no sign, but indented to the same column
	return " ", line, ui.text
}

// countPlan summarises a review, which is longer than the screen on a first apply.
func countPlan(plan []string) string {
	var links, unlinks, hooks int
	for _, line := range plan {
		switch {
		case strings.HasPrefix(line, "link "):
			links++
		case strings.HasPrefix(line, "unlink "):
			unlinks++
		case strings.HasPrefix(line, "update hooks"):
			hooks++
		}
	}
	return fmt.Sprintf("%d links · %d unlinks · %d hook files", links, unlinks, hooks)
}

// detail is the third column: everything about the row under the cursor, so a
// choice never needs a file open.
func (m *model) detail(width int) []string {
	out := []string{ui.dim.Render("DETAIL")}
	r, ok := m.at()
	if !ok {
		return append(out, "", ui.dim.Render("nothing matches "+m.filter))
	}
	var body string
	switch r.kind {
	case rowTarget:
		for _, t := range targets {
			if t.name != r.name {
				continue
			}
			state := ui.dim.Render(boxOff + " off")
			if slices.Contains(m.cfg.Targets, r.name) {
				state = ui.ok.Render(boxOn + " on")
			}
			body = ui.text.Bold(true).Render(r.name) + "\n" + state + "\n\n" +
				ui.label.Render("skills     ") + "~/" + filepath.ToSlash(t.dir) + "\n" +
				ui.label.Render("hooks      ") + "~/" + filepath.ToSlash(t.hooks)
		}
	case rowSource:
		on, total := m.counts(r.name)
		body = ui.text.Bold(true).Render(r.name) + "\n" +
			ui.dim.Render(fmt.Sprintf("%d/%d on", on, total)) + "\n\n"
		if s := m.sources[r.name]; s.Repo == "" {
			body += ui.dim.Render("in the repo, in no source")
		} else {
			body += ui.label.Render("repo       ") + trimScheme(s.Repo) + "\n" +
				ui.label.Render("branch     ") + s.Ref + "\n" +
				ui.label.Render("path       ") + s.Path
		}
	case rowProfile:
		state := ui.dim.Render(boxOff + " off")
		if slices.Contains(m.cfg.Profiles, r.name) {
			state = ui.ok.Render(boxOn + " on")
		}
		body = ui.text.Bold(true).Render(r.name) + "\n" + state + "\n"
		if desc := m.profiles[r.name].Description; desc != "" {
			body += "\n" + ui.dim.Render(desc) + "\n"
		}
		on, total := m.profileCounts(r.name)
		body += "\n" + ui.label.Render("skills     ") + fmt.Sprintf("%d/%d on", on, total)
		var missing []string
		for _, skill := range m.profiles[r.name].Skills {
			if !isSkill(filepath.Join(m.cfg.Repo, "skills"), skill) {
				missing = append(missing, skill)
			}
		}
		if len(missing) > 0 {
			body += "\n" + ui.label.Render("missing    ") + strings.Join(missing, ", ")
		}
		if r.note == "missing" {
			body += "\n\n" + ui.dim.Render("no longer in profiles.json; space turns it off here")
		} else {
			body += "\n\n" + ui.dim.Render("space turns its skills on, adding to what's already on. Turning it off turns them off again, except skills you turned on yourself, another profile has, or a skill needs. Skills are never removed from the repo.")
		}
	case rowSkill:
		body = m.skillDetail(r)
	default:
		body = ui.text.Bold(true).Render("not ours") + "\n\n" + r.name + "\n" + r.note +
			"\n\n" + ui.dim.Render("shown so it isn't a surprise; skillset never changes it")
	}
	return append(out, strings.Split(lipgloss.NewStyle().Width(width).Render(body), "\n")...)
}

func (m *model) skillDetail(r row) string {
	var b strings.Builder
	state := ui.dim.Render(boxOff + " off")
	if slices.Contains(m.cfg.Enabled, r.name) {
		state = ui.ok.Render(boxOn + " enabled")
	}
	context := r.src
	if name, ok := profileName(r.src); ok {
		context = "profile " + name
	}
	b.WriteString(ui.text.Bold(true).Render(r.name) + "\n" + state + ui.dim.Render("  ·  "+context))
	if r.note == "new" {
		b.WriteString("  " + ui.badge.Render(" new "))
	}
	b.WriteString("\n")
	if desc := m.descriptionOf(r.name); desc != "" {
		b.WriteString("\n" + ui.dim.Render(desc) + "\n")
	}
	b.WriteString("\n")

	var dependents []string
	withDependents(r.name, m.deps, &dependents)
	dependents = slices.DeleteFunc(dependents, func(s string) bool { return s == r.name })
	field := func(k, v string) {
		if v != "" {
			b.WriteString(ui.label.Render(fmt.Sprintf("%-11s", k)) + v + "\n")
		}
	}
	d := m.deps[r.name]
	field("requires", strings.Join(d.Requires, ", "))
	field("optional", strings.Join(d.Optional, ", "))
	field("needed by", strings.Join(dependents, ", "))
	// the git log behind this is a command, so the pane draws at once and says so
	// while the answer is still coming
	switch patches, loaded := m.patches[r.name]; {
	case !loaded:
		field("patches", ui.dim.Render(m.spin()))
	case len(patches) > 0:
		field("patches", fmt.Sprintf("%d local", len(patches)))
		for _, p := range patches {
			b.WriteString(ui.dim.Render("  "+p) + "\n")
		}
	}
	return b.String()
}

// pair puts one thing against the left margin and one against the right.
func pair(left, right string, width int) string {
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-3)
	return "  " + left + strings.Repeat(" ", gap) + right
}

// selected is the row under the cursor: a block in its first cell and the rest of
// it bold. A filled row can only carry dark ink, and on the light accents two of
// the themes use that ink read as plain black.
func selected(plain string, width int) string {
	rest := []rune(trunc(plain, width))[1:]
	return ui.bar.Render(" ") + ui.text.Bold(true).Render(string(rest))
}

// trunc cuts a string to w columns, ending in an ellipsis when it doesn't fit.
func trunc(s string, w int) string {
	if w < 1 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if len(r) > w {
		r = r[:w] // no rune is narrower than one column, so the rest cannot fit
	}
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// fit cuts or pads a plain string to exactly w columns, so a filled row can't wrap.
func fit(s string, w int) string {
	s = trunc(s, w)
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// pad fits a column into its box: height lines, each exactly width columns.
func pad(col []string, width, height int) string {
	out := make([]string, height)
	cut := lipgloss.NewStyle().MaxWidth(width)
	for i := range out {
		line := ""
		if i < len(col) {
			line = cut.Render(col[i])
		}
		out[i] = line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
	}
	return strings.Join(out, "\n")
}

// windowed scrolls a column to the lines that fit: its first line is a heading and
// stays pinned, the last is given over to a count of what didn't fit, and the
// selected line is kept in view between them. top is this column's own position.
func windowed(col []string, top *int, sel, height int) []string {
	if height < 1 || len(col) == 0 {
		return col
	}
	head, body, h := col[0], col[1:], height-1
	if len(body) <= h {
		*top = 0
		return col
	}
	view := h - 1 // the last line says how many are below it
	if sel > 0 {
		if sel-1 < *top {
			*top = sel - 1
		}
		if sel-1 >= *top+view {
			*top = sel - view
		}
	}
	*top = min(max(*top, 0), len(body)-h)
	out := slices.Clone(body[*top : *top+h])
	if hidden := len(body) - *top - h; hidden > 0 {
		out[len(out)-1] = ui.dim.Render(fmt.Sprintf("  … %d more", hidden+1))
	}
	return append([]string{head}, out...)
}

// drawn cuts a column's hit list the way windowed cut the lines it belongs to,
// so a click lands on the row that is under it and not on one scrolled away.
func drawn(hits []int, top, height int) []int {
	if height < 1 || len(hits) == 0 {
		return hits
	}
	body := hits[1:]
	if len(body) <= height-1 {
		return hits
	}
	out := append([]int{-1}, body[top:top+height-1]...)
	if len(body)-top-(height-1) > 0 {
		out[len(out)-1] = -1 // windowed gave that line over to the count of the rest
	}
	return out
}

// plural is a count and its noun, with an s on the noun when it needs one.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// trimScheme drops the https:// a repo URL carries, which says nothing on screen.
func trimScheme(url string) string {
	for _, p := range []string{"https://", "http://", "git@"} {
		if after, ok := strings.CutPrefix(url, p); ok {
			return after
		}
	}
	return url
}
