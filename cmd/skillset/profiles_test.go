package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func profileRepo(t *testing.T) (home, repo string) {
	t.Helper()
	home, repo = depsRepo(t)
	write(t, filepath.Join(repo, "profiles.json"), `{
		"work":     {"description": "Everyday work", "skills": ["app", "nice"]},
		"overlap":  {"skills": ["app", "base"]},
		"core-only": {"skills": ["core"]},
		"broken-set": {"skills": ["app", "ghost"]}
	}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})
	return home, repo
}

func currentConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestProfileAddAndLs(t *testing.T) {
	home, repo := setup(t, "pr", "unslop", "research", "app")
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app"]}}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	skillset(t, "profile", "add", "docs", "pr", "unslop", "--desc", "write docs")
	skillset(t, "profile", "add", "docs", "research") // merge into the existing profile

	got := read(t, filepath.Join(repo, "profiles.json"))
	for _, want := range []string{`"docs"`, `"pr"`, `"unslop"`, `"research"`, `"write docs"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("profiles.json is missing %q:\n%s", want, got)
		}
	}
	if out := skillset(t, "profile", "ls"); !strings.Contains(out, "docs") || !strings.Contains(out, "write docs") {
		t.Fatalf("ls = %q, want docs listed with its description", out)
	}
	_ = home
}

func TestProfileAddRefusesUnknownSkill(t *testing.T) {
	home, repo := setup(t, "app", "core", "base")
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app"]}}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	err := run([]string{"profile", "add", "bad", "ghost"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "not skills") {
		t.Fatalf("err = %v, want the unknown skill refused", err)
	}
	got := read(t, filepath.Join(repo, "profiles.json"))
	if strings.Contains(got, "ghost") || strings.Contains(got, "bad") {
		t.Fatalf("profiles.json should be unchanged, got:\n%s", got)
	}
	_ = home
}

func TestProfileSetReplacesAndRmDeletes(t *testing.T) {
	_, repo := profileRepo(t)

	skillset(t, "profile", "set", "core-only", "base")
	skillset(t, "profile", "rm", "overlap")

	profiles, err := loadProfiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	p := profiles["core-only"]
	if !slices.Equal(p.Skills, []string{"base"}) {
		t.Fatalf("core-only members = %v, want only base after set", p.Skills)
	}
	if _, ok := profiles["overlap"]; ok {
		t.Fatalf("overlap should be gone after rm: %v", profiles)
	}
	err = run([]string{"profile", "rm", "overlap"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "no profile named") {
		t.Fatalf("rm of a missing profile = %v, want an error", err)
	}
}

func TestEnableProfileMergesSkillsAndRequirements(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	out := skillset(t, "enable", "work")

	cfg := currentConfig(t)
	if got := slices.Sorted(slices.Values(cfg.Enabled)); !slices.Equal(got, []string{"app", "base", "core", "nice"}) {
		t.Fatalf("enabled = %v, want the profile and app's requirements", got)
	}
	if !slices.Equal(cfg.Profiles, []string{"work"}) {
		t.Fatalf("profiles = %v, want work on", cfg.Profiles)
	}
	if len(cfg.Direct) != 0 {
		t.Fatalf("direct = %v, profile membership is not a direct skill selection", cfg.Direct)
	}
	if !strings.Contains(out, "enabled profile work") || !strings.Contains(out, "run `skillset apply`") {
		t.Fatalf("output = %q, want the profile and apply hint", out)
	}

	if out := skillset(t, "enable", "work"); !strings.Contains(out, "already enabled: work") {
		t.Fatalf("second enable = %q, want an idempotent result", out)
	}
}

func TestDisableProfileKeepsOverlapAndDirectSelections(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})
	skillset(t, "enable", "work")
	skillset(t, "enable", "overlap")
	skillset(t, "enable", "nice") // explicitly selected as well as supplied by work

	skillset(t, "disable", "work")

	cfg := currentConfig(t)
	if got := slices.Sorted(slices.Values(cfg.Enabled)); !slices.Equal(got, []string{"app", "base", "core", "nice"}) {
		t.Fatalf("enabled = %v, want overlap, direct and non-member requirements kept", got)
	}
	if !slices.Equal(cfg.Profiles, []string{"overlap"}) {
		t.Fatalf("profiles = %v, want only overlap left", cfg.Profiles)
	}

	skillset(t, "disable", "overlap")
	cfg = currentConfig(t)
	if got := slices.Sorted(slices.Values(cfg.Enabled)); !slices.Equal(got, []string{"base", "core", "nice"}) {
		t.Fatalf("enabled after overlap off = %v, want only app removed", got)
	}
}

