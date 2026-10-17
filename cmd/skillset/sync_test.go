package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// syncRepos builds a throwaway upstream and a throwaway local clone, so sync never
// runs against the real repo. The local clone's sources.json points at the upstream.
func syncRepos(t *testing.T, alphaBody string) (local, upstream string) {
	t.Helper()
	home, local := setup(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "no-such-gitconfig")) // keep the machine's git config out
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	upstream = t.TempDir()
	write(t, filepath.Join(upstream, "skills", "alpha", "SKILL.md"), alphaBody)
	write(t, filepath.Join(upstream, "skills", "alpha", "notes.txt"), "extra file")
	write(t, filepath.Join(upstream, "skills", "group", "beta", "SKILL.md"), "# beta") // regrouped upstream
	write(t, filepath.Join(upstream, "hooks", "demo-one.js"), "one")
	write(t, filepath.Join(upstream, "hooks", "ignore.txt"), "not matched")
	write(t, filepath.Join(upstream, "skills", "LICENSE"), "nearest")
	write(t, filepath.Join(upstream, "LICENSE"), "furthest")
	gitInit(t, upstream)

	write(t, filepath.Join(local, "sources.json"), `{"demo": {
		"repo": "`+filepath.ToSlash(upstream)+`",
		"ref": "main",
		"path": "skills",
		"skills": ["alpha", "beta"],
		"files": ["hooks/demo-*.js"],
		"deps": {"alpha": {"optional": ["beta"]}, "beta": {}}
	}}`)
	gitInit(t, local)
	save(t, &Config{Repo: local, Targets: []string{"claude"}})
	return local, upstream
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "--quiet", "-b", "main"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"},
		{"add", "-A"},
		{"commit", "--quiet", "-m", "start"},
	} {
		if _, err := git(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
}

// the structs sync reads with have to match the sources.json this repo actually ships
func TestThisRepoSourcesAreLoadable(t *testing.T) {
	sources, err := loadSources(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range sources {
		if src.Repo == "" || src.Ref == "" || src.Path == "" || len(src.Skills) == 0 {
			t.Fatalf("source %s loaded as %+v", name, src)
		}
	}
}

func TestSyncMergesSkillsFilesAndLicense(t *testing.T) {
	local, _ := syncRepos(t, "# alpha")

	out := skillset(t, "sync", "demo")

	if !strings.Contains(out, "committed vendor/demo: demo @ ") {
		t.Fatalf("output = %q, want the vendor commit reported", out)
	}
	for path, want := range map[string]string{
		filepath.Join("skills", "alpha", "SKILL.md"):  "# alpha",
		filepath.Join("skills", "alpha", "notes.txt"): "extra file",
		filepath.Join("skills", "beta", "SKILL.md"):   "# beta",
		filepath.Join("hooks", "demo-one.js"):         "one",
		filepath.Join("licenses", "demo-LICENSE"):     "nearest", // the one inside path, not the repo root
	} {
		if got := read(t, filepath.Join(local, path)); got != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(local, "hooks", "ignore.txt")); err == nil {
		t.Fatal("hooks/ignore.txt was copied, want only the files globs")
	}
}

func TestSyncAgainWithNoUpstreamChangeCommitsNothing(t *testing.T) {
	local, _ := syncRepos(t, "# alpha")
	skillset(t, "sync", "demo")
	before, err := git(local, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	out := skillset(t, "sync", "demo")

	if !strings.Contains(out, "already matches") {
		t.Fatalf("output = %q, want it to say the vendor branch is unchanged", out)
	}
	if after, err := git(local, "rev-parse", "HEAD"); err != nil || after != before {
		t.Fatalf("HEAD = %q (%v), want it left at %q", after, err, before)
	}
}

func TestSyncRefusesUncommittedChanges(t *testing.T) {
	local, _ := syncRepos(t, "# alpha")
	write(t, filepath.Join(local, "sources.json"), `{"demo": {"repo": "nope"}}`)

	err := run([]string{"sync", "demo"}, &bytes.Buffer{}, strings.NewReader(""))

	if err == nil || !strings.Contains(err.Error(), "commit or stash") {
		t.Fatalf("err = %v, want it to refuse a dirty clone", err)
	}
}

func TestSyncWarnsAboutALinkMissingFromDeps(t *testing.T) {
	// beta is optional for alpha; gamma is a skill of another source, in no deps entry
	local, _ := syncRepos(t, "# alpha\nsee [beta](../beta/SKILL.md) and [gamma](../gamma/README.md)\n")
	write(t, filepath.Join(local, "skills", "gamma", "SKILL.md"), "# gamma")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "--quiet", "-m", "gamma"}} {
		if _, err := git(local, args...); err != nil {
			t.Fatal(err)
		}
	}

	out := skillset(t, "sync", "demo")

	if !strings.Contains(out, "warning: alpha links ../gamma/ but doesn't list gamma in deps") {
		t.Fatalf("output = %q, want the undeclared link warned about", out)
	}
	if strings.Contains(out, "../beta/") {
		t.Fatalf("output = %q, want no warning for the declared link", out)
	}
}

// syncModel is a screen over the sync harness's clone, before anything is synced.
func syncModel(t *testing.T) (*model, string) {
	t.Helper()
	local, _ := syncRepos(t, "# alpha")
	m, err := newModel(&Config{Repo: local, Targets: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	return m, local
}

// finish runs the command a key returned and hands its answer back to the screen.
func finish(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("the key started nothing")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				if r := c(); r != nil {
					if _, tick := r.(tickMsg); !tick {
						msg = r
					}
				}
			}
		}
	}
	m.Update(msg)
}

