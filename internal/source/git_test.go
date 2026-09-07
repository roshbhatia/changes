package source

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roshbhatia/go-utils/git"
)

func TestHistoryAndCommitComparisonUseFirstParent(t *testing.T) {
	directory := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, "file.txt")
	for index, body := range []string{"first\n", "second\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := git.Run(directory, "add", "file.txt"); err != nil {
			t.Fatal(err)
		}
		if err := git.Run(directory, "commit", "--quiet", "-m", []string{"root", "second"}[index]); err != nil {
			t.Fatal(err)
		}
	}
	history, err := History(directory, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Summary != "second" || history[1].Summary != "root" {
		t.Fatalf("history = %#v", history)
	}
	if _, err := time.Parse(time.RFC3339, history[0].AuthoredAt); err != nil {
		t.Fatalf("authored time = %q: %v", history[0].AuthoredAt, err)
	}
	second, commit, err := CommitComparison(directory, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if second.From != history[0].Parent || second.To != history[0].OID || commit != history[0] {
		t.Fatalf("comparison = %#v, commit = %#v, history = %#v", second, commit, history[0])
	}
	root, rootCommit, err := CommitComparison(directory, history[1].OID)
	if err != nil {
		t.Fatal(err)
	}
	if root.From == "" || root.To != history[1].OID || rootCommit.Parent != root.From {
		t.Fatalf("root comparison = %#v, commit = %#v", root, rootCommit)
	}
	patch, err := root.Diff()
	if err != nil || !strings.Contains(patch, "+first") {
		t.Fatalf("root patch = %q, error = %v", patch, err)
	}
}

func TestRepositoryIdentitySupportsDetachedHead(t *testing.T) {
	directory := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "file"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "add", "file"); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "commit", "--quiet", "-m", "root"); err != nil {
		t.Fatal(err)
	}
	branch, head, err := RepositoryIdentity(directory)
	if err != nil || branch == "" || head == "" {
		t.Fatalf("identity = %q %q, %v", branch, head, err)
	}
	if err := git.Run(directory, "checkout", "--quiet", "--detach", "HEAD"); err != nil {
		t.Fatal(err)
	}
	branch, detached, err := RepositoryIdentity(directory)
	if err != nil || branch != "" || detached != head {
		t.Fatalf("detached identity = %q %q, %v", branch, detached, err)
	}
}

func TestStagedDiffRejectsTwoRevisionsBeforeGit(t *testing.T) {
	_, err := (Spec{Staged: true, From: "HEAD~1", To: "HEAD"}).Diff()
	if err == nil || !strings.Contains(err.Error(), "at most one revision") {
		t.Fatalf("error = %v", err)
	}
}

func TestNoLazyFetchEnvironmentIsScopedBySpec(t *testing.T) {
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	t.Setenv("GIT_NO_REPLACE_OBJECTS", "0")
	t.Setenv("GIT_REPLACE_REF_BASE", "refs/replace/custom/")
	values := func(spec Spec) map[string]string {
		found := map[string]string{}
		for _, entry := range spec.command("version").Env {
			name, value, ok := strings.Cut(entry, "=")
			if ok && strings.HasPrefix(name, "GIT_") {
				found[name] = value
			}
		}
		return found
	}
	if got := values(Spec{}); got["GIT_NO_LAZY_FETCH"] != "0" || got["GIT_NO_REPLACE_OBJECTS"] != "0" ||
		got["GIT_REPLACE_REF_BASE"] != "refs/replace/custom/" {
		t.Fatalf("default environment = %#v", got)
	}
	if got := values(Spec{NoLazyFetch: true}); got["GIT_NO_LAZY_FETCH"] != "1" || got["GIT_NO_REPLACE_OBJECTS"] != "1" {
		t.Fatalf("note environment = %#v", got)
	} else if _, ok := got["GIT_REPLACE_REF_BASE"]; ok {
		t.Fatalf("note environment kept replacement ref base: %#v", got)
	}
}