func TestDisableProfileKeepsMembersRequiredByRemainingSkills(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})
	skillset(t, "enable", "core-only")
	skillset(t, "enable", "work")

	skillset(t, "disable", "core-only")

	if got := enabled(t); !slices.Contains(got, "core") {
		t.Fatalf("enabled = %v, core is still required by app", got)
	}
}

func TestProfileEnableIsAtomic(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	err := run([]string{"enable", "broken-set"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), `profile "broken-set" includes "ghost"`) {
		t.Fatalf("err = %v, want the missing member named", err)
	}
	cfg := currentConfig(t)
	if len(cfg.Enabled) != 0 || len(cfg.Profiles) != 0 {
		t.Fatalf("selection changed after a failed profile enable: %+v", cfg)
	}
}

func TestProfilePrefixAvoidsSkillNameCollision(t *testing.T) {
	_, repo := setup(t, "work")
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["work"]}}`)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	skillset(t, "enable", "profile:work")

	cfg := currentConfig(t)
	if !slices.Equal(cfg.Profiles, []string{"work"}) || len(cfg.Direct) != 0 {
		t.Fatalf("selection = %+v, want the profile rather than the same-named skill", cfg)
	}
}

func TestDisableProfileWhoseDefinitionWasRemoved(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Profiles: []string{"old"}})

	skillset(t, "disable", "old")

	if got := currentConfig(t).Profiles; len(got) != 0 {
		t.Fatalf("profiles = %v, want the stale enabled profile cleared", got)
	}
}

func TestTUIProfileEditCreatesAndDeletes(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")

	// edit the existing profile's members
	press(m, "n")
	if m.pop != profileEdit {
		t.Fatalf("n should open the editor, pop = %q", m.pop)
	}
	m.editField, m.edit[fieldSkills] = fieldSkills, "app core"
	press(m, "enter") // enter on skills saves
	if got := read(t, filepath.Join(repo, "profiles.json")); !strings.Contains(got, `"core"`) {
		t.Fatalf("edited profile should contain core:\n%s", got)
	}

	// create a fresh profile from a skill row
	goTo(t, m, "nice")
	press(m, "n")
	if m.editOrig != "" {
		t.Fatalf("n off a profile should start a new one, editing %q", m.editOrig)
	}
	m.edit[fieldName] = "new"
	press(m, "enter") // to description
	press(m, "enter") // to skills
	m.edit[fieldSkills] = "nice base"
	press(m, "enter")
	got := read(t, filepath.Join(repo, "profiles.json"))
	if !strings.Contains(got, `"new"`) {
		t.Fatalf("create should add a profile, got:\n%s", got)
	}

	// delete a profile: d asks, d again deletes, save writes it
	goTo(t, m, "work")
	press(m, "d")
	if !strings.Contains(read(t, filepath.Join(repo, "profiles.json")), `"work"`) {
		t.Fatal("one d only asks")
	}
	press(m, "d")
	if !strings.Contains(read(t, filepath.Join(repo, "profiles.json")), `"work"`) {
		t.Fatal("the delete waits for save")
	}
	m.save()
	got = read(t, filepath.Join(repo, "profiles.json"))
	if strings.Contains(got, `"work"`) || strings.Contains(got, `"app"`) {
		t.Fatalf("delete should remove the profile, got:\n%s", got)
	}
}

func TestTUIProfileEditRejectsUnknownSkill(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, "n")
	m.editField, m.edit[fieldSkills] = fieldSkills, "app ghost"
	press(m, "enter")

	// the save should be refused and the bad member this repo lacks named
	if m.pop != profileEdit {
		t.Fatal("an invalid skill must keep the editor open")
	}
	body := strings.Join(m.popBody, "\n")
	if !strings.Contains(body, "ghost") {
		t.Fatalf("editor should name the bad skill, got:\n%s", body)
	}
	if got := read(t, filepath.Join(repo, "profiles.json")); strings.Contains(got, "ghost") {
		t.Fatalf("profiles.json must not be written with the bad skill:\n%s", got)
	}
}

func TestTUIProfileHasItsOwnGroupAndMembers(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"description": "Everyday work", "skills": ["app", "nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")

	if r, _ := m.at(); r.kind != rowProfile {
		t.Fatalf("row = %+v, want a profile", r)
	}
	if view := plain(m.View()); !strings.Contains(view, "PROFILES") || !strings.Contains(view, "Everyday work") {
		t.Fatalf("profile screen is missing its group or description:\n%s", view)
	}
	press(m, "l")
	if r, _ := m.at(); r.kind != rowSkill || r.name != "app" || r.src != "profile:work" {
		t.Fatalf("right landed on %+v, want the profile's first member", r)
	}
	press(m, "h")
	if r, _ := m.at(); r.kind != rowProfile || r.name != "work" {
		t.Fatalf("left landed on %+v, want the profile again", r)
	}

	press(m, " ")
	if !slices.Equal(m.cfg.Profiles, []string{"work"}) || !slices.Contains(m.cfg.Enabled, "nice") {
		t.Fatalf("selection = %+v, want the profile merged", m.cfg)
	}
	m.save()
	if got := currentConfig(t).Profiles; !slices.Equal(got, []string{"work"}) {
		t.Fatalf("saved profiles = %v, want work", got)
	}
}

func TestTUIProfileOnlyChangeIsPending(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app", "nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	all := []string{"app", "base", "core", "nice"}
	m.cfg.Enabled, m.cfg.Direct = slices.Clone(all), slices.Clone(all)
	m.savedOn, m.savedDirect = slices.Clone(all), slices.Clone(all)
	m.build()
	goTo(t, m, "work")

	press(m, " ")
	if !m.dirty || !strings.Contains(m.pending(), "1 profile on") || strings.Contains(m.pending(), "skill") {
		t.Fatalf("pending = %q, dirty = %v, want only a profile change", m.pending(), m.dirty)
	}
	press(m, " ")
	if m.dirty || m.pending() != "" {
		t.Fatalf("pending = %q, dirty = %v, toggling back should leave nothing to save", m.pending(), m.dirty)
	}
}

func TestThemePickKeepsUnsavedProfileOffDisk(t *testing.T) {
	defer useTheme(defaultTheme)
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["app"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, " ")

	m.setTheme(themeNames()[1])

	cfg := currentConfig(t)
	if len(cfg.Profiles) != 0 || len(cfg.Enabled) != 0 || len(cfg.Direct) != 0 {
		t.Fatalf("disk selection = %+v, want the profile toggle still pending", cfg)
	}
	if !slices.Equal(m.cfg.Profiles, []string{"work"}) || !m.dirty {
		t.Fatalf("memory selection = %+v, dirty = %v, want work pending", m.cfg, m.dirty)
	}
}

func TestLegacyEnabledSkillsAreDirectWhenAProfileIsDisabled(t *testing.T) {
	_, repo := profileRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"nice"}})
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a config written before direct skill provenance existed.
	data = bytes.Replace(data, []byte(`"direct": [`), []byte(`"ignored": [`), 1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	skillset(t, "enable", "work")

	skillset(t, "disable", "work")

	if got := enabled(t); !slices.Contains(got, "nice") {
		t.Fatalf("enabled = %v, a legacy direct selection must survive", got)
	}
}

func TestProfileNamesAreChecked(t *testing.T) {
	_, repo := profileRepo(t)
	before := read(t, filepath.Join(repo, "profiles.json"))
	for _, name := range []string{"-h", "--x", "profile:x", "two words", "app"} {
		err := run([]string{"profile", "add", name, "app"}, &bytes.Buffer{}, strings.NewReader(""))
		if err == nil {
			t.Errorf("profile add %q should be refused", name)
		}
	}
	if got := read(t, filepath.Join(repo, "profiles.json")); got != before {
		t.Fatalf("a refused name must write nothing:\n%s", got)
	}
}

func TestProfileSetDescKeepsMembers(t *testing.T) {
	_, repo := profileRepo(t)
	skillset(t, "profile", "set", "work", "--desc", "new", "words")
	got, err := loadProfiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	if w := got["work"]; w.Description != "new words" || !slices.Equal(w.Skills, []string{"app", "nice"}) {
		t.Fatalf("work = %+v, want new description and the same members", w)
	}

	skillset(t, "profile", "set", "work", "--desc=one", "two")
	skillset(t, "profile", "add", "overlap", "--desc=")
	got, _ = loadProfiles(repo)
	if got["work"].Description != "one two" || got["overlap"].Description != "" {
		t.Fatalf("--desc= forms: work=%q overlap=%q", got["work"].Description, got["overlap"].Description)
	}
}

func TestProfileNeedsASkill(t *testing.T) {
	_, repo := profileRepo(t)
	if err := run([]string{"profile", "add", "empty"}, &bytes.Buffer{}, strings.NewReader("")); err == nil {
		t.Fatal("a new profile with no skills should be refused")
	}
	if strings.Contains(read(t, filepath.Join(repo, "profiles.json")), `"empty"`) {
		t.Fatal("the refused profile must not be written")
	}
}

func TestProfileEditOutput(t *testing.T) {
	profileRepo(t)
	if out := skillset(t, "profile", "add", "fresh", "app"); out != "created fresh: app\n" {
		t.Fatalf("add new = %q", out)
	}
	if out := skillset(t, "profile", "add", "fresh", "nice"); out != "fresh: app, nice\n" {
		t.Fatalf("add existing = %q", out)
	}
	if out := skillset(t, "profile", "rm", "fresh"); out != "removed fresh\n" {
		t.Fatalf("rm = %q", out)
	}
}

// A repo from before profiles has no profiles.json: nothing may fail on that,
// and the first profile write creates it.
func TestNoProfilesFileStillWorks(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})
	skillset(t, "enable", "app")
	skillset(t, "disable", "app")
	if out := skillset(t, "profile", "ls"); !strings.HasPrefix(out, "no profiles yet") {
		t.Fatalf("ls with no profiles.json = %q", out)
	}
	if _, err := newModel(currentConfig(t)); err != nil {
		t.Fatalf("the selection screen should open without profiles.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "profiles.json")); !os.IsNotExist(err) {
		t.Fatal("reading must not create profiles.json")
	}
	skillset(t, "profile", "add", "work", "app")
	if !strings.Contains(read(t, filepath.Join(repo, "profiles.json")), `"work"`) {
		t.Fatal("the first profile write should create profiles.json")
	}
}

func TestProfileRmTurnsTheProfileOff(t *testing.T) {
	_, repo := profileRepo(t)
	// app is in work and overlap; nice only in work; core is required by app
	skillset(t, "enable", "work")
	skillset(t, "enable", "overlap")
	skillset(t, "enable", "nice") // direct now as well

	out := skillset(t, "profile", "rm", "overlap")
	cfg := currentConfig(t)
	if slices.Contains(cfg.Profiles, "overlap") {
		t.Fatalf("profiles = %v, want overlap gone from the machine", cfg.Profiles)
	}
	// app stays (work), base stays (core needs it)
	for _, keep := range []string{"app", "core", "base", "nice"} {
		if !slices.Contains(cfg.Enabled, keep) {
			t.Fatalf("enabled = %v, want %s kept", cfg.Enabled, keep)
		}
	}
	if !strings.Contains(out, "removed overlap") {
		t.Fatalf("out = %q", out)
	}

	out = skillset(t, "profile", "rm", "work")
	cfg = currentConfig(t)
	if slices.Contains(cfg.Enabled, "app") || !slices.Contains(cfg.Enabled, "nice") {
		t.Fatalf("enabled = %v, want app off and the direct nice kept", cfg.Enabled)
	}
	if !strings.Contains(out, "skills disabled: app") || !strings.Contains(out, "skillset apply") {
		t.Fatalf("out = %q, want the disabled skills and the apply reminder", out)
	}
	if p, _ := loadProfiles(repo); len(p) != 2 {
		t.Fatalf("profiles.json = %v, want work and overlap deleted", p)
	}
}

func TestTUIProfileDeleteIsPendingAndDiscardable(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, " ")
	m.save()
	goTo(t, m, "work")
	press(m, "d")
	press(m, "j") // moving away cancels the question
	goTo(t, m, "work")
	press(m, "d")
	if m.deleted != nil {
		t.Fatal("a d after moving away must ask again")
	}
	press(m, "d")
	if !strings.Contains(m.pending(), "1 profile deleted") || slices.Contains(m.cfg.Enabled, "nice") {
		t.Fatalf("pending = %q enabled = %v", m.pending(), m.cfg.Enabled)
	}
	if !strings.Contains(read(t, filepath.Join(repo, "profiles.json")), `"work"`) {
		t.Fatal("nothing is written before save, so discarding keeps the profile")
	}
}

func TestEditingAnEnabledProfileResyncs(t *testing.T) {
	_, repo := profileRepo(t)
	skillset(t, "enable", "core-only") // core, base

	out := skillset(t, "profile", "add", "core-only", "app")
	cfg := currentConfig(t)
	if !slices.Contains(cfg.Enabled, "app") {
		t.Fatalf("enabled = %v, want the new member on", cfg.Enabled)
	}
	if !strings.Contains(out, "skills enabled: app") || !strings.Contains(out, "skillset apply") {
		t.Fatalf("out = %q", out)
	}

	// dropping core: app still requires it, so it stays; dropping app turns it off
	skillset(t, "profile", "set", "core-only", "core")
	cfg = currentConfig(t)
	if slices.Contains(cfg.Enabled, "app") || !slices.Contains(cfg.Enabled, "core") {
		t.Fatalf("enabled = %v, want app off and core kept", cfg.Enabled)
	}
	_ = repo
}

func TestEditingADisabledProfileLeavesTheSelection(t *testing.T) {
	profileRepo(t)
	skillset(t, "enable", "nice")
	before := currentConfig(t).Enabled
	out := skillset(t, "profile", "set", "work", "base")
	if got := currentConfig(t).Enabled; !slices.Equal(got, before) {
		t.Fatalf("enabled = %v, want %v untouched", got, before)
	}
	if strings.Contains(out, "apply") {
		t.Fatalf("out = %q, nothing to apply", out)
	}
}

func TestTUIEditOfEnabledProfileIsPending(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, " ")
	m.save()
	goTo(t, m, "work")
	press(m, "n")
	m.editField, m.edit[fieldSkills] = fieldSkills, "base"
	press(m, "enter")
	if !slices.Contains(m.cfg.Enabled, "base") || slices.Contains(m.cfg.Enabled, "nice") {
		t.Fatalf("enabled = %v, want base on and nice off", m.cfg.Enabled)
	}
	if p := m.pending(); !strings.Contains(p, "skill on") || !strings.Contains(p, "skill off") {
		t.Fatalf("pending = %q, want the change waiting for save", p)
	}
}

func TestTUIRenameMovesTheProfile(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"description": "d", "skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, " ")
	m.save()
	goTo(t, m, "work")
	press(m, "n")
	m.edit[fieldName] = "job"
	m.editField = fieldSkills
	press(m, "enter")

	got, _ := loadProfiles(repo)
	if _, old := got["work"]; old || got["job"].Description != "d" {
		t.Fatalf("profiles = %v, want work renamed to job with its description", got)
	}
	if cfg := currentConfig(t); !slices.Equal(cfg.Profiles, []string{"job"}) {
		t.Fatalf("saved profiles = %v, want the rename on disk already", cfg.Profiles)
	}
	if m.pending() != "" {
		t.Fatalf("pending = %q, a rename changes no selection", m.pending())
	}
}

func TestTUICreateOverExistingIsRefused(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "nice")
	for _, name := range []string{"work", "app", "-x", "two words"} {
		press(m, "n")
		m.edit = [3]string{name, "", "base"}
		m.editField = fieldSkills
		press(m, "enter")
		if m.pop != profileEdit {
			t.Fatalf("creating %q should be refused", name)
		}
		press(m, "esc")
	}
	if got := read(t, filepath.Join(repo, "profiles.json")); strings.Contains(got, "base") {
		t.Fatalf("nothing should be written:\n%s", got)
	}
}

func TestTUIEditorFieldsAndKeys(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, "n")
	if !strings.Contains(m.popBody[0], "edit profile work") {
		t.Fatalf("heading = %q", m.popBody[0])
	}
	press(m, "tab") // description
	for _, r := range "clear prose" {
		press(m, string(r))
	}
	press(m, "ctrl+w")
	if m.edit[fieldDesc] != "clear " {
		t.Fatalf("ctrl+w left %q", m.edit[fieldDesc])
	}
	press(m, "ctrl+u")
	for _, r := range "prose" {
		press(m, string(r))
	}
	press(m, "enter")
	press(m, "enter")
	if got, _ := loadProfiles(repo); got["work"].Description != "prose" {
		t.Fatalf("description = %q", got["work"].Description)
	}

	m.setStep(tabTargets)
	press(m, "n")
	if m.pop == profileEdit {
		t.Fatal("n should do nothing on the targets step")
	}
}

func TestTUIProfileCountSkipsMissingMembers(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice", "ghost"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	if _, total := m.profileCounts("work"); total != 1 {
		t.Fatalf("total = %d, want only the member in the repo counted", total)
	}
}

func TestTUIEditorChecklistPicksMembers(t *testing.T) {
	m, _, repo := tuiModel(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"work": {"skills": ["nice"]}}`)
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	goTo(t, m, "work")
	press(m, "n")
	m.editField = fieldSkills
	press(m, " ")
	if !m.picking || !m.pickOn["nice"] {
		t.Fatalf("space on skills should open the checklist with nice checked")
	}
	press(m, "/")
	for _, r := range "bas" {
		press(m, string(r))
	}
	press(m, "enter") // ends the filter
	if shown := m.pickShown(); len(shown) != 1 || shown[0].name != "base" {
		t.Fatalf("filter shows %v, want base", shown)
	}
	press(m, " ") // check base
	press(m, "enter")
	if m.picking || m.edit[fieldSkills] != "base nice" && m.edit[fieldSkills] != "nice base" {
		t.Fatalf("skills field = %q, want base and nice", m.edit[fieldSkills])
	}

	// esc drops the checklist's changes
	press(m, " ")
	press(m, " ") // toggles the first skill
	press(m, "esc")
	if !strings.Contains(m.edit[fieldSkills], "base") || !strings.Contains(m.edit[fieldSkills], "nice") {
		t.Fatalf("esc should keep the field as it was, got %q", m.edit[fieldSkills])
	}
	press(m, "enter") // saves
	if got, _ := loadProfiles(repo); !slices.Equal(got["work"].Skills, []string{"base", "nice"}) {
		t.Fatalf("work = %v", got["work"].Skills)
	}
}

