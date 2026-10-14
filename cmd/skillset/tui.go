package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	bold  = lipgloss.NewStyle().Bold(true)
	dim   = lipgloss.NewStyle().Faint(true)
	warn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	on    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	cur   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	fresh = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
)

type rowKind int

const (
	rowHeader rowKind = iota // a label, never selectable
	rowTarget
	rowSource
	rowSkill
	rowOther // an entry in an agent folder that this repo didn't create
)

type row struct {
	kind rowKind
	name string
	src  string
	note string
}

type model struct {
	cfg     *Config
	deps    map[string]dep
	sources map[string]source
	home    string

	rows    []row // everything, unfiltered
	shown   []int // indexes into rows that pass the filter
	cursor  int   // index into shown
	top     int   // first visible line, for scrolling
	filter  string
	typing  bool     // the filter line has focus
	review  []string // the plan, when it is on screen
	cascade string   // what the last toggle pulled in or dropped
	status  string   // git position, or the result of the last action
	dirty   bool
	asked   bool // q pressed once with unsaved changes

	desc    map[string]string   // skill -> description, read on demand
	patches map[string][]string // skill -> local patch commits, read on demand

	width, height int
}

// tuiCmd opens the selection screen. It never commits or pushes; `p` pulls, and
// saving goes through the same apply as the command line.
func tuiCmd() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	m, err := newModel(cfg)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(cfg *Config) (*model, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	m := &model{cfg: cfg, home: home, width: 100, height: 30,
		desc: map[string]string{}, patches: map[string][]string{}}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// reload re-reads the repo, which is what a pull or a new skill needs.
func (m *model) reload() error {
	sources, err := loadSources(m.cfg.Repo)
	if err != nil {
		return err
	}
	deps, err := loadDeps(m.cfg.Repo)
	if err != nil {
		return err
	}
	m.sources, m.deps = sources, deps
	m.desc, m.patches = map[string]string{}, map[string][]string{}
	m.build()
	return nil
}

func (m *model) build() {
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	inRepo := repoSkills(m.cfg.Repo)
	slices.Sort(inRepo)

	m.rows = []row{{kind: rowHeader, name: "agent targets"}}
	for _, t := range targets {
		m.rows = append(m.rows, row{kind: rowTarget, name: t.name, note: filepath.Join("~", t.dir)})
	}

	listed := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(m.sources)) {
		m.rows = append(m.rows, row{kind: rowSource, name: name})
		for _, skill := range slices.Sorted(slices.Values(m.sources[name].Skills)) {
			if !isSkill(skillsDir, skill) {
				continue
			}
			listed[skill] = true
			m.rows = append(m.rows, m.skillRow(skill, name))
		}
	}
	var loose []string
	for _, skill := range inRepo {
		if !listed[skill] {
			loose = append(loose, skill)
		}
	}
	if len(loose) > 0 {
		m.rows = append(m.rows, row{kind: rowSource, name: "unlisted"})
		for _, skill := range loose {
			m.rows = append(m.rows, m.skillRow(skill, "unlisted"))
		}
	}

	if other := m.unmanaged(); len(other) > 0 {
		m.rows = append(m.rows, row{kind: rowHeader, name: "not managed here, never touched"})
		m.rows = append(m.rows, other...)
	}
	m.refilter()
}

func (m *model) skillRow(skill, src string) row {
	note := ""
	if !slices.Contains(m.cfg.Seen, skill) {
		note = "new"
	}
	return row{kind: rowSkill, name: skill, src: src, note: note}
}

// unmanaged lists what else lives in the agent folders: anything that isn't a link
// into this repo. They are shown so they aren't a surprise, and never changed.
func (m *model) unmanaged() []row {
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	var out []row
	for _, t := range targets {
		dir := filepath.Join(m.home, t.dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dest := linkDest(filepath.Join(dir, e.Name()))
			if dest != "" && sameDir(filepath.Dir(dest), skillsDir) {
				continue
			}
			where := "folder"
			if dest != "" {
				where = "-> " + dest
			}
			out = append(out, row{kind: rowOther, name: filepath.Join("~", t.dir, e.Name()), note: where})
		}
	}
	return out
}

func (m *model) refilter() {
	m.shown = nil
	f := strings.ToLower(m.filter)
	for i, r := range m.rows {
		switch {
		case f == "":
		case r.kind == rowHeader || r.kind == rowSource:
			continue // a header only earns its place through a child below
		case !strings.Contains(strings.ToLower(r.name), f):
			continue
		}
		m.shown = append(m.shown, i)
	}
	if m.cursor >= len(m.shown) {
		m.cursor = max(0, len(m.shown)-1)
	}
	m.skipHeader(1)
}

