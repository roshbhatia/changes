package workspaceview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/roshbhatia/changes/internal/provider"
)

func testSnapshot(repository, body string) Snapshot {
	return Snapshot{
		Version:    SnapshotVersion,
		Repository: Repository{Root: repository, Name: filepath.Base(repository)},
		Comparison: Comparison{Kind: "working", Fingerprint: "fingerprint", Layout: "unified"},
		Freshness:  Freshness{State: "fresh", GeneratedAt: "2026-09-07T00:00:00Z"},
		History:    []HistoryEntry{}, Groups: []Group{}, Files: []File{}, Notes: []provider.Note{},
		Threads: []NoteThread{}, Failures: []Failure{}, Rendered: body,
	}
}

func TestStoreSeparatesStateAndReplaceableSnapshots(t *testing.T) {
	store := Store{StateRoot: filepath.Join(t.TempDir(), "state"), CacheRoot: filepath.Join(t.TempDir(), "cache"), CacheMaxEntries: 2}
	repository := "/work/repository"
	state := State{Repository: repository, Dock: "left", Navigator: "tree", Tab: "files", Layout: "unified", View: "working"}
	if err := store.SaveState(state); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSnapshot("working", testSnapshot(repository, "first")); err != nil {
		t.Fatal(err)
	}
	loadedState, found, err := store.LoadState(repository)
	if err != nil || !found || loadedState.Dock != "left" || loadedState.Version != StateVersion {
		t.Fatalf("state = %#v, found = %v, error = %v", loadedState, found, err)
	}
	snapshot, found, err := store.LoadSnapshot(repository, "working")
	if err != nil || !found || snapshot.Rendered != "first" || snapshot.Freshness.State != "cached" || !snapshot.Freshness.CacheHit {
		t.Fatalf("snapshot = %#v, found = %v, error = %v", snapshot, found, err)
	}
	if filepath.Dir(store.statePath(repository)) == filepath.Dir(store.snapshotPath(repository, "working")) {
		t.Fatal("state and snapshot share a directory")
	}
}

func TestStoreRejectsSymlinkedDirectoriesAndDocuments(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	cache := filepath.Join(root, "cache")
	if err := os.Symlink(external, cache); err != nil {
		t.Fatal(err)
	}
	store := Store{StateRoot: filepath.Join(root, "state"), CacheRoot: cache, CacheMaxEntries: 2}
	if err := store.SaveSnapshot("working", testSnapshot("/repo", "body")); err == nil {
		t.Fatal("symlinked cache directory was accepted")
	}
	externalDocument := filepath.Join(external, digest("/repo\x00working")+".json")
	data, err := json.Marshal(testSnapshot("/repo", "external"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalDocument, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSnapshot("/repo", "working"); err == nil {
		t.Fatal("symlinked cache directory was read")
	}

	store.CacheRoot = filepath.Join(root, "regular-cache")
	if err := secureDirectory(store.CacheRoot); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(external, "snapshot.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := store.snapshotPath("/repo", "working")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSnapshot("/repo", "working"); err == nil {
		t.Fatal("symlinked snapshot document was accepted")
	}
}

func TestStoreRejectsInvalidState(t *testing.T) {
	store := Store{StateRoot: filepath.Join(t.TempDir(), "state"), CacheRoot: filepath.Join(t.TempDir(), "cache")}
	state := State{Repository: "/repo", Dock: "left", Navigator: "tree", Tab: "files", Layout: "unified", View: "unknown"}
	if err := store.SaveState(state); err == nil {
		t.Fatal("invalid state was accepted")
	}
}

func TestStorePrunesOldestSnapshots(t *testing.T) {
	store := Store{StateRoot: filepath.Join(t.TempDir(), "state"), CacheRoot: filepath.Join(t.TempDir(), "cache"), CacheMaxEntries: 2}
	for _, slot := range []string{"one", "two", "three"} {
		if err := store.SaveSnapshot(slot, testSnapshot("/repo", slot)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(store.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("cache entries = %d, want 2", len(entries))
	}
}