func TestProfileLsAndShow(t *testing.T) {
	profileRepo(t)
	skillset(t, "enable", "work")
	out := skillset(t, "profile", "ls")
	if !strings.Contains(out, "* work  Everyday work") || !strings.Contains(out, "  overlap") {
		t.Fatalf("ls = %q, want work marked on and overlap not", out)
	}
	if !strings.Contains(out, "missing from the repo: ghost") {
		t.Fatalf("ls = %q, want broken-set's missing member named", out)
	}
	out = skillset(t, "profile", "show", "work")
	if !strings.Contains(out, "enabled here") || !strings.Contains(out, boxOn+" app") {
		t.Fatalf("show = %q", out)
	}
	err := run([]string{"profile", "show", "wrk"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "did you mean work") {
		t.Fatalf("err = %v, want a suggestion", err)
	}
}

func TestProfileRmMembers(t *testing.T) {
	_, repo := profileRepo(t)
	skillset(t, "enable", "work")
	out := skillset(t, "profile", "rm", "work", "nice")
	got, _ := loadProfiles(repo)
	if !slices.Equal(got["work"].Skills, []string{"app"}) || slices.Contains(currentConfig(t).Enabled, "nice") {
		t.Fatalf("work = %v, enabled = %v", got["work"].Skills, currentConfig(t).Enabled)
	}
	if !strings.Contains(out, "skills disabled: nice") {
		t.Fatalf("out = %q", out)
	}
	skillset(t, "profile", "rm", "broken-set", "ghost") // a member gone from the repo
	for _, args := range [][]string{{"work", "base"}, {"work", "app"}} {
		if err := run(append([]string{"profile", "rm"}, args...), &bytes.Buffer{}, strings.NewReader("")); err == nil {
			t.Errorf("profile rm %v should be refused", args)
		}
	}
}

func TestUnknownSkillsAreAllNamed(t *testing.T) {
	profileRepo(t)
	err := run([]string{"profile", "add", "x", "ap", "zzzzzz", "nicee"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"ap (did you mean app?)", "zzzzzz", "nicee (did you mean nice?)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	}
}

func TestSkillShadowingAProfileHints(t *testing.T) {
	_, repo := profileRepo(t)
	write(t, filepath.Join(repo, "profiles.json"), `{"app": {"skills": ["nice"]}}`)
	if out := skillset(t, "enable", "app"); !strings.Contains(out, "use profile:app") {
		t.Fatalf("enable = %q, want the profile: hint", out)
	}
	if out := skillset(t, "disable", "app"); !strings.Contains(out, "use profile:app") {
		t.Fatalf("disable = %q, want the profile: hint", out)
	}
}

func TestProfileHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		if out := skillset(t, "profile", arg); !strings.Contains(out, "drop just those skills") {
			t.Fatalf("profile %s = %q", arg, out)
		}
	}
}
