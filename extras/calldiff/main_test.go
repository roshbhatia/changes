package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/go-utils/git"
)

func TestCallsPreservesPathsWithSpacesAndCleansGitEnvironment(t *testing.T) {
	bin := t.TempDir()
	calldiff := filepath.Join(bin, "calldiff")
	record := filepath.Join(t.TempDir(), "arguments")
	script := `#!/bin/sh
if [ -n "${GIT_DIR-}" ]; then
  echo "inherited GIT_DIR=$GIT_DIR" >&2
  exit 33
fi
printf '<call>\n' >> "$CALLDIFF_RECORD"
for argument in "$@"; do
  printf '[%s]\n' "$argument" >> "$CALLDIFF_RECORD"
done
file=
for argument in "$@"; do
  file=$argument
done
printf '{"trees":[{"ascii":"+ changed()  %s:12"}]}\n' "$file"
`
	if err := os.WriteFile(calldiff, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(path))
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "poison.git"))
	t.Setenv("CALLDIFF_RECORD", record)

	edges, err := calls(provider.Request{
		Directory: t.TempDir(),
		Files:     []string{"world.go", "src/hello world.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := edges["src/hello world.go"]; len(got) != 1 || got[0].Line != 12 || !got[0].Added {
		t.Fatalf("edges = %+v", edges)
	}
	arguments, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(arguments), "<call>") != 2 {
		t.Fatalf("calldiff invocations =\n%s", arguments)
	}
	for _, file := range []string{"[world.go]", "[src/hello world.go]"} {
		if strings.Count(string(arguments), file) != 1 {
			t.Fatalf("calldiff did not receive one file per invocation:\n%s", arguments)
		}
	}
}

func TestCallsTreatsDashPrefixedFilenameAsData(t *testing.T) {
	bin := t.TempDir()
	calldiff := filepath.Join(bin, "calldiff")
	record := filepath.Join(t.TempDir(), "arguments")
	script := `#!/bin/sh
for argument in "$@"; do
  printf '[%s]\n' "$argument" >> "$CALLDIFF_RECORD"
done
file=
for argument in "$@"; do
  file=$argument
done
case "$file" in
  -*) echo "filename was parsed as an option: $file" >&2; exit 2 ;;
esac
printf '{"trees":[{"ascii":"+ changed()  %s:7"}]}\n' "$file"
`
	if err := os.WriteFile(calldiff, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(shell))
	t.Setenv("CALLDIFF_RECORD", record)

	edges, err := calls(provider.Request{Directory: t.TempDir(), Files: []string{"-dash.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := edges["-dash.go"]; len(got) != 1 || got[0].Line != 7 || !got[0].Added {
		t.Fatalf("edges = %+v", edges)
	}
	arguments, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	want := "[." + string(filepath.Separator) + "-dash.go]"
	if !strings.Contains(string(arguments), want) || strings.Contains(string(arguments), "[-dash.go]") {
		t.Fatalf("arguments =\n%s", arguments)
	}
}

func TestComparisonRefsUseTheIndexForStagedChanges(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.ts")
	initial := "function main(): void {}\n"
	staged := "function main(): void {\n  ready()\n}\n"
	working := "function main(): void {\n  unfinished()\n}\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.ts"},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(staged), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "add", "main.ts"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(working), 0o600); err != nil {
		t.Fatal(err)
	}

	refsBefore, err := git.Output(directory, "for-each-ref", "--format=%(refname):%(objectname)")
	if err != nil {
		t.Fatal(err)
	}
	from, to, err := comparisonRefs(provider.Request{Directory: directory, Staged: true})
	if err != nil {
		t.Fatal(err)
	}
	if from != "HEAD" || to == "" {
		t.Fatalf("comparison = %q %q", from, to)
	}
	content, err := git.Output(directory, "show", to+":main.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "ready()") || strings.Contains(content, "unfinished()") {
		t.Fatalf("index tree content = %q", content)
	}
	assertCommitObjects(t, directory, from, to)
	refsAfter, err := git.Output(directory, "for-each-ref", "--format=%(refname):%(objectname)")
	if err != nil {
		t.Fatal(err)
	}
	if refsAfter != refsBefore {
		t.Fatalf("staged comparison mutated refs:\nbefore: %s\nafter: %s", refsBefore, refsAfter)
	}
}

func TestComparisonRefsUseEmptyTreeForUnbornStagedChanges(t *testing.T) {
	directory := t.TempDir()
	if err := git.Run(directory, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.ts"), []byte("ready()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "add", "main.ts"); err != nil {
		t.Fatal(err)
	}

	from, to, err := comparisonRefs(provider.Request{Directory: directory, Staged: true})
	if err != nil {
		t.Fatal(err)
	}
	if from == "" || from == "HEAD" || to == "" {
		t.Fatalf("comparison = %q %q", from, to)
	}
	paths, err := git.Output(directory, "diff", "--name-only", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(paths) != "main.ts" {
		t.Fatalf("unborn staged paths = %q", paths)
	}
	assertCommitObjects(t, directory, from, to)
	refs, err := git.Output(directory, "for-each-ref", "--format=%(refname)")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(refs) != "" {
		t.Fatalf("unborn staged comparison created refs: %s", refs)
	}
}

func TestComparisonRefsWrapATreeBaseWithoutCreatingARef(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "main.ts"), []byte("before()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.ts"},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := git.Output(directory, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.ts"), []byte("after()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "add", "main.ts"); err != nil {
		t.Fatal(err)
	}
	refsBefore, err := git.Output(directory, "for-each-ref", "--format=%(refname):%(objectname)")
	if err != nil {
		t.Fatal(err)
	}

	from, to, err := comparisonRefs(provider.Request{
		Directory: directory,
		From:      strings.TrimSpace(tree),
		Staged:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCommitObjects(t, directory, from, to)
	refsAfter, err := git.Output(directory, "for-each-ref", "--format=%(refname):%(objectname)")
	if err != nil {
		t.Fatal(err)
	}
	if refsAfter != refsBefore {
		t.Fatalf("tree snapshot mutated refs:\nbefore: %s\nafter: %s", refsBefore, refsAfter)
	}
}

func TestComparisonRefsRejectTwoStagedRevisions(t *testing.T) {
	_, _, err := comparisonRefs(provider.Request{Staged: true, From: "HEAD~1", To: "HEAD"})
	if err == nil || !strings.Contains(err.Error(), "at most one revision") {
		t.Fatalf("error = %v", err)
	}
}

func assertCommitObjects(t *testing.T, directory string, revisions ...string) {
	t.Helper()
	for _, revision := range revisions {
		objectType, err := git.Output(directory, "cat-file", "-t", revision)
		if err != nil {
			t.Fatalf("inspect %q: %v", revision, err)
		}
		if strings.TrimSpace(objectType) != "commit" {
			t.Fatalf("%q is %q, want commit", revision, strings.TrimSpace(objectType))
		}
	}
}
