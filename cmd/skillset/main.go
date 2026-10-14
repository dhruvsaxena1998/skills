// Command skillset links the skills of this repo into each agent's skills folder,
// following a per-machine selection kept in ~/.config/skillset/config.json.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// version is the release tag, stamped in by .goreleaser.yaml. Left empty by any other
// build, which then has to describe itself from what the toolchain recorded.
var version string

const usage = `skillset links this repo's skills into your agent folders.

  skillset                 open the selection screen
  skillset init [path]     remember the skills clone (clones one if there is none)
  skillset enable <skill>  turn a skill on here, with what it requires
  skillset disable <skill> turn it off, with the skills that require it
  skillset apply           link enabled skills, unlink the rest
  skillset sync <source>   pull a source's skills from upstream into this repo
  skillset update          replace this binary with the latest release
  skillset version         print the version of this binary (--version, -v)
`

// binaryVersion names the running build. Only the linker's stamp proves a release: Go's
// own build info hands a build of a dirty worktree the nearest tag regardless, and a wrong
// number is worse than a vague one, so anything unstamped stays dev and just says where
// it came from.
func binaryVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return "dev (" + s.Value[:7] + ")"
		}
	}
	// No commit recorded, which is what `go install module@version` leaves behind.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return "dev (" + v + ")"
	}
	return "dev"
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "skillset:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer, in io.Reader) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "init":
		return initCmd(args[1:], out, in)
	case "enable":
		return enableCmd(args[1:], out)
	case "disable":
		return disableCmd(args[1:], out)
	case "sync":
		return syncCmd(args[1:], out)
	case "apply":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		deps, err := loadDeps(cfg.Repo)
		if err != nil {
			return err
		}
		if err := apply(cfg, out, false); err != nil {
			return err
		}
		for _, w := range warnings(cfg, deps) {
			fmt.Fprintln(out, w)
		}
		return nil
	case "update":
		return updateCmd(args[1:], out)
	case "version", "--version", "-v":
		fmt.Fprintln(out, "skillset", binaryVersion())
		return nil
	case "":
		return tuiCmd()
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}
