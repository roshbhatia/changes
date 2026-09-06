package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/completion"
	"github.com/roshbhatia/go-utils/diffview"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

func TestCompletionGeneratorSupportsEveryPublishedShell(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"bash", "zsh", "fish", "nu"} {
		out, err := completion.Generate(shell, completionGeneratorMetadata())
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		for _, want := range []string{"changes", "difftool", "provider", "validate"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s completion omitted %q", shell, want)
			}
		}
	}
}

func TestBashCompletionPreservesDynamicPathsAsData(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash is unavailable: %v", err)
	}
	directory := t.TempDir()
	completionPath := filepath.Join(directory, "changes.bash")
	generated, err := completion.Generate("bash", completionGeneratorMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completionPath, []byte(generated), 0o600); err != nil {
		t.Fatal(err)
	}
	sideEffect := filepath.Join(directory, "unexpected-side-effect")
	changes := filepath.Join(directory, "changes")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' 'hello world.go' '$(touch %s)'
`, sideEffect)
	if err := os.WriteFile(changes, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(bash, "--noprofile", "--norc", "-c", `
complete() { :; }
source "$1"
COMP_LINE='changes difftool '
COMP_POINT=${#COMP_LINE}
COMP_WORDS=(changes difftool "")
COMP_CWORD=2
_changes_complete
printf '%s\0' "${COMPREPLY[@]}"
`, "completion-test", completionPath)
	command.Env = append(os.Environ(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run Bash completion: %v\n%s", err, out)
	}
	candidates := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for _, want := range []string{"hello world.go", "$(touch " + sideEffect + ")"} {
		if !slices.Contains(candidates, want) {
			t.Fatalf("Bash completion changed %q: got %#v", want, candidates)
		}
	}
	for _, fragment := range []string{"hello", "world.go"} {
		if slices.Contains(candidates, fragment) {
			t.Fatalf("Bash completion leaked fragment %q: got %#v", fragment, candidates)
		}
	}
	if _, err := os.Stat(sideEffect); !os.IsNotExist(err) {
		t.Fatalf("Bash evaluated a completion candidate: %v", err)
	}
}

func TestCompletionMetadataIncludesContextualArguments(t *testing.T) {
	t.Parallel()
	metadata := commandMetadata()
	wants := []struct {
		path []string
		kind string
	}{
		{kind: "repository"},
		{path: []string{"difftool"}, kind: "paths"},
		{path: []string{"provider", "list"}, kind: "providers"},
		{path: []string{"provider", "validate"}, kind: "providers"},
	}
	for _, want := range wants {
		command := metadata
		if len(want.path) > 0 {
			command = subcommandMetadata(want.path...)
		}
		expected := []string{"changes", "__values", want.kind}
		if len(want.path) > 0 && want.path[0] == "provider" {
			expected = append(expected, completion.ContextPlaceholder)
		}
		if !slices.Equal(command.CompletionCommand, expected) {
			t.Fatalf("%v completion = %#v, want %#v", want.path, command.CompletionCommand, expected)
		}
	}
	for _, shell := range []string{"bash", "zsh", "fish", "nu"} {
		generated, err := completion.Generate(shell, completionGeneratorMetadata())
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"repository", "paths", "providers"} {
			if !strings.Contains(generated, kind) {
				t.Fatalf("%s completion lacks %q", shell, kind)
			}
		}
	}
	for _, test := range []struct {
		path []string
		kind string
	}{
		{path: []string{"note", "add"}, kind: "note-writers"},
		{path: []string{"note", "generate"}, kind: "note-generators"},
		{path: []string{"note", "list"}, kind: "note-readers"},
	} {
		command := subcommandMetadata(test.path...)
		for _, flag := range command.Flags {
			if flag.Name == "provider" {
				want := []string{"changes", "__values", test.kind, completion.ContextPlaceholder}
				if !slices.Equal(flag.CompletionCommand, want) {
					t.Fatalf("%v provider completion = %#v, want %#v", test.path, flag.CompletionCommand, want)
				}
			}
		}
	}
}

func TestSplitCompletionContextPreservesQuotedConfigPath(t *testing.T) {
	context := `changes note add --config "/tmp/config folder/changes.yaml" --provider `
	arguments := splitCompletionContext(context)
	if got := argumentValue(arguments, "config"); got != "/tmp/config folder/changes.yaml" {
		t.Fatalf("config path = %q; arguments = %#v", got, arguments)
	}
}

func TestWatchRejectsNonpositiveInterval(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"--watch", "--interval=0", "--no-notes")
	command.Dir = repository
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--interval must be greater than zero") {
		t.Fatalf("error = %v, output = %s", err, output)
	}
}

func TestNoteRefreshUsesItsOwnInterval(t *testing.T) {
	last := time.Unix(100, 0)
	if noteRefreshDue(false, last, last, 30*time.Second) {
		t.Fatal("disabled notes requested a refresh")
	}
	if noteRefreshDue(true, last, last.Add(29*time.Second), 30*time.Second) {
		t.Fatal("notes refreshed before their interval")
	}
	if !noteRefreshDue(true, last, last.Add(30*time.Second), 30*time.Second) {
		t.Fatal("notes did not refresh at their interval")
	}
}

func TestStaleNoteLabelMarksRefreshPending(t *testing.T) {
	note := provider.Note{
		State: provider.NoteStateOpen, Author: "reviewer", Source: "test",
		Placement: provider.NotePlacement{Path: "main.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact},
	}
	got := noteTreeLabel(note, true)
	if !strings.Contains(got, "refresh-pending") || !strings.Contains(got, "line 1@right") {
		t.Fatalf("stale note label = %q", got)
	}
}

func TestGitCompletionValuesPreservesPathsAndUsesCurrentRepository(t *testing.T) {
	repository := t.TempDir()
	redirected := t.TempDir()
	for _, directory := range []string{repository, redirected} {
		command := exec.Command("git", "init", "--quiet")
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", directory, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "hello world.txt"), []byte("local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(redirected, "from-b.txt"), []byte("redirected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{repository, redirected} {
		command := exec.Command("git", "add", ".")
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git add %s: %v\n%s", directory, err, output)
		}
	}
	t.Setenv("GIT_DIR", filepath.Join(redirected, ".git"))
	t.Setenv("GIT_WORK_TREE", redirected)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(redirected, ".git", "index"))

	values := gitCompletionValues("-C", repository, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if !slices.Equal(values, []string{"hello world.txt"}) {
		t.Fatalf("completion values = %#v", values)
	}
}

func TestCompletionValuesOmitNewlinePathsWithoutLeakingFragments(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writeCompletionValues(&output, []string{"safe path.go", "line\nbreak.go", "another.go"})
	if got, want := output.String(), "another.go\nsafe path.go\n"; got != want {
		t.Fatalf("completion output = %q, want %q", got, want)
	}
}

func TestNormalizeDiffPathsDecodesGitQuotedPath(t *testing.T) {
	t.Parallel()
	files, rawPaths := normalizeDiffPaths([]diffview.File{
		{Path: `"b/space and\nnewline.go"`},
		{Path: "b/literal-prefix.go"},
	})
	if got, want := files[0].Path, `"space and\nnewline.go"`; got != want {
		t.Fatalf("normalized path = %q, want %q", got, want)
	}
	if got, want := rawPaths[files[0].Path], "space and\nnewline.go"; got != want {
		t.Fatalf("raw path = %q, want %q", got, want)
	}
	if got, want := files[1].Path, "b/literal-prefix.go"; got != want {
		t.Fatalf("literal path = %q, want %q", got, want)
	}
}

func TestDisplayDiffPathQuotesTheCompleteRecursivePath(t *testing.T) {
	got := displayDiffPath("repo/space and\nnewline.go")
	if want := `"repo/space and\nnewline.go"`; got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}
	if !sameDiffPath(got, "repo/space and\nnewline.go") {
		t.Fatalf("quoted display path lost its raw identity: %q", got)
	}
}

func TestParseDiffFilesKeepsDeletedAndBodylessFiles(t *testing.T) {
	patch := "diff --git a/deleted.go b/deleted.go\n" +
		"deleted file mode 100644\n--- a/deleted.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n" +
		"diff --git a/mode.go b/mode.go\nold mode 100644\nnew mode 100755\n" +
		"diff --git a/image.bin b/image.bin\nBinary files a/image.bin and b/image.bin differ\n"
	files, rawPaths := parseDiffFiles(patch)
	if len(files) != 3 {
		t.Fatalf("parsed files = %+v", files)
	}
	for index, want := range []string{"deleted.go", "mode.go", "image.bin"} {
		if files[index].Path != want || rawPaths[files[index].Path] != want {
			t.Fatalf("file %d = %+v, paths = %#v", index, files[index], rawPaths)
		}
	}
	if len(files[0].Hunks) != 1 || len(files[1].Hunks) != 0 || len(files[2].Hunks) != 0 {
		t.Fatalf("file bodies = %+v", files)
	}
}

func TestParseDiffFilesSanitizesTerminalControls(t *testing.T) {
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -0,0 +1 @@\n+safe\x1b]52;c;secret\x07text\n"
	files, _ := parseDiffFiles(patch)
	if len(files) != 1 || len(files[0].Hunks) != 1 || len(files[0].Hunks[0].Lines) < 1 {
		t.Fatalf("parsed files = %+v", files)
	}
	text := files[0].Hunks[0].Lines[0].Text
	if strings.ContainsAny(text, "\x1b\x07") || !strings.Contains(text, "safe") || !strings.Contains(text, "text") {
		t.Fatalf("sanitized diff text = %q", text)
	}
}

func TestDisplayedNotePathsAndRenameProjection(t *testing.T) {
	patch := "diff --git a/old.go b/new.go\nsimilarity index 90%\nrename from old.go\nrename to new.go\n--- a/old.go\n+++ b/new.go\n@@ -1 +1 @@\n-old\n+new\n"
	if got := notePathsInPatch(patch); !slices.Equal(got, []string{"old.go", "new.go"}) {
		t.Fatalf("note paths = %#v", got)
	}
	note := provider.Note{
		Anchor:    provider.NoteAnchor{Path: "old.go"},
		Placement: provider.NotePlacement{Path: "old.go", Side: provider.NoteSideLeft, Line: 1, Quality: provider.PlacementExact},
	}
	remapped := remapNotePaths(noteLayer{values: []provider.Note{note}}, diffPathAliases(patch))
	if remapped.values[0].Anchor.Path != "new.go" || remapped.values[0].Placement.Path != "new.go" {
		t.Fatalf("remapped note = %+v", remapped.values[0])
	}
}

func TestRestorePathSeparatorKeepsRefLikePaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		raw    []string
		parsed []string
		want   []string
	}{
		{name: "leading separator", raw: []string{"--", "HEAD"}, parsed: []string{"HEAD"}, want: []string{"--", "HEAD"}},
		{name: "preserved separator", raw: []string{"HEAD~1", "--", "HEAD"}, parsed: []string{"HEAD~1", "--", "HEAD"}, want: []string{"HEAD~1", "--", "HEAD"}},
		{name: "separator path", raw: []string{"--", "--"}, parsed: []string{"--"}, want: []string{"--", "--"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := restorePathSeparator(test.raw, test.parsed); !slices.Equal(got, test.want) {
				t.Fatalf("restored arguments = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestQuotedPathStaysEscapedWhileProviderReceivesRawIdentity(t *testing.T) {
	directory := t.TempDir()
	name := "space and\nnewline.go"
	if err := os.WriteFile(filepath.Join(directory, name), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "."},
		{"commit", "--quiet", "-m", "before"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "."}, {"commit", "--quiet", "-m", "after"}} {
		command := exec.Command("git", arguments...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	capture := filepath.Join(t.TempDir(), "request.json")
	script := filepath.Join(t.TempDir(), "provider")
	program := `#!/bin/sh
cat > "$CHANGES_CAPTURE"
printf '%s\n' '{"version":"changes.provider/v1","symbols":{"space and\nnewline.go":[{"kind":"function","name":"ready","from":1,"to":2}]}}'
`
	if err := os.WriteFile(script, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	configured := provider.Manifest{
		Version: providerlib.Version,
		Name:    "symbols", Description: "symbols", Command: []string{script},
		Actions: map[string]providerlib.Action{
			provider.ActionSymbols: {
				Description: "symbols",
				Env:         map[string]string{"CHANGES_CAPTURE": capture},
			},
		},
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	patch := "diff --git \"a/space and\\nnewline.go\" \"b/space and\\nnewline.go\"\n" +
		"--- \"a/space and\\nnewline.go\"\n" +
		"+++ \"b/space and\\nnewline.go\"\n" +
		"@@ -1 +1 @@\n-old\n+new\n"
	view := renderer{
		specs: []source.Spec{{Dir: directory, From: "HEAD~1", To: "HEAD"}}, syms: true, budget: 5 * time.Second,
		providers: []provider.LoadedManifest{{Manifest: configured}}, width: 100,
	}
	options := view.diffOptions([]string{patch}, true)
	if len(options.Files) != 1 || options.Files[0].Path != `"space and\nnewline.go"` {
		t.Fatalf("display paths = %+v", options.Files)
	}
	if symbols := options.Symbols[`"space and\nnewline.go"`]; len(symbols) != 1 || symbols[0].Name != "ready" {
		t.Fatalf("symbols = %+v", options.Symbols)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	request := provider.Request{}
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(request.Files, []string{"space and\nnewline.go"}) {
		t.Fatalf("provider paths = %#v", request.Files)
	}
	if request.Base != "" || request.Head != "" || request.Validation {
		t.Fatalf("legacy symbol request gained note fields: %+v", request)
	}
}

func TestCompletionsKeepNestedProviderContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		shell string
		want  []string
	}{
		{"bash", []string{
			"':provider') context='provider'",
			"'provider')\n      __changes_completion_filter",
			"printf '%s\\n' 'list' 'validate'",
		}},
		{"zsh", []string{
			"values=( 'completion' 'difftool' 'render' 'generate' 'note' 'provider')",
			"'*:argument:__changes_completion_values_",
			"'changes' '__values' 'repository'",
			"'2:command:(list validate)'",
		}},
		{"fish", []string{
			`= ""' -a provider`,
			`= "provider"' -a list`,
			`= "provider"' -a validate`,
		}},
		{"nu", []string{
			`export extern "changes provider"`,
			`export extern "changes provider list"`,
			`export extern "changes provider validate"`,
		}},
	}
	for _, test := range tests {
		shell := test.shell
		out, err := completion.Generate(shell, completionGeneratorMetadata())
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		for _, want := range test.want {
			if !strings.Contains(out, want) {
				t.Fatalf("%s completion lost context %q:\n%s", shell, want, out)
			}
		}
	}
}

