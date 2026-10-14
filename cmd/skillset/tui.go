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
)

type rowKind int

// The order is the order of the first column: sources, then targets, then the
// entries we don't own. Column one prints a heading wherever the kind changes.
const (
	rowSource rowKind = iota
	rowSkill
	rowTarget
	rowOther // an entry in an agent folder that this repo didn't create
)

var headings = map[rowKind]string{rowSource: "SOURCES", rowTarget: "TARGETS", rowOther: "NOT OURS"}

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

	rows   []row  // everything, unfiltered
	shown  []int  // indexes into rows that pass the filter
	cursor int    // index into shown
	source string // the source whose skills the middle column is showing

	left, mid int // first visible line of each column, for scrolling

	filter  string
	typing  bool     // the filter line has focus
	review  []string // the plan, when it is on screen
	cascade string   // what the last toggle pulled in or dropped
	status  string   // git position, or the result of the last action
	dirty   bool
	asked   bool // q pressed once with unsaved changes

	pop     string   // "skill" or "keys" while an overlay is up, "" when none is
	popBody []string // its lines, the first of them its heading
	popTop  int      // the first of those on screen
	query   string   // the keys overlay's search box

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
	useTheme(cfg.Theme)
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

	m.rows = nil
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

	for _, t := range targets {
		m.rows = append(m.rows, row{kind: rowTarget, name: t.name, note: filepath.Join("~", t.dir)})
	}
	m.rows = append(m.rows, m.unmanaged()...)

	if m.sourceRow(m.source) < 0 {
		m.source = ""
		for _, r := range m.rows {
			if r.kind == rowSource {
				m.source = r.name
				break
			}
		}
	}
	m.refilter()
}

// sourceRow is where a source sits in rows, or -1 when it no longer exists.
func (m *model) sourceRow(name string) int {
	for i, r := range m.rows {
		if r.kind == rowSource && r.name == name {
			return i
		}
	}
	return -1
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
		case r.kind == rowSource:
			continue // a filter is a search for skills; a source can't match one
		case !strings.Contains(strings.ToLower(r.name), f):
			continue
		}
		m.shown = append(m.shown, i)
	}
	if m.cursor >= len(m.shown) {
		m.cursor = max(0, len(m.shown)-1)
	}
	m.mark()
}

// at is the row under the cursor, and whether there is one at all.
func (m *model) at() (row, bool) {
	if len(m.shown) == 0 {
		return row{}, false
	}
	return m.rows[m.shown[m.cursor]], true
}

// mark remembers which source the skills column is showing, so stepping onto a
// target or a foreign entry doesn't empty it.
func (m *model) mark() {
	switch r, ok := m.at(); {
	case !ok:
	case r.kind == rowSource:
		m.source = r.name
	case r.kind == rowSkill:
		m.source = r.src
	}
}

// column lists the rows the cursor can reach without changing columns: the skills
// of the current source, or everything in the first column. While a filter is on,
// every matching skill is in the middle column, whatever source it came from.
func (m *model) column() []int {
	r, ok := m.at()
	if !ok {
		return nil
	}
	var out []int
	for i, idx := range m.shown {
		other := m.rows[idx]
		if (other.kind == rowSkill) != (r.kind == rowSkill) {
			continue
		}
		if other.kind == rowSkill && m.filter == "" && other.src != m.source {
			continue
		}
		out = append(out, i)
	}
	return out
}

// move steps up or down inside the current column.
func (m *model) move(step int) {
	col := m.column()
	if at := slices.Index(col, m.cursor); at >= 0 {
		m.cursor = col[min(max(at+step, 0), len(col)-1)]
	}
	m.mark()
}

// jump puts the cursor on the first shown row of a kind, with an optional name.
func (m *model) jump(kind rowKind, name string) bool {
	for i, idx := range m.shown {
		if r := m.rows[idx]; r.kind == kind && (name == "" || r.name == name || r.src == name) {
			m.cursor = i
			m.mark()
			return true
		}
	}
	return false
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
	if m.pop != "" {
		m.popKey(msg) // an overlay is over everything and swallows the rest
		return nil
	}
	if m.typing {
		switch msg.String() {
		case "enter", "esc":
			m.typing = false
			if msg.String() == "esc" {
				m.filter = ""
			}
		case " ":
			m.toggle() // skill names are kebab-case, so a space in a filter only empties the list
		case "up":
			m.move(-1) // the arrows, not j and k: those are still text
		case "down":
			m.move(1)
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
			// the rail, not the top line: at 80 columns the top line would cut it
			m.cascade = "unsaved changes — s to review them, q again to discard"
			return nil
		}
		return tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "left", "h":
		if r, ok := m.at(); ok && r.kind == rowSkill {
			m.jump(rowSource, r.src)
		}
	case "right", "l":
		if r, ok := m.at(); ok && r.kind != rowSkill {
			m.jump(rowSkill, m.source)
		}
	case " ", "x":
		m.toggle()
	case "/":
		m.typing = true
	case "p":
		m.status = "pulling..."
		return pull(m.cfg.Repo)
	case "s":
		m.openReview()
	case "enter":
		if r, ok := m.at(); ok && r.kind == rowSkill {
			m.openSkill(r.name)
		}
	case "?":
		m.openKeys()
	}
	return nil
}

func (m *model) openReview() {
	var buf bytes.Buffer
	if err := apply(m.cfg, &buf, true); err != nil {
		m.status = err.Error()
		return
	}
	m.review = lines(buf.String())
	if len(m.review) == 0 {
		m.review = []string{"nothing to change"}
	}
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
	r, ok := m.at()
	if !ok {
		return
	}
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
