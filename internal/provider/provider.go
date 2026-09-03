// Package provider discovers and runs optional change-analysis providers.
package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/roshbhatia/go-utils/diffview"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

const (
	ProtocolVersion = "changes.provider/v1"
	ActionCalls     = "changes.calls"
	ActionSymbols   = "changes.symbols"
)

// Manifest is the shared provider/v1 manifest.
type Manifest = providerlib.Manifest

// LoadedManifest retains the manifest source for diagnostics.
type LoadedManifest struct {
	Manifest Manifest `json:"manifest"`
	Path     string   `json:"path"`
}

// Schema returns the shared provider/v1 manifest schema.
func Schema() ([]byte, error) { return providerlib.Schema() }

// Request describes one repository comparison without naming an implementation.
type Request struct {
	Version     string   `json:"version"`
	Action      string   `json:"action"`
	Directory   string   `json:"directory"`
	Files       []string `json:"files"`
	From        string   `json:"from,omitempty"`
	Fingerprint string   `json:"fingerprint"`
	Staged      bool     `json:"staged,omitempty"`
	To          string   `json:"to,omitempty"`
}

// Response carries analysis layers produced by a provider.
type Response struct {
	Version string                       `json:"version"`
	Edges   map[string][]diffview.Edge   `json:"edges,omitempty"`
	Symbols map[string][]diffview.Symbol `json:"symbols,omitempty"`
}

// Validation combines shared manifest checks with an action-level probe.
type Validation struct {
	Manifest Manifest            `json:"manifest"`
	Path     string              `json:"path,omitempty"`
	Checks   []providerlib.Check `json:"checks"`
}

// OK reports whether all validation checks passed.
func (validation Validation) OK() bool {
	if len(validation.Checks) == 0 {
		return false
	}
	for _, check := range validation.Checks {
		if check.Status == providerlib.CheckFailed {
			return false
		}
	}
	return true
}

// Supports reports whether a provider advertises an action.
func Supports(manifest Manifest, action string) bool {
	_, ok := manifest.Actions[action]
	return ok
}

// Discover loads the user provider directory followed by installed manifests.
// The first manifest with a given name wins, so user manifests override packages.
func Discover(directory string) ([]LoadedManifest, error) {
	directories, err := searchDirectories(directory)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var manifests []LoadedManifest
	for _, one := range directories {
		loaded, err := discoverDirectory(one)
		if err != nil {
			return nil, err
		}
		for _, candidate := range loaded {
			if seen[candidate.Manifest.Name] {
				continue
			}
			seen[candidate.Manifest.Name] = true
			manifests = append(manifests, LoadedManifest{
				Manifest: candidate.Manifest,
				Path:     candidate.Path,
			})
		}
	}
	sort.SliceStable(manifests, func(left, right int) bool {
		leftPriority := manifests[left].Manifest.Defaults.Priority
		rightPriority := manifests[right].Manifest.Defaults.Priority
		if leftPriority == rightPriority {
			return manifests[left].Manifest.Name < manifests[right].Manifest.Name
		}
		return leftPriority > rightPriority
	})
	return manifests, nil
}

func discoverDirectory(root string) ([]providerlib.LoadedManifest, error) {
	loaded, err := providerlib.Discover(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return loaded, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read provider directory %s: %w", root, err)
	}
	seen := make(map[string]string, len(loaded))
	for _, candidate := range loaded {
		seen[candidate.Manifest.Name] = candidate.Path
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, fmt.Errorf("inspect provider path %s: %w", path, statErr)
		}
		if !info.IsDir() {
			continue
		}
		nested, discoverErr := providerlib.Discover(path)
		if discoverErr != nil {
			return nil, discoverErr
		}
		for _, candidate := range nested {
			if previous, exists := seen[candidate.Manifest.Name]; exists {
				return nil, fmt.Errorf("duplicate provider %q in %s and %s", candidate.Manifest.Name, previous, candidate.Path)
			}
			seen[candidate.Manifest.Name] = candidate.Path
			loaded = append(loaded, candidate)
		}
	}
	return loaded, nil
}

