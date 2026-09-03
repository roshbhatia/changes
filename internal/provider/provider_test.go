package provider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	providerlib "github.com/roshbhatia/go-utils/provider"
)

func manifest(name, action string, command []string) Manifest {
	return Manifest{
		Version: providerlib.Version, Name: name, Description: name + " provider", Command: command,
		Actions: map[string]providerlib.Action{action: {Description: action}},
	}
}

var discoveryManifestTemplate = template.Must(template.New("provider manifest").Option("missingkey=error").Parse(`version: provider/v1
name: symbols
description: {{ .Description }}
command: [symbols]
actions:
  changes.symbols:
    description: symbols
`))

func providerYAML(t *testing.T, description string) string {
	t.Helper()
	var output strings.Builder
	if err := discoveryManifestTemplate.Execute(&output, struct{ Description string }{description}); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestSupportsAdvertisedAction(t *testing.T) {
	t.Parallel()
	configured := manifest("symbols", ActionSymbols, []string{"one"})
	if !Supports(configured, ActionSymbols) || Supports(configured, ActionCalls) {
		t.Fatalf("unexpected actions: %v", configured.Actions)
	}
}

func TestResultPathIncludesManifestActionAndRequest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	configured := manifest("symbols", ActionSymbols, []string{os.Args[0]})
	one, err := resultPath(configured, ActionSymbols, []byte("first"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	two, err := resultPath(configured, ActionSymbols, []byte("second"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	three, err := resultPath(configured, ActionCalls, []byte("first"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if one == two || one == three {
		t.Fatalf("cache identities collided: %q %q %q", one, two, three)
	}
}

func TestResultPathIncludesResolvedExecutableIdentity(t *testing.T) {
	cache := t.TempDir()
	oneBin := t.TempDir()
	twoBin := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	script := []byte(`#!/bin/sh
exit 0
`)
	for _, path := range []string{filepath.Join(oneBin, "provider"), filepath.Join(twoBin, "provider")} {
		if err := os.WriteFile(path, script, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configured := manifest("symbols", ActionSymbols, []string{"provider"})
	t.Setenv("PATH", oneBin)
	one, err := resultPath(configured, ActionSymbols, []byte("request"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", twoBin)
	two, err := resultPath(configured, ActionSymbols, []byte("request"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatalf("cache identity omitted executable path: %q", one)
	}
}

func TestResultPathIncludesRequiredRuntimeIdentity(t *testing.T) {
	oneBin := t.TempDir()
	twoBin := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	script := []byte(`#!/bin/sh
exit 0
`)
	for _, path := range []string{filepath.Join(oneBin, "runtime"), filepath.Join(twoBin, "runtime")} {
		if err := os.WriteFile(path, script, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configured := manifest("symbols", ActionSymbols, []string{os.Args[0]})
	configured.Requires.Commands = []string{"runtime"}
	t.Setenv("PATH", oneBin)
	one, err := resultPath(configured, ActionSymbols, []byte("request"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", twoBin)
	two, err := resultPath(configured, ActionSymbols, []byte("request"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatalf("cache identity omitted required runtime path: %q", one)
	}
}

func TestRunRendersActionArgumentsAndEnvironmentWithoutShell(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	script := `test "$1" = "main.go"
test "$VALUE" = "fixture"
cat <<'JSON'
{
  "version": "changes.provider/v1",
  "symbols": {}
}
JSON
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script, "provider"})
	action := configured.Actions[ActionSymbols]
	action.Argv = []string{"{{index .Files 0}}"}
	action.Env = map[string]string{"VALUE": "{{.Fingerprint}}"}
	configured.Actions[ActionSymbols] = action
	configured.Requires.Environment = []string{"VALUE"}
	request := Request{Directory: t.TempDir(), Files: []string{"main.go"}, Fingerprint: "fixture"}
	if _, err := Run(context.Background(), configured, ActionSymbols, request); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsMissingRequiredEnvironment(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHANGES_TEST_REQUIRED_ENVIRONMENT", "")
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", "exit 0"})
	configured.Requires.Environment = []string{"CHANGES_TEST_REQUIRED_ENVIRONMENT"}
	_, err = Run(context.Background(), configured, ActionSymbols, Request{
		Directory: t.TempDir(), Fingerprint: t.Name(),
	})
	if err == nil || !strings.Contains(err.Error(), "CHANGES_TEST_REQUIRED_ENVIRONMENT") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsInvalidResponseVersion(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	configured := manifest("calls", ActionCalls, []string{shell, "-c", "printf '{}'"})
	_, err = Run(context.Background(), configured, ActionCalls, Request{Directory: t.TempDir(), Fingerprint: t.Name()})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReportsParentCancellation(t *testing.T) {
	t.Parallel()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	configured := manifest("symbols", ActionSymbols, []string{sleep, "5"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Run(ctx, configured, ActionSymbols, Request{
		Directory: t.TempDir(), Fingerprint: t.Name(),
	})
	if err == nil || !strings.Contains(err.Error(), "canceled: context canceled") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunReusesCachedResponse(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	directory := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs")
	t.Setenv("XDG_CACHE_HOME", cache)
	script := `printf x >> "$1"
cat <<'JSON'
{
  "version": "changes.provider/v1",
  "symbols": {}
}
JSON
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script, "provider", counter})
	request := Request{Directory: directory, Fingerprint: t.Name()}
	for range 2 {
		if _, err := Run(context.Background(), configured, ActionSymbols, request); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if string(runs) != "x" {
		t.Fatalf("provider ran %d times", len(runs))
	}
}

func TestValidateRequiresSemanticSymbolOutput(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	script := `cat >/dev/null
cat <<'JSON'
{
  "version": "changes.provider/v1",
  "symbols": {}
}
JSON
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script})
	result := Validate(context.Background(), LoadedManifest{Manifest: configured, Path: "test.yaml"})
	if result.OK() || !strings.Contains(result.Checks[len(result.Checks)-1].Message, "ready") {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateAcceptsSemanticSymbolOutput(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	script := `cat >/dev/null
cat <<'JSON'
{
  "version": "changes.provider/v1",
  "symbols": {
    "main.ts": [
      {
        "kind": "function",
        "name": "ready",
        "from": 3,
        "to": 3
      }
    ]
  }
}
JSON
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script})
	result := Validate(context.Background(), LoadedManifest{Manifest: configured, Path: "test.yaml"})
	if !result.OK() {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateRejectsProviderWithoutChangesAction(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	configured := manifest("foreign", "traces.query", []string{shell, "-c", "exit 0"})
	result := Validate(context.Background(), LoadedManifest{Manifest: configured, Path: "test.yaml"})
	if result.OK() || !strings.Contains(result.Checks[len(result.Checks)-1].Message, ActionSymbols) {
		t.Fatalf("validation = %+v", result)
	}
}

func TestDiscoverUsesUserManifestBeforeInstalledProvider(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", dataHome)
	userDirectory := filepath.Join(configHome, "changes", "providers")
	installedDirectory := filepath.Join(dataHome, "changes", "providers", "symbols")
	for _, directory := range []string{userDirectory, installedDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(userDirectory, "symbols.yaml"), []byte(providerYAML(t, "user")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDirectory, "symbols.yaml"), []byte(providerYAML(t, "installed")), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Manifest.Description != "user" {
		t.Fatalf("providers = %+v", loaded)
	}
}

func TestDiscoverExplicitDirectoryOverridesUserDirectory(t *testing.T) {
	configHome := t.TempDir()
	explicit := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", "")
	userDirectory := filepath.Join(configHome, "changes", "providers")
	if err := os.MkdirAll(userDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `version: provider/v1
name: explicit
description: explicit
command: [explicit]
actions:
  changes.symbols:
    description: symbols
`
	if err := os.WriteFile(filepath.Join(explicit, "provider.yaml"), []byte(manifestYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Discover(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Manifest.Name != "explicit" {
		t.Fatalf("providers = %+v", loaded)
	}
}

func TestDiscoverUsesXDGDataHomeBeforeInstalledProviders(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	dataDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", dataDirectory)

	userDirectory := filepath.Join(dataHome, "changes", "providers", "symbols")
	installedDirectory := filepath.Join(dataDirectory, "changes", "providers", "symbols")
	for _, directory := range []string{userDirectory, installedDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(userDirectory, "provider.yaml"), []byte(providerYAML(t, "user data")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDirectory, "provider.yaml"), []byte(providerYAML(t, "installed")), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Manifest.Description != "user data" {
		t.Fatalf("providers = %+v", loaded)
	}
}

func TestSearchDirectoriesIncludesExecutableAdjacentData(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "share", "changes", "providers"))
	directories, err := searchDirectories("")
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range directories {
		if directory == want {
			return
		}
	}
	t.Fatalf("executable-adjacent provider directory %q is absent from %v", want, directories)
}
