package main

import (
	_ "embed"
	"fmt"
	"io"
)

// playbook is the repo's AGENTS.md, copied here because go:embed can't read above the
// package folder. `go generate ./cmd/skillset` refreshes it, and TestPlaybookIsAgentsMD
// fails when the copy is stale.
//
//go:generate cp ../../AGENTS.md playbook.md
//go:embed playbook.md
var playbook string

// playbookCmd prints the playbook, so `skillset playbook > AGENTS.md` sets up a repo
// without copying anything by hand.
func playbookCmd(out io.Writer) error {
	_, err := fmt.Fprint(out, playbook)
	return err
}