func TestCommandMetadataIncludesEveryDispatchedCommand(t *testing.T) {
	t.Parallel()
	for _, path := range [][]string{
		{"completion"},
		{"difftool"},
		{"generate"},
		{"note"},
		{"note", "add"},
		{"note", "generate"},
		{"note", "list"},
		{"provider"},
		{"provider", "list"},
		{"provider", "validate"},
		{"render"},
	} {
		_ = subcommandMetadata(path...)
	}
}

func TestRuntimeHelpUsesCommandMetadata(t *testing.T) {
	t.Parallel()
	metadata := commandMetadata()
	flags := flag.NewFlagSet("changes", flag.ContinueOnError)
	flags.Bool("staged", false, flagDescription(metadata, "staged"))
	var out bytes.Buffer
	flags.SetOutput(&out)
	printCommandHelp(&out, "changes [flags]", metadata, flags)

	for _, want := range []string{
		metadata.Synopsis,
		metadata.LongDescription,
		flagDescription(metadata, "staged"),
		subcommandMetadata("difftool").Synopsis,
		subcommandMetadata("provider").Synopsis,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("runtime help omitted generated metadata %q:\n%s", want, out.String())
		}
	}
}

func TestVersionFlag(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--version"}, {"--color", "never", "--version"}} {
		command := exec.Command(os.Args[0], append([]string{"-test.run=TestVersionHelperProcess", "--"}, args...)...)
		command.Env = append(os.Environ(), "GO_WANT_VERSION_HELPER=1")
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("changes %v: %v\n%s", args, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "dev" {
			t.Fatalf("changes %v = %q, want dev", args, got)
		}
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

func TestProviderGroupHelpExitsSuccessfully(t *testing.T) {
	t.Parallel()
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--", "provider", "--help")
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("changes provider --help: %v\n%s", err, out)
	}
	for _, want := range []string{"changes provider <command>", "list", "validate"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("provider help omitted %q:\n%s", want, out)
		}
	}
}