// skipHeader moves off a label row, which can't be selected.
func (m *model) skipHeader(step int) {
	for len(m.shown) > 0 && m.rows[m.shown[m.cursor]].kind == rowHeader {
		next := m.cursor + step
		if next < 0 || next >= len(m.shown) {
			step = -step
			next = m.cursor + step
		}
		if next < 0 || next >= len(m.shown) {
			return
		}
		m.cursor = next
	}
}

func (m *model) Init() tea.Cmd { return gitPosition(m.cfg.Repo) }

type gitMsg string

// gitPosition asks origin where we stand, without changing anything.
func gitPosition(repo string) tea.Cmd {
	return func() tea.Msg {
		gitOK(repo, "fetch", "--quiet")
		behind, err := git(repo, "rev-list", "--count", "HEAD..@{upstream}")
		if err != nil {
			return gitMsg("")
		}
		if behind == "0" {
			return gitMsg("up to date with origin")
		}
		return gitMsg(behind + " commits behind origin — press p to pull")
	}
}

func pull(repo string) tea.Cmd {
	return func() tea.Msg {
		out, err := git(repo, "pull", "--ff-only")
		if err != nil {
			return gitMsg(err.Error())
		}
		return gitMsg(strings.SplitN(out, "\n", 2)[0])
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case gitMsg:
		m.status = string(msg)
		if err := m.reload(); err != nil {
			m.status = err.Error()
		}
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) key(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit // before the filter and review screens, which swallow keys
	}
	if m.typing {
		switch msg.String() {
		case "enter", "esc":
			m.typing = false
			if msg.String() == "esc" {
				m.filter = ""
			}
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
			}
		default:
			if len(msg.Runes) > 0 {
				m.filter += string(msg.Runes)
			}
		}
		m.refilter()
		return nil
	}
	if m.review != nil {
		switch msg.String() {
		case "enter":
			return m.save()
		case "esc", "q":
			m.review = nil
		}
		return nil
	}

	switch msg.String() {
	case "q":
		if m.dirty && !m.asked {
			m.asked = true
			m.status = "unsaved changes — s to review them, q again to discard"
			return nil
		}
		return tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.skipHeader(-1)
		}
	case "down", "j":
		if m.cursor < len(m.shown)-1 {
			m.cursor++
			m.skipHeader(1)
		}
	case " ", "x":
		m.toggle()
	case "/":
		m.typing = true
	case "p":
		m.status = "pulling..."
		return pull(m.cfg.Repo)
	case "s":
		var buf bytes.Buffer
		if err := apply(m.cfg, &buf, true); err != nil {
			m.status = err.Error()
			return nil
		}
		m.review = lines(buf.String())
		if len(m.review) == 0 {
			m.review = []string{"nothing to change"}
		}
	}
	return nil
}

// save applies the selection for real, through the same code as `skillset apply`.
func (m *model) save() tea.Cmd {
	var buf bytes.Buffer
	if err := apply(m.cfg, &buf, false); err != nil {
		m.status = err.Error()
		return nil
	}
	for _, s := range repoSkills(m.cfg.Repo) {
		if !slices.Contains(m.cfg.Seen, s) {
			m.cfg.Seen = append(m.cfg.Seen, s) // shown once, so no longer new
		}
	}
	if err := m.cfg.save(); err != nil {
		m.status = err.Error()
		return nil
	}
	m.review, m.dirty, m.asked = nil, false, false
	m.status = fmt.Sprintf("applied: %d changes", len(lines(buf.String())))
	m.build()
	return nil
}

func (m *model) toggle() {
	if len(m.shown) == 0 {
		return
	}
	r := m.rows[m.shown[m.cursor]]
	m.cascade, m.asked = "", false
	switch r.kind {
	case rowTarget:
		if i := slices.Index(m.cfg.Targets, r.name); i >= 0 {
			m.cfg.Targets = slices.Delete(m.cfg.Targets, i, i+1)
			m.cascade = r.name + " off: its links and hooks go when you save"
		} else {
			m.cfg.Targets = append(m.cfg.Targets, r.name)
		}
		m.dirty = true
	case rowSource:
		m.toggleSource(r.name)
	case rowSkill:
		m.toggleSkill(r.name)
	default:
		return // headers and foreign entries aren't ours to change, and clear nothing
	}
	m.build()
}

