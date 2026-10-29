package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// profileCmd edits profiles.json: add creates and merges members, rm deletes,
// set replaces the members, ls lists. These change the repo, not the machine
// selection, so they never need `skillset apply`.
func profileCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: skillset profile <ls|add|rm|set> ...\n%s", profileUsage)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	repo := cfg.Repo
	profiles, err := loadProfiles(repo)
	if err != nil {
		return err
	}

	switch args[0] {
	case "ls":
		return profileList(out, profiles)
	case "add":
		return profileAdd(out, repo, profiles, args[1:])
	case "rm":
		return profileRemove(out, repo, profiles, args[1:])
	case "set":
		return profileSet(out, repo, profiles, args[1:])
	default:
		return fmt.Errorf("unknown profile action %q\n%s", args[0], profileUsage)
	}
}

const profileUsage = `  skillset profile ls
  skillset profile add <name> <skill> [skill...] [--desc <text>]
  skillset profile rm <name>
  skillset profile set <name> <skill> [skill...] [--desc <text>]`

func profileList(out io.Writer, profiles map[string]profile) error {
	for _, name := range profileNames(profiles) {
		p := profiles[name]
		members := "none"
		if len(p.Skills) > 0 {
			members = strings.Join(p.Skills, ", ")
		}
		if p.Description != "" {
			fmt.Fprintf(out, "%s  %s\n  %s\n", name, p.Description, members)
		} else {
			fmt.Fprintf(out, "%s\n  %s\n", name, members)
		}
	}
	return nil
}

// profileArgs splits a subcommand's tail into positional words and any --desc
// flag. --desc takes the rest of the line as its value, so a description with
// spaces needs no quoting; --desc=<text> works too. hasDesc tells an empty value,
// which clears the description, from no flag at all.
func profileArgs(args []string) (words []string, desc string, hasDesc bool, err error) {
	words = args
	for i, arg := range args {
		if arg == "--desc" {
			words, desc, hasDesc = args[:i], strings.Join(args[i+1:], " "), true
			break
		}
		if v, ok := strings.CutPrefix(arg, "--desc="); ok {
			words, desc, hasDesc = args[:i], strings.Join(append([]string{v}, args[i+1:]...), " "), true
			break
		}
	}
	if len(words) == 0 {
		return nil, "", false, fmt.Errorf("missing <name>")
	}
	return words, desc, hasDesc, nil
}

// checkProfileName refuses a new profile name that `skillset enable <name>`
// couldn't reach: one a skill shadows, one that reads as a flag, or one the
// profile: prefix or a shell would split.
func checkProfileName(skillsDir, name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a profile needs a name")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("profile name %q can't start with -", name)
	case strings.HasPrefix(name, profilePrefix):
		return fmt.Errorf("profile name %q can't start with %s", name, profilePrefix)
	case strings.ContainsFunc(name, unicode.IsSpace):
		return fmt.Errorf("profile name %q can't contain spaces", name)
	case isSkill(skillsDir, name):
		return fmt.Errorf("%q is already a skill; pick another profile name", name)
	}
	return nil
}

// parseMembers checks every skill exists and returns them deduplicated, so a
// profile can never be written pointing at a skill that isn't in the repo.
func parseMembers(repo string, names []string) ([]string, error) {
	skillsDir := filepath.Join(repo, "skills")
	var members []string
	for _, name := range names {
		if !isSkill(skillsDir, name) {
			return nil, fmt.Errorf("%s isn't a skill in %s", name, skillsDir)
		}
		if !slices.Contains(members, name) {
			members = append(members, name)
		}
	}
	return members, nil
}

// editProfile is what add and set share: parse, check a new name, apply the
// change, refuse a profile left with no skills, then write.
func editProfile(out io.Writer, repo string, profiles map[string]profile, args []string, verb string,
	change func(p *profile, members []string)) error {
	words, desc, hasDesc, err := profileArgs(args)
	if err != nil {
		return fmt.Errorf("profile %s: %w", verb, err)
	}
	name := words[0]
	p, exists := profiles[name]
	if !exists {
		if err := checkProfileName(filepath.Join(repo, "skills"), name); err != nil {
			return err
		}
	}
	members, err := parseMembers(repo, words[1:])
	if err != nil {
		return err
	}
	if hasDesc {
		p.Description = desc
	}
	change(&p, members)
	if len(p.Skills) == 0 {
		return fmt.Errorf("profile %s needs at least one skill", name)
	}
	profiles[name] = p
	if err := saveProfiles(repo, profiles); err != nil {
		return err
	}
	if !exists {
		fmt.Fprintf(out, "created %s: %s\n", name, strings.Join(sortedUnique(p.Skills), ", "))
	} else {
		fmt.Fprintf(out, "%s: %s\n", name, strings.Join(sortedUnique(p.Skills), ", "))
	}
	return nil
}

func profileAdd(out io.Writer, repo string, profiles map[string]profile, args []string) error {
	return editProfile(out, repo, profiles, args, "add", func(p *profile, members []string) {
		for _, member := range members {
			if !slices.Contains(p.Skills, member) {
				p.Skills = append(p.Skills, member)
			}
		}
	})
}

// profileSet replaces the members. With none given it leaves them alone, so
// `set <name> --desc <text>` changes only the description.
func profileSet(out io.Writer, repo string, profiles map[string]profile, args []string) error {
	return editProfile(out, repo, profiles, args, "set", func(p *profile, members []string) {
		if len(members) > 0 {
			p.Skills = members
		}
	})
}

func profileRemove(out io.Writer, repo string, profiles map[string]profile, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: skillset profile rm <name>")
	}
	name := args[0]
	if _, ok := profiles[name]; !ok {
		return fmt.Errorf("no profile named %q", name)
	}
	delete(profiles, name)
	if err := saveProfiles(repo, profiles); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %s\n", name)
	return nil
}

// sortedUnique sorts and deduplicates a slice, returning a copy.
func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}
