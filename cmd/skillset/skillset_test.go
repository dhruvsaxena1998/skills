package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// setup gives the test its own home folder and a skills clone holding the named skills.
func setup(t *testing.T, skills ...string) (home, repo string) {
	t.Helper()
	home, repo = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	write(t, filepath.Join(repo, "sources.json"), "{}")
	for _, s := range skills {
		write(t, filepath.Join(repo, "skills", s, "SKILL.md"), "# "+s)
	}
	return home, repo
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func symlink(t *testing.T, dest, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := createLink(dest, link); err != nil {
		t.Fatal(err)
	}
}

func skillset(t *testing.T, args ...string) string {
	t.Helper()
	out := &bytes.Buffer{}
	if err := run(args, out, strings.NewReader("")); err != nil {
		t.Fatalf("skillset %v: %v", args, err)
	}
	return out.String()
}

func save(t *testing.T, cfg *Config) {
	t.Helper()
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
}

// entries lists what a folder holds: a link by its destination, anything else by kind.
func entries(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	list, err := os.ReadDir(dir)
	if err != nil {
		return got
	}
	for _, e := range list {
		path := filepath.Join(dir, e.Name())
		if dest := linkDest(path); dest != "" {
			got[e.Name()] = dest
			continue
		}
		got[e.Name()] = "plain"
	}
	return got
}

func TestCommandBeforeInitExplainsInit(t *testing.T) {
	setup(t)
	err := run([]string{"apply"}, &bytes.Buffer{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "skillset init") {
		t.Fatalf("want an error pointing at init, got %v", err)
	}
}

func TestInitInsideCloneSavesItsPath(t *testing.T) {
	_, repo := setup(t, "a")
	t.Chdir(filepath.Join(repo, "skills", "a"))

	skillset(t, "init")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.Stat(repo)
	got, err := os.Stat(cfg.Repo)
	if err != nil || !os.SameFile(want, got) {
		t.Fatalf("repo = %q, want %q", cfg.Repo, repo)
	}
	if !slices.Equal(cfg.Targets, []string{"agents", "claude"}) {
		t.Fatalf("targets = %v, want both on", cfg.Targets)
	}
}

func TestInitOutsideCloneRefusesWithoutAnAnswer(t *testing.T) {
	home, _ := setup(t)
	t.Chdir(home)
	if err := run([]string{"init"}, &bytes.Buffer{}, strings.NewReader("n\n")); err == nil {
		t.Fatal("want an error when the clone offer is declined")
	}
}

func TestExpandHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	abs := filepath.Join(t.TempDir(), "x")
	cases := map[string]string{
		"~":       home,
		"~/a/b":   filepath.Join(home, "a", "b"),
		"~user/x": "~user/x",
		"a/~/b":   "a/~/b",
		"a/b":     "a/b",
		abs:       abs,
	}
	if runtime.GOOS == "windows" {
		cases[`~\a`] = filepath.Join(home, "a")
	}
	for in, want := range cases {
		if got := expandHome(in, home); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitAdoptsSkillsAlreadyLinked(t *testing.T) {
	home, repo := setup(t, "a", "b")
	symlink(t, filepath.Join(repo, "skills", "a"), filepath.Join(home, ".claude", "skills", "a"))
	symlink(t, filepath.Join(repo, "skills", "a"), filepath.Join(home, ".pi", "agent", "skills", "a"))

	skillset(t, "init", repo)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Enabled, []string{"a"}) {
		t.Fatalf("enabled = %v, want the linked skill only", cfg.Enabled)
	}
	if !slices.Equal(cfg.Seen, []string{"a", "b"}) {
		t.Fatalf("seen = %v, want every skill in the repo", cfg.Seen)
	}
}

func TestApplyLinksEnabledAndRemovesOurStaleLinks(t *testing.T) {
	home, repo := setup(t, "a", "b")
	claude := filepath.Join(home, ".claude", "skills")
	symlink(t, filepath.Join(repo, "skills", "b"), filepath.Join(claude, "b"))       // disabled now
	symlink(t, filepath.Join(repo, "skills", "gone"), filepath.Join(claude, "gone")) // deleted from the repo
	symlink(t, filepath.Join(repo, "skills", "a"), filepath.Join(home, ".pi", "agent", "skills", "a"))
	elsewhere := t.TempDir()
	symlink(t, elsewhere, filepath.Join(claude, "foreign"))
	write(t, filepath.Join(claude, "synced", "SKILL.md"), "# synced")
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"a"}, Seen: []string{"a", "b"}})

	skillset(t, "apply")

	want := map[string]string{
		"a":       filepath.Join(repo, "skills", "a"),
		"foreign": elsewhere,
		"synced":  "plain",
	}
	if got := entries(t, claude); !mapsEqual(got, want) {
		t.Fatalf("claude folder = %v, want %v", got, want)
	}
	if got := entries(t, filepath.Join(home, ".agents", "skills")); len(got) != 1 || got["a"] == "" {
		t.Fatalf("agents folder = %v, want only a", got)
	}
	if got := entries(t, filepath.Join(home, ".pi", "agent", "skills")); len(got) != 0 {
		t.Fatalf("pi folder = %v, want the old links gone", got)
	}
}