func (m *model) toggleSkill(name string) {
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	if slices.Contains(m.cfg.Enabled, name) {
		var drop []string
		withDependents(name, m.deps, &drop)
		var also []string
		for _, n := range drop {
			if i := slices.Index(m.cfg.Enabled, n); i >= 0 {
				m.cfg.Enabled = slices.Delete(m.cfg.Enabled, i, i+1)
				if n != name {
					also = append(also, n)
				}
			}
		}
		m.cascade = "off: " + name
		if len(also) > 0 {
			slices.Sort(also)
			m.cascade += " — also off, they require it: " + strings.Join(also, ", ")
		}
		m.dirty = true
		return
	}
	var want []string
	if err := withRequires(name, m.deps, skillsDir, &want); err != nil {
		m.cascade = "can't enable " + name + ": " + err.Error()
		return // nothing changed, so earlier changes stay unsaved
	}
	var also []string
	for _, n := range want {
		if !slices.Contains(m.cfg.Enabled, n) {
			m.cfg.Enabled = append(m.cfg.Enabled, n)
			if n != name {
				also = append(also, n)
			}
		}
	}
	m.cascade, m.dirty = "on: "+name, true
	if len(also) > 0 {
		slices.Sort(also)
		m.cascade += " — also on, it requires them: " + strings.Join(also, ", ")
	}
}

func (m *model) toggleSource(src string) {
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	var skills []string
	for _, s := range m.sourceSkills(src) {
		if isSkill(skillsDir, s) {
			skills = append(skills, s)
		}
	}
	allOn := len(skills) > 0
	for _, s := range skills {
		if !slices.Contains(m.cfg.Enabled, s) {
			allOn = false
			break
		}
	}
	var touched []string
	for _, s := range skills {
		if allOn == slices.Contains(m.cfg.Enabled, s) {
			m.toggleSkill(s)
			touched = append(touched, s)
		}
	}
	verb := "on"
	if allOn {
		verb = "off"
	}
	m.cascade = fmt.Sprintf("%s %s: %d skills", src, verb, len(touched))
}

func (m *model) sourceSkills(src string) []string {
	if s, ok := m.sources[src]; ok {
		return s.Skills
	}
	var loose []string // the "unlisted" group: in the repo, in no source
	for _, s := range repoSkills(m.cfg.Repo) {
		if m.sourceOf(s) == "" {
			loose = append(loose, s)
		}
	}
	return loose
}

func (m *model) sourceOf(skill string) string {
	for name, s := range m.sources {
		if slices.Contains(s.Skills, skill) {
			return name
		}
	}
	return ""
}

func lines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

const help = "space toggle · / filter · s review and save · p pull · q quit"

func (m *model) View() string {
	if m.review != nil {
		fits := max(5, m.height-6)
		body := m.review
		more := ""
		if len(body) > fits {
			more = dim.Render(fmt.Sprintf("\n… and %d more", len(body)-fits))
			body = body[:fits]
		}
		return bold.Render("review ") + dim.Render(countPlan(m.review)) + "\n\n" +
			strings.Join(body, "\n") + more + "\n\n" +
			dim.Render("enter applies this · esc goes back")
	}

	var foot []string
	if m.cascade != "" {
		foot = append(foot, m.cascade)
	}
	for _, w := range warnings(m.cfg, m.deps) {
		foot = append(foot, warn.Render(w))
	}
	if m.status != "" {
		foot = append(foot, dim.Render(m.status))
	}
	detailW := max(30, m.width/3)
	listW := max(30, m.width-detailW-3)
	rows := m.list(listW, max(5, m.height-len(foot)-5))
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(listW).Render(rows),
		dim.Render(" │ "),
		lipgloss.NewStyle().Width(detailW).Render(m.detail()))

	filter := ""
	if m.typing || m.filter != "" {
		filter = "  filter: " + m.filter + "▌"
	}
	return bold.Render("skillset") + dim.Render("  "+m.cfg.Repo) + filter + "\n\n" +
		panes + "\n\n" + strings.Join(foot, "\n") + "\n" + dim.Render(help)
}

