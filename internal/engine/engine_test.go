package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequiresCommandExecutable(t *testing.T) {
	t.Parallel()
	err := Validate("command", Options{Layout: "unified"})
	if err == nil || !strings.Contains(err.Error(), "diff.command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilesExpandsGitDifftoolPlaceholders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local := filepath.Join(dir, "local.txt")
	remote := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(local, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remote, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Files("command", local, remote, Options{
		Color: "never", Layout: "unified", Command: []string{"git", "diff", "--no-index", "--", "$LOCAL", "$REMOTE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+new") {
		t.Fatalf("missing file changes:\n%s", out)
	}
}

func TestFilesUsesGitWithoutExternalTools(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local := filepath.Join(dir, "local.txt")
	remote := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(local, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remote, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Files("git", local, remote, Options{Color: "never", Layout: "unified", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+new") {
		t.Fatalf("missing file changes:\n%s", out)
	}
}

func TestFilesUsesMergedLabel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local := filepath.Join(dir, "git-blob", "file.txt")
	remote := filepath.Join(dir, "file.txt")
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remote, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Files("git", local, remote, Options{Color: "never", Label: "src/file.txt", Layout: "unified", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "git-blob") || !strings.Contains(out, "src/file.txt") {
		t.Fatalf("label was not applied:\n%s", out)
	}
}

func TestNormalizeColorStripsANSIWhenDisabled(t *testing.T) {
	t.Parallel()
	const colored = "\x1b[31mchanged\x1b[0m"
	if got := normalizeColor(colored, "never"); got != "changed" {
		t.Fatalf("unexpected output: %q", got)
	}
	if got := normalizeColor(colored, "always"); got != colored {
		t.Fatalf("color was not preserved: %q", got)
	}
}

func TestInternalEngineHonorsUnifiedLayout(t *testing.T) {
	t.Parallel()
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	out, err := Patch("internal", patch, Options{Color: "never", Layout: "unified", Width: 120})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, " │ ") || !strings.Contains(out, "- old") || !strings.Contains(out, "+ new") {
		t.Fatalf("internal engine did not render a unified diff:\n%s", out)
	}
}