func TestApplyTwiceChangesNothing(t *testing.T) {
	home, repo := setup(t, "a", "b")
	save(t, &Config{Repo: repo, Targets: []string{"claude", "agents"}, Enabled: []string{"a", "b"}})
	skillset(t, "apply")
	before := entries(t, filepath.Join(home, ".claude", "skills"))

	if out := skillset(t, "apply"); out != "" {
		t.Fatalf("second apply said %q, want nothing to do", out)
	}
	if got := entries(t, filepath.Join(home, ".claude", "skills")); !mapsEqual(got, before) {
		t.Fatalf("claude folder = %v, want %v", got, before)
	}
}

func TestApplySkipsDisabledTargetAndMissingSkill(t *testing.T) {
	home, repo := setup(t, "a")
	symlink(t, filepath.Join(repo, "skills", "a"), filepath.Join(home, ".agents", "skills", "a"))
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"a", "gone"}})

	out := skillset(t, "apply")

	if !strings.Contains(out, "skip gone: not in the repo") {
		t.Fatalf("output = %q, want a word about the missing skill", out)
	}
	if got := entries(t, filepath.Join(home, ".agents", "skills")); len(got) != 0 {
		t.Fatalf("agents folder = %v, want it emptied of our links", got)
	}
	if got := entries(t, filepath.Join(home, ".claude", "skills")); len(got) != 1 || got["a"] == "" {
		t.Fatalf("claude folder = %v, want only a", got)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !samePath(b[k], v) {
			return false
		}
	}
	return true
}

// depsRepo is a clone whose skills depend on each other: app -> core -> base,
// with `nice` optional for app and `broken` needing a skill that isn't here.
func depsRepo(t *testing.T) (home, repo string) {
	t.Helper()
	home, repo = setup(t, "app", "core", "base", "nice", "broken")
	write(t, filepath.Join(repo, "sources.json"), `{"s": {"deps": {
		"app":    {"requires": ["core"], "optional": ["nice"]},
		"core":   {"requires": ["base"]},
		"base":   {},
		"nice":   {},
		"broken": {"requires": ["ghost"]}
	}}}`)
	return home, repo
}

func enabled(t *testing.T) []string {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Enabled
}

func TestEnablePullsInWhatTheSkillRequires(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	out := skillset(t, "enable", "app")

	if got := enabled(t); !slices.Equal(got, []string{"app", "base", "core"}) {
		t.Fatalf("enabled = %v, want app with its chain", got)
	}
	if !strings.Contains(out, "also enabled (required): base, core") {
		t.Fatalf("output = %q, want the chain listed", out)
	}
	if !strings.Contains(out, "warning: app is enabled without nice (optional)") {
		t.Fatalf("output = %q, want the optional warning", out)
	}
}

