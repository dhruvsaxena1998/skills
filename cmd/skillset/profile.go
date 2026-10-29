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
// set replaces the members, ls lists. An edit to a profile enabled here also
// updates this machine's selection, which then needs `skillset apply`.
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
		return profileAdd(out, cfg, profiles, args[1:])
	case "rm":
		return profileRemove(out, cfg, profiles, args[1:])
	case "set":
		return profileSet(out, cfg, profiles, args[1:])
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
// change, refuse a profile left with no skills, then write. A profile enabled on
// this machine brings the selection along.
func editProfile(out io.Writer, cfg *Config, profiles map[string]profile, args []string, verb string,
	change func(p *profile, members []string)) error {
	repo := cfg.Repo
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
	old := profile{Description: p.Description, Skills: slices.Clone(p.Skills)}
	if hasDesc {
		p.Description = desc
	}
	change(&p, members)
	if len(p.Skills) == 0 {
		return fmt.Errorf("profile %s needs at least one skill", name)
	}
	profiles[name] = p
	deps, err := loadDeps(repo)
	if err != nil {
		return err
	}
	added, removed, err := resyncProfile(cfg, name, old, p, profiles, deps, filepath.Join(repo, "skills"))
	if err != nil {
		return err
	}
	if err := saveProfiles(repo, profiles); err != nil {
		return err
	}
	if len(added)+len(removed) > 0 {
		if err := cfg.save(); err != nil {
			return err
		}
	}
	if !exists {
		fmt.Fprintf(out, "created %s: %s\n", name, strings.Join(sortedUnique(p.Skills), ", "))
	} else {
		fmt.Fprintf(out, "%s: %s\n", name, strings.Join(sortedUnique(p.Skills), ", "))
	}
	if len(added) > 0 {
		fmt.Fprintf(out, "skills enabled: %s\n", strings.Join(sortedUnique(added), ", "))
	}
	if len(removed) > 0 {
		fmt.Fprintf(out, "skills disabled: %s\n", strings.Join(sortedUnique(removed), ", "))
	}
	if len(added)+len(removed) > 0 {
		fmt.Fprintln(out, "run `skillset apply` to link")
	}
	return nil
}

func profileAdd(out io.Writer, cfg *Config, profiles map[string]profile, args []string) error {
	return editProfile(out, cfg, profiles, args, "add", func(p *profile, members []string) {
		for _, member := range members {
			if !slices.Contains(p.Skills, member) {
				p.Skills = append(p.Skills, member)
			}
		}
	})
}

// profileSet replaces the members. With none given it leaves them alone, so
// `set <name> --desc <text>` changes only the description.
func profileSet(out io.Writer, cfg *Config, profiles map[string]profile, args []string) error {
	return editProfile(out, cfg, profiles, args, "set", func(p *profile, members []string) {
		if len(members) > 0 {
			p.Skills = members
		}
	})
}

// profileRemove turns the profile off on this machine before deleting it. The
// selection is saved first: if profiles.json then fails to write, the profile is
// still defined, just off, and nothing is orphaned.
func profileRemove(out io.Writer, cfg *Config, profiles map[string]profile, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: skillset profile rm <name>")
	}
	name := args[0]
	if _, ok := profiles[name]; !ok {
		return fmt.Errorf("no profile named %q", name)
	}
	deps, err := loadDeps(cfg.Repo)
	if err != nil {
		return err
	}
	removed := removeProfile(cfg, name, profiles, deps)
	if err := cfg.save(); err != nil {
		return err
	}
	if err := saveProfiles(cfg.Repo, profiles); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %s\n", name)
	if len(removed) > 0 {
		fmt.Fprintf(out, "skills disabled: %s\n", strings.Join(removed, ", "))
		fmt.Fprintln(out, "run `skillset apply` to link")
	}
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