func TestNamedProviderValidationIgnoresMalformedSibling(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "symbols")
	providerScript := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"version":"changes.provider/v1","symbols":{"main.ts":[{"kind":"function","name":"ready","from":1,"to":1}]}}'
`
	if err := os.WriteFile(script, []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`version: provider/v1
name: symbols
description: symbols
command: [%q]
actions:
  changes.symbols:
    description: symbols
`, script)
	if err := os.WriteFile(filepath.Join(directory, "symbols.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "broken.yaml"), []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", directory)), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--", "provider", "validate", "--config", config, "symbols")
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("named validation failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "+ symbols") || strings.Contains(string(out), "broken") {
		t.Fatalf("named validation = %s", out)
	}
}

func TestNamedProviderValidationReportsMalformedSameNameOverride(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	userProviders := filepath.Join(configHome, "changes", "providers")
	installedProvider := filepath.Join(dataHome, "changes", "providers", "symbols")
	for _, directory := range []string{userProviders, installedProvider} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(userProviders, "symbols.yaml"), []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "symbols")
	providerScript := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"version":"changes.provider/v1","symbols":{"main.ts":[{"kind":"function","name":"ready","from":1,"to":1}]}}'
`
	if err := os.WriteFile(script, []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`version: provider/v1
name: symbols
description: symbols
command: [%q]
actions:
  changes.symbols:
    description: symbols
`, script)
	if err := os.WriteFile(filepath.Join(installedProvider, "provider.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--", "provider", "validate", "--json", "symbols")
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
		"XDG_DATA_DIRS="+dataHome,
	)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("named validation ignored malformed override:\n%s", out)
	}
	var results []provider.Validation
	if decodeErr := json.Unmarshal(out, &results); decodeErr != nil {
		t.Fatalf("decode validation output: %v\n%s", decodeErr, out)
	}
	if len(results) != 2 {
		t.Fatalf("validation results = %+v", results)
	}
	passed, failed := 0, 0
	for _, result := range results {
		if result.OK() {
			passed++
		} else {
			failed++
		}
	}
	if passed != 1 || failed != 1 {
		t.Fatalf("validation health = %d passed, %d failed: %+v", passed, failed, results)
	}
}

