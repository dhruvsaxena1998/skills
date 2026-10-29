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
		return fmt.Errorf("usage: skillset profile <ls|show|add|set|rm> ...\n%s", profileUsage)
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(out, profileUsage)
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	profiles, err := loadProfiles(cfg.Repo)
	if err != nil {
		return err
	}

	switch args[0] {
	case "ls":
		return profileList(out, cfg, profiles)
	case "show":
		return profileShow(out, cfg, profiles, args[1:])
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

const profileUsage = `skillset profile edits profiles.json, the repo's named groups of skills.

  skillset profile ls                                   list profiles; * marks one on here
  skillset profile show <name>                          one profile and its skills' state
  skillset profile add <name> <skill>... [--desc <text>]  create a profile, or add skills to it
  skillset profile set <name> [<skill>...] [--desc <text>] replace its skills, or just the description
  skillset profile rm <name>                            turn it and its skills off, delete the profile
  skillset profile rm <name> <skill>...                 drop just those skills from it
`

// profileList prints every profile, * before the ones enabled on this machine,
// and names any member the repo no longer has.
func profileList(out io.Writer, cfg *Config, profiles map[string]profile) error {
	if len(profiles) == 0 {
		fmt.Fprintln(out, "no profiles yet: skillset profile add <name> <skill>...")
		return nil
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	for _, name := range profileNames(profiles) {
		p := profiles[name]
		mark := " "
		if slices.Contains(cfg.Profiles, name) {
			mark = "*"
		}
		line := mark + " " + name
		if p.Description != "" {
			line += "  " + p.Description
		}
		fmt.Fprintln(out, line)
		fmt.Fprintf(out, "    %s\n", strings.Join(p.Skills, ", "))
		if missing := missingMembers(skillsDir, p); len(missing) > 0 {
			fmt.Fprintf(out, "    missing from the repo: %s\n", strings.Join(missing, ", "))
		}
	}
	return nil
}

func missingMembers(skillsDir string, p profile) []string {
	var missing []string
	for _, member := range p.Skills {
		if !isSkill(skillsDir, member) {
			missing = append(missing, member)
		}
	}
	return missing
}

func profileShow(out io.Writer, cfg *Config, profiles map[string]profile, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: skillset profile show <name>")
	}
	name := args[0]
	p, ok := profiles[name]
	if !ok {
		return fmt.Errorf("no profile named %q%s", name, suggest(name, profileNames(profiles)))
	}
	state := "not enabled here"
	if slices.Contains(cfg.Profiles, name) {
		state = "enabled here"
	}
	fmt.Fprintf(out, "%s  (%s)\n", name, state)
	if p.Description != "" {
		fmt.Fprintln(out, p.Description)
	}
	skillsDir := filepath.Join(cfg.Repo, "skills")
	for _, member := range sortedUnique(p.Skills) {
		switch {
		case !isSkill(skillsDir, member):
			fmt.Fprintf(out, "  ! %s  missing from the repo\n", member)
		case slices.Contains(cfg.Enabled, member):
			fmt.Fprintf(out, "  %s %s\n", boxOn, member)
		default:
			fmt.Fprintf(out, "  %s %s\n", boxOff, member)
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
// profile can never be written pointing at a skill that isn't in the repo. Every
// unknown name is reported at once, each with a near match when there is one.
func parseMembers(repo string, names []string) ([]string, error) {
	skillsDir := filepath.Join(repo, "skills")
	var members, unknown []string
	for _, name := range names {
		if !isSkill(skillsDir, name) {
			unknown = append(unknown, name+suggest(name, repoSkills(repo)))
			continue
		}
		if !slices.Contains(members, name) {
			members = append(members, name)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("not skills in %s: %s", skillsDir, strings.Join(unknown, ", "))
	}
	return members, nil
}

// suggest names the closest of options to a mistyped name, or "" when nothing
// is within two edits.
func suggest(name string, options []string) string {
	best, bestD := "", 3
	for _, o := range options {
		if d := editDistance(name, o); d < bestD {
			best, bestD = o, d
		}
	}
	if best == "" {
		return ""
	}
	return " (did you mean " + best + "?)"
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
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
	return commitProfile(out, cfg, profiles, name, old, p, exists)
}

// commitProfile writes an edited profile, bringing this machine's selection along
// when the profile is on here, and reports what changed.
func commitProfile(out io.Writer, cfg *Config, profiles map[string]profile, name string, old, p profile, existed bool) error {
	repo := cfg.Repo
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
	if !existed {
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
	if len(args) == 0 {
		return fmt.Errorf("usage: skillset profile rm <name> [<skill>...]")
	}
	name := args[0]
	p, ok := profiles[name]
	if !ok {
		return fmt.Errorf("no profile named %q%s", name, suggest(name, profileNames(profiles)))
	}
	if len(args) > 1 {
		return dropMembers(out, cfg, profiles, name, p, args[1:])
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

// dropMembers takes just the named skills out of a profile. They needn't exist in
// the repo any more, which is how a stale member gets cleaned up.
func dropMembers(out io.Writer, cfg *Config, profiles map[string]profile, name string, p profile, drop []string) error {
	var notMembers []string
	for _, skill := range drop {
		if !slices.Contains(p.Skills, skill) {
			notMembers = append(notMembers, skill)
		}
	}
	if len(notMembers) > 0 {
		return fmt.Errorf("not in profile %s: %s", name, strings.Join(notMembers, ", "))
	}
	updated := profile{Description: p.Description, Skills: slices.DeleteFunc(slices.Clone(p.Skills), func(s string) bool {
		return slices.Contains(drop, s)
	})}
	if len(updated.Skills) == 0 {
		return fmt.Errorf("that would leave profile %s with no skills; use `skillset profile rm %s` to delete it", name, name)
	}
	return commitProfile(out, cfg, profiles, name, p, updated, true)
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
