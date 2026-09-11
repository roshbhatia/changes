package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/workspaceview"
)

func TestPatchReaderScopesAndLiteralArguments(t *testing.T) {
	root := t.TempDir()
	prepareRepository(t, root, map[string]string{"dir/a file.txt": "before\n", "other/a file.txt": "sibling\n"})
	if err := os.WriteFile(filepath.Join(root, "dir/a file.txt"), []byte("after  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other/a file.txt"), []byte("changed sibling\n"), 0600); err != nil {
		t.Fatal(err)
	}
	argv := []string{"sh", "-c", `printf '%s\n' "$PWD" "$1"; cat`, "reader", "literal; not a command"}
	for _, view := range []string{"working", "staged", "commit"} {
		if view != "working" {
			command := exec.Command("git", "add", ".")
			command.Dir = root
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("stage: %v %s", err, output)
			}
		}
		if view == "commit" {
			command := exec.Command("git", "commit", "-qm", "update")
			command.Dir = root
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("commit: %v %s", err, output)
			}
		}
		command, err := patchReader(root, workspaceOptions{view: view, commit: "HEAD"}, []string{":(literal)dir/"}, argv)
		if err != nil || command == nil {
			spec, _ := workspaceSource(root, workspaceOptions{view: view, commit: "HEAD"})
			raw, _ := spec.Diff()
			t.Fatalf("%s: %v patch=%q sections=%#v", view, err, raw, splitPatchFiles(raw))
		}
		output, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		text := string(output)
		if !strings.Contains(text, "literal; not a command") || !strings.Contains(text, "+after  \n") || strings.Contains(text, "changed sibling") || command.Dir != root {
			t.Fatalf("invalid scoped reader: %s", text)
		}
	}
	command, err := patchReader(root, workspaceOptions{view: "commit", commit: "HEAD~1"}, nil, []string{"cat"})
	if err != nil || command == nil {
		t.Fatalf("root commit: %v", err)
	}
	patch, _ := io.ReadAll(command.Stdin)
	if !strings.Contains(string(patch), "new file mode") {
		t.Fatalf("root commit patch: %s", patch)
	}
	command, err = patchReader(root, workspaceOptions{view: "working"}, nil, []string{"cat"})
	if err != nil || command != nil {
		t.Fatalf("empty comparison: %v %v", command, err)
	}
}

