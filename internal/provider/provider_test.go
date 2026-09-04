package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	providerlib "github.com/roshbhatia/go-utils/provider"
)

func manifest(name, action string, command []string) Manifest {
	return Manifest{
		Version: providerlib.Version, Name: name, Description: name + " provider", Command: command,
		Actions: map[string]providerlib.Action{action: {Description: action}},
	}
}

func cacheExecution(t *testing.T, configured Manifest, directory string) preparedExecution {
	t.Helper()
	resolved, providerFiles, err := resolveManifestCommand(configured, directory)
	if err != nil {
		t.Fatal(err)
	}
	return preparedExecution{
		manifest: resolved, manifestDirectory: directory, workingDirectory: directory,
		providerFiles: providerFiles, plan: providerlib.Plan{Argv: resolved.Command},
	}
}

func runProvider(ctx context.Context, configured Manifest, action string, request Request) (Response, error) {
	return Run(ctx, LoadedManifest{Manifest: configured}, action, request, CachePolicy{})
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
	one, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	two, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	three, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionCalls, []byte("first"))
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
	one, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("request"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", twoBin)
	two, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("request"))
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
	one, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("request"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", twoBin)
	two, err := resultPath(cacheExecution(t, configured, t.TempDir()), ActionSymbols, []byte("request"))
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
	if _, err := runProvider(context.Background(), configured, ActionSymbols, request); err != nil {
		t.Fatal(err)
	}
}

