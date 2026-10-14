// Command skillset links the skills of this repo into each agent's skills folder,
// following a per-machine selection kept in ~/.config/skillset/config.json.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `skillset links this repo's skills into your agent folders.

  skillset init [path]   remember the skills clone (clones one if there is none)
  skillset apply         link enabled skills, unlink the rest
`

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
	case "apply":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return apply(cfg, out)
	case "", "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}
