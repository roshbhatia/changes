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
	"time"

	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/diffview"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

const (
	ProtocolVersion = "changes.provider/v1"
	ActionCalls     = "changes.calls"
	ActionSymbols   = "changes.symbols"

	maxProviderCacheEntryBytes = 4 << 20
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

// CachePolicy bounds persistent provider results and their lifetime.
type CachePolicy struct {
	TTL        time.Duration
	MaxEntries int
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
func Run(
	ctx context.Context,
	loaded LoadedManifest,
	action string,
	request Request,
	cache CachePolicy,
) (Response, error) {
	manifest := loaded.Manifest
	if !Supports(manifest, action) {
		return Response{}, fmt.Errorf("provider %s does not implement %s", manifest.Name, action)
	}
	request, payload, err := prepareRequest(request, action)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	prepared, err := prepareExecution(loaded, action, request)
	if err != nil {
		return Response{}, err
	}
	cachePath, cacheErr := resultPath(prepared, action, payload)
	if cacheErr == nil && cache.TTL > 0 && cache.MaxEntries > 0 {
		pruneResults(filepath.Dir(cachePath), cache.TTL, cache.MaxEntries)
		if response, ok := readResult(cachePath, cache.TTL); ok {
			return response, nil
		}
	}
	response, err := execute(ctx, prepared, payload)
	if err != nil {
		return Response{}, err
	}
	if cacheErr == nil && cache.TTL > 0 && cache.MaxEntries > 0 {
		_ = writeResult(cachePath, response, cache.TTL, cache.MaxEntries)
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

type preparedExecution struct {
	manifest          Manifest
	manifestDirectory string
	workingDirectory  string
	providerFiles     []string
	plan              providerlib.Plan
	environment       []string
}

func prepareExecution(loaded LoadedManifest, action string, request Request) (preparedExecution, error) {
	manifestDirectory, err := providerManifestDirectory(loaded.Path, request.Directory)
	if err != nil {
		return preparedExecution{}, fmt.Errorf("resolve provider %s manifest directory: %w", loaded.Manifest.Name, err)
	}
	manifest, providerFiles, err := resolveManifestCommand(loaded.Manifest, manifestDirectory)
	if err != nil {
		return preparedExecution{}, fmt.Errorf("resolve provider %s: %w", loaded.Manifest.Name, err)
	}
	plan, err := manifest.Render(action, request)
	if err != nil {
		return preparedExecution{}, fmt.Errorf("render provider %s: %w", manifest.Name, err)
	}
	for index := len(manifest.Command); index < len(plan.Argv); index++ {
		if isExplicitRelativePath(plan.Argv[index]) {
			plan.Argv[index] = filepath.Clean(filepath.Join(manifestDirectory, plan.Argv[index]))
			providerFiles = append(providerFiles, plan.Argv[index])
		}
	}
	effectiveEnvironment := append(os.Environ(), sortedEnvironment(plan.Env)...)
	report := providerlib.Validator{LookupEnv: environmentLookup(effectiveEnvironment)}.Validate(manifest, request.Directory)
	if !report.OK() {
		return preparedExecution{}, fmt.Errorf("provider %s is invalid: %w", manifest.Name, report.Error())
	}
	return preparedExecution{
		manifest: manifest, manifestDirectory: manifestDirectory, workingDirectory: request.Directory,
		providerFiles: uniquePaths(providerFiles), plan: plan, environment: effectiveEnvironment,
	}, nil
}

func providerManifestDirectory(path, fallback string) (string, error) {
	directory := fallback
	if path != "" {
		directory = filepath.Dir(path)
	}
	if directory == "" {
		directory = "."
	}
	return filepath.Abs(directory)
}

func execute(ctx context.Context, prepared preparedExecution, payload []byte) (Response, error) {
	manifest := prepared.manifest
	plan := prepared.plan
	if plan.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(ctx, plan.Argv[0], plan.Argv[1:]...)
	command.Dir = prepared.workingDirectory
	command.Env = prepared.environment
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
	Digest   string `json:"digest,omitempty"`
}

func resultPath(prepared preparedExecution, action string, payload []byte) (string, error) {
	manifest := prepared.manifest
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
	commands := append([]string{prepared.plan.Argv[0]}, manifest.Requires.Commands...)
	executables := make([]executableIdentity, 0, len(commands))
	for _, command := range commands {
		executable, err := identifyExecutable(command, prepared.manifestDirectory)
		if err != nil {
			return "", err
		}
		executables = append(executables, executable)
	}
	arguments := make([]executableIdentity, 0, len(prepared.providerFiles))
	for _, argument := range prepared.providerFiles {
		identity, err := identifyFile(argument, true)
		if err == nil {
			arguments = append(arguments, identity)
		}
	}
	identity := struct {
		Manifest    Manifest             `json:"manifest"`
		Executables []executableIdentity `json:"executables"`
		Arguments   []executableIdentity `json:"arguments,omitempty"`
		Environment map[string]string    `json:"environment,omitempty"`
		Action      string               `json:"action"`
		Input       []byte               `json:"input"`
	}{manifest, executables, arguments, cacheEnvironment(prepared), action, payload}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return filepath.Join(root, "changes", "providers", digest+".json"), nil
}

func cacheEnvironment(prepared preparedExecution) map[string]string {
	values := make(map[string]string, len(prepared.plan.Env)+len(prepared.manifest.Requires.Environment))
	for name, value := range prepared.plan.Env {
		values[name] = value
	}
	lookup := environmentLookup(prepared.environment)
	for _, name := range prepared.manifest.Requires.Environment {
		if value, ok := lookup(name); ok {
			values[name] = value
		}
	}
	return values
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
	return identifyFile(path, false)
}

func identifyFile(path string, digest bool) (executableIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return executableIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return executableIdentity{}, fmt.Errorf("%s is not a regular file", path)
	}
	identity := executableIdentity{Path: path, Size: info.Size(), Modified: info.ModTime().UnixNano()}
	if digest {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return executableIdentity{}, readErr
		}
		identity.Digest = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	return identity, nil
}

type cachedResult struct {
	CreatedAt time.Time `json:"createdAt"`
	Response  Response  `json:"response"`
}

func readResult(path string, ttl time.Duration) (Response, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxProviderCacheEntryBytes {
		return Response{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Response{}, false
	}
	var cached cachedResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cached); err != nil || cached.CreatedAt.IsZero() || time.Since(cached.CreatedAt) > ttl {
		_ = os.Remove(path)
		return Response{}, false
	}
	encoded, err := json.Marshal(cached.Response)
	if err != nil || decodeResponse(bytes.NewReader(encoded), &cached.Response) != nil {
		_ = os.Remove(path)
		return Response{}, false
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return cached.Response, true
}

func writeResult(path string, response Response, ttl time.Duration, maxEntries int) error {
	data, err := json.Marshal(cachedResult{CreatedAt: time.Now().UTC(), Response: response})
	if err != nil {
		return err
	}
	if len(data) > maxProviderCacheEntryBytes {
		return nil
	}
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	pruneResults(filepath.Dir(path), ttl, maxEntries)
	return nil
}

func pruneResults(directory string, ttl time.Duration, maxEntries int) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	type candidate struct {
		path     string
		modified time.Time
	}
	now := time.Now()
	kept := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		if info.Size() > maxProviderCacheEntryBytes || now.Sub(info.ModTime()) > ttl {
			_ = os.Remove(path)
			continue
		}
		kept = append(kept, candidate{path: path, modified: info.ModTime()})
	}
	if len(kept) <= maxEntries {
		return
	}
	sort.Slice(kept, func(left, right int) bool {
		if kept[left].modified.Equal(kept[right].modified) {
			return kept[left].path < kept[right].path
		}
		return kept[left].modified.Before(kept[right].modified)
	})
	for _, entry := range kept[:len(kept)-maxEntries] {
		_ = os.Remove(entry.path)
	}
}

func resolveManifestCommand(manifest Manifest, manifestDirectory string) (Manifest, []string, error) {
	if len(manifest.Command) == 0 {
		return manifest, nil, errors.New("provider command is empty")
	}
	resolved := append([]string(nil), manifest.Command...)
	var providerFiles []string
	command := resolved[0]
	if strings.ContainsRune(command, filepath.Separator) {
		if !filepath.IsAbs(command) {
			command = filepath.Join(manifestDirectory, command)
		}
		resolved[0] = filepath.Clean(command)
	} else {
		path, err := exec.LookPath(command)
		if err != nil {
			return manifest, nil, err
		}
		resolved[0] = path
	}
	for index := 1; index < len(resolved); index++ {
		argument := resolved[index]
		if strings.HasPrefix(argument, "-") || filepath.IsAbs(argument) {
			continue
		}
		candidate := filepath.Join(manifestDirectory, argument)
		if isExplicitRelativePath(argument) {
			resolved[index] = filepath.Clean(candidate)
			providerFiles = append(providerFiles, resolved[index])
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			resolved[index] = filepath.Clean(candidate)
			providerFiles = append(providerFiles, resolved[index])
		}
	}
	manifest.Command = resolved
	manifest.Requires.Commands = resolveRequiredCommands(manifest.Requires.Commands, manifestDirectory)
	return manifest, uniquePaths(providerFiles), nil
}

func resolveRequiredCommands(commands []string, manifestDirectory string) []string {
	resolved := append([]string(nil), commands...)
	for index, command := range resolved {
		if strings.ContainsRune(command, filepath.Separator) && !filepath.IsAbs(command) {
			resolved[index] = filepath.Clean(filepath.Join(manifestDirectory, command))
		}
	}
	return resolved
}

func isExplicitRelativePath(value string) bool {
	return value == "." || value == ".." ||
		strings.HasPrefix(value, "."+string(filepath.Separator)) ||
		strings.HasPrefix(value, ".."+string(filepath.Separator))
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
	manifestDirectory, err := providerManifestDirectory(loaded.Path, "")
	if err != nil {
		return Validation{
			Manifest: manifest, Path: loaded.Path,
			Checks: []providerlib.Check{failedCheck("manifest-directory", loaded.Path, err)},
		}
	}
	resolved, _, err := resolveManifestCommand(manifest, manifestDirectory)
	if err != nil {
		return Validation{
			Manifest: manifest, Path: loaded.Path,
			Checks: []providerlib.Check{failedCheck("command", manifest.Name, err)},
		}
	}
	directory, err := source.ValidationFixture()
	if err != nil {
		return Validation{
			Manifest: manifest, Path: loaded.Path,
			Checks: []providerlib.Check{failedCheck("fixture", manifest.Name, err)},
		}
	}
	defer func() { _ = os.RemoveAll(directory) }()
	report := providerlib.Validator{LookupEnv: validationEnvironmentLookup(resolved)}.Validate(resolved, directory)
	validation := Validation{Manifest: manifest, Path: loaded.Path, Checks: report.Checks}
	if !report.OK() {
		return validation
	}
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
			execution, prepareErr := prepareExecution(LoadedManifest{Manifest: resolved, Path: loaded.Path}, action, prepared)
			if prepareErr != nil {
				err = prepareErr
			} else {
				response, err = execute(ctx, execution, payload)
			}
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