func TestMalformedOptionalProviderDoesNotDisableCoreRender(t *testing.T) {
	directory, err := source.ValidationFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()
	providers := t.TempDir()
	if err := os.WriteFile(filepath.Join(providers, "broken.yaml"), []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"--config", config, "--color", "never", "--no-symbols", "--no-calls",
	)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("core render failed: %v\n%s", err, out)
	}
	for _, want := range []string{"skipped provider broken", "main.ts", "return true"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("core render omitted %q:\n%s", want, out)
		}
	}
}

func TestInaccessibleOptionalProviderPathDoesNotDisableCoreRender(t *testing.T) {
	directory, err := source.ValidationFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()
	providerPath := filepath.Join(t.TempDir(), "providers")
	if err := os.WriteFile(providerPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providerPath)), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"--config", config, "--color", "never", "--no-symbols", "--no-calls",
	)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("core render failed: %v\n%s", err, out)
	}
	for _, want := range []string{"skipped provider providers", "not a directory", "main.ts", "return true"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("core render omitted %q:\n%s", want, out)
		}
	}
}

func TestStagedRejectsTwoRevisionsBeforeGit(t *testing.T) {
	directory := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	path := filepath.Join(directory, "main.go")
	for index, content := range []string{"package main\n", "package main\n\nfunc main() {}\n"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, arguments := range [][]string{{"add", "main.go"}, {"commit", "--quiet", "-m", fmt.Sprintf("fixture %d", index)}} {
			command := exec.Command("git", arguments...)
			command.Dir = directory
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", arguments, err, output)
			}
		}
	}
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--", "--staged", "HEAD~1", "HEAD")
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("two staged revisions succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "--staged accepts at most one revision") {
		t.Fatalf("unexpected error:\n%s", out)
	}
}

