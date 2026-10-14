package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const repoURL = "https://github.com/dhruvsaxena1998/skills"

// initCmd remembers where the skills clone is, cloning one if this machine has none,
// and on its first run adopts whatever is already linked as this machine's selection.
func initCmd(args []string, out io.Writer, in io.Reader) error {
	var repo string
	switch {
	case len(args) > 1:
		return errors.New("usage: skillset init [path]")
	case len(args) == 1:
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		repo = abs
	default:
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if repo = findRepo(cwd); repo == "" {
			if repo, err = offerClone(out, in); err != nil {
				return err
			}
		}
	}
	if err := checkRepo(repo); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if errors.Is(err, errNotSetUp) {
		cfg = &Config{Targets: []string{"claude", "agents"}}
	} else if err != nil {
		return err
	}
	cfg.Repo = repo
	if len(cfg.Seen) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cfg.Enabled = linkedSkills(home, repo)
		cfg.Seen = repoSkills(repo)
		fmt.Fprintf(out, "adopted %d skills already linked on this machine\n", len(cfg.Enabled))
	}
	if err := cfg.save(); err != nil {
		return err
	}
	path, _ := configPath()
	fmt.Fprintf(out, "repo %s saved in %s\nrun `skillset apply` to link\n", repo, path)
	return nil
}

// findRepo returns the nearest folder at or above dir that holds the skills repo.
func findRepo(dir string) string {
	for {
		if checkRepo(dir) == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func checkRepo(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); err != nil {
		return fmt.Errorf("%s is not a skills clone: no sources.json", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills")); err != nil {
		return fmt.Errorf("%s is not a skills clone: no skills folder", dir)
	}
	return nil
}

func offerClone(out io.Writer, in io.Reader) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	def := filepath.Join(home, "skills")
	fmt.Fprintf(out, "No skills clone here. Clone %s to [%s] (enter a path, or n to cancel): ", repoURL, def)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", errNotSetUp
	}
	switch answer := strings.TrimSpace(line); answer {
	case "n", "N", "no":
		return "", errNotSetUp
	case "":
	default:
		if def, err = filepath.Abs(answer); err != nil {
			return "", err
		}
	}
	cmd := exec.Command("git", "clone", repoURL, def)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("clone into %s: %w", def, err)
	}
	return def, nil
}