func searchDirectories(explicit string) ([]string, error) {
	var directories []string
	if strings.TrimSpace(explicit) != "" {
		directories = append(directories, filepath.Clean(explicit))
	} else {
		root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if root == "" || !filepath.IsAbs(root) {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("find provider config directory: %w", err)
			}
			root = filepath.Join(home, ".config")
		}
		directories = append(directories, filepath.Join(root, "changes", "providers"))
	}

	dataDirectories := filepath.SplitList(os.Getenv("XDG_DATA_DIRS"))
	if len(dataDirectories) == 0 && runtime.GOOS != "windows" {
		dataDirectories = []string{"/usr/local/share", "/usr/share"}
	}
	for _, root := range dataDirectories {
		if root != "" {
			directories = append(directories, filepath.Join(root, "changes", "providers"))
		}
	}
	return uniquePaths(directories), nil
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

// Run renders an action plan, sends one request on stdin, and decodes one response.
func Run(ctx context.Context, manifest Manifest, action string, request Request) (Response, error) {
	if !Supports(manifest, action) {
		return Response{}, fmt.Errorf("provider %s does not implement %s", manifest.Name, action)
	}
	request.Version = ProtocolVersion
	request.Action = action
	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	cachePath, err := resultPath(manifest, action, payload)
	if err == nil {
		if cached, readErr := os.ReadFile(cachePath); readErr == nil {
			response := Response{}
			if decodeResponse(bytes.NewReader(cached), &response) == nil {
				return response, nil
			}
		}
	}
	response, err := execute(ctx, manifest, action, request, payload)
	if err != nil {
		return Response{}, err
	}
	if cachePath != "" {
		encoded, _ := json.Marshal(response)
		_ = writeResult(cachePath, encoded)
	}
	return response, nil
}

func execute(ctx context.Context, manifest Manifest, action string, request Request, payload []byte) (Response, error) {
	plan, err := manifest.Render(action, request)
	if err != nil {
		return Response{}, fmt.Errorf("render provider %s: %w", manifest.Name, err)
	}
	effectiveEnvironment := append(os.Environ(), sortedEnvironment(plan.Env)...)
	report := providerlib.Validator{LookupEnv: environmentLookup(effectiveEnvironment)}.Validate(manifest, request.Directory)
	if !report.OK() {
		return Response{}, fmt.Errorf("provider %s is invalid: %w", manifest.Name, report.Error())
	}
	if plan.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(ctx, plan.Argv[0], plan.Argv[1:]...)
	command.Dir = request.Directory
	command.Env = effectiveEnvironment
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Response{}, fmt.Errorf("provider %s timed out: %w", manifest.Name, ctx.Err())
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return Response{}, fmt.Errorf("provider %s canceled: %w", manifest.Name, ctx.Err())
		}
		if message != "" {
			return Response{}, fmt.Errorf("provider %s: %s: %w", manifest.Name, message, err)
		}
		return Response{}, fmt.Errorf("provider %s: %w", manifest.Name, err)
	}
	response := Response{}
	if err := decodeResponse(&stdout, &response); err != nil {
		return Response{}, fmt.Errorf("provider %s returned invalid JSON: %w", manifest.Name, err)
	}
	return response, nil
}

func decodeResponse(reader io.Reader, response *Response) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("response must contain one JSON value")
		}
		return err
	}
	if response.Version != ProtocolVersion {
		return fmt.Errorf("version must be %q", ProtocolVersion)
	}
	return nil
}

func sortedEnvironment(environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+environment[key])
	}
	return values
}

