package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/roshbhatia/go-utils/completion"
)

func TestCompletionGeneratorSupportsEveryPublishedShell(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"bash", "zsh", "fish", "nu"} {
		out, err := completion.Generate(shell, completion.Command{Name: "changes"})
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.Contains(out, "changes") {
			t.Fatalf("%s completion omitted command", shell)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	t.Parallel()
	command := exec.Command(os.Args[0], "-test.run=TestVersionHelperProcess", "--", "--version")
	command.Env = append(os.Environ(), "GO_WANT_VERSION_HELPER=1")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("changes --version: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "dev" {
		t.Fatalf("changes --version = %q, want dev", got)
	}
}

func TestVersionHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_VERSION_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"changes"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}
