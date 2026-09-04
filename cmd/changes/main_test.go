package main

import (
	"bytes"
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
		specs: []source.Spec{{Dir: directory}}, syms: true, budget: 5 * time.Second,
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
			"values=( 'completion' 'difftool' 'render' 'generate' 'provider')",
			"'*:argument:__changes_completion_values_3'",
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
	for _, want := range []string{"skipped provider broken", "diff --git a/main.ts b/main.ts"} {
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
	for _, want := range []string{"skipped provider providers", "not a directory", "diff --git a/main.ts b/main.ts"} {
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
	if !strings.Contains(out, "diff --git a/HEAD b/HEAD") || strings.Contains(out, "other.md") {
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
	if !strings.Contains(out, "nested/target.md") || strings.Contains(out, "other.md") {
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

func TestDefaultRendererKeepsUnifiedGitPatch(t *testing.T) {
	directory, err := source.ValidationFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()
	view := renderer{
		specs:  []source.Spec{{Dir: directory}},
		width:  80,
		color:  "never",
		engine: "builtin",
		engineOptions: engine.Options{
			Color: "never", Layout: "unified", Width: 80,
		},
	}
	out, err := view.render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"diff --git a/main.ts b/main.ts", "-  return false", "+  return true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("default output omitted %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "1 file") {
		t.Fatalf("default output repeated the file summary:\n%s", out)
	}
}

func TestSemanticContextKeepsSymbolsAndCallsWithoutAFileTree(t *testing.T) {
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
	rows := semanticRows(options)
	if !strings.Contains(rows, "main.go:1 · function run") || !strings.Contains(rows, "+1 call") {
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
	rows := semanticRows(options)
	for _, symbol := range []string{"function one", "function two"} {
		if !strings.Contains(rows, symbol) {
			t.Fatalf("semantic context omitted %q:\n%s", symbol, rows)
		}
	}
}