func environmentLookup(environment []string) func(string) (string, bool) {
	values := make(map[string]string, len(environment))
	for _, assignment := range environment {
		name, value, ok := strings.Cut(assignment, "=")
		if ok {
			values[name] = value
		}
	}
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func resultPath(manifest Manifest, action string, payload []byte) (string, error) {
	root := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	identity := struct {
		Manifest Manifest `json:"manifest"`
		Action   string   `json:"action"`
		Input    []byte   `json:"input"`
	}{manifest, action, payload}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return filepath.Join(root, "changes", "providers", digest+".json"), nil
}

func writeResult(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".provider-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

// Validate checks the shared manifest and probes every Changes action.
func Validate(ctx context.Context, loaded LoadedManifest) Validation {
	manifest := loaded.Manifest
	report := providerlib.Validator{LookupEnv: validationEnvironmentLookup(manifest)}.Validate(manifest, "")
	validation := Validation{Manifest: manifest, Path: loaded.Path, Checks: report.Checks}
	if !report.OK() {
		return validation
	}
	directory, err := validationRepository()
	if err != nil {
		validation.Checks = append(validation.Checks, failedCheck("fixture", manifest.Name, err))
		return validation
	}
	defer func() { _ = os.RemoveAll(directory) }()
	request := Request{
		Directory: directory, Files: []string{"main.go"}, Fingerprint: "provider-validation",
	}
	probed := false
	for _, action := range []string{ActionSymbols, ActionCalls} {
		if !Supports(manifest, action) {
			continue
		}
		probed = true
		response, err := execute(ctx, manifest, action, request, mustJSON(request, action))
		if err == nil {
			err = validateSemantics(action, response)
		}
		check := providerlib.Check{Kind: "action", Target: action, Status: providerlib.CheckOK}
		if err != nil {
			check.Status = providerlib.CheckFailed
			check.Message = err.Error()
		}
		validation.Checks = append(validation.Checks, check)
	}
	if !probed {
		validation.Checks = append(validation.Checks, failedCheck(
			"action", manifest.Name, errors.New("provider does not advertise changes.symbols or changes.calls"),
		))
	}
	return validation
}

func validationEnvironmentLookup(manifest Manifest) func(string) (string, bool) {
	host := environmentLookup(os.Environ())
	return func(name string) (string, bool) {
		if value, ok := host(name); ok && value != "" {
			return value, true
		}
		for _, action := range manifest.Actions {
			if source, ok := action.Env[name]; ok && source != "" {
				return source, true
			}
		}
		return "", false
	}
}

func mustJSON(request Request, action string) []byte {
	request.Version = ProtocolVersion
	request.Action = action
	payload, _ := json.Marshal(request)
	return payload
}

func validationRepository() (string, error) {
	directory, err := os.MkdirTemp("", "changes-provider-validation-")
	if err != nil {
		return "", err
	}
	cleanup := func(err error) (string, error) {
		_ = os.RemoveAll(directory)
		return "", err
	}
	initial := "package validation\n\nfunc Ready() bool { return false }\nfunc main() {}\n"
	changed := "package validation\n\nfunc Ready() bool { return true }\nfunc main() { Ready() }\n"
	path := filepath.Join(directory, "main.go")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return cleanup(err)
	}
	commands := [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes validation"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.go"},
		{"commit", "--quiet", "-m", "validation fixture"},
	}
	for _, arguments := range commands {
		command := exec.Command("git", arguments...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			return cleanup(fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(string(output)), err))
		}
	}
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		return cleanup(err)
	}
	return directory, nil
}

func validateSemantics(action string, response Response) error {
	switch action {
	case ActionSymbols:
		for _, symbol := range response.Symbols["main.go"] {
			if strings.Contains(symbol.Name, "Ready") {
				return nil
			}
		}
		return errors.New("response did not identify Ready in main.go")
	case ActionCalls:
		if len(response.Edges["main.go"]) == 0 {
			return errors.New("response did not identify the changed call in main.go")
		}
	}
	return nil
}

func failedCheck(kind, target string, err error) providerlib.Check {
	return providerlib.Check{Kind: kind, Target: target, Status: providerlib.CheckFailed, Message: err.Error()}
}
