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
	"sort"
	"strings"

	"github.com/roshbhatia/changes/internal/source"
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
	Problem  string   `json:"problem,omitempty"`
}

// Valid reports whether discovery decoded and validated the manifest.
func (loaded LoadedManifest) Valid() bool { return loaded.Problem == "" }

// Discovery keeps runnable providers separate from manifest diagnostics.
type Discovery struct {
	Providers   []LoadedManifest
	Diagnostics []LoadedManifest
}

// All returns runnable providers followed by diagnostics for inspection.
func (discovery Discovery) All() []LoadedManifest {
	all := make([]LoadedManifest, 0, len(discovery.Providers)+len(discovery.Diagnostics))
	all = append(all, discovery.Providers...)
	return append(all, discovery.Diagnostics...)
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

// Discover loads providers by precedence; invalid manifests do not reserve names.
func Discover(directory string) (Discovery, error) {
	directories, err := searchDirectories(directory)
	if err != nil {
		return Discovery{}, err
	}
	seen := map[string]bool{}
	var discovery Discovery
	for _, one := range directories {
		loaded, err := discoverDirectory(one)
		if err != nil {
			return Discovery{}, err
		}
		for _, candidate := range loaded {
			if !candidate.Valid() {
				discovery.Diagnostics = append(discovery.Diagnostics, candidate)
				continue
			}
			if seen[candidate.Manifest.Name] {
				continue
			}
			seen[candidate.Manifest.Name] = true
			discovery.Providers = append(discovery.Providers, candidate)
		}
	}
	sort.SliceStable(discovery.Providers, func(left, right int) bool {
		leftPriority := discovery.Providers[left].Manifest.Defaults.Priority
		rightPriority := discovery.Providers[right].Manifest.Defaults.Priority
		if leftPriority == rightPriority {
			return discovery.Providers[left].Manifest.Name < discovery.Providers[right].Manifest.Name
		}
		return leftPriority > rightPriority
	})
	sort.SliceStable(discovery.Diagnostics, func(left, right int) bool {
		if discovery.Diagnostics[left].Manifest.Name == discovery.Diagnostics[right].Manifest.Name {
			return discovery.Diagnostics[left].Path < discovery.Diagnostics[right].Path
		}
		return discovery.Diagnostics[left].Manifest.Name < discovery.Diagnostics[right].Manifest.Name
	})
	return discovery, nil
}

func discoverDirectory(root string) ([]LoadedManifest, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return []LoadedManifest{invalidManifest(root, filepath.Dir(root), fmt.Errorf("read provider directory %s: %w", root, err))}, nil
	}
	var paths []string
	var invalid []LoadedManifest
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		isDirectory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, statErr := os.Stat(path)
			if statErr != nil {
				invalid = append(invalid, invalidManifest(path, root, fmt.Errorf("inspect provider path %s: %w", path, statErr)))
				continue
			}
			isDirectory = info.IsDir()
		}
		if !isDirectory {
			if isFlatManifest(path) {
				paths = append(paths, path)
			}
			continue
		}
		nested, readErr := os.ReadDir(path)
		if readErr != nil {
			invalid = append(invalid, invalidManifest(path, root, fmt.Errorf("read provider directory %s: %w", path, readErr)))
			continue
		}
		for _, candidate := range nested {
			candidatePath := filepath.Join(path, candidate.Name())
			if !candidate.IsDir() && isProviderManifest(candidate.Name()) {
				paths = append(paths, candidatePath)
			}
		}
	}
	sort.Strings(paths)
	loaded := make([]LoadedManifest, 0, len(paths)+len(invalid))
	loaded = append(loaded, invalid...)
	for _, path := range paths {
		candidate := loadManifest(path, root)
		loaded = append(loaded, candidate)
	}
	return loaded, nil
}

func invalidManifest(path, root string, err error) LoadedManifest {
	return LoadedManifest{
		Manifest: Manifest{Name: inferredName(path, root)},
		Path:     path,
		Problem:  err.Error(),
	}
}

func isFlatManifest(path string) bool {
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		return true
	default:
		return isProviderManifest(filepath.Base(path))
	}
}

func isProviderManifest(name string) bool {
	switch name {
	case "provider.json", "provider.yaml", "provider.yml":
		return true
	default:
		return false
	}
}

func loadManifest(path, root string) LoadedManifest {
	loaded := invalidManifest(path, root, errors.New("manifest was not read"))
	data, err := os.ReadFile(path)
	if err == nil {
		manifest, decodeErr := providerlib.Decode(bytes.NewReader(data), filepath.Ext(path))
		if decodeErr == nil {
			loaded.Manifest = manifest
		}
		err = decodeErr
	}
	if err != nil {
		loaded.Problem = err.Error()
	} else {
		loaded.Problem = ""
	}
	return loaded
}