func TestNoLazyFetchDisablesRepositoryFSMonitorHook(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "file.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
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
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(directory, "fsmonitor")
	marker := filepath.Join(directory, "fsmonitor-ran")
	script := "#!/bin/sh\n: > \"$(dirname \"$0\")/fsmonitor-ran\"\nprintf '%s\\0' \"$2\"\n"
	if err := os.WriteFile(hook, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "config", "core.fsmonitor", hook); err != nil {
		t.Fatal(err)
	}

	if _, err := (Spec{Dir: directory}).Diff(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("ordinary diff did not run the configured fsmonitor hook: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := (Spec{Dir: directory}).NoteDiff(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("note diff ran the configured fsmonitor hook: %v", err)
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

func TestDiffDisablesTextConversion(t *testing.T) {
	t.Parallel()
	arguments, err := (Spec{}).args("never")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(arguments, "--no-textconv") {
		t.Fatalf("diff arguments omitted --no-textconv: %#v", arguments)
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

func TestNoteDiffIgnoresLocalDiffPresentationConfiguration(t *testing.T) {
	root := t.TempDir()
	sourceDirectory := filepath.Join(root, "source")
	if err := os.Mkdir(sourceDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sourceDirectory, "main.txt")
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve\n"
	after := strings.Replace(before, "six\n", "changed six\n", 1)
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.txt"},
		{"commit", "--quiet", "-m", "base"},
	} {
		if err := git.Run(sourceDirectory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "main.txt"}, {"commit", "--quiet", "-m", "head"}} {
		if err := git.Run(sourceDirectory, arguments...); err != nil {
			t.Fatal(err)
		}
	}

	one := filepath.Join(root, "one")
	two := filepath.Join(root, "two")
	for _, clone := range []string{one, two} {
		if err := git.Run(root, "clone", "--quiet", sourceDirectory, clone); err != nil {
			t.Fatal(err)
		}
	}
	oneAttributes := filepath.Join(root, "one.attributes")
	twoAttributes := filepath.Join(root, "two.attributes")
	if err := os.WriteFile(oneAttributes, []byte("*.txt binary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(twoAttributes, []byte("*.txt diff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := configureDiffFixture(one, map[string]string{
		"core.attributesFile": oneAttributes, "core.quotePath": "false", "diff.algorithm": "minimal", "diff.context": "1",
		"diff.indentHeuristic": "true", "diff.interHunkContext": "9",
		"diff.mnemonicPrefix": "true", "diff.suppressBlankEmpty": "true",
	}); err != nil {
		t.Fatal(err)
	}
	if err := configureDiffFixture(two, map[string]string{
		"core.attributesFile": twoAttributes, "core.quotePath": "true", "diff.algorithm": "histogram", "diff.context": "8",
		"diff.indentHeuristic": "false", "diff.interHunkContext": "0",
		"diff.mnemonicPrefix": "false", "diff.suppressBlankEmpty": "false",
	}); err != nil {
		t.Fatal(err)
	}

	oneSpec := Spec{Dir: one, From: "HEAD^", To: "HEAD"}
	twoSpec := Spec{Dir: two, From: "HEAD^", To: "HEAD"}
	oneDisplayed, err := oneSpec.Diff()
	if err != nil {
		t.Fatal(err)
	}
	twoDisplayed, err := twoSpec.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if oneDisplayed == twoDisplayed {
		t.Fatal("fixture diff configuration did not change the displayed patch")
	}
	oneNotes, err := oneSpec.NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	twoNotes, err := twoSpec.NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	if oneNotes == "" || strings.Contains(oneNotes, "diff --git") || strings.Contains(oneNotes, "@@") {
		t.Fatalf("note identity is not a non-empty raw diff: %q", oneNotes)
	}
	if oneNotes != twoNotes {
		t.Fatalf("note identities differ across clones:\none: %q\ntwo: %q", oneNotes, twoNotes)
	}
	oneFiles, err := oneSpec.NoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	twoFiles, err := twoSpec.NoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(oneFiles, twoFiles) {
		t.Fatalf("note files differ across clones: %#v and %#v", oneFiles, twoFiles)
	}
}

func TestNoteDiffChangesWithTheComparedTree(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.txt")
	prepare := func(value, message string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, arguments := range [][]string{{"add", "main.txt"}, {"commit", "--quiet", "-m", message}} {
			if err := git.Run(directory, arguments...); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := git.Run(directory, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if err := configureDiffFixture(directory, map[string]string{"user.name": "Changes test", "user.email": "changes@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	prepare("base\n", "base")
	base, err := git.Output(directory, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	prepare("first\n", "first")
	firstHead, err := git.Output(directory, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	first, err := (Spec{Dir: directory, From: base, To: firstHead}).NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	if err := git.Run(directory, "checkout", "--quiet", "--detach", base); err != nil {
		t.Fatal(err)
	}
	prepare("second\n", "second")
	secondHead, err := git.Output(directory, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Spec{Dir: directory, From: base, To: secondHead}).NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second == "" || first == second {
		t.Fatalf("raw identities for different trees = %q and %q", first, second)
	}
}

func TestNoteDiffIgnoresReplacementRefs(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.txt")
	if err := os.WriteFile(path, []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.txt"},
		{"commit", "--quiet", "-m", "base"},
	} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("head\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "main.txt"}, {"commit", "--quiet", "-m", "head"}} {
		if err := git.Run(directory, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	base, err := git.Output(directory, "rev-parse", "HEAD^")
	if err != nil {
		t.Fatal(err)
	}
	head, err := git.Output(directory, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Dir: directory, From: base, To: head}
	want, err := spec.NoteDiff()
	if err != nil || want == "" {
		t.Fatalf("initial note identity = %q, %v", want, err)
	}
	if err := git.Run(directory, "replace", base, head); err != nil {
		t.Fatal(err)
	}
	got, err := spec.NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("replacement ref changed note identity from %q to %q", want, got)
	}
	files, err := spec.NoteFiles()
	if err != nil || !slices.Equal(files, []string{"main.txt"}) {
		t.Fatalf("replacement ref changed note files: %#v, %v", files, err)
	}
}

func configureDiffFixture(directory string, values map[string]string) error {
	for name, value := range values {
		if err := git.Run(directory, "config", name, value); err != nil {
			return err
		}
	}
	return nil
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