func TestExplicitSeparatorTreatsHEADAsAPath(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "repository")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	prepareRepository(t, directory, map[string]string{
		"HEAD":     "before\n",
		"other.md": "unchanged\n",
	})
	if err := os.WriteFile(filepath.Join(directory, "HEAD"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "other.md"), []byte("changed but excluded\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runChangesHelper(t, parent, "--root", "repository", "--color", "never", "--no-symbols", "--no-calls", "--", "HEAD")
	if !strings.Contains(out, "HEAD") || !strings.Contains(out, "+ after") || strings.Contains(out, "other.md") {
		t.Fatalf("path-limited output =\n%s", out)
	}
}

func TestRootPathspecIsRelativeToSelectedRoot(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "repository")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	prepareRepository(t, directory, map[string]string{
		"nested/target.md": "before\n",
		"other.md":         "unchanged\n",
	})
	if err := os.WriteFile(filepath.Join(directory, "nested", "target.md"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "other.md"), []byte("changed but excluded\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runChangesHelper(t, parent, "--root", "repository", "--color", "never", "--no-symbols", "--no-calls", "--", "nested/target.md")
	if !strings.Contains(out, "nested/") || !strings.Contains(out, "target.md") || strings.Contains(out, "other.md") {
		t.Fatalf("selected-root output =\n%s", out)
	}
}

func prepareRepository(t *testing.T, directory string, files map[string]string) {
	t.Helper()
	for path, contents := range files {
		fullPath := filepath.Join(directory, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes test"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "."},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
}

func runChangesHelper(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestMainHelperProcess", "--"}, arguments...)...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("changes %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func TestMainHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MAIN_HELPER") != "1" {
		return
	}
	for index, argument := range os.Args {
		if argument == "--" {
			os.Args = append([]string{"changes"}, os.Args[index+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestDefaultRendererEmbedsUnifiedDiffInTree(t *testing.T) {
	directory, err := source.ValidationFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()
	view := renderer{
		specs:  []source.Spec{{Dir: directory}},
		width:  80,
		engine: "builtin",
		engineOptions: engine.Options{
			Color: "never", Layout: "unified", Width: 80,
		},
	}
	out, err := view.render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 file", "main.ts", "── line 1", "-   return false", "+   return true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("default output omitted %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "diff --git") {
		t.Fatalf("default output escaped the tree view:\n%s", out)
	}
}

func TestSemanticContextNestsSymbolsAndCallsUnderFiles(t *testing.T) {
	files := []diffview.File{{
		Path: "main.go",
		Hunks: []diffview.Hunk{{
			NewAt: 2,
			Lines: []diffview.Line{{Kind: '-', Text: "old()"}, {Kind: '+', Text: "new()"}},
		}},
	}}
	options := diffview.Options{
		Files: files,
		Symbols: map[string][]diffview.Symbol{
			"main.go": {{Kind: "function", Name: "run", From: 1, To: 4}},
		},
		Edges: map[string][]diffview.Edge{
			"main.go": {{Line: 2, Added: true}},
		},
	}
	rows := semanticTreeRows(options, noteLayer{}, 80)
	if !strings.Contains(rows, "├── main.go") && !strings.Contains(rows, "└── main.go") ||
		!strings.Contains(rows, "function run") || !strings.Contains(rows, "+1 call") {
		t.Fatalf("semantic context =\n%s", rows)
	}
	if strings.Contains(rows, "1 file") || strings.Contains(rows, "old()") || strings.Contains(rows, "new()") {
		t.Fatalf("semantic context repeated file or hunk data:\n%s", rows)
	}
}

func TestSemanticContextFindsEveryChangedSymbolInOneHunk(t *testing.T) {
	options := diffview.Options{
		Files: []diffview.File{{
			Path: "main.go",
			Hunks: []diffview.Hunk{{
				NewAt: 1,
				Lines: []diffview.Line{
					{Kind: ' ', Text: "func one() {"},
					{Kind: '+', Text: "first()"},
					{Kind: ' ', Text: "}"},
					{Kind: ' ', Text: "func two() {"},
					{Kind: '+', Text: "second()"},
					{Kind: ' ', Text: "}"},
				},
			}},
		}},
		Symbols: map[string][]diffview.Symbol{
			"main.go": {
				{Kind: "function", Name: "one", From: 1, To: 3},
				{Kind: "function", Name: "two", From: 4, To: 6},
			},
		},
	}
	rows := semanticTreeRows(options, noteLayer{}, 80)
	for _, symbol := range []string{"function one", "function two"} {
		if !strings.Contains(rows, symbol) {
			t.Fatalf("semantic context omitted %q:\n%s", symbol, rows)
		}
	}
}

func TestSemanticContextNestsNoteUnderOwningSymbol(t *testing.T) {
	options := diffview.Options{
		Files: []diffview.File{{
			Path:  "main.go",
			Hunks: []diffview.Hunk{{NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "changed()"}}}},
		}},
		Symbols: map[string][]diffview.Symbol{
			"main.go": {{Kind: "function", Name: "run", From: 1, To: 3}},
		},
		Edges: map[string][]diffview.Edge{},
	}
	note := provider.Note{
		Summary: "Review the call", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact,
		},
	}
	rows := semanticTreeRows(options, noteLayer{values: []provider.Note{note}}, 80)
	fileLine := strings.Index(rows, "main.go")
	symbolLine := strings.Index(rows, "function run")
	noteLine := strings.Index(rows, "● line 1@right")
	if fileLine < 0 || symbolLine < fileLine || noteLine < symbolLine {
		t.Fatalf("semantic note tree =\n%s", rows)
	}
}

func TestSemanticContextKeepsLeftNoteAtFileLevel(t *testing.T) {
	options := diffview.Options{
		Files: []diffview.File{{
			Path:  "main.go",
			Hunks: []diffview.Hunk{{NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "new()"}}}},
		}},
		Symbols: map[string][]diffview.Symbol{
			"main.go": {{Kind: "function", Name: "newOwner", From: 1, To: 3}},
		},
		Edges: map[string][]diffview.Edge{},
	}
	note := provider.Note{
		Summary: "Old-side note", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "main.go", Side: provider.NoteSideLeft, Line: 1, Quality: provider.PlacementExact},
	}
	rows := semanticTreeRows(options, noteLayer{values: []provider.Note{note}}, 80)
	for _, row := range strings.Split(rows, "\n") {
		if strings.Contains(row, "● line 1@left") && strings.HasPrefix(ansi.Strip(row), "        ") {
			t.Fatalf("left note was nested under a current-file symbol:\n%s", rows)
		}
	}
}

func TestEmbeddedNoteSitsBelowItsDiffLine(t *testing.T) {
	options := diffview.Options{
		Width: 80, Unified: true,
		Files: []diffview.File{{
			Path: "main.go", Add: 1, Del: 1,
			Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{
				{Kind: '-', Text: "old()"}, {Kind: '+', Text: "new()"},
			}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		ID: "test:1", Summary: "Keep the new behavior", Rationale: "Callers require it.",
		Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact,
		},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	diffLine := strings.Index(output, "+ new()")
	noteLine := strings.Index(output, "● line 1@right")
	if diffLine < 0 || noteLine < diffLine || !strings.Contains(output[noteLine:], "Keep the new behavior") {
		t.Fatalf("embedded note =\n%s", output)
	}
}

func TestEmbeddedNoteUsesTheSelectedSideInSideBySideView(t *testing.T) {
	options := diffview.Options{
		Width: 100,
		Files: []diffview.File{{
			Path: "main.go", Add: 1, Del: 1,
			Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{
				{Kind: '-', Text: "old()"}, {Kind: '+', Text: "new()"},
			}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		Summary: "Review the new call", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact,
		},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	diffLine := strings.Index(output, "new()")
	noteLine := strings.Index(output, "● line 1@right")
	if diffLine < 0 || noteLine < diffLine {
		t.Fatalf("side-by-side note =\n%s", output)
	}
}

func TestFileNoteSitsUnderItsFileNode(t *testing.T) {
	options := diffview.Options{
		Width: 80, Unified: true,
		Files: []diffview.File{{
			Path: "main.go", Add: 1,
			Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "new()"}}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		Summary: "Review the whole file", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "main.go", Quality: provider.PlacementExact},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	fileLine := strings.Index(output, "main.go")
	noteLine := strings.Index(output, "● file")
	hunkLine := strings.Index(output, "── line 1")
	if fileLine < 0 || noteLine < fileLine || hunkLine < noteLine {
		t.Fatalf("file note =\n%s", output)
	}
}

func TestStatViewKeepsNotesUnderFiles(t *testing.T) {
	options := diffview.Options{
		Width: 80, Stat: true,
		Files:   []diffview.File{{Path: "main.go", Add: 1}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		Summary: "Review this change", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "main.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if file, annotation := strings.Index(output, "main.go"), strings.Index(output, "● line 1@right"); file < 0 || annotation < file {
		t.Fatalf("stat note =\n%s", output)
	}
}

func TestLogicalGroupsOrderAndNestDiffsByChangeFlow(t *testing.T) {
	options := diffview.Options{
		Width: 90, Unified: true,
		Files: []diffview.File{
			{Path: "z_handler.go", Add: 1, Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "handle()"}}}}},
			{Path: "a_store.go", Add: 1, Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "save()"}}}}},
		},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	groups := []provider.ChangeGroup{
		{ID: "flow", Title: "request flow", Order: 1},
		{ID: "handler", ParentID: "flow", Title: "accept request", Order: 2, Anchors: []provider.GroupAnchor{{Path: "z_handler.go", Side: provider.NoteSideRight, Line: 1}}},
		{ID: "storage", Title: "persist result", Order: 3, Anchors: []provider.GroupAnchor{{Path: "a_store.go", Side: provider.NoteSideRight, Line: 1}}},
	}
	note := provider.Note{
		Summary: "Keep validation first", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "z_handler.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact},
	}
	output, err := renderTreeWithGroups(options, groups, noteLayer{values: []provider.Note{note}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	flow := strings.Index(output, "request flow/")
	child := strings.Index(output, "accept request/")
	handler := strings.Index(output, "z_handler.go")
	noteAt := strings.Index(output, "● line 1@right")
	storage := strings.Index(output, "persist result/")
	storeFile := strings.Index(output, "a_store.go")
	if flow < 0 || child < flow || handler < child || noteAt < handler || storage < noteAt || storeFile < storage {
		t.Fatalf("logical group tree =\n%s", output)
	}
}

func TestLogicalGroupPartsKeepOnlyTheirSemanticEdges(t *testing.T) {
	options := diffview.Options{
		Files: []diffview.File{{Path: "main.go", Hunks: []diffview.Hunk{
			{OldAt: 1, NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "first()"}}},
			{OldAt: 20, NewAt: 20, Lines: []diffview.Line{{Kind: '+', Text: "second()"}}},
		}}},
		Symbols: map[string][]diffview.Symbol{"main.go": {{Kind: "function", Name: "run", From: 1, To: 30}}},
		Edges:   map[string][]diffview.Edge{"main.go": {{Line: 1, Added: true}}}, Pins: map[string]bool{},
	}
	groups := []provider.ChangeGroup{
		{ID: "first", Title: "first", Anchors: []provider.GroupAnchor{{Path: "main.go", Side: provider.NoteSideRight, Line: 1}}},
		{ID: "second", Title: "second", Anchors: []provider.GroupAnchor{{Path: "main.go", Side: provider.NoteSideRight, Line: 20}}},
	}
	parts := partitionChangeGroups(options, groups, nil)
	if len(parts) != 2 || len(parts[0].options.Edges["main.go"]) != 1 || len(parts[1].options.Edges["main.go"]) != 0 {
		t.Fatalf("grouped semantic edges = %+v", parts)
	}
}