func TestRunResolvesProviderCommandBesideManifest(t *testing.T) {
	manifestDirectory := t.TempDir()
	workingDirectory := t.TempDir()
	scriptPath := filepath.Join(manifestDirectory, "provider.sh")
	script := `#!/bin/sh
test "$PWD" = "$TARGET_DIRECTORY"
cat >/dev/null
printf '%s\n' '{"version":"changes.provider/v1","symbols":{}}'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	configured := manifest("symbols", ActionSymbols, []string{"./provider.sh"})
	action := configured.Actions[ActionSymbols]
	action.Env = map[string]string{"TARGET_DIRECTORY": "{{.Directory}}"}
	configured.Actions[ActionSymbols] = action
	_, err := Run(
		context.Background(),
		LoadedManifest{Manifest: configured, Path: filepath.Join(manifestDirectory, "provider.yaml")},
		ActionSymbols,
		Request{Directory: workingDirectory, Fingerprint: t.Name()},
		CachePolicy{},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunResolvesRequiredPathsFromTargetRepository(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, ".provider-ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	configured := manifest("symbols", ActionSymbols, []string{
		shell, "-c", `cat >/dev/null; printf '%s\n' '{"version":"changes.provider/v1","symbols":{}}'`,
	})
	configured.Requires.Paths = []string{".provider-ready"}
	_, err = Run(
		context.Background(),
		LoadedManifest{Manifest: configured, Path: filepath.Join(t.TempDir(), "provider.yaml")},
		ActionSymbols,
		Request{Directory: workingDirectory, Fingerprint: t.Name()},
		CachePolicy{},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateResolvesInterpreterScriptBesideManifest(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	manifestDirectory := t.TempDir()
	scriptPath := filepath.Join(manifestDirectory, "provider.sh")
	script := `cat >/dev/null
cat <<'JSON'
{"version":"changes.provider/v1","symbols":{"main.ts":[{"kind":"function","name":"ready","from":3,"to":3}]}}
JSON
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := manifest("symbols", ActionSymbols, []string{shell, "provider.sh"})
	result := Validate(context.Background(), LoadedManifest{
		Manifest: configured,
		Path:     filepath.Join(manifestDirectory, "provider.yaml"),
	})
	if !result.OK() {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateResolvesRequiredCommandBesideManifest(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	manifestDirectory := t.TempDir()
	helperPath := filepath.Join(manifestDirectory, "helper")
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := `cat >/dev/null
cat <<'JSON'
{"version":"changes.provider/v1","symbols":{"main.ts":[{"kind":"function","name":"ready","from":3,"to":3}]}}
JSON
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script})
	configured.Requires.Commands = []string{"./helper"}
	result := Validate(context.Background(), LoadedManifest{
		Manifest: configured,
		Path:     filepath.Join(manifestDirectory, "provider.yaml"),
	})
	if !result.OK() {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateRendersProtocolFieldsLikeExecution(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	script := `test "$1" = "changes.provider/v1"
test "$2" = "changes.symbols"
test "$VERSION" = "changes.provider/v1"
test "$ACTION" = "changes.symbols"
cat >/dev/null
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
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script, "provider"})
	action := configured.Actions[ActionSymbols]
	action.Argv = []string{"{{.Version}}", "{{.Action}}"}
	action.Env = map[string]string{"VERSION": "{{.Version}}", "ACTION": "{{.Action}}"}
	configured.Actions[ActionSymbols] = action
	result := Validate(context.Background(), LoadedManifest{Manifest: configured, Path: "test.yaml"})
	if !result.OK() {
		t.Fatalf("validation = %+v", result)
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
	_, err = runProvider(context.Background(), configured, ActionSymbols, Request{
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
	_, err = runProvider(context.Background(), configured, ActionCalls, Request{Directory: t.TempDir(), Fingerprint: t.Name()})
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
	_, err = runProvider(ctx, configured, ActionSymbols, Request{
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
		if _, err := Run(
			context.Background(), LoadedManifest{Manifest: configured}, ActionSymbols, request,
			CachePolicy{TTL: time.Hour, MaxEntries: 256},
		); err != nil {
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

func TestRunInvalidatesCacheWhenInterpreterScriptChanges(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	cacheDirectory := t.TempDir()
	root := t.TempDir()
	manifestDirectory := filepath.Join(root, "provider")
	sharedDirectory := filepath.Join(root, "shared")
	for _, directory := range []string{manifestDirectory, sharedDirectory} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workingDirectory := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDirectory)
	scriptPath := filepath.Join(sharedDirectory, "provider.sh")
	writeScript := func(name string) {
		t.Helper()
		script := `cat >/dev/null
printf '%s\n' '{"version":"changes.provider/v1","symbols":{"main.go":[{"kind":"function","name":"` + name + `","from":1,"to":1}]}}'
`
		if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeScript("one")
	initialInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	configured := manifest("symbols", ActionSymbols, []string{shell, "../shared/provider.sh"})
	loaded := LoadedManifest{Manifest: configured, Path: filepath.Join(manifestDirectory, "provider.yaml")}
	request := Request{Directory: workingDirectory, Fingerprint: t.Name()}
	policy := CachePolicy{TTL: time.Hour, MaxEntries: 256}
	first, err := Run(context.Background(), loaded, ActionSymbols, request, policy)
	if err != nil {
		t.Fatal(err)
	}
	writeScript("two")
	if err := os.Chtimes(scriptPath, initialInfo.ModTime(), initialInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	second, err := Run(context.Background(), loaded, ActionSymbols, request, policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.Symbols["main.go"][0].Name != "one" || second.Symbols["main.go"][0].Name != "two" {
		t.Fatalf("cached symbols = %q then %q", first.Symbols["main.go"][0].Name, second.Symbols["main.go"][0].Name)
	}
}

func TestRunInvalidatesCacheWhenRequiredEnvironmentChanges(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	script := `cat >/dev/null
printf '{"version":"changes.provider/v1","symbols":{"main.go":[{"kind":"variable","name":"%s","from":1,"to":1}]}}\n' "$MODE"
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script})
	configured.Requires.Environment = []string{"MODE"}
	request := Request{Directory: t.TempDir(), Fingerprint: t.Name()}
	policy := CachePolicy{TTL: time.Hour, MaxEntries: 256}
	t.Setenv("MODE", "one")
	first, err := Run(context.Background(), LoadedManifest{Manifest: configured}, ActionSymbols, request, policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODE", "two")
	second, err := Run(context.Background(), LoadedManifest{Manifest: configured}, ActionSymbols, request, policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.Symbols["main.go"][0].Name != "one" || second.Symbols["main.go"][0].Name != "two" {
		t.Fatalf("cached environment = %q then %q", first.Symbols["main.go"][0].Name, second.Symbols["main.go"][0].Name)
	}
}

func TestRunExpiresCachedResponse(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	counter := filepath.Join(t.TempDir(), "runs")
	script := `printf x >> "$1"
printf '%s\n' '{"version":"changes.provider/v1","symbols":{}}'
`
	configured := manifest("symbols", ActionSymbols, []string{shell, "-c", script, "provider", counter})
	request := Request{Directory: t.TempDir(), Fingerprint: t.Name()}
	policy := CachePolicy{TTL: 10 * time.Millisecond, MaxEntries: 256}
	if _, err := Run(context.Background(), LoadedManifest{Manifest: configured}, ActionSymbols, request, policy); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, err := Run(context.Background(), LoadedManifest{Manifest: configured}, ActionSymbols, request, policy); err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if string(runs) != "xx" {
		t.Fatalf("provider ran %d times", len(runs))
	}
}

func TestRunBoundsPersistentCacheEntries(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	configured := manifest("symbols", ActionSymbols, []string{
		shell, "-c", `cat >/dev/null; printf '%s\n' '{"version":"changes.provider/v1","symbols":{}}'`,
	})
	loaded := LoadedManifest{Manifest: configured}
	policy := CachePolicy{TTL: time.Hour, MaxEntries: 5}
	workingDirectory := t.TempDir()
	requests := make([]Request, 0, policy.MaxEntries)
	for index := range 5 {
		request := Request{Directory: workingDirectory, Fingerprint: fmt.Sprintf("%s-%d", t.Name(), index)}
		requests = append(requests, request)
		if _, err := Run(context.Background(), loaded, ActionSymbols, request, policy); err != nil {
			t.Fatal(err)
		}
	}
	policy.MaxEntries = 2
	if _, err := Run(context.Background(), loaded, ActionSymbols, requests[0], policy); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "changes", "providers"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != policy.MaxEntries {
		t.Fatalf("cache contains %d entries, want %d", len(entries), policy.MaxEntries)
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
	configured := manifest("foreign", "unrelated.query", []string{shell, "-c", "exit 0"})
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
	if err := os.WriteFile(filepath.Join(installedDirectory, "provider.yaml"), []byte(providerYAML(t, "installed")), 0o600); err != nil {
		t.Fatal(err)
	}
	discovery, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Description != "user" {
		t.Fatalf("discovery = %+v", discovery)
	}
}

func TestDiscoverMalformedOverrideDoesNotReserveProviderName(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(userDirectory, "symbols.yaml"), []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDirectory, "provider.yaml"), []byte(providerYAML(t, "installed")), 0o600); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Description != "installed" {
		t.Fatalf("providers = %+v", discovery.Providers)
	}
	if len(discovery.Diagnostics) != 1 || discovery.Diagnostics[0].Manifest.Name != "symbols" {
		t.Fatalf("diagnostics = %+v", discovery.Diagnostics)
	}
}

func TestDiscoverContinuesPastMalformedOptionalManifest(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "broken.yaml"), []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "symbols.yaml"), []byte(providerYAML(t, "valid")), 0o600); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || len(discovery.Diagnostics) != 1 {
		t.Fatalf("discovery = %+v", discovery)
	}
	byName := map[string]LoadedManifest{}
	for _, candidate := range discovery.All() {
		byName[candidate.Manifest.Name] = candidate
	}
	if !byName["symbols"].Valid() {
		t.Fatalf("valid provider was disabled: %+v", byName["symbols"])
	}
	if byName["broken"].Valid() || byName["broken"].Problem == "" {
		t.Fatalf("malformed provider has no diagnostic: %+v", byName["broken"])
	}
	validation := Validate(context.Background(), byName["broken"])
	if validation.OK() || validation.Checks[0].Kind != "manifest" {
		t.Fatalf("validation = %+v", validation)
	}
}

func TestDiscoverIgnoresNonManifestJSONFiles(t *testing.T) {
	directory := t.TempDir()
	nested := filepath.Join(directory, "symbols")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "provider.yaml"), []byte(providerYAML(t, "valid")), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(directory, "package.json"),
		filepath.Join(nested, "package.json"),
	} {
		if err := os.WriteFile(path, []byte(`{"name":"adapter","private":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	discovery, err := Discover(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Name != "symbols" {
		t.Fatalf("providers = %+v", discovery.Providers)
	}
	if len(discovery.Diagnostics) != 0 {
		t.Fatalf("package metadata was treated as a manifest: %+v", discovery.Diagnostics)
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
	discovery, err := Discover(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Name != "explicit" {
		t.Fatalf("discovery = %+v", discovery)
	}
}

func TestDiscoverFollowsProviderDirectorySymlinks(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "provider.yaml"), []byte(providerYAML(t, "linked")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "symbols")); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Description != "linked" {
		t.Fatalf("discovery = %+v", discovery)
	}
}

func TestDiscoverReportsBrokenProviderDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing-provider")
	if err := os.Symlink(filepath.Join(root, "missing-target"), path); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Diagnostics) != 1 || discovery.Diagnostics[0].Valid() {
		t.Fatalf("discovery = %+v", discovery)
	}
	diagnostic := discovery.Diagnostics[0]
	if diagnostic.Manifest.Name != "missing-provider" || !strings.Contains(diagnostic.Problem, "inspect provider path") {
		t.Fatalf("diagnostic = %+v", diagnostic)
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

	discovery, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Providers) != 1 || discovery.Providers[0].Manifest.Description != "user data" {
		t.Fatalf("discovery = %+v", discovery)
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
