package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequiresDifftoolExecutable(t *testing.T) {
	t.Parallel()
	err := ValidateFiles("difftool", Options{Layout: "unified"})
	if err == nil || !strings.Contains(err.Error(), "diff.difftool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiresPatchFilterExecutable(t *testing.T) {
	t.Parallel()
	err := ValidatePatch("filter", Options{Layout: "unified"})
	if err == nil || !strings.Contains(err.Error(), "diff.filter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilesExpandsDifftoolPlaceholders(t *testing.T) {
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
	out, err := Files("difftool", local, remote, Options{
		Color: "never", Layout: "unified",
		Difftool: []string{"/bin/sh", "-c", `diff -u "$1" "$2"`, "difftool", "$LOCAL", "$REMOTE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+new") {
		t.Fatalf("missing file changes:\n%s", out)
	}
}

func TestFilesUsesBuiltinFallbackWithoutExternalTools(t *testing.T) {
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
	out, err := Files("builtin", local, remote, Options{Color: "never", Layout: "unified", Width: 80})
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
	local := filepath.Join(dir, "temporary", "file.txt")
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
	out, err := Files("builtin", local, remote, Options{Color: "always", Label: "src/file.txt", Layout: "unified", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	plain := normalizeColor(out, "never")
	if out == plain || strings.Contains(plain, "temporary") || !strings.Contains(plain, "src/file.txt") {
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

func TestBuiltinEnginePreservesUnifiedPatch(t *testing.T) {
	t.Parallel()
	patch := `--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-old
+new
`
	out, err := Patch("builtin", patch, Options{Color: "never", Layout: "unified", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if out != patch {
		t.Fatalf("builtin unified output changed the patch:\n%s", out)
	}
}

func TestBuiltinUnifiedStripsColorWhenDisabled(t *testing.T) {
	t.Parallel()
	patch := "\x1b[31m-old\x1b[0m\n"
	out, err := Patch("builtin", patch, Options{Color: "never", Layout: "unified", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if out != "-old\n" {
		t.Fatalf("builtin unified output retained color: %q", out)
	}
}

func TestPatchFilterRejectsFilePlaceholders(t *testing.T) {
	t.Parallel()
	_, err := Patch("filter", "patch", Options{
		Color: "never", Filter: []string{"review", "$LOCAL", "$REMOTE"}, Layout: "unified",
	})
	if err == nil || !strings.Contains(err.Error(), "diff.difftool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuiltinEngineHonorsSideBySideLayout(t *testing.T) {
	t.Parallel()
	patch := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`
	out, err := Patch("builtin", patch, Options{Color: "never", Layout: "side-by-side", Width: 120})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, " │ ") {
		t.Fatalf("builtin engine did not render a side-by-side diff:\n%s", out)
	}
}
