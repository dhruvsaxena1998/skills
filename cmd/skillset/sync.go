package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// syncCmd snapshots a source's skills from upstream onto its orphan vendor/<source>
// branch, then merges that branch into the current one so a 3-way merge keeps local edits.
func syncCmd(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: skillset sync <source>")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return syncSource(cfg.Repo, args[0], out)
}

// syncSource is the whole sync of one source into the clone at root.
func syncSource(root, name string, out io.Writer) error {
	sources, err := loadSources(root)
	if err != nil {
		return err
	}
	src, ok := sources[name]
	if !ok {
		return fmt.Errorf("unknown source: %s", name)
	}
	if !gitOK(root, "diff", "--quiet", "HEAD") {
		return fmt.Errorf("commit or stash your changes first")
	}

	tmp, err := os.MkdirTemp("", "skillset-sync")
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real // so `git worktree remove` recognises the path it recorded
	}
	up, vendor := filepath.Join(tmp, "up"), filepath.Join(tmp, "vendor")
	defer func() {
		gitOK(root, "worktree", "remove", "--force", vendor)
		os.RemoveAll(tmp)
	}()

	if _, err := git(root, "clone", "--quiet", "--depth", "1", "--branch", src.Ref, src.Repo, up); err != nil {
		return err
	}
	sha, err := git(up, "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}

	branch := "vendor/" + name
	gitOK(root, "worktree", "prune") // a tmp worktree a previous run failed to remove still holds the branch
	add := []string{"worktree", "add", "--quiet", "--orphan", "-b", branch, vendor}
	if gitOK(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) {
		add = []string{"worktree", "add", "--quiet", vendor, branch}
	}
	if _, err := git(root, add...); err != nil {
		return err
	}

	// the vendor branch holds only what sync writes, so rebuild it from scratch
	if err := clearDir(vendor); err != nil {
		return err
	}
	for _, skill := range src.Skills {
		found, err := findSkill(filepath.Join(up, filepath.FromSlash(src.Path)), skill)
		if err != nil {
			return fmt.Errorf("%s in %s: %w", skill, src.Repo, err)
		}
		if err := os.CopyFS(filepath.Join(vendor, "skills", skill), os.DirFS(found)); err != nil {
			return err
		}
	}
	// extra non-skill files (e.g. hooks), copied to the same relative path
	for _, pattern := range src.Files {
		matches, err := filepath.Glob(filepath.Join(up, filepath.FromSlash(pattern)))
		if err != nil {
			return err
		}
		found := false
		for _, f := range matches {
			if info, err := os.Stat(f); err != nil || !info.Mode().IsRegular() {
				continue
			}
			rel, err := filepath.Rel(up, f)
			if err != nil {
				return err
			}
			if err := copyFile(f, filepath.Join(vendor, rel)); err != nil {
				return err
			}
			found = true
		}
		if !found {
			return fmt.Errorf("no files match %q in %s", pattern, src.Repo)
		}
	}
	// nearest LICENSE wins: inside path, its parent, then repo root
	dir := filepath.Join(up, filepath.FromSlash(src.Path))
	for _, lic := range []string{filepath.Join(dir, "LICENSE"), filepath.Join(filepath.Dir(dir), "LICENSE"), filepath.Join(up, "LICENSE")} {
		if copyFile(lic, filepath.Join(vendor, "licenses", name+"-LICENSE")) == nil {
			break
		}
	}

	deps, err := loadDeps(root)
	if err != nil {
		return err
	}
	reportDrift(out, filepath.Join(vendor, "skills"), filepath.Join(root, "skills"), src, deps)

	if _, err := git(vendor, "add", "-A"); err != nil {
		return err
	}
	if gitOK(vendor, "diff", "--cached", "--quiet") {
		fmt.Fprintf(out, "%s already matches %s@%s\n", branch, src.Repo, sha)
	} else {
		if _, err := git(vendor, "commit", "--quiet", "-m", fmt.Sprintf("sync %s @ %s", name, sha), "-m", src.Repo); err != nil {
			return err
		}
		fmt.Fprintf(out, "committed %s: %s @ %s\n", branch, name, sha)
	}

	merged, err := git(root, "merge", "--no-edit", "--allow-unrelated-histories", branch, "-m", fmt.Sprintf("merge %s (%s)", branch, sha))
	if err != nil {
		return fmt.Errorf("%w\nresolve the conflicts above, then: git commit", err)
	}
	fmt.Fprintln(out, merged)
	return nil
}

// git runs a git command in dir and returns its output, with that output in any error.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// gitOK reports whether a git command succeeded, for the ones asked as a question.
func gitOK(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// findSkill looks a skill up by folder name, so upstream can regroup its folders without breaking us.
func findSkill(dir, name string) (string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == name && isSkill(filepath.Dir(path), name) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("expected one folder under %s, found none", dir)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("expected one folder under %s, found: %s", dir, strings.Join(found, ", "))
	}
	return found[0], nil
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

// a link to a sibling skill folder, as skills write it: [tdd](../tdd/SKILL.md)
var siblingLink = regexp.MustCompile(`\.\./([\w.-]+)/`)

// reportDrift warns when a skill links to a sibling skill it doesn't list in deps,
// which is how the dependency map drifts away from the skills.
func reportDrift(out io.Writer, vendorSkills, repoSkills string, src source, deps map[string]dep) {
	for _, name := range src.Skills {
		declared := append(slices.Clone(deps[name].Requires), deps[name].Optional...)
		var undeclared []string
		filepath.WalkDir(filepath.Join(vendorSkills, name), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for _, m := range siblingLink.FindAllStringSubmatch(string(body), -1) {
				linked := m[1]
				if linked == name || slices.Contains(declared, linked) || slices.Contains(undeclared, linked) {
					continue
				}
				if isSkill(vendorSkills, linked) || isSkill(repoSkills, linked) {
					undeclared = append(undeclared, linked)
				}
			}
			return nil
		})
		slices.Sort(undeclared)
		for _, linked := range undeclared {
			fmt.Fprintf(out, "warning: %s links ../%s/ but doesn't list %s in deps\n", name, linked, linked)
		}
	}
}
