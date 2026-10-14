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

// tab is read off where the cursor is rather than stored, so the bar can never
// disagree with what you are looking at.
func (m *model) tab() tabID {
	if m.review != nil {
		return tabChanges
	}
	if r, ok := m.at(); ok && (r.kind == rowTarget || r.kind == rowOther) {
		return tabTargets
	}
	return tabSkills
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
		if r.kind != rowSkill {
			continue
		}
		total++
		if slices.Contains(m.cfg.Enabled, r.name) {
			on++
		}
	}
	noun := " targets"
	if len(m.cfg.Targets) == 1 {
		noun = " target"
	}
	stats := fmt.Sprintf("%d/%d on  ·  %d", on, total, len(m.cfg.Targets)) + noun
	if m.status != "" {
		stats += "  ·  " + m.status
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
	t := m.tab()
	return ui.tab("skills", t == tabSkills) + ui.tab("targets", t == tabTargets) +
		ui.tab(changes, t == tabChanges)
}

// columns draws the three of them: the sources and targets, the skills of the one
// you are on, and everything about the one skill.
func (m *model) columns(h int) string {
	// below 90 the detail column has nothing but truncation to offer, so it goes and
	// the descriptions move inline instead; the description is never absent from both
	showDetail := m.width >= 90
	inline := !showDetail || m.width >= 120
	sourceW, detailW := 18, 32
	skillW := max(24, m.width-25)
	if showDetail {
		skillW = max(24, m.width-60)
	}

	c1, sel1 := m.sourceColumn(sourceW)
	c2, sel2 := m.skillColumn(skillW, inline)
	first := windowed(c1, &m.left, sel1, h)
	// NOT OURS is the last section, so a short screen drops it first, and the whole
	// point of it is that you weren't looking: its count takes the last line instead
	// shortcut: targets cut along with it aren't counted, they are a fixed list the
	// cursor can always walk to; count them too if that stops being true
	if cut := len(c1) - m.left - h; cut > 0 {
		if n := min(m.notOurs(), cut+1); n > 0 {
			first[len(first)-1] = ui.dim.Render(trunc(fmt.Sprintf("NOT OURS · %d more", n), sourceW))
		}
	}
	panes := []string{
		pad(first, sourceW, h),
		pad(windowed(c2, &m.mid, sel2, h), skillW, h),
	}
	if showDetail {
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

// sourceColumn is the first column: every source with its own rule and tally, the
// agent targets, and the entries in those folders that aren't ours. It returns the
// lines and which of them the cursor is on.
func (m *model) sourceColumn(width int) ([]string, int) {
	here := -1
	if len(m.shown) > 0 {
		here = m.shown[m.cursor]
	}
	var out []string
	sel, kind, nth := -1, rowKind(-1), 0
	for i, r := range m.rows {
		if r.kind == rowSkill {
			continue
		}
		if r.kind != kind {
			if len(out) > 0 {
				out = append(out, "")
			}
			out, kind = append(out, ui.dim.Render(headings[r.kind])), r.kind
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
		case rowTarget:
			plain = "  ○ " + r.name
			dot, name := ui.dim.Render("○"), ui.dim.Render(r.name)
			if slices.Contains(m.cfg.Targets, r.name) {
				plain = "  ◉ " + r.name
				dot, name = ui.ok.Render("◉"), ui.text.Render(r.name)
			}
			styled = "  " + dot + " " + name
		default:
			plain = "  " + trunc(filepath.Base(r.name), width-2)
			styled = ui.dim.Render(plain)
		}
		if i == here {
			sel, styled = len(out), ui.bar.Render(fit(plain, width))
		}
		out = append(out, styled)
	}
	return out, sel
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
func (m *model) skillColumn(width int, inline bool) ([]string, int) {
	head := ui.dim.Render("SKILLS")
	if s, ok := m.sources[m.source]; ok && s.Repo != "" && m.filter == "" {
		head += ui.dim.Render("  " + trunc(trimScheme(s.Repo), width-10))
	}
	if m.filter != "" {
		head = ui.dim.Render("SKILLS  matching ") + ui.text.Render(trunc(m.filter, width-18))
	}
	out, sel := []string{head}, -1

	nameW := 24
	descW := width - nameW - 4
	if !inline || descW < 18 {
		nameW, descW = width-5, 0
	}
	for _, i := range m.midRows() {
		r := m.rows[m.shown[i]]
		name := trunc(r.name, nameW)
		dot, glyph := ui.dim.Render("○"), "○"
		if slices.Contains(m.cfg.Enabled, r.name) {
			dot, glyph = ui.ok.Render("◉"), "◉"
		}
		badge, badgePlain := "", ""
		if r.note == "new" {
			badge, badgePlain = ui.badge.Render(" new ")+" ", " new  "
		}
		plain, styled := " "+glyph+" "+name, " "+dot+" "+name
		if descW > 0 {
			gap := strings.Repeat(" ", max(1, nameW+1-lipgloss.Width(name)))
			desc := trunc(m.descriptionOf(r.name), descW-lipgloss.Width(badgePlain))
			plain += gap + badgePlain + desc
			styled += gap + badge + ui.dim.Render(desc)
		} else {
			plain, styled = plain+badgePlain, styled+badge
		}
		if i == m.cursor {
			sel, styled = len(out), ui.bar.Render(fit(plain, width))
		}
		out = append(out, styled)
	}
	if len(out) == 1 {
		empty := " no skills here"
		if m.filter != "" {
			empty = " nothing matches " + trunc(m.filter, width-18)
		}
		out = append(out, "", ui.dim.Render(empty))
	}
	return out, sel
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
	w := m.popWidth()
	return lipgloss.NewStyle().MarginLeft(2).
		Render(ui.box.Render(pad(windowed(m.popBody, &m.popTop, -1, h-2), w-4, h-2)))
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
		out = append(out, "  "+ui.text.Render(trunc(line, max(10, m.width-4))))
	}
	if more != "" {
		out = append(out, more)
	}
	return strings.Join(out, "\n")
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
			state := ui.dim.Render("○ off")
			if slices.Contains(m.cfg.Targets, r.name) {
				state = ui.ok.Render("◉ on")
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
	state := ui.dim.Render("○ off")
	if slices.Contains(m.cfg.Enabled, r.name) {
		state = ui.ok.Render("◉ enabled")
	}
	b.WriteString(ui.text.Bold(true).Render(r.name) + "\n" + state + ui.dim.Render("  ·  "+r.src))
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
	if patches := m.patchesFor(r.name); len(patches) > 0 {
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

// trimScheme drops the https:// a repo URL carries, which says nothing on screen.
func trimScheme(url string) string {
	for _, p := range []string{"https://", "http://", "git@"} {
		if after, ok := strings.CutPrefix(url, p); ok {
			return after
		}
	}
	return url
}
