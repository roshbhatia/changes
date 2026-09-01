package main

import (
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
