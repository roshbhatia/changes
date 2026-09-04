package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutlineSeparatesDashPrefixedFilenameFromOptions(t *testing.T) {
	bin := t.TempDir()
	commandPath := filepath.Join(bin, "ast-grep")
	script := `#!/bin/sh
separator=false
for argument in "$@"; do
  if [ "$argument" = "--" ]; then
    separator=true
    continue
  fi
  if [ "$argument" = "-dash.go" ] && [ "$separator" != true ]; then
    echo "filename was parsed as an option" >&2
    exit 2
  fi
done
printf '[{"path":"-dash.go","items":[{"symbolType":"function","name":"ready","signature":"func ready() {}","range":{"start":{"line":0},"end":{"line":0}}}]}]\n'
`
	if err := os.WriteFile(commandPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(shell))

	symbols, err := outline(t.TempDir(), []string{"-dash.go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := symbols["-dash.go"]; !ok {
		t.Fatalf("symbols = %#v", symbols)
	}
}

func TestInstalledAstGrepAcceptsArgumentSeparator(t *testing.T) {
	path, err := exec.LookPath("ast-grep")
	if err != nil {
		t.Skip("ast-grep is not installed")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "-dash.go"), []byte("package dash\n\nfunc ready() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(path, "outline", "--json=compact", "--", "-dash.go")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("ast-grep outline: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "-dash.go") {
		t.Fatalf("output = %s", output)
	}
}
