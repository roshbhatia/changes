package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/roshbhatia/changes/internal/workspaceview"
)

func TestInteractiveRepositoryOptions(t *testing.T) {
	for _, flag := range []string{"--recursive", "-r"} {
		options, _, err := parseWorkspaceOptions([]string{flag, "--root", "../workspace", "--staged"}, true)
		if err != nil || !options.recursive || options.root != "../workspace" || options.view != "staged" {
			t.Fatalf("options = %#v, error = %v", options, err)
		}
	}
	if _, _, err := parseWorkspaceOptions([]string{"--staged", "--view", "commit"}, true); err == nil {
		t.Fatal("conflicting comparison flags accepted")
	}
}

func TestInteractiveRepositoryDiscovery(t *testing.T) {
	boundary := t.TempDir()
	first, second := filepath.Join(boundary, "a"), filepath.Join(boundary, "b")
	nested := filepath.Join(first, "nested")
	for _, root := range []string{first, second, nested} {
		prepareRepository(t, root, map[string]string{"file.txt": "before\n"})
	}
	linked := filepath.Join(boundary, "linked")
	gitWorkspaceTest(t, first, "worktree", "add", "--detach", linked, "HEAD")
	t.Setenv("SYSINIT_WORKSPACE", boundary)
	roots, resolved, err := interactiveRepositories(workspaceOptions{root: first, recursive: true})
	if err != nil || resolved != boundary || !reflect.DeepEqual(roots, []string{first, nested, second, linked}) {
		t.Fatalf("roots = %v, boundary = %q, error = %v", roots, resolved, err)
	}
	canonicalSecond, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	roots, _, err = interactiveRepositories(workspaceOptions{root: second})
	if err != nil || len(roots) != 1 || roots[0] != canonicalSecond {
		t.Fatalf("single repository = %v, error = %v", roots, err)
	}
	if _, _, err := interactiveRepositories(workspaceOptions{root: t.TempDir(), recursive: true}); err == nil || !strings.Contains(err.Error(), "no git repository") {
		t.Fatalf("empty workspace error = %v", err)
	}
}

func TestInteractiveRepositorySwitchIsolatesComparison(t *testing.T) {
	boundary := t.TempDir()
	first, second := filepath.Join(boundary, "a"), filepath.Join(boundary, "b")
	for _, root := range []string{first, second} {
		prepareRepository(t, root, map[string]string{"file.txt": "before\n"})
		if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte(filepath.Base(root)+" change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	options, configured, err := parseWorkspaceOptions([]string{"-r", "--no-notes", "--no-symbols", "--no-calls", "--no-groups"}, true)
	if err != nil {
		t.Fatal(err)
	}
	configured.Providers.Directory = t.TempDir()
	store := workspaceview.Store{StateRoot: t.TempDir(), CacheRoot: t.TempDir()}
	model := newInteractiveModel(first, options, configured, store)
	model.repositories, model.repositoryBoundary = []string{first, second}, boundary
	model.width, model.height = 120, 30
	model.previewActive, model.previewPath = true, "file.txt"
	model.markedOIDs = []string{"old-commit"}
	model.options.commit = "old-commit"
	oldLoad := model.refreshCommand()
	updated, _ := model.runPaletteCommand("tab repositories")
	model = updated.(interactiveModel)
	if model.activeTab() != "repositories" || model.focus != "navigator" {
		t.Fatal("repository command did not focus the repository list")
	}
	model.selected = 1
	if items := model.navigatorItems(); len(items) != 2 || items[1].label != "b" {
		t.Fatalf("repository items = %#v", items)
	}
	updated, command := model.activateSelection()
	next := updated.(interactiveModel)
	if next.root != second || next.previewActive || len(next.markedOIDs) != 0 || next.options.commit != "HEAD" || len(next.snapshot.Files) != 0 {
		t.Fatalf("repository switch leaked state: %#v", next)
	}
	loaded := interactiveCommandMessage[workspaceLoaded](t, command)
	if loaded.err != nil || !strings.Contains(loaded.snapshot.Rendered, "b change") || strings.Contains(loaded.snapshot.Rendered, "a change") {
		t.Fatalf("second repository load: %v\n%s", loaded.err, loaded.snapshot.Rendered)
	}
	updated, _ = next.Update(loaded)
	next = updated.(interactiveModel)
	updated, _ = next.Update(oldLoad())
	next = updated.(interactiveModel)
	if next.snapshot.Repository.Root != second {
		t.Fatalf("old refresh replaced selected repository: %q", next.snapshot.Repository.Root)
	}
	next.tab = "repositories"
	if err := next.persistState(); err != nil {
		t.Fatalf("persist repository tab: %v", err)
	}
	if _, found, err := store.LoadState(first); err != nil || !found {
		t.Fatalf("previous repository state missing: %v", err)
	}
	next.tab = "files"
	next.moveFocusedTab(-1)
	if next.activeTab() != "repositories" {
		t.Fatalf("reverse tab cycle = %q", next.activeTab())
	}
}
