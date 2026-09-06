package source

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roshbhatia/go-utils/git"
)

func TestStagedDiffRejectsTwoRevisionsBeforeGit(t *testing.T) {
	_, err := (Spec{Staged: true, From: "HEAD~1", To: "HEAD"}).Diff()
	if err == nil || !strings.Contains(err.Error(), "at most one revision") {
		t.Fatalf("error = %v", err)
	}
}

func TestFilesPreservesDeletedPathAndComparisonIDs(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "deleted.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "deleted.txt"},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	spec := Spec{Dir: directory}
	files, err := spec.Files()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"deleted.txt"}) {
		t.Fatalf("files = %#v", files)
	}
	base, head := spec.ComparisonIDs()
	if base == "" || head != "" {
		t.Fatalf("working comparison = %q %q", base, head)
	}

	commit := Spec{Dir: directory, From: "HEAD", To: "HEAD"}
	_, head = commit.ComparisonIDs()
	if head == "" {
		t.Fatal("commit comparison omitted head object id")
	}
	trees := Spec{Dir: directory, From: "HEAD^{tree}", To: "HEAD^{tree}"}
	base, head = trees.ComparisonIDs()
	if base == "" || head == "" {
		t.Fatalf("tree comparison omitted object ids: %q %q", base, head)
	}
}

func TestWorkingComparisonUsesIndexTree(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "file.txt")
	if err := os.WriteFile(path, []byte("committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "file.txt"},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "add", "file.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := git.Output(directory, "count-objects", "-v")
	if err != nil {
		t.Fatal(err)
	}
	base, head := (Spec{Dir: directory}).ComparisonIDs()
	commit, err := git.Output(directory, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(base, "index:") || base == strings.TrimSpace(commit) || head != "" {
		t.Fatalf("working comparison = %q %q, commit = %q", base, head, commit)
	}
	if repeated, _ := (Spec{Dir: directory}).ComparisonIDs(); repeated != base {
		t.Fatalf("repeated working base = %q, want %q", repeated, base)
	}
	after, err := git.Output(directory, "count-objects", "-v")
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("comparison identity changed object storage:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestFilesPreservesLeadingWhitespaceAndNewlines(t *testing.T) {
	directory := t.TempDir()
	name := " leading\nname.txt"
	if err := git.Run(directory, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", name},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte("changed again\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := (Spec{Dir: directory}).Files()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{name}) {
		t.Fatalf("files = %#v, want %#v", files, []string{name})
	}
}

func TestNoteFilesIncludesBothRenamePaths(t *testing.T) {
	directory := t.TempDir()
	oldPath := filepath.Join(directory, "before.go")
	if err := os.WriteFile(oldPath, []byte("package before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "before.go"},
		{"commit", "--quiet", "-m", "fixture"},
		{"mv", "before.go", "after.go"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	files, err := (Spec{Dir: directory, Staged: true}).NoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"before.go", "after.go"}) {
		t.Fatalf("note files = %#v", files)
	}
}

func TestDiffArgumentsSeparateRefLikePaths(t *testing.T) {
	t.Parallel()
	arguments, err := (Spec{From: "HEAD~1", Paths: []string{"HEAD", "main.go"}}).args("never")
	if err != nil {
		t.Fatal(err)
	}
	wantTail := []string{"HEAD~1", "--", "HEAD", "main.go"}
	if len(arguments) < len(wantTail) || !slices.Equal(arguments[len(arguments)-len(wantTail):], wantTail) {
		t.Fatalf("arguments = %#v, want tail %#v", arguments, wantTail)
	}
}

func TestDiffUsesStablePrefixesWithMnemonicPrefixConfigured(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.go")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"config", "diff.mnemonicPrefix", "true"},
		{"add", "main.go"},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch, err := (Spec{Dir: directory}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "diff --git a/main.go b/main.go") ||
		!strings.Contains(patch, "--- a/main.go") || !strings.Contains(patch, "+++ b/main.go") {
		t.Fatalf("diff used unstable prefixes:\n%s", patch)
	}
}

func TestLimitedBufferBoundsGitPatchOutput(t *testing.T) {
	buffer := limitedBuffer{limit: 4}
	if written, err := buffer.Write([]byte("123456")); err != nil || written != 6 {
		t.Fatalf("write = %d, %v", written, err)
	}
	if !buffer.exceeded || buffer.String() != "1234" {
		t.Fatalf("limited buffer = %q, exceeded=%v", buffer.String(), buffer.exceeded)
	}
}