func TestSelectGroupProviderUsesPriorityOrderOrExplicitName(t *testing.T) {
	providers := []provider.LoadedManifest{
		{Manifest: provider.Manifest{Name: "preferred", Actions: map[string]providerlib.Action{provider.ActionGroups: {}}}},
		{Manifest: provider.Manifest{Name: "selected", Actions: map[string]providerlib.Action{provider.ActionGroups: {}}}},
	}
	got, found, err := selectGroupProvider(providers, "")
	if err != nil || !found || got.Manifest.Name != "preferred" {
		t.Fatalf("default provider = %+v, found=%v, err=%v", got, found, err)
	}
	got, found, err = selectGroupProvider(providers, "selected")
	if err != nil || !found || got.Manifest.Name != "selected" {
		t.Fatalf("explicit provider = %+v, found=%v, err=%v", got, found, err)
	}
	if _, _, err := selectGroupProvider(providers, "missing"); err == nil {
		t.Fatal("missing explicit group provider was accepted")
	}
}

func TestValidateGroupSelectionRejectsUnsupportedOrMissingProvider(t *testing.T) {
	providers := []provider.LoadedManifest{{Manifest: provider.Manifest{
		Name: "groups", Actions: map[string]providerlib.Action{provider.ActionGroups: {}},
	}}}
	if err := validateGroupSelection("filter", true, "groups", providers); err == nil || !strings.Contains(err.Error(), "builtin") {
		t.Fatalf("filter selection error = %v", err)
	}
	if err := validateGroupSelection("builtin", true, "missing", providers); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing selection error = %v", err)
	}
	if err := validateGroupSelection("filter", false, "groups", providers); err != nil {
		t.Fatalf("disabled grouping = %v", err)
	}
}

