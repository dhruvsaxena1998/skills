package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPlaybookIsAgentsMD(t *testing.T) {
	want, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != playbook {
		t.Fatal("cmd/skillset/playbook.md is stale: run `go generate ./cmd/skillset`")
	}
	var out bytes.Buffer
	if err := run([]string{"playbook"}, &out, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if out.String() != playbook {
		t.Fatal("playbook command did not print the playbook")
	}
}