func TestDisableCascadesToEverySkillThatRequiresIt(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"app", "base", "core", "nice"}})

	out := skillset(t, "disable", "base")

	if got := enabled(t); !slices.Equal(got, []string{"nice"}) {
		t.Fatalf("enabled = %v, want only the unrelated skill", got)
	}
	if !strings.Contains(out, "also disabled (they require it): app, core") {
		t.Fatalf("output = %q, want the cascade listed", out)
	}
}

func TestDisablingAnOptionalDependencyOnlyWarns(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"app", "base", "core", "nice"}})

	out := skillset(t, "disable", "nice")

	if got := enabled(t); !slices.Equal(got, []string{"app", "base", "core"}) {
		t.Fatalf("enabled = %v, want the dependent left alone", got)
	}
	if !strings.Contains(out, "warning: app is enabled without nice (optional)") {
		t.Fatalf("output = %q, want a warning", out)
	}
}

func TestCannotEnableASkillWhoseRequirementIsMissing(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}})

	err := run([]string{"enable", "broken"}, &bytes.Buffer{}, strings.NewReader(""))

	if err == nil || !strings.Contains(err.Error(), "requires ghost, which isn't in the repo") {
		t.Fatalf("err = %v, want the missing skill named", err)
	}
	if got := enabled(t); len(got) != 0 {
		t.Fatalf("enabled = %v, want nothing changed", got)
	}
}

func TestApplyPrintsCurrentWarnings(t *testing.T) {
	_, repo := depsRepo(t)
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"app", "base", "core"}})

	if out := skillset(t, "apply"); !strings.Contains(out, "warning: app is enabled without nice (optional)") {
		t.Fatalf("output = %q, want the warning", out)
	}
}

// Windows CI runs as administrator, so createLink gets a real symlink and the junction
// branch never fires. Make a junction by hand and check we still recognise it as ours.
func TestJunctionIsRecognisedAsOurLink(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows-only")
	}
	home, repo := setup(t, "a")
	dir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(repo, "skills", "a")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dir, "a"), dest).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
	if got := linkDest(filepath.Join(dir, "a")); !samePath(got, dest) {
		t.Fatalf("linkDest = %q, want %q", got, dest)
	}

	save(t, &Config{Repo: repo, Targets: []string{"claude"}}) // a is not enabled
	skillset(t, "apply")

	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("folder = %v, want the junction pruned like any link of ours", got)
	}
}

func TestApplyRelinksAfterTheCloneMoves(t *testing.T) {
	home, repo := setup(t, "a")
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"a"}, Seen: []string{"a"}})
	skillset(t, "apply")
	moved := filepath.Join(filepath.Dir(repo), "moved-clone")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}

	skillset(t, "init", moved)
	skillset(t, "apply")

	link := filepath.Join(home, ".claude", "skills", "a")
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatalf("the link should point at the moved clone: %v", err)
	}
}

func TestApplyOwnsItsLinksWhateverTheRepoPathLooksLike(t *testing.T) {
	home, repo := setup(t, "a")
	other := strings.ToUpper(filepath.Base(repo))
	if _, err := os.Stat(filepath.Join(filepath.Dir(repo), other)); err != nil {
		t.Skip("case-sensitive filesystem: one folder really is two paths here")
	}
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"a"}, Seen: []string{"a"}})
	skillset(t, "apply")

	// the same clone, spelled differently, as Windows and macOS both allow
	save(t, &Config{Repo: filepath.Join(filepath.Dir(repo), other), Targets: []string{"claude"}, Seen: []string{"a"}})
	skillset(t, "apply")

	if got := entries(t, filepath.Join(home, ".claude", "skills")); len(got) != 0 {
		t.Fatalf("folder = %v, want our link gone even under the other spelling", got)
	}
}

func TestDisableRefusesANameItDoesNotKnow(t *testing.T) {
	_, repo := setup(t, "a")
	save(t, &Config{Repo: repo, Targets: []string{"claude"}, Enabled: []string{"a"}})

	err := run([]string{"disable", "ponytial"}, &bytes.Buffer{}, strings.NewReader(""))

	if err == nil || !strings.Contains(err.Error(), "no skill named") {
		t.Fatalf("err = %v, want a typo to be refused, not reported as done", err)
	}
}
