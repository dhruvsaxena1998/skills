package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