func inferredName(path, root string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "provider" && filepath.Dir(path) != root {
		return filepath.Base(filepath.Dir(path))
	}
	return name
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

	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" || !filepath.IsAbs(dataHome) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("find provider data directory: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	directories = append(directories, filepath.Join(dataHome, "changes", "providers"))

	if executable, err := os.Executable(); err == nil {
		binaryDirectory := filepath.Dir(executable)
		directories = append(directories,
			filepath.Join(binaryDirectory, "providers"),
			filepath.Join(binaryDirectory, "..", "share", "changes", "providers"),
		)
	}

	dataDirectories := filepath.SplitList(os.Getenv("XDG_DATA_DIRS"))
	if len(dataDirectories) == 0 {
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
	request, payload, err := prepareRequest(request, action)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	cachePath, err := resultPath(manifest, action, payload, request.Directory)
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

func prepareRequest(request Request, action string) (Request, []byte, error) {
	request.Version = ProtocolVersion
	request.Action = action
	payload, err := json.Marshal(request)
	if err != nil {
		return Request{}, nil, err
	}
	return request, payload, nil
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

type executableIdentity struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

func resultPath(manifest Manifest, action string, payload []byte, workingDirectory string) (string, error) {
	if len(manifest.Command) == 0 {
		return "", errors.New("provider command is empty")
	}
	root := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	commands := append([]string{manifest.Command[0]}, manifest.Requires.Commands...)
	executables := make([]executableIdentity, 0, len(commands))
	for _, command := range commands {
		executable, err := identifyExecutable(command, workingDirectory)
		if err != nil {
			return "", err
		}
		executables = append(executables, executable)
	}
	identity := struct {
		Manifest    Manifest             `json:"manifest"`
		Executables []executableIdentity `json:"executables"`
		Action      string               `json:"action"`
		Input       []byte               `json:"input"`
	}{manifest, executables, action, payload}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return filepath.Join(root, "changes", "providers", digest+".json"), nil
}

func identifyExecutable(command, workingDirectory string) (executableIdentity, error) {
	path := command
	var err error
	if strings.ContainsRune(command, filepath.Separator) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDirectory, path)
		}
	} else {
		path, err = exec.LookPath(command)
		if err != nil {
			return executableIdentity{}, err
		}
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolved
	}
	info, err := os.Stat(path)
	if err != nil {
		return executableIdentity{}, err
	}
	return executableIdentity{Path: path, Size: info.Size(), Modified: info.ModTime().UnixNano()}, nil
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
	if !loaded.Valid() {
		return Validation{
			Manifest: manifest,
			Path:     loaded.Path,
			Checks:   []providerlib.Check{failedCheck("manifest", loaded.Path, errors.New(loaded.Problem))},
		}
	}
	report := providerlib.Validator{LookupEnv: validationEnvironmentLookup(manifest)}.Validate(manifest, "")
	validation := Validation{Manifest: manifest, Path: loaded.Path, Checks: report.Checks}
	if !report.OK() {
		return validation
	}
	directory, err := source.ValidationFixture()
	if err != nil {
		validation.Checks = append(validation.Checks, failedCheck("fixture", manifest.Name, err))
		return validation
	}
	defer func() { _ = os.RemoveAll(directory) }()
	request := Request{
		Directory: directory, Files: []string{"main.ts"}, Fingerprint: "provider-validation",
	}
	probed := false
	for _, action := range []string{ActionSymbols, ActionCalls} {
		if !Supports(manifest, action) {
			continue
		}
		probed = true
		prepared, payload, err := prepareRequest(request, action)
		if err == nil {
			var response Response
			response, err = execute(ctx, manifest, action, prepared, payload)
			if err == nil {
				err = validateSemantics(action, response)
			}
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

func validateSemantics(action string, response Response) error {
	switch action {
	case ActionSymbols:
		for _, symbol := range response.Symbols["main.ts"] {
			if strings.Contains(symbol.Name, "ready") {
				return nil
			}
		}
		return errors.New("response did not identify ready in main.ts")
	case ActionCalls:
		if len(response.Edges["main.ts"]) == 0 {
			return errors.New("response did not identify the changed call in main.ts")
		}
	}
	return nil
}

func failedCheck(kind, target string, err error) providerlib.Check {
	return providerlib.Check{Kind: kind, Target: target, Status: providerlib.CheckFailed, Message: err.Error()}
}