// list renders the visible slice of rows, keeping the cursor on screen.
func (m *model) list(width, height int) string {
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+height {
		m.top = m.cursor - height + 1
	}
	var b strings.Builder
	for i := m.top; i < len(m.shown) && i < m.top+height; i++ {
		r := m.rows[m.shown[i]]
		line := m.line(r)
		if i == m.cursor {
			line = cur.Render("› ") + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	if len(m.shown) > m.top+height {
		b.WriteString(dim.Render(fmt.Sprintf("  … %d more", len(m.shown)-m.top-height)))
	}
	return b.String()
}

func (m *model) line(r row) string {
	switch r.kind {
	case rowHeader:
		return dim.Render(strings.ToUpper(r.name))
	case rowTarget:
		return m.box(slices.Contains(m.cfg.Targets, r.name)) + " " + r.name + dim.Render("  "+r.note)
	case rowSource:
		count, total := 0, 0
		skillsDir := filepath.Join(m.cfg.Repo, "skills")
		for _, s := range m.sourceSkills(r.name) {
			if !isSkill(skillsDir, s) {
				continue
			}
			total++
			if slices.Contains(m.cfg.Enabled, s) {
				count++
			}
		}
		state := m.box(count == total && total > 0)
		if count > 0 && count < total {
			state = "[~]"
		}
		return state + " " + bold.Render(r.name) + dim.Render(fmt.Sprintf("  %d/%d", count, total))
	case rowSkill:
		line := "  " + m.box(slices.Contains(m.cfg.Enabled, r.name)) + " " + r.name
		if r.note != "" {
			line += " " + fresh.Render("new")
		}
		return line
	default:
		return dim.Render("  " + r.name + "  " + r.note)
	}
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

func (m *model) box(checked bool) string {
	if checked {
		return on.Render("[x]")
	}
	return "[ ]"
}

// detail explains the highlighted skill, so a choice doesn't need a file open.
func (m *model) detail() string {
	if len(m.shown) == 0 {
		return dim.Render("nothing matches " + m.filter)
	}
	r := m.rows[m.shown[m.cursor]]
	switch r.kind {
	case rowTarget:
		for _, t := range targets {
			if t.name == r.name {
				return bold.Render(r.name) + "\n\nskills: ~/" + filepath.ToSlash(t.dir) +
					"\nhooks:  ~/" + filepath.ToSlash(t.hooks)
			}
		}
	case rowSource:
		s := m.sources[r.name]
		if s.Repo == "" {
			return bold.Render(r.name) + "\n\n" + dim.Render("in the repo, in no source")
		}
		return bold.Render(r.name) + "\n\n" + s.Repo + "\nbranch " + s.Ref + "\npath " + s.Path
	case rowSkill:
		return m.skillDetail(r)
	case rowOther:
		return bold.Render("not ours") + "\n\n" + r.name + "\n" + r.note +
			"\n\n" + dim.Render("shown so it isn't a surprise; skillset never changes it")
	}
	return ""
}

func (m *model) skillDetail(r row) string {
	var b strings.Builder
	b.WriteString(bold.Render(r.name) + "\n")
	state := "off"
	if slices.Contains(m.cfg.Enabled, r.name) {
		state = on.Render("enabled")
	}
	if r.note == "new" {
		state += " " + fresh.Render("new")
	}
	b.WriteString(state + dim.Render("  "+r.src) + "\n\n")

	d := m.deps[r.name]
	var dependents []string
	withDependents(r.name, m.deps, &dependents)
	dependents = slices.DeleteFunc(dependents, func(s string) bool { return s == r.name })
	for _, pair := range [][2]any{{"requires", d.Requires}, {"optional", d.Optional}, {"required by", dependents}} {
		if list := pair[1].([]string); len(list) > 0 {
			b.WriteString(dim.Render(pair[0].(string)+": ") + strings.Join(list, ", ") + "\n")
		}
	}
	if patches := m.patchesFor(r.name); len(patches) > 0 {
		b.WriteString(dim.Render("local patches:") + "\n")
		for _, p := range patches {
			b.WriteString("  " + p + "\n")
		}
	}
	if desc := m.descriptionOf(r.name); desc != "" {
		b.WriteString("\n" + desc)
	}
	return b.String()
}

// descriptionOf reads the skill's own description out of its frontmatter.
func (m *model) descriptionOf(name string) string {
	if d, ok := m.desc[name]; ok {
		return d
	}
	m.desc[name] = ""
	body, err := os.ReadFile(filepath.Join(m.cfg.Repo, "skills", name, "SKILL.md"))
	if err != nil {
		return ""
	}
	rows := strings.Split(string(body), "\n")
	for i, line := range rows {
		after, ok := strings.CutPrefix(line, "description:")
		if !ok {
			continue
		}
		value := strings.TrimSpace(after)
		if value == "" || value == ">" || value == "|" || value == ">-" || value == "|-" {
			value = blockValue(rows[i+1:]) // YAML folded or literal: the text is below
		}
		m.desc[name] = strings.Trim(value, `"`)
		break
	}
	return m.desc[name]
}

// blockValue joins the indented lines of a YAML block scalar into one line.
func blockValue(rows []string) string {
	var parts []string
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			break
		}
		if !strings.HasPrefix(row, " ") && !strings.HasPrefix(row, "\t") {
			break
		}
		parts = append(parts, strings.TrimSpace(row))
	}
	return strings.Join(parts, " ")
}

// patchesFor lists this repo's own commits against the skill, by the patch( convention.
func (m *model) patchesFor(name string) []string {
	if p, ok := m.patches[name]; ok {
		return p
	}
	m.patches[name] = nil
	out, err := git(m.cfg.Repo, "log", "--oneline", "--grep", "^patch", "--", filepath.Join("skills", name))
	if err == nil {
		m.patches[name] = lines(out)
	}
	return m.patches[name]
}
