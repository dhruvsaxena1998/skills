package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// target is an agent's skills folder and hook config, relative to the home folder.
type target struct {
	name  string
	dir   string
	hooks string
}

var targets = []target{
	// .claude/skills is the only folder Claude Code reads; .agents/skills is read by Codex, pi and OpenCode.
	{"claude", filepath.Join(".claude", "skills"), filepath.Join(".claude", "settings.json")},
	{"agents", filepath.Join(".agents", "skills"), filepath.Join(".codex", "hooks.json")},
}

// legacyDirs held links from scripts/link.sh and are emptied of our links on every apply.
var legacyDirs = []string{filepath.Join(".pi", "agent", "skills")}

// linkDest is what path points at, or "" if path is not a link.
func linkDest(path string) string {
	dest, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimPrefix(dest, `\??\`)) // junctions come back with that prefix
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// createLink links link -> dest and reports which kind of link it made.
func createLink(dest, link string) (string, error) {
	err := os.Symlink(dest, link)
	if err == nil {
		return "symlink", nil
	}
	if runtime.GOOS != "windows" {
		return "", err
	}
	// Windows without Developer Mode can't make symlinks; a directory junction needs no privilege.
	if out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, dest).CombinedOutput(); jerr != nil {
		return "", fmt.Errorf("symlink failed (%v), junction failed: %v: %s", err, jerr, strings.TrimSpace(string(out)))
	}
	return "junction", nil
}

// isSkill reports whether the repo holds a skill of that name.
func isSkill(skillsDir, name string) bool {
	_, err := os.Stat(filepath.Join(skillsDir, name, "SKILL.md"))
	return err == nil
}

// repoSkills lists every skill folder in the repo.
func repoSkills(repo string) []string {
	skillsDir := filepath.Join(repo, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if isSkill(skillsDir, e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

// linkedSkills lists the repo's skills that are already linked into any agent folder,
// which is what the first init on this machine adopts as enabled.
func linkedSkills(home, repo string) []string {
	skillsDir := filepath.Join(repo, "skills")
	var names []string
	dirs := slices.Clone(legacyDirs)
	for _, t := range targets {
		dirs = append(dirs, t.dir)
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(filepath.Join(home, d))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !samePath(linkDest(filepath.Join(home, d, name)), filepath.Join(skillsDir, name)) {
				continue
			}
			if isSkill(skillsDir, name) && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}
