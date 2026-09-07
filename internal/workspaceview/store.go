package workspaceview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	StateVersion     = "changes.workspace.state/v1"
	maxStateBytes    = 1 << 20
	maxSnapshotBytes = 64 << 20
)

type Store struct {
	StateRoot       string
	CacheRoot       string
	CacheMaxEntries int
}

func DefaultStore(cacheMaxEntries int) (Store, error) {
	state, err := xdgRoot("XDG_STATE_HOME", ".local/state")
	if err != nil {
		return Store{}, err
	}
	cache, err := xdgRoot("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return Store{}, err
	}
	return Store{
		StateRoot: filepath.Join(state, "changes", "workspaces"),
		CacheRoot: filepath.Join(cache, "changes", "workspaces"), CacheMaxEntries: cacheMaxEntries,
	}, nil
}

func (store Store) LoadState(repository string) (State, bool, error) {
	if err := rejectSymlinkRoot(store.StateRoot); err != nil {
		return State{}, false, err
	}
	var state State
	found, err := readDocument(store.statePath(repository), maxStateBytes, &state)
	if err != nil || !found {
		return State{}, found, err
	}
	if state.Version != StateVersion || state.Repository != repository {
		return State{}, false, errors.New("workspace state identity does not match its repository")
	}
	if err := validateState(state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (store Store) SaveState(state State) error {
	state.Version = StateVersion
	if state.Repository == "" {
		return errors.New("workspace state repository is required")
	}
	if err := validateState(state); err != nil {
		return err
	}
	return writeDocument(store.statePath(state.Repository), state, maxStateBytes)
}

func validateState(state State) error {
	if state.Dock != "left" && state.Dock != "bottom" {
		return errors.New("workspace state dock is invalid")
	}
	if state.Navigator != "tree" && state.Navigator != "list" {
		return errors.New("workspace state navigator is invalid")
	}
	if state.Tab != "files" && state.Tab != "history" {
		return errors.New("workspace state tab is invalid")
	}
	if state.Layout != "unified" && state.Layout != "side-by-side" {
		return errors.New("workspace state layout is invalid")
	}
	if state.View != "working" && state.View != "staged" && state.View != "commit" {
		return errors.New("workspace state view is invalid")
	}
	if state.SelectedLine < 0 || len(state.Commands) > 20 {
		return errors.New("workspace state selection or command history is invalid")
	}
	for _, command := range state.Commands {
		if strings.IndexFunc(command, func(value rune) bool { return value < 0x20 && value != '\t' }) >= 0 || len([]rune(command)) > 4096 {
			return errors.New("workspace state command history is invalid")
		}
	}
	return nil
}

func (store Store) LoadSnapshot(repository, slot string) (Snapshot, bool, error) {
	if err := rejectSymlinkRoot(store.CacheRoot); err != nil {
		return Snapshot{}, false, err
	}
	var snapshot Snapshot
	found, err := readDocument(store.snapshotPath(repository, slot), maxSnapshotBytes, &snapshot)
	if err != nil || !found {
		return Snapshot{}, found, err
	}
	if snapshot.Version != SnapshotVersion || snapshot.Repository.Root != repository {
		return Snapshot{}, false, errors.New("workspace snapshot identity does not match its repository")
	}
	snapshot.Freshness.State = "cached"
	snapshot.Freshness.CacheHit = true
	return snapshot, true, nil
}

func (store Store) SaveSnapshot(slot string, snapshot Snapshot) error {
	if snapshot.Version != SnapshotVersion || snapshot.Repository.Root == "" {
		return errors.New("workspace snapshot identity is incomplete")
	}
	if err := writeDocument(store.snapshotPath(snapshot.Repository.Root, slot), snapshot, maxSnapshotBytes); err != nil {
		return err
	}
	return store.prune()
}

func (store Store) statePath(repository string) string {
	return filepath.Join(store.StateRoot, digest(repository)+".json")
}

func (store Store) snapshotPath(repository, slot string) string {
	return filepath.Join(store.CacheRoot, digest(repository+"\x00"+slot)+".json")
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func xdgRoot(name, fallback string) (string, error) {
	root := os.Getenv(name)
	if root == "" || !filepath.IsAbs(root) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, fallback)
	}
	return filepath.Clean(root), nil
}

func readDocument(path string, limit int64, target any) (bool, error) {
	pathInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("workspace document path is a symlink: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return false, fmt.Errorf("workspace document is not a bounded regular file: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return false, err
	}
	if int64(len(data)) > limit {
		return false, fmt.Errorf("workspace document exceeds %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false, fmt.Errorf("decode workspace document: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return false, errors.New("workspace document must contain one JSON value")
	}
	return true, nil
}

func rejectSymlinkRoot(root string) error {
	info, err := os.Lstat(filepath.Clean(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace directory is a symlink: %s", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace directory is not a directory: %s", root)
	}
	return nil
}

func writeDocument(path string, value any, limit int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("workspace document exceeds %d bytes", limit)
	}
	directory := filepath.Dir(path)
	if err := secureDirectory(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace document path is a symlink: %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".workspace-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func secureDirectory(directory string) error {
	directory = filepath.Clean(directory)
	missing := []string{}
	current := directory
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("workspace directory component is a symlink: %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("workspace directory component is not a directory: %s", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("no existing parent for workspace directory: %s", directory)
		}
		current = parent
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := os.Mkdir(missing[index], 0o700); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			info, statErr := os.Lstat(missing[index])
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("workspace directory changed while creating it: %s", missing[index])
			}
		}
	}
	return nil
}

func (store Store) prune() error {
	if store.CacheMaxEntries <= 0 {
		return nil
	}
	entries, err := os.ReadDir(store.CacheRoot)
	if err != nil {
		return err
	}
	type candidate struct {
		path string
		at   int64
	}
	files := []candidate{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err == nil {
				files = append(files, candidate{path: filepath.Join(store.CacheRoot, entry.Name()), at: info.ModTime().UnixNano()})
			}
		}
	}
	if len(files) <= store.CacheMaxEntries {
		return nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at < files[j].at })
	for _, file := range files[:len(files)-store.CacheMaxEntries] {
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
