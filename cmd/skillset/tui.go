package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

type rowKind int

// The order is the order of the steps: the sources and profiles with their skills,
// then the targets and the entries we don't own. Column one prints a heading wherever
// the kind changes.
const (
	rowSource rowKind = iota
	rowProfile
	rowSkill
	rowTarget
	rowOther // an entry in an agent folder that this repo didn't create
)

const profilePrefix = "profile:"

func profileGroup(name string) string { return profilePrefix + name }

func profileName(group string) (string, bool) {
	return strings.CutPrefix(group, profilePrefix)
}

func isProfileGroup(group string) bool {
	_, ok := profileName(group)
	return ok
}

// stepOf is the screen a row belongs to. Targets and what else lives in their
// folders are the second step; everything else is the first.
func stepOf(k rowKind) tabID {
	if k == rowTarget || k == rowOther {
		return tabTargets
	}
	return tabSkills
}

var headings = map[rowKind]string{rowSource: "SOURCES", rowProfile: "PROFILES", rowTarget: "TARGETS", rowOther: "NOT OURS"}

// profileEdit is the overlay that creates or edits a profile: name on the first
// line, skills on the rest.
const profileEdit = "profiles"

type row struct {
	kind rowKind
	name string
	src  string // a skill's source/profile group, or the folder a target or foreign entry is
	note string
}

type model struct {
	cfg      *Config
	deps     map[string]dep
	sources  map[string]source
	profiles map[string]profile
	home     string

	step   tabID  // the step on screen: the skills or the targets
	rows   []row  // everything, unfiltered
	shown  []int  // indexes into rows that are on this step and pass the filter
	cursor int    // index into shown
	source string // the source or profile group whose skills the middle column is showing

	left, mid int // first visible line of each column, for scrolling

	filter  string
	typing  bool     // the filter line has focus
	review  []string // the plan, when it is on screen
	cascade string   // what a save would change, and what the last toggle dragged along
	status  string   // git position, or the result of the last action
	dirty   bool     // there is something a save would write
	asked   bool     // q pressed once with unsaved changes
	// deleting is the profile a first d asked about; a second d on it deletes.
	// deleted holds profiles whose definitions go from profiles.json on save.
	deleting string
	deleted  []string

	// the selection as it was last written, so the rail can say what a save would
	// change rather than only what the last toggle did
	savedOn, savedProfiles, savedTargets []string

	pop     string   // "skill:<name>" or "keys" while an overlay is up, "" when none is
	popBody []string // its lines, the first of them its heading
	popTop  int      // the first of those on screen
	query   string   // the keys overlay's search box
	setAt   int      // the settings row the overlay's cursor is on, len(settings) for the list

	// profile editing: the overlay holds three text fields, name, description and
	// skills (space-separated), and editField is the one with focus. editOrig is
	// the profile being edited, "" when creating one.
	editOrig   string
	edit       [3]string
	editField  int
	editedDisk bool // a profile edit has been written to profiles.json this session

	// the skills checklist the editor shows while its skills field has focus:
	// every skill in the main screen's order, the cursor, and a filter typed after /
	pickAll    []row
	pickAt     int
	pickFilter string
	pickTyping bool
	pickLine   int // the popBody line the cursor is on, for scrolling

	desc    map[string]string   // skill -> description, read on demand
	patches map[string][]string // skill -> local patch commits, loaded off the draw

	loading map[string]bool // the slow work that is outstanding, by key
	frame   int             // the spinner's frame while any of it is
	ticking bool            // a tick is already in flight, and one is enough

	hit           layout // where the last draw put things, so a click can find its row
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
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func newModel(cfg *Config) (*model, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	m := &model{cfg: cfg, home: home, width: 100, height: 30,
		desc: map[string]string{}, patches: map[string][]string{}, loading: map[string]bool{},
		savedOn:       slices.Clone(cfg.Enabled),
		savedProfiles: slices.Clone(cfg.Profiles), savedTargets: slices.Clone(cfg.Targets)}
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
	profiles, err := loadProfiles(m.cfg.Repo)
	if err != nil {
		return err
	}
	for _, name := range m.deleted {
		delete(profiles, name) // still pending: a reload mustn't bring it back
	}
	m.sources, m.deps, m.profiles = sources, deps, profiles
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

	for _, name := range m.profileNames() {
		r := row{kind: rowProfile, name: name, src: profileGroup(name)}
		p, exists := m.profiles[name]
		if !exists {
			r.note = "missing"
		}
		m.rows = append(m.rows, r)
		listed := map[string]bool{}
		for _, skill := range slices.Sorted(slices.Values(p.Skills)) {
			if listed[skill] || !isSkill(skillsDir, skill) {
				continue
			}
			listed[skill] = true
			m.rows = append(m.rows, m.skillRow(skill, r.src))
		}
	}

	for _, t := range targets {
		m.rows = append(m.rows, row{kind: rowTarget, name: t.name,
			src: filepath.Join(m.home, t.dir), note: filepath.Join("~", t.dir)})
	}
	m.rows = append(m.rows, m.unmanaged()...)

	if m.groupRow(m.source) < 0 {
		m.source = ""
		for _, r := range m.rows {
			if r.kind == rowSource || r.kind == rowProfile {
				m.source = groupKey(r)
				break
			}
		}
	}
	m.refilter()
}

func groupKey(r row) string {
	if r.kind == rowProfile {
		return r.src
	}
	return r.name
}

// groupRow is where a source or profile sits in rows, or -1 when it no longer exists.
func (m *model) groupRow(name string) int {
	for i, r := range m.rows {
		if (r.kind == rowSource || r.kind == rowProfile) && groupKey(r) == name {
			return i
		}
	}
	return -1
}

// profileNames includes stale enabled profiles so their membership record can be
// turned off even after the definition disappears from profiles.json.
func (m *model) profileNames() []string {
	names := map[string]bool{}
	for name := range m.profiles {
		names[name] = true
	}
	for _, name := range m.cfg.Profiles {
		names[name] = true
	}
	return slices.Sorted(maps.Keys(names))
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
			out = append(out, row{kind: rowOther, name: filepath.Join("~", t.dir, e.Name()),
				src: filepath.Join(dir, e.Name()), note: where})
		}
	}
	return out
}