func TestAnalysisBudgetIsSharedAcrossProviderActions(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	manifest := provider.Manifest{
		Version: providerlib.Version, Name: "slow", Description: "slow provider",
		Command: []string{shell, "-c", "sleep 5"},
		Actions: map[string]providerlib.Action{
			provider.ActionSymbols: {Description: "symbols"},
			provider.ActionCalls:   {Description: "calls"},
			provider.ActionGroups:  {Description: "groups"},
		},
	}
	renderer := renderer{
		providers: []provider.LoadedManifest{{Manifest: manifest}}, syms: true, calls: true, groups: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	renderer.layers(ctx, source.Spec{Dir: t.TempDir()}, []string{"main.go"}, "diff --git a/main.go b/main.go\n")
	if elapsed := time.Since(started); elapsed > 400*time.Millisecond {
		t.Fatalf("shared analysis budget took %s", elapsed)
	}
}

func TestEmbeddedNoteUsesStructuralLineAnchor(t *testing.T) {
	options := diffview.Options{
		Width: 80, Unified: true,
		Files: []diffview.File{{
			Path: "main.go", Add: 1,
			Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{
				{Kind: ' ', Text: "text := `   2 + misleading`"},
				{Kind: '+', Text: "target()"},
			}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		Summary: "Review the target", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "main.go", Side: provider.NoteSideRight, Line: 2, Quality: provider.PlacementExact},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if target, annotation := strings.Index(output, "+ target()"), strings.Index(output, "● line 2@right"); target < 0 || annotation < target {
		t.Fatalf("structurally anchored note =\n%s", output)
	}
}

func TestEmbeddedLeftNoteUsesOldLineCoordinates(t *testing.T) {
	options := diffview.Options{
		Width: 50,
		Files: []diffview.File{{
			Path: "main.go", Add: 1,
			Hunks: []diffview.Hunk{{OldAt: 10, NewAt: 10, Lines: []diffview.Line{
				{Kind: '+', Text: "inserted()"},
				{Kind: ' ', Text: "kept()"},
			}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	note := provider.Note{
		Summary: "Old-side context", Author: "reviewer", Source: "test", State: provider.NoteStateOpen,
		Placement: provider.NotePlacement{Path: "main.go", Side: provider.NoteSideLeft, Line: 10, Quality: provider.PlacementExact},
	}
	output, err := renderTreeWithNotes(options, noteLayer{values: []provider.Note{note}}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if kept, annotation := strings.Index(output, "kept()"), strings.Index(output, "● line 10@left"); kept < 0 || annotation < kept {
		t.Fatalf("old-side note =\n%s", output)
	}
}

func TestTreeOutputHonorsWidth(t *testing.T) {
	options := diffview.Options{
		Width: 40, Unified: true,
		Files: []diffview.File{{
			Path: "main.go", Add: 1,
			Hunks: []diffview.Hunk{{OldAt: 1, NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: strings.Repeat("long", 60)}}}},
		}},
		Symbols: map[string][]diffview.Symbol{}, Edges: map[string][]diffview.Edge{}, Pins: map[string]bool{},
	}
	output, err := renderTreeWithNotes(options, noteLayer{}, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range strings.Split(output, "\n") {
		if width := ansi.StringWidth(row); width > 40 {
			t.Fatalf("row width = %d:\n%s", width, output)
		}
	}
}

func TestNoteMarkerIsInvisibleAndDistinct(t *testing.T) {
	one, two := noteMarker(1), noteMarker(2)
	if one == two || ansi.StringWidth(one) != 0 || ansi.StringWidth(two) != 0 {
		t.Fatalf("markers are not distinct zero-width values: %q %q", one, two)
	}
}

func TestFailedNoteRefreshKeepsPreviousLayerStale(t *testing.T) {
	previous := noteLayer{values: []provider.Note{{ID: "github:1", Source: "github"}}}
	got := refreshNoteLayer(previous, noteRead{
		values: []provider.Note{{ID: "local:2", Source: "local"}}, failedSources: map[string]bool{"github": true},
	})
	if len(got.values) != 2 || got.values[0].ID != "github:1" || got.values[1].ID != "local:2" || !got.stale {
		t.Fatalf("refreshed notes = %+v", got)
	}
}

func TestPatchChangeSuppressesOldNotesUntilRefresh(t *testing.T) {
	previous := noteLayer{values: []provider.Note{{ID: "local:1"}}}
	if got := noteLayerForFrame(previous, true, false); len(got.values) != 0 {
		t.Fatalf("changed patch reused old notes: %+v", got)
	}
	if got := noteLayerForFrame(previous, true, true); len(got.values) != 1 {
		t.Fatalf("due refresh suppressed notes: %+v", got)
	}
}

func TestReadPatchRejectsOversizedInput(t *testing.T) {
	if _, err := readPatch(strings.NewReader("12345"), 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized patch error = %v", err)
	}
	if got, err := readPatch(strings.NewReader("1234"), 4); err != nil || string(got) != "1234" {
		t.Fatalf("bounded patch = %q, %v", got, err)
	}
}

func TestInitialPartialNoteRefreshKeepsSuccessfulNotes(t *testing.T) {
	got := refreshNoteLayer(noteLayer{}, noteRead{values: []provider.Note{{ID: "local:2"}}})
	if len(got.values) != 1 || got.values[0].ID != "local:2" || !got.stale {
		t.Fatalf("refreshed notes = %+v", got)
	}
}