func TestScopePatchPreservesRenameDeletionAndBinary(t *testing.T) {
	root := t.TempDir()
	prepareRepository(t, root, map[string]string{"old.txt": "unchanged\n", "delete.txt": "remove\n", "image blob.bin": "\x00before", "sibling.txt": "same\n"})
	for _, args := range [][]string{{"mv", "old.txt", "new name.txt"}, {"rm", "delete.txt"}} {
		command := exec.Command("git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "image blob.bin"), []byte("\x00after"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "add", ".")
	command.Dir = root
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	for file, expected := range map[string]string{"new name.txt": "rename from old.txt", "delete.txt": "deleted file mode", "image blob.bin": "Binary files"} {
		reader, err := patchReader(root, workspaceOptions{view: "staged"}, []string{":(literal)" + file}, []string{"cat"})
		if err != nil || reader == nil {
			spec, _ := workspaceSource(root, workspaceOptions{view: "staged"})
			raw, _ := spec.Diff()
			t.Fatalf("%s: %v patch=%q sections=%#v", file, err, raw, splitPatchFiles(raw))
		}
		patch, _ := io.ReadAll(reader.Stdin)
		if !strings.Contains(string(patch), expected) || len(splitPatchFiles(string(patch))) != 1 {
			t.Fatalf("%s: %s", file, patch)
		}
	}
}

func TestReaderRefreshAndDirectoryState(t *testing.T) {
	model := newInteractiveModel(t.TempDir(), workspaceOptions{view: "working", layout: "unified"}, appconfig.Default(), workspaceview.Store{})
	model.focus = "navigator"
	model.setSnapshot(workspaceview.Snapshot{Comparison: workspaceview.Comparison{Kind: "working"}, Files: []workspaceview.File{{Path: "dir/a.txt"}, {Path: "other/a.txt"}}, Rendered: strings.Repeat("line\n", 80)})
	for index, item := range model.navigatorItems() {
		if item.path == "dir/a.txt" {
			model.selected = index
		}
	}
	model.setSnapshot(workspaceview.Snapshot{Files: []workspaceview.File{{Path: "before.txt"}, {Path: "dir/a.txt"}, {Path: "other/a.txt"}}, Rendered: strings.Repeat("line\n", 80)})
	if model.selectedFile() != "dir/a.txt" {
		t.Fatalf("selection changed: %q", model.selectedFile())
	}
	model.setNavigator("list")
	if model.selectedFile() != "dir/a.txt" {
		t.Fatal("tree to list lost selection")
	}
	model.setNavigator("tree")
	before := model.viewport.YOffset
	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.viewport.YOffset <= before {
		t.Fatal("reader scroll changed focus or did not scroll")
	}
	model.collapsed["dir/"] = true
	for _, item := range model.navigatorItems() {
		if item.path == "dir/a.txt" {
			t.Fatal("collapsed child remains visible")
		}
	}
	model.previewActive, model.previewPath, model.previewDirectory = true, "dir/a.txt", false
	model.focus = "main"
	if model.selectedFile() != "dir/a.txt" {
		t.Fatal("note target differs from displayed file")
	}
	model.previewDirectory = true
	if model.selectedFile() != "" {
		t.Fatal("directory preview accepted a file note")
	}
	model.cycleView()
	if model.previewActive {
		t.Fatal("comparison retained old scope")
	}
	model.refreshEpoch.Store(2)
	updated, _ = model.Update(workspaceLoaded{epoch: 1, snapshot: workspaceview.Snapshot{Rendered: "obsolete"}})
	model = updated.(interactiveModel)
	if model.snapshot.Rendered == "obsolete" {
		t.Fatal("obsolete refresh applied")
	}
	updated, _ = model.Update(workspaceLoaded{epoch: 2, err: errors.New("Git unavailable")})
	model = updated.(interactiveModel)
	if model.snapshot.Freshness.State != "stale" || !strings.Contains(model.message, "Git unavailable") {
		t.Fatal("refresh failure not visible")
	}
	offset, selected, focus := model.viewport.YOffset, model.selected, model.focus
	updated, _ = model.Update(readerFinished{err: errors.New("reader exited")})
	model = updated.(interactiveModel)
	if model.viewport.YOffset != offset || model.selected != selected || model.focus != focus || !strings.Contains(model.message, "reader exited") {
		t.Fatal("reader return lost state or error")
	}
}

func TestScopePreviewFailureRetainsDocument(t *testing.T) {
	model := newInteractiveModel(t.TempDir(), workspaceOptions{}, appconfig.Default(), workspaceview.Store{})
	model.previewActive, model.previewPath = true, "main.go"
	model.viewport.SetContent("previous document")
	before := model.viewport.View()
	updated, _ := model.Update(scopePreviewLoaded{path: "main.go", err: errors.New("render failed")})
	model = updated.(interactiveModel)
	if model.viewport.View() != before || model.snapshot.Freshness.State != "stale" || model.message != "render failed" {
		t.Fatal("failed preview discarded the document or hid the error")
	}
	model.previewEpoch.Store(2)
	updated, _ = model.Update(scopePreviewLoaded{path: "main.go", previewEpoch: 1, rendered: "obsolete"})
	model = updated.(interactiveModel)
	if model.viewport.View() != before {
		t.Fatal("obsolete preview replaced the document")
	}
	updated, _ = model.Update(scopePreviewLoaded{path: "main.go", previewEpoch: 2, rendered: "new document", freshness: workspaceview.Freshness{State: "stale"}})
	model = updated.(interactiveModel)
	if model.snapshot.Freshness.State != "stale" {
		t.Fatal("preview lost provider freshness")
	}
}

func TestScopedPreviewValidatesNotesAgainstFullComparison(t *testing.T) {
	root := t.TempDir()
	prepareRepository(t, root, map[string]string{"main.go": "one\ntwo\n", "other.go": "old\n"})
	for file, content := range map[string]string{"main.go": "one\nchanged\n", "other.go": "new\n"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	configured := appconfig.Default()
	configured.Providers.Directory = t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf("version: provider/v1\nname: fake\ncommand: [%q, %q, %q, %q]\nactions:\n  changes.notes: {}\n", os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(configured.Providers.Directory, "fake.yaml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	options := workspaceOptions{view: "working", layout: "unified", noSymbols: true, noCalls: true, noGroups: true}
	full, err := buildWorkspaceSnapshot(root, options, configured)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := buildWorkspaceScopedSnapshot(root, options, configured, nil, []string{":(literal)main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if scoped.Freshness.State != "fresh" || len(scoped.Failures) != 0 {
		t.Fatalf("scoped notes marked stale: %#v", scoped.Failures)
	}
	if scoped.Comparison.Fingerprint != full.Comparison.Fingerprint {
		t.Fatal("scope changed comparison identity")
	}
	if len(scoped.Files) != 1 || scoped.Files[0].Path != "main.go" || strings.Contains(scoped.Rendered, "other.go") {
		t.Fatalf("scope leaked sibling: %s", scoped.Rendered)
	}
}