func (m *model) refilter() {
	m.shown = nil
	f := strings.ToLower(m.filter)
	for i, r := range m.rows {
		if stepOf(r.kind) != m.step {
			continue // another step's rows are not on this screen to walk onto
		}
		switch {
		case f == "":
		case r.kind != rowSkill:
			continue // a filter is a search for skills, not their source or profile
		case isProfileGroup(r.src):
			continue // source rows already carry each skill; don't show profile duplicates
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

// mark remembers which source the skills column is showing, so a step away and
// back doesn't empty it.
func (m *model) mark() {
	switch r, ok := m.at(); {
	case !ok:
	case r.kind == rowSource || r.kind == rowProfile:
		m.source = groupKey(r)
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
	m.clear()
	m.mark()
}

// jump puts the cursor on the first shown row of a kind, with an optional name.
func (m *model) jump(kind rowKind, name string) bool {
	for i, idx := range m.shown {
		if r := m.rows[idx]; r.kind == kind && (name == "" || r.name == name || r.src == name) {
			m.cursor = i
			m.clear()
			m.mark()
			return true
		}
	}
	return false
}

func (m *model) jumpGroup(name string) bool {
	for i, idx := range m.shown {
		r := m.rows[idx]
		if (r.kind == rowSource || r.kind == rowProfile) && groupKey(r) == name {
			m.cursor = i
			m.clear()
			m.mark()
			return true
		}
	}
	return false
}

// clear drops the rail's line once the cursor leaves the row it was about, and
// the pending quit with it, so a q after wandering off still asks once.
func (m *model) clear() { m.cascade, m.asked, m.deleting = "", false, "" }

// setStep moves to a step of the flow. The changes step is the review, which is
// built fresh and sits over the step you opened it from.
func (m *model) setStep(s tabID) {
	if s == tabChanges {
		m.openReview()
		return
	}
	if s == m.step {
		return
	}
	m.step, m.cursor = s, 0
	m.refilter()
	if s == tabSkills {
		m.jumpGroup(m.source) // back on the source or profile you were reading
	}
}

// back dismisses the topmost thing there is: the review, then the filter, then
// the step you are on, and when there is nothing left it quits. Popups and the
// filter's own keys are handled before this.
func (m *model) back() tea.Cmd {
	switch {
	case m.review != nil:
		m.review = nil
	case m.filter != "":
		m.filter = ""
		m.refilter()
	case m.step > tabSkills:
		m.setStep(m.step - 1)
	case m.dirty && !m.asked:
		m.asked = true
		// the rail, not the top line: at 80 columns the top line would cut it
		m.cascade = "unsaved changes — s to review them, q again to discard"
		if m.editedDisk {
			m.cascade += " (profile edits are already in profiles.json)"
		}
	default:
		return tea.Quit
	}
	return nil
}

// inside is what an agent folder or a foreign entry actually holds, so the middle
// column on the targets step shows that rather than the last source's skills.
// shortcut: read per draw, not cached; cache it like desc if a huge folder drags.
func inside(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // a file or a dangling link has nothing to list
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// Init draws first and asks origin afterwards: the fetch is a second or more of
// network, and the screen is useful before it answers.
func (m *model) Init() tea.Cmd {
	m.status = "checking origin"
	return tea.Batch(m.work("origin", gitPosition(m.cfg.Repo)), m.load())
}

// work starts something slow and keeps the spinner turning while it runs. Every
// slow thing in here goes through it, so nothing can block a draw.
func (m *model) work(key string, cmd tea.Cmd) tea.Cmd {
	m.loading[key] = true
	if m.ticking {
		return cmd
	}
	m.ticking = true
	return tea.Batch(cmd, tick())
}

// load is the slow work the row under the cursor needs, if it isn't loaded or
// already on its way. The detail pane renders what is known and nothing waits.
func (m *model) load() tea.Cmd {
	r, ok := m.at()
	if !ok || r.kind != rowSkill {
		return nil
	}
	key := "patch:" + r.name
	if _, done := m.patches[r.name]; done || m.loading[key] {
		return nil
	}
	return m.work(key, patchesFor(m.cfg.Repo, r.name))
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// spin is the sign that something is still running. It only moves while work is
// outstanding, so an idle screen is never redrawn.
func (m *model) spin() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}
	return frames[m.frame%len(frames)]
}

type gitMsg string

type patchMsg struct {
	name string
	list []string
}

// patchesFor lists this repo's own commits against the skill, by the patch(
// convention. It is a command because git against a real repo is tens of
// milliseconds, and the detail pane asks for it on every cursor move.
func patchesFor(repo, name string) tea.Cmd {
	return func() tea.Msg {
		out, err := git(repo, "log", "--oneline", "--grep", "^patch", "--", filepath.Join("skills", name))
		if err != nil {
			return patchMsg{name: name}
		}
		return patchMsg{name: name, list: lines(out)}
	}
}

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

// syncing runs `skillset sync` for each source in turn and reports the first line
// of each. It stops at the first failure, which is usually a conflict to resolve.
func syncing(repo string, names []string) tea.Cmd {
	return func() tea.Msg {
		var done []string
		for _, name := range names {
			var buf bytes.Buffer
			if err := syncSource(repo, name, &buf); err != nil {
				return gitMsg(name + ": " + strings.SplitN(err.Error(), "\n", 2)[0])
			}
			done = append(done, strings.SplitN(buf.String(), "\n", 2)[0])
		}
		return gitMsg(strings.Join(done, " · "))
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizePop() // a SKILL.md is wrapped at a width, so a resize re-renders it
	case tickMsg:
		m.frame++
		if len(m.loading) > 0 {
			return m, tick()
		}
		m.ticking = false
		return m, nil
	case patchMsg:
		delete(m.loading, "patch:"+msg.name)
		m.patches[msg.name] = msg.list
		return m, nil
	case gitMsg:
		delete(m.loading, "origin")
		m.status = string(msg)
		if err := m.reload(); err != nil {
			m.status = err.Error()
		}
	case tea.KeyMsg:
		cmd = m.key(msg)
	case tea.MouseMsg:
		m.mouse(msg)
	}
	return m, tea.Batch(cmd, m.load())
}

// mouse is the pointer: the wheel scrolls whatever is under it, a click puts the
// cursor on a row or steps to a tab. A click never toggles, so a stray one can
// never change what a save would write.
func (m *model) mouse(msg tea.MouseMsg) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.scroll(msg.X, -1)
	case msg.Button == tea.MouseButtonWheelDown:
		m.scroll(msg.X, 1)
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		m.click(msg.X, msg.Y)
	}
}

// scroll moves what the pointer is over. A column scrolls by moving the cursor
// inside it, since the column follows the cursor; the pointer picks which one.
func (m *model) scroll(x, step int) {
	switch r, ok := m.at(); {
	case m.pop != "":
		m.popTop += step
		return
	case m.review != nil || x >= m.hit.x3:
		return // the review and the detail column are cut to fit, not scrolled
	case !ok:
	case x >= m.hit.x2 && r.kind != rowSkill:
		m.jump(rowSkill, m.source)
	case x < m.hit.x2 && r.kind == rowSkill:
		m.jumpGroup(r.src)
	}
	m.move(step)
}

// click puts the cursor where you pointed. Toggling stays on space alone.
func (m *model) click(x, y int) {
	if m.pop != "" {
		return // the overlay is over everything; esc is the way out of it
	}
	if y == tabLine {
		for i, end := range m.hit.tabs {
			if x < end {
				m.review = nil // a tab is a way out of the review, as esc is
				m.setStep(tabID(i))
				return
			}
		}
		return
	}
	if m.review != nil || x >= m.hit.x3 {
		return // neither the review nor the detail column holds rows to land on
	}
	hits := m.hit.first
	if x >= m.hit.x2 {
		hits = m.hit.mid
	}
	if line := y - bodyTop; line >= 0 && line < len(hits) && hits[line] >= 0 {
		m.cursor = hits[line]
		m.clear()
		m.mark()
	}
}

func (m *model) key(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit // before the filter and review screens, which swallow keys
	}
	if m.pop != "" {
		if m.pop == profileEdit {
			m.editKey(msg)
			return nil
		}
		if msg.String() == "q" && m.pop != "keys" {
			msg = tea.KeyMsg{Type: tea.KeyEsc} // only the keys overlay takes typing, so here q dismisses
		}
		m.popKey(msg) // an overlay is over everything and swallows the rest
		return nil
	}
	if m.typing {
		switch msg.String() {
		// esc peels one level: the focus now, the filter itself on the next press
		case "enter", "esc":
			m.typing = false
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
		case "esc", "q", "shift+tab":
			return m.back()
		}
		return nil // the review is the last step: tab has nowhere forward to go
	}

	key := msg.String()
	if m.loading["origin"] && (key == "p" || key == "u" || key == "U") {
		return nil // one git job at a time; the status line names the one running
	}
	switch key {
	case "esc", "q":
		return m.back()
	case "tab":
		m.setStep(min(m.step+1, tabChanges))
	case "shift+tab":
		m.setStep(max(m.step-1, tabSkills)) // tab doesn't wrap; this is the way back
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "left", "h":
		if r, ok := m.at(); ok && r.kind == rowSkill {
			m.jumpGroup(r.src)
		}
	case "right", "l":
		if r, ok := m.at(); ok && r.kind != rowSkill {
			m.jump(rowSkill, m.source)
		}
	case " ", "x":
		m.toggle()
	case "n":
		if m.step == tabSkills {
			m.openEdit(row{}) // always a new profile: a typed name must never rename one
		}
	case "e":
		if r, ok := m.at(); ok && m.step == tabSkills && profileNameGroup(r) != "" {
			m.openEdit(r)
		}
	case "d":
		if r, ok := m.at(); ok && r.kind == rowProfile {
			m.deleteProfile(r.name)
		}
	case "/":
		m.typing = true
	case "p":
		m.status = "pulling"
		return m.work("origin", pull(m.cfg.Repo))
	case "u":
		r, ok := m.at()
		if !ok || stepOf(r.kind) != tabSkills {
			return nil
		}
		src := r.name
		if r.kind == rowSkill {
			src = r.src
		}
		if _, listed := m.sources[src]; !listed {
			m.status = src + " has no upstream to sync"
			return nil
		}
		m.status = "syncing " + src
		return m.work("origin", syncing(m.cfg.Repo, []string{src}))
	case "U":
		m.status = "syncing every source"
		return m.work("origin", syncing(m.cfg.Repo, slices.Sorted(maps.Keys(m.sources))))
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

// openReview builds the plan. It walks the agent folders rather than the repo's
// history, which measures in single milliseconds, so it stays on the key path.
func (m *model) openReview() {
	var buf bytes.Buffer
	if err := apply(m.cfg, &buf, true); err != nil {
		m.status = err.Error()
		return
	}
	m.review = lines(buf.String())
	for _, name := range m.deleted {
		m.review = append([]string{"delete profile " + name + " from profiles.json"}, m.review...)
	}
	if len(m.review) == 0 {
		m.review = []string{"nothing to change"}
	}
}

// save applies the selection for real, through the same code as `skillset apply`.
func (m *model) save() tea.Cmd {
	if len(m.deleted) > 0 {
		if err := saveProfiles(m.cfg.Repo, m.profiles); err != nil {
			m.status = err.Error()
			return nil
		}
		m.deleted = nil
	}
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
	m.savedOn = slices.Clone(m.cfg.Enabled)
	m.savedProfiles, m.savedTargets = slices.Clone(m.cfg.Profiles), slices.Clone(m.cfg.Targets)
	m.status = fmt.Sprintf("applied: %d changes", len(lines(buf.String())))
	m.build()
	return nil
}

func (m *model) toggle() {
	r, ok := m.at()
	if !ok {
		return
	}
	m.clear()
	switch r.kind {
	case rowTarget:
		if i := slices.Index(m.cfg.Targets, r.name); i >= 0 {
			m.cfg.Targets = slices.Delete(m.cfg.Targets, i, i+1)
			m.cascade = r.name + " off: its links and hooks go when you save"
		} else {
			m.cfg.Targets = append(m.cfg.Targets, r.name)
		}
	case rowSource:
		m.toggleSource(r.name)
	case rowProfile:
		if !m.toggleProfile(r.name) {
			return
		}
	case rowSkill:
		if !m.toggleSkill(r.name) {
			return // nothing changed: the line already says why not
		}
	default:
		return // headers and foreign entries aren't ours to change, and clear nothing
	}
	// the whole pending state first, so a second toggle can't leave the line
	// describing only the first, then what this one dragged along
	pending := m.pending()
	switch {
	case pending == "":
	case m.cascade == "":
		m.cascade = pending
	default:
		m.cascade = pending + " — " + m.cascade
	}
	m.dirty = pending != "" // a skill turned on and off again leaves nothing to save
	m.build()
}

// pending is everything a save would change, measured against the selection as
// it was last written rather than against the last keypress.
func (m *model) pending() string {
	var parts []string
	if n := extra(m.cfg.Enabled, m.savedOn); n > 0 {
		parts = append(parts, plural(n, "skill")+" on")
	}
	if n := extra(m.savedOn, m.cfg.Enabled); n > 0 {
		parts = append(parts, plural(n, "skill")+" off")
	}
	if n := extra(m.cfg.Profiles, m.savedProfiles); n > 0 {
		parts = append(parts, plural(n, "profile")+" on")
	}
	if n := extra(m.savedProfiles, m.cfg.Profiles); n > 0 {
		parts = append(parts, plural(n, "profile")+" off")
	}
	if n := len(m.deleted); n > 0 {
		parts = append(parts, plural(n, "profile")+" deleted")
	}
	if n := extra(m.cfg.Targets, m.savedTargets) + extra(m.savedTargets, m.cfg.Targets); n > 0 {
		parts = append(parts, plural(n, "target")+" changed")
	}
	if len(parts) == 0 {
		return ""
	}
	return "pending: " + strings.Join(parts, ", ")
}

// extra is how many of a are not in b.
func extra(a, b []string) int {
	n := 0
	for _, s := range a {
		if !slices.Contains(b, s) {
			n++
		}
	}
	return n
}

// toggleSkill flips one skill with everything that hangs off it, and reports
// whether anything actually changed.
func (m *model) toggleSkill(name string) bool {
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
		if len(also) > 0 {
			slices.Sort(also)
			m.cascade = name + " off — also off, they require it: " + strings.Join(also, ", ")
		}
		return true
	}
	var want []string
	if err := withRequires(name, m.deps, skillsDir, &want); err != nil {
		m.cascade = "can't enable " + name + ": " + err.Error()
		return false // nothing changed, so earlier changes stay unsaved
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
	if len(also) > 0 {
		slices.Sort(also)
		m.cascade = name + " on — also on, it requires them: " + strings.Join(also, ", ")
	}
	return true
}

// profileNameGroup returns the profile name a row belongs to. A profile row is
// its own name; a skill row returns its profile only when it sits under one.
func profileNameGroup(r row) string {
	if r.kind == rowProfile {
		return r.name
	}
	if n, ok := profileName(r.src); ok {
		return n
	}
	return ""
}

// The editor's fields, in the order tab and enter walk them.
const (
	fieldName = iota
	fieldDesc
	fieldSkills
)

// openEdit puts the row's profile into the editor, or starts an empty one when
// the row isn't a profile or one of its skills.
func (m *model) openEdit(r row) {
	m.editOrig, m.edit, m.editField = "", [3]string{}, fieldName
	if name := profileNameGroup(r); name != "" {
		if p, exists := m.profiles[name]; exists {
			m.editOrig = name
			m.edit = [3]string{name, p.Description, strings.Join(p.Skills, " ")}
		}
	}
	m.pickAll = nil
	for _, r := range m.rows {
		if r.kind == rowSkill && !isProfileGroup(r.src) {
			m.pickAll = append(m.pickAll, r)
		}
	}
	m.pickAt, m.pickFilter, m.pickTyping = 0, "", false
	m.pop, m.popTop = profileEdit, 0
	m.popBody = m.editBody()
}

// members is the skills field as a list. edit[fieldSkills] keeps it as one
// string so a save reads one place, whatever filled it.
func (m *model) members() []string {
	return strings.Fields(m.edit[fieldSkills])
}

// toggleMember checks or unchecks a skill in the skills field.
func (m *model) toggleMember(name string) {
	list := m.members()
	if at := slices.Index(list, name); at >= 0 {
		list = slices.Delete(list, at, at+1)
	} else {
		list = append(list, name)
	}
	m.edit[fieldSkills] = strings.Join(list, " ")
}

// pickShown is the checklist after the filter.
func (m *model) pickShown() []row {
	f := strings.ToLower(m.pickFilter)
	return slices.DeleteFunc(slices.Clone(m.pickAll), func(r row) bool {
		return f != "" && !strings.Contains(strings.ToLower(r.name), f)
	})
}

// editBody renders the editor. Name and description are text fields; skills is a
// list: the chosen ones while it is out of focus, and a checklist of every skill,
// grouped like the main screen, while it has focus.
func (m *model) editBody() []string {
	title := "new profile"
	if m.editOrig != "" {
		title = "edit profile " + m.editOrig
	}
	hint := "tab/enter next field  ·  ctrl+u clears  ·  esc cancels"
	if m.editField == fieldSkills {
		hint = "space toggles  ·  / filters  ·  enter saves  ·  tab next field  ·  esc cancels"
	}
	out := []string{ui.text.Bold(true).Render(title), ui.dim.Render(hint), ""}
	for i, label := range []string{"name        ", "description "} {
		value := ui.dim.Render(m.edit[i])
		if i == m.editField {
			value = ui.text.Render(m.edit[i]) + ui.bar.Render(" ") + ui.label.Render("◂")
		}
		out = append(out, ui.label.Render(label)+value)
	}

	members := m.members()
	heading := ui.label.Render("skills      ") + ui.dim.Render(plural(len(members), "skill")+" picked")
	if m.editField == fieldSkills {
		heading += ui.label.Render("  ◂")
	}
	out = append(out, "", heading)
	m.pickLine = -1
	if m.editField != fieldSkills {
		if len(members) == 0 {
			return append(out, ui.dim.Render("  none yet: tab here, then space picks skills"))
		}
		for _, name := range sortedUnique(members) {
			out = append(out, "  "+ui.ok.Render(boxOn)+" "+ui.text.Render(name))
		}
		return out
	}

	if m.pickFilter != "" || m.pickTyping {
		filter := "  " + ui.label.Render("filter ") + ui.text.Render(m.pickFilter)
		if m.pickTyping {
			filter += ui.bar.Render(" ")
		}
		out = append(out, filter)
	}
	empty, group := len(out), ""
	for i, r := range m.pickShown() {
		if r.src != group {
			group = r.src
			out = append(out, "  "+ui.label.Render(strings.ToUpper(group)))
		}
		box, name := ui.dim.Render(boxOff), ui.dim.Render(r.name)
		if slices.Contains(members, r.name) {
			box, name = ui.ok.Render(boxOn), ui.text.Render(r.name)
		}
		cursor := "    "
		if i == m.pickAt {
			cursor, m.pickLine = "  "+ui.label.Render("▸ "), len(out)
		}
		out = append(out, cursor+box+" "+name)
	}
	if len(out) == empty {
		out = append(out, ui.dim.Render("    no skill matches"))
	}
	return out
}

// editKey handles the profile editor overlay's keys.
func (m *model) editKey(msg tea.KeyMsg) {
	key := msg.String()
	if m.editField == fieldSkills && m.pickTyping {
		switch key {
		case "enter", "esc":
			m.pickTyping = false
		case "backspace":
			if r := []rune(m.pickFilter); len(r) > 0 {
				m.pickFilter = string(r[:len(r)-1])
			}
		case "ctrl+u":
			m.pickFilter = ""
		default:
			m.pickFilter += string(msg.Runes)
		}
		m.pickAt = 0
		m.popBody = m.editBody()
		return
	}
	switch key {
	case "esc":
		m.pop, m.popBody = "", nil
		return
	case "enter":
		if m.editField == fieldSkills {
			m.saveEdit()
			return
		}
		m.editField++
	case "tab":
		m.editField = (m.editField + 1) % len(m.edit)
	case "shift+tab":
		m.editField = (m.editField + len(m.edit) - 1) % len(m.edit)
	default:
		if m.editField == fieldSkills {
			m.listKey(key)
		} else {
			m.textKey(msg)
		}
	}
	m.popBody = m.editBody()
}

// listKey moves through and toggles the skills checklist.
func (m *model) listKey(key string) {
	shown := m.pickShown()
	switch key {
	case "/":
		m.pickTyping = true
	case "up", "k":
		m.pickAt = max(m.pickAt-1, 0)
	case "down", "j":
		m.pickAt = min(m.pickAt+1, max(len(shown)-1, 0))
	case " ", "x":
		if m.pickAt < len(shown) {
			m.toggleMember(shown[m.pickAt].name)
		}
	}
}

// textKey types into the name or description field.
func (m *model) textKey(msg tea.KeyMsg) {
	f := &m.edit[m.editField]
	switch msg.String() {
	case "backspace":
		if r := []rune(*f); len(r) > 0 {
			*f = string(r[:len(r)-1])
		}
	case "ctrl+u":
		*f = ""
	case "ctrl+w":
		trimmed := strings.TrimRightFunc(*f, unicode.IsSpace)
		*f = trimmed[:strings.LastIndexFunc(trimmed, unicode.IsSpace)+1]
	default:
		if len(msg.Runes) > 0 {
			*f += string(msg.Runes)
		}
	}
}

// editError keeps the editor open with the reason the save was refused.
func (m *model) editError(msg string) {
	m.popBody = append(m.editBody(), "", ui.warn.Render(msg))
}

// saveEdit writes the profile to profiles.json under the same rules as the CLI.
// A rename moves the definition, and the machine's record of it, to the new name.
func (m *model) saveEdit() {
	name := strings.TrimSpace(m.edit[fieldName])
	members := m.members()
	skillsDir := filepath.Join(m.cfg.Repo, "skills")
	if name != m.editOrig {
		if _, taken := m.profiles[name]; taken {
			m.editError("a profile named " + name + " already exists")
			return
		}
		if err := checkProfileName(skillsDir, name); err != nil {
			m.editError(err.Error())
			return
		}
	}
	members, err := parseMembers(m.cfg.Repo, members)
	if err != nil {
		m.editError(err.Error())
		return
	}
	if len(members) == 0 {
		m.editError("a profile needs at least one skill")
		return
	}

	old := m.profiles[m.editOrig]
	updated := profile{Description: strings.TrimSpace(m.edit[fieldDesc]), Skills: members}
	profiles := maps.Clone(m.profiles)
	delete(profiles, m.editOrig)
	profiles[name] = updated
	cfg := *m.cfg
	cfg.Profiles = slices.Clone(m.cfg.Profiles)
	if m.editOrig != "" && name != m.editOrig {
		renameIn(cfg.Profiles, m.editOrig, name)
	}
	added, removed, err := resyncProfile(&cfg, name, old, updated, profiles, m.deps, skillsDir)
	if err != nil {
		m.editError(err.Error())
		return
	}
	if err := saveProfiles(m.cfg.Repo, profiles); err != nil {
		m.editError(err.Error())
		return
	}
	if m.editOrig != "" && name != m.editOrig {
		// a rename is no selection change, so the disk's selection takes it now:
		// otherwise discarding would leave the old name enabled with no definition
		renameIn(m.savedProfiles, m.editOrig, name)
		m.writeSaved()
	}
	m.profiles, *m.cfg, m.editedDisk = profiles, cfg, true
	m.cascade = "saved profile " + name
	if len(added) > 0 {
		m.cascade += "  ·  turned on: " + strings.Join(added, ", ")
	}
	if len(removed) > 0 {
		m.cascade += "  ·  turned off: " + strings.Join(removed, ", ")
	}
	m.dirty = m.pending() != ""
	m.pop, m.popBody = "", nil
	m.build()
}

// renameIn replaces old with name wherever it is in list.
func renameIn(list []string, old, name string) {
	if at := slices.Index(list, old); at >= 0 {
		list[at] = name
	}
}

// deleteProfile asks once, then turns the profile off and marks its definition
// for removal. Both join the pending review: save writes profiles.json with the
// selection, and discarding brings the profile back.
func (m *model) deleteProfile(name string) {
	if _, ok := m.profiles[name]; !ok {
		return
	}
	if m.deleting != name {
		m.clear()
		m.deleting = name
		m.cascade = "press d again to delete profile " + name + " on save: its skills are turned off, not removed from the repo"
		return
	}
	removed := removeProfile(m.cfg, name, m.profiles, m.deps)
	m.deleted = append(m.deleted, name)
	m.clear()
	m.cascade = "profile " + name + " deleted on save"
	if len(removed) > 0 {
		m.cascade += "  ·  turned off: " + strings.Join(removed, ", ")
	}
	m.dirty = m.pending() != ""
	m.build()
}

func (m *model) toggleProfile(name string) bool {
	p, exists := m.profiles[name]
	if slices.Contains(m.cfg.Profiles, name) {
		removed, _ := disableProfile(m.cfg, name, p, m.profiles, m.deps)
		m.cascade = "profile " + name + " off"
		if len(removed) > 0 {
			m.cascade += "  ·  turned off: " + strings.Join(removed, ", ")
		}
		var kept []string
		for _, member := range sortedUnique(p.Skills) {
			if slices.Contains(m.cfg.Enabled, member) {
				kept = append(kept, member)
			}
		}
		if len(kept) > 0 {
			m.cascade += "  ·  still on (another profile has them, or a skill needs them): " + strings.Join(kept, ", ")
		}
		return true
	}
	if !exists {
		m.cascade = "can't turn on " + name + ": it's no longer in profiles.json"
		return false
	}
	added, _, err := enableProfile(m.cfg, name, p, m.deps, filepath.Join(m.cfg.Repo, "skills"))
	if err != nil {
		m.cascade = "can't turn on " + name + ": " + err.Error()
		return false
	}
	m.cascade = "profile " + name + " on"
	if len(added) > 0 {
		m.cascade += "  ·  turned on: " + strings.Join(added, ", ")
	}
	return true
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
	for _, s := range skills {
		if allOn == slices.Contains(m.cfg.Enabled, s) {
			m.toggleSkill(s)
		}
	}
	verb := "on"
	if allOn {
		verb = "off"
	}
	m.cascade = src + " " + verb // the counts ride on the pending half of the line
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
// shortcut: one small cached file read, still on the draw path; make it a command
// like patchesFor if a source ever holds enough skills for it to show.
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