func TestTUIUSyncsTheSourceUnderTheCursor(t *testing.T) {
	m, local := syncModel(t)
	goTo(t, m, "demo")

	finish(t, m, press(m, "u"))

	if !strings.Contains(m.status, "committed vendor/demo") {
		t.Fatalf("status = %q, want the vendor commit reported", m.status)
	}
	if got := read(t, filepath.Join(local, "skills", "alpha", "SKILL.md")); got != "# alpha" {
		t.Fatalf("alpha = %q, want it synced", got)
	}
	goTo(t, m, "alpha") // the screen re-read the repo, so the new skill has a row
}

func TestTUIShiftUSyncsEverySource(t *testing.T) {
	m, local := syncModel(t)
	sources := read(t, filepath.Join(local, "sources.json"))
	write(t, filepath.Join(local, "sources.json"), strings.Replace(sources, `{"demo": {`,
		`{"other": {"repo": "`+m.sources["demo"].Repo+`", "ref": "main", "path": "skills", "skills": ["beta"]}, "demo": {`, 1))
	for _, args := range [][]string{{"add", "-A"}, {"commit", "--quiet", "-m", "other"}} {
		if _, err := git(local, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}

	finish(t, m, press(m, "U"))

	for _, want := range []string{"committed vendor/demo", "committed vendor/other"} {
		if !strings.Contains(m.status, want) {
			t.Fatalf("status = %q, want %q", m.status, want)
		}
	}
}

func TestTUIGitKeysWaitForTheRunningJob(t *testing.T) {
	m, _ := syncModel(t)
	goTo(t, m, "demo")
	for _, key := range []string{"p", "u", "U"} {
		m.loading["origin"] = true
		m.status = "checking origin"
		if cmd := press(m, key); cmd != nil || m.status != "checking origin" {
			t.Fatalf("%s mid-job: cmd = %v, status = %q, want nil and the running job kept", key, cmd != nil, m.status)
		}
		m.Update(gitMsg("done"))
		goTo(t, m, "demo")
		if press(m, key) == nil {
			t.Fatalf("%s after the job: got no command", key)
		}
	}
}

func TestTUISyncRefusesADirtyCloneOnTheStatusLine(t *testing.T) {
	m, local := syncModel(t)
	write(t, filepath.Join(local, "sources.json"), read(t, filepath.Join(local, "sources.json"))+"\n")
	goTo(t, m, "demo")

	finish(t, m, press(m, "u"))

	if !strings.Contains(m.status, "commit or stash") {
		t.Fatalf("status = %q, want the refusal shown", m.status)
	}
	if _, err := os.Stat(filepath.Join(local, "skills", "alpha")); err == nil {
		t.Fatal("alpha arrived, want nothing synced")
	}
}
