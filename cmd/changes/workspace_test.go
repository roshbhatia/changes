package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/changes/internal/workspaceview"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

func TestWorkspaceSnapshotSharesRenderedDiffAndStructuredLines(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "package main\n\nfunc value() int { return 1 }\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("package main\n\nfunc value() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := appconfig.Default()
	configured.Providers.Directory = t.TempDir()
	options := workspaceOptions{view: "working", commit: "HEAD", layout: "unified", width: 100, historyLimit: 10, noCalls: true, noGroups: true, noNotes: true, noSymbols: true}
	snapshot, err := buildWorkspaceSnapshot(repository, options, configured)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != workspaceview.SnapshotVersion || snapshot.Comparison.Kind != "working" || snapshot.Comparison.Fingerprint == "" {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if len(snapshot.Files) != 1 || len(snapshot.Files[0].Hunks) != 1 {
		t.Fatalf("structured files = %#v", snapshot.Files)
	}
	if !strings.Contains(snapshot.Rendered, "return 2") {
		t.Fatalf("rendered diff = %q", snapshot.Rendered)
	}
	foundAdded := false
	for _, line := range snapshot.Files[0].Hunks[0].Lines {
		foundAdded = foundAdded || line.Kind == "added" && line.NewLine == 3 && strings.Contains(line.Text, "return 2")
	}
	if !foundAdded {
		t.Fatalf("structured lines = %#v", snapshot.Files[0].Hunks[0].Lines)
	}
	data, err := json.Marshal(snapshot)
	if err != nil || !json.Valid(data) {
		t.Fatalf("snapshot JSON: %v %s", err, data)
	}
}

func TestWorkspaceViewsDoNotMutateRepository(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "before\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitWorkspaceTest(t, repository, "add", "main.go")
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := gitWorkspaceOutput(t, repository, "status", "--porcelain=v1")
	configured := appconfig.Default()
	configured.Providers.Directory = t.TempDir()
	for _, view := range []string{"working", "staged", "commit"} {
		options := workspaceOptions{view: view, commit: "HEAD", layout: "unified", width: 80, historyLimit: 5, noCalls: true, noGroups: true, noNotes: true, noSymbols: true}
		snapshot, err := buildWorkspaceSnapshot(repository, options, configured)
		if err != nil {
			t.Fatalf("%s: %v", view, err)
		}
		if snapshot.Comparison.Kind != view {
			t.Fatalf("%s comparison = %#v", view, snapshot.Comparison)
		}
	}
	after := gitWorkspaceOutput(t, repository, "status", "--porcelain=v1")
	if before != after {
		t.Fatalf("workspace mutated repository:\nbefore %q\nafter  %q", before, after)
	}
}

func TestWorkspaceCommandKeepsJSONOnStandardOutput(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "before\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := workspaceHelperCommand(t, repository, "workspace", "--no-notes", "--no-symbols", "--no-calls", "--no-groups", "--quiet")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("workspace command: %v\n%s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("workspace stderr = %q", stderr.String())
	}
	var snapshot workspaceview.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		t.Fatalf("workspace stdout is not one JSON snapshot: %v\n%s", err, stdout.String())
	}
	if snapshot.Version != workspaceview.SnapshotVersion || len(snapshot.Files) != 1 {
		t.Fatalf("workspace snapshot = %#v", snapshot)
	}
}

func TestWorkspaceRefreshWatchStartsWithVersionedEvents(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "before\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := workspaceHelperCommand(t, repository, "workspace", "--watch", "--refresh", "--interval", "1s", "--no-notes", "--no-symbols", "--no-calls", "--no-groups", "--quiet")
	command = exec.CommandContext(ctx, command.Path, command.Args[1:]...)
	command.Dir = repository
	command.Env = workspaceHelperEnvironment(t)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	for index, expected := range []string{"refreshing", "snapshot"} {
		if !scanner.Scan() {
			t.Fatalf("event %d missing: %v", index, scanner.Err())
		}
		var event workspaceview.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("event %d: %v\n%s", index, err, scanner.Text())
		}
		if event.Version != workspaceview.EventVersion || event.Type != expected {
			t.Fatalf("event %d = %#v", index, event)
		}
	}
	cancel()
	_ = command.Wait()
}

func TestInteractiveModelTogglesDockLayoutAndHistory(t *testing.T) {
	configured := appconfig.Default()
	store := workspaceview.Store{StateRoot: filepath.Join(t.TempDir(), "state"), CacheRoot: filepath.Join(t.TempDir(), "cache"), CacheMaxEntries: 2}
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, configured, store)
	model.setSnapshot(workspaceview.Snapshot{
		Version: workspaceview.SnapshotVersion, Repository: workspaceview.Repository{Root: "/repo", Name: "repo"},
		Comparison: workspaceview.Comparison{Kind: "working", Layout: "unified"}, Freshness: workspaceview.Freshness{State: "fresh"},
		Files:   []workspaceview.File{{Path: "internal/main.go", Added: 1, Hunks: []workspaceview.Hunk{}}},
		History: []workspaceview.HistoryEntry{{OID: strings.Repeat("a", 40), Summary: "first", Author: "test", AuthoredAt: "2026-09-07T00:00:00Z"}},
		Groups:  []workspaceview.Group{}, Notes: nil, Threads: []workspaceview.NoteThread{}, Failures: []workspaceview.Failure{}, Rendered: "rendered",
	})
	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(interactiveModel)
	if model.configured.Interactive.Dock != "bottom" {
		t.Fatalf("dock = %q", model.configured.Interactive.Dock)
	}
	model.focus = "navigator"
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(interactiveModel)
	if model.activeTab() != "history" || len(model.navigatorItems()) != 1 {
		t.Fatalf("history navigator = %#v", model.navigatorItems())
	}
}

func TestInteractiveSpatialFocusAndPaneLocalTabs(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.width, model.height = 100, 30
	model.resize()

	press := func(key tea.KeyType) {
		updated, _ := model.handleKey(tea.KeyMsg{Type: key})
		model = updated.(interactiveModel)
	}
	model.focus = "main"
	press(tea.KeyTab)
	if model.focus != "main" || model.activeTab() != "files" {
		t.Fatalf("Tab in Changes changed state: focus=%q tab=%q", model.focus, model.activeTab())
	}
	press(tea.KeyCtrlJ)
	if model.focus != "main" {
		t.Fatalf("Ctrl-j crossed a left dock: focus=%q", model.focus)
	}
	press(tea.KeyCtrlH)
	if model.focus != "navigator" {
		t.Fatalf("Ctrl-h did not move left: focus=%q", model.focus)
	}
	press(tea.KeyTab)
	if model.focus != "navigator" || model.activeTab() != "history" {
		t.Fatalf("Tab did not stay in Explorer: focus=%q tab=%q", model.focus, model.activeTab())
	}
	press(tea.KeyShiftTab)
	if model.focus != "navigator" || model.activeTab() != "files" {
		t.Fatalf("Shift-Tab did not stay in Explorer: focus=%q tab=%q", model.focus, model.activeTab())
	}
	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(interactiveModel)
	if model.activeTab() != "files" {
		t.Fatalf("f remained an alternate tab binding: tab=%q", model.activeTab())
	}
	press(tea.KeyCtrlL)
	if model.focus != "main" {
		t.Fatalf("Ctrl-l did not move right: focus=%q", model.focus)
	}

	model.configured.Interactive.Dock = "bottom"
	model.focus = "main"
	press(tea.KeyCtrlH)
	if model.focus != "main" {
		t.Fatalf("Ctrl-h crossed a bottom dock: focus=%q", model.focus)
	}
	press(tea.KeyCtrlJ)
	if model.focus != "navigator" {
		t.Fatalf("Ctrl-j did not move down: focus=%q", model.focus)
	}
	press(tea.KeyCtrlL)
	if model.focus != "navigator" {
		t.Fatalf("Ctrl-l crossed a bottom dock: focus=%q", model.focus)
	}
	press(tea.KeyCtrlK)
	if model.focus != "main" {
		t.Fatalf("Ctrl-k did not move up: focus=%q", model.focus)
	}
}

func TestInteractiveCleanWorkspaceDrawsTwoFullPanes(t *testing.T) {
	configured := appconfig.Default()
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", color: "always", historyLimit: 5}, configured, workspaceview.Store{})
	model.width, model.height, model.loading = 100, 30, false
	model.resize()
	model.setSnapshot(workspaceview.Snapshot{
		Version:    workspaceview.SnapshotVersion,
		Repository: workspaceview.Repository{Root: "/repo", Name: "repo", Branch: "main"},
		Comparison: workspaceview.Comparison{Kind: "working", Layout: "unified"},
		Freshness:  workspaceview.Freshness{State: "fresh"},
		Files:      []workspaceview.File{},
		History:    []workspaceview.HistoryEntry{},
		Groups:     []workspaceview.Group{},
		Notes:      []provider.Note{},
		Threads:    []workspaceview.NoteThread{},
		Failures:   []workspaceview.Failure{},
	})

	frame := model.View()
	plain := ansi.Strip(frame)
	for _, expected := range []string{"explorer", "changes", "No changed files", "Working tree clean", "ctrl+h/j/k/l pane"} {
		if !strings.Contains(plain, expected) {
			t.Errorf("frame omitted %q:\n%s", expected, plain)
		}
	}
	lines := strings.Split(frame, "\n")
	if len(lines) != model.height {
		t.Fatalf("frame has %d lines, want %d", len(lines), model.height)
	}
	for index, line := range lines {
		if width := ansi.StringWidth(line); width != model.width {
			t.Fatalf("line %d has width %d, want %d: %q", index, width, model.width, ansi.Strip(line))
		}
	}
}

func TestInteractiveResizeFitsLongNavigatorRows(t *testing.T) {
	configured := appconfig.Default()
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", color: "always", historyLimit: 5}, configured, workspaceview.Store{})
	model.width, model.height, model.loading = 72, 16, false
	model.setSnapshot(workspaceview.Snapshot{
		Version:    workspaceview.SnapshotVersion,
		Repository: workspaceview.Repository{Root: "/repo", Name: "repo", Branch: "main"},
		Comparison: workspaceview.Comparison{Kind: "working", Layout: "unified"},
		Freshness:  workspaceview.Freshness{State: "fresh"},
		Files:      []workspaceview.File{{Path: "one/two/three/four/five/a-very-long-file-name.go", Added: 2, Deleted: 1}},
		History:    []workspaceview.HistoryEntry{}, Groups: []workspaceview.Group{}, Notes: []provider.Note{}, Threads: []workspaceview.NoteThread{}, Failures: []workspaceview.Failure{},
		Rendered: "a bounded diff",
	})
	model.resize()

	for index, line := range strings.Split(model.View(), "\n") {
		if width := ansi.StringWidth(line); width != model.width {
			t.Fatalf("line %d has width %d, want %d: %q", index, width, model.width, ansi.Strip(line))
		}
	}
	if model.viewport.Width != model.diffWidth() || model.viewport.Height != model.diffHeight() {
		t.Fatalf("viewport = %dx%d, want %dx%d", model.viewport.Width, model.viewport.Height, model.diffWidth(), model.diffHeight())
	}
}

func TestInteractiveKeysIgnoreTerminalRepliesAndCycleOneView(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.focus = "main"
	for _, reply := range []string{"11;rgb:2424/2727/3a3a", ">|WezTerm 20260907;OK"} {
		updated, command := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(reply)})
		unchanged := updated.(interactiveModel)
		if command != nil || unchanged.focus != "main" || unchanged.options.view != "working" {
			t.Fatalf("terminal reply %q changed model: %#v", reply, unchanged)
		}
	}
	updated, command := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	cycled := updated.(interactiveModel)
	if command == nil || cycled.options.view != "staged" {
		t.Fatalf("v cycled to %q with command %#v", cycled.options.view, command)
	}
}

func TestInteractiveKeyCatalogPreservesCaseAndGeneratesHelp(t *testing.T) {
	binding, matched, err := interactiveKeyCatalog.Match("g", nil)
	if err != nil || !matched || binding.ID != "generate" {
		t.Fatalf("g match = %+v, %t, %v", binding, matched, err)
	}
	if binding, matched, err = interactiveKeyCatalog.Match("G", nil); err != nil || matched {
		t.Fatalf("G match = %+v, %t, %v", binding, matched, err)
	}
	rows, err := interactiveKeyCatalog.HelpRows(nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		found = found || row.ID == "focus" && row.Keys == "ctrl+h/j/k/l"
	}
	if !found || interactiveBindingHint("focus") != "ctrl+h/j/k/l pane" {
		t.Fatalf("generated key help omitted focus: %+v", rows)
	}
}

func TestInteractiveMouseFocusesPanes(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.width, model.height = 100, 30
	model.resize()
	model.setSnapshot(workspaceview.Snapshot{
		Version: workspaceview.SnapshotVersion,
		Files: []workspaceview.File{
			{Path: "first.go"},
			{Path: "second.go"},
		},
		History: []workspaceview.HistoryEntry{
			{OID: strings.Repeat("a", 40), Summary: "first"},
			{OID: strings.Repeat("b", 40), Summary: "second"},
		},
		Rendered: strings.Repeat("line\n", 80),
	})
	updated, _ := model.handleMouse(tea.MouseMsg{X: 2, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.selected != 0 {
		t.Fatalf("first row click = focus %q, selection %d", model.focus, model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 2, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.selected != 1 {
		t.Fatalf("second row click selected %d", model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 9, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.activeTab() != "history" || model.selected != 0 {
		t.Fatalf("History tab click = focus %q, tab %q, selection %d", model.focus, model.activeTab(), model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 2, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.selected != 1 {
		t.Fatalf("Explorer wheel = focus %q, selection %d", model.focus, model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 2, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.activeTab() != "files" || model.selected != 0 {
		t.Fatalf("Files tab click = focus %q, tab %q, selection %d", model.focus, model.activeTab(), model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 80, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.focus != "main" {
		t.Fatalf("main click focus = %q", model.focus)
	}
	before := model.viewport.YOffset
	updated, _ = model.handleMouse(tea.MouseMsg{X: 80, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	model = updated.(interactiveModel)
	if model.focus != "main" || model.viewport.YOffset <= before {
		t.Fatalf("Changes wheel = focus %q, offset %d after %d", model.focus, model.viewport.YOffset, before)
	}

	model.showHelp = true
	model.height = 10
	model.helpOffset = 0
	updated, _ = model.handleMouse(tea.MouseMsg{X: 50, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	model = updated.(interactiveModel)
	if model.helpOffset != 1 {
		t.Fatalf("help wheel offset = %d", model.helpOffset)
	}
}

func TestInteractiveMouseUsesBottomDockRegions(t *testing.T) {
	configured := appconfig.Default()
	configured.Interactive.Dock = "bottom"
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, configured, workspaceview.Store{})
	model.width, model.height = 100, 30
	model.resize()
	model.setSnapshot(workspaceview.Snapshot{
		Version:  workspaceview.SnapshotVersion,
		Files:    []workspaceview.File{{Path: "main.go"}},
		History:  []workspaceview.HistoryEntry{{OID: strings.Repeat("a", 40), Summary: "first"}},
		Rendered: "diff",
	})
	navigator := model.navigatorRegion()
	updated, _ := model.handleMouse(tea.MouseMsg{
		X: navigator.X + 9, Y: navigator.Y + 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	model = updated.(interactiveModel)
	if model.focus != "navigator" || model.activeTab() != "history" {
		t.Fatalf("bottom History click = focus %q, tab %q", model.focus, model.activeTab())
	}
	updated, _ = model.handleMouse(tea.MouseMsg{
		X: navigator.X + 2, Y: navigator.Y + 2,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	model = updated.(interactiveModel)
	if model.selected != 0 {
		t.Fatalf("bottom row click selected %d", model.selected)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 50, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if model.focus != "main" {
		t.Fatalf("bottom Changes click focus = %q", model.focus)
	}
}

func TestInteractiveHistoryMarksCanonicalCommitsWithoutChangingGitState(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "before\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))
	gitWorkspaceTest(t, repository, "add", "main.go")
	gitWorkspaceTest(t, repository, "commit", "--quiet", "-m", "second")
	second := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeHead := gitWorkspaceOutput(t, repository, "rev-parse", "HEAD")
	beforeStatus := gitWorkspaceOutput(t, repository, "status", "--porcelain=v1")

	model := newInteractiveModel(repository, workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.width, model.height = 100, 30
	model.focus, model.tab = "navigator", "history"
	model.setSnapshot(workspaceview.Snapshot{
		Version: workspaceview.SnapshotVersion,
		History: []workspaceview.HistoryEntry{
			{OID: second, Summary: "second"},
			{OID: first, Summary: "first"},
		},
	})
	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(interactiveModel)
	if model.selected != 0 {
		t.Fatalf("mark moved cursor to %d", model.selected)
	}
	model.moveSelection(1)
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(interactiveModel)
	if model.selected != 1 || !slices.Equal(model.selectedCommitsForGeneration(), []string{second, first}) {
		t.Fatalf("marked commits = %#v at cursor %d", model.selectedCommitsForGeneration(), model.selected)
	}
	plain := ansi.Strip(model.navigatorView())
	if strings.Count(plain, "[x]") != 2 || !strings.Contains(plain, "› [x]") {
		t.Fatalf("History did not distinguish marks from cursor:\n%s", plain)
	}
	updated, _ = model.handleMouse(tea.MouseMsg{X: 3, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	model = updated.(interactiveModel)
	if !slices.Equal(model.selectedCommitsForGeneration(), []string{first}) {
		t.Fatalf("mouse marker toggle = %#v", model.selectedCommitsForGeneration())
	}
	model.markedOIDs = nil
	if selected := model.selectedCommitsForGeneration(); !slices.Equal(selected, []string{second}) {
		t.Fatalf("cursor fallback = %#v", selected)
	}
	if after := gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"); after != beforeHead {
		t.Fatalf("marking changed HEAD from %q to %q", beforeHead, after)
	}
	if after := gitWorkspaceOutput(t, repository, "status", "--porcelain=v1"); after != beforeStatus {
		t.Fatalf("marking changed Git state from %q to %q", beforeStatus, after)
	}
}

func TestInteractiveHistoryMarkSelectionIsBounded(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 100}, appconfig.Default(), workspaceview.Store{})
	model.focus, model.tab = "navigator", "history"
	for index := range maxInteractiveCommitSelection + 1 {
		model.snapshot.History = append(model.snapshot.History, workspaceview.HistoryEntry{
			OID: fmt.Sprintf("%040d", index), Summary: fmt.Sprintf("commit %d", index),
		})
	}
	for index := range model.snapshot.History {
		model.toggleMarkedCommit(index)
	}
	if len(model.markedOIDs) != maxInteractiveCommitSelection || !strings.Contains(model.message, "select up to") {
		t.Fatalf("bounded marks = %d, message %q", len(model.markedOIDs), model.message)
	}
}

func TestInteractiveGenerationUsesMarksThenCursor(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "first\n"})
	first := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitWorkspaceTest(t, repository, "add", "main.go")
	gitWorkspaceTest(t, repository, "commit", "--quiet", "-m", "second")
	second := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))

	previousDiscover := interactiveDiscoverProviders
	t.Cleanup(func() { interactiveDiscoverProviders = previousDiscover })
	interactiveDiscoverProviders = func(string) (provider.Discovery, error) {
		return provider.Discovery{Providers: []provider.LoadedManifest{{Manifest: provider.Manifest{
			Name: "generator", Actions: map[string]providerlib.Action{provider.ActionNotesGenerate: {}},
		}}}}, nil
	}
	newModel := func() interactiveModel {
		model := newInteractiveModel(repository, workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
		model.focus, model.tab = "navigator", "history"
		model.snapshot.History = []workspaceview.HistoryEntry{{OID: second, Summary: "second"}, {OID: first, Summary: "first"}}
		return model
	}

	marked := newModel()
	marked.selected = 1
	marked.toggleMarkedCommit(1)
	marked.selected = 0
	updated, command := marked.startDraftGeneration()
	marked = updated.(interactiveModel)
	prepared := interactiveCommandMessage[interactiveGenerationPrepared](t, command)
	updated, next := marked.Update(prepared)
	marked = updated.(interactiveModel)
	if next == nil || len(marked.generationSpecs) != 1 || marked.generationSpecs[0].commit != first {
		t.Fatalf("marked generation = %#v, next %#v", marked.generationSpecs, next)
	}

	cursor := newModel()
	cursor.selected = 0
	updated, command = cursor.startDraftGeneration()
	cursor = updated.(interactiveModel)
	prepared = interactiveCommandMessage[interactiveGenerationPrepared](t, command)
	updated, next = cursor.Update(prepared)
	cursor = updated.(interactiveModel)
	if next == nil || len(cursor.generationSpecs) != 1 || cursor.generationSpecs[0].commit != second {
		t.Fatalf("cursor generation = %#v, next %#v", cursor.generationSpecs, next)
	}
}

func TestInteractiveGenerationUsesConfiguredProvider(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "first\n"})
	head := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))

	previousDiscover := interactiveDiscoverProviders
	t.Cleanup(func() { interactiveDiscoverProviders = previousDiscover })
	interactiveDiscoverProviders = func(string) (provider.Discovery, error) {
		return provider.Discovery{Providers: []provider.LoadedManifest{
			interactiveNoteProvider("alpha", provider.ActionNotesGenerate),
			interactiveNoteProvider("beta", provider.ActionNotesGenerate),
		}}, nil
	}

	configured := appconfig.Default()
	configured.Notes.Generator = "beta"
	model := newInteractiveModel(repository, workspaceOptions{}, configured, workspaceview.Store{})
	prepared := model.prepareDraftGenerationCommand([]string{head})().(interactiveGenerationPrepared)
	if prepared.err != nil || prepared.generator.Manifest.Name != "beta" {
		t.Fatalf("configured generator = %#v, error %v", prepared.generator, prepared.err)
	}

	for _, test := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "ambiguous", want: "multiple note generators implement changes.notes.generate (alpha, beta); select one with --provider"},
		{name: "missing", configured: "missing", want: `unknown provider "missing"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			configured := appconfig.Default()
			configured.Notes.Generator = test.configured
			model := newInteractiveModel(repository, workspaceOptions{}, configured, workspaceview.Store{})
			prepared := model.prepareDraftGenerationCommand([]string{head})().(interactiveGenerationPrepared)
			if prepared.err == nil || prepared.err.Error() != test.want {
				t.Fatalf("selection error = %v, want %q", prepared.err, test.want)
			}
		})
	}
}

func TestInteractiveReviewUsesConfiguredStore(t *testing.T) {
	previousDiscover := interactiveDiscoverProviders
	previousVerify := interactiveVerifyNoteSnapshot
	t.Cleanup(func() {
		interactiveDiscoverProviders = previousDiscover
		interactiveVerifyNoteSnapshot = previousVerify
	})
	interactiveDiscoverProviders = func(string) (provider.Discovery, error) {
		return provider.Discovery{Providers: []provider.LoadedManifest{
			interactiveNoteProvider("alpha", provider.ActionNotesCreate),
			interactiveNoteProvider("beta", provider.ActionNotesCreate),
		}}, nil
	}
	interactiveVerifyNoteSnapshot = func(source.Spec, noteSnapshot, string) error { return nil }
	batches := []noteGeneratedComparison{{commit: strings.Repeat("a", 40)}}

	configured := appconfig.Default()
	configured.Notes.Store = "beta"
	model := newInteractiveModel("/repo", workspaceOptions{}, configured, workspaceview.Store{})
	prepared := model.prepareDraftWriteCommand(batches)().(interactiveWritePrepared)
	if prepared.err != nil || prepared.writer.Manifest.Name != "beta" {
		t.Fatalf("configured store = %#v, error %v", prepared.writer, prepared.err)
	}

	for _, test := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "ambiguous", want: "multiple note stores implement changes.notes.create (alpha, beta); select one with --store"},
		{name: "missing", configured: "missing", want: `unknown provider "missing"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			configured := appconfig.Default()
			configured.Notes.Store = test.configured
			model := newInteractiveModel("/repo", workspaceOptions{}, configured, workspaceview.Store{})
			prepared := model.prepareDraftWriteCommand(batches)().(interactiveWritePrepared)
			if prepared.err == nil || prepared.err.Error() != test.want {
				t.Fatalf("selection error = %v, want %q", prepared.err, test.want)
			}
		})
	}
}

func interactiveNoteProvider(name string, actions ...string) provider.LoadedManifest {
	configured := make(map[string]providerlib.Action, len(actions))
	for _, action := range actions {
		configured[action] = providerlib.Action{}
	}
	return provider.LoadedManifest{Manifest: provider.Manifest{Name: name, Actions: configured}}
}

func TestInteractiveDraftReviewRequiresConfirmationAndReportsEachCommit(t *testing.T) {
	previousDiscover := interactiveDiscoverProviders
	previousVerify := interactiveVerifyNoteSnapshot
	previousWrite := interactiveWriteNoteComparison
	t.Cleanup(func() {
		interactiveDiscoverProviders = previousDiscover
		interactiveVerifyNoteSnapshot = previousVerify
		interactiveWriteNoteComparison = previousWrite
	})
	writer := provider.LoadedManifest{Manifest: provider.Manifest{
		Name: "writer", Actions: map[string]providerlib.Action{provider.ActionNotesCreate: {}},
	}}
	discoveries := 0
	interactiveDiscoverProviders = func(string) (provider.Discovery, error) {
		discoveries++
		return provider.Discovery{Providers: []provider.LoadedManifest{writer}}, nil
	}
	verifications := 0
	interactiveVerifyNoteSnapshot = func(source.Spec, noteSnapshot, string) error {
		verifications++
		return nil
	}
	writes := []noteGeneratedComparison{}
	interactiveWriteNoteComparison = func(_ context.Context, batch noteGeneratedComparison, _ provider.LoadedManifest, _ time.Duration) ([]provider.Note, error) {
		writes = append(writes, batch)
		if len(writes) == 2 {
			return nil, errors.New("second store failed")
		}
		return []provider.Note{{ID: "saved"}}, nil
	}

	first := strings.Repeat("a", 40)
	second := strings.Repeat("b", 40)
	batches := []noteGeneratedComparison{
		{
			commit: first, snapshot: noteSnapshot{head: first},
			drafts: []provider.NoteDraft{
				{Summary: "exclude me", Anchor: provider.NoteAnchor{Path: "a.go"}},
				{Summary: "edit me", Rationale: "old", Anchor: provider.NoteAnchor{Path: "b.go"}},
			},
		},
		{
			commit: second, snapshot: noteSnapshot{head: second},
			drafts: []provider.NoteDraft{{Summary: "keep me", Anchor: provider.NoteAnchor{Path: "c.go"}}},
		},
	}
	newReview := func() interactiveModel {
		model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
		model.width, model.height = 100, 30
		model.snapshot.History = []workspaceview.HistoryEntry{{OID: first, Summary: "first"}, {OID: second, Summary: "second"}}
		model.draftBatches = batches
		model.beginDraftReview()
		return model
	}

	cancelled := newReview()
	updated, command := cancelled.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	cancelled = updated.(interactiveModel)
	if command != nil || cancelled.mode != "normal" || discoveries != 0 || len(writes) != 0 {
		t.Fatalf("cancel = mode %q, command %#v, discoveries %d, writes %d", cancelled.mode, command, discoveries, len(writes))
	}

	model := newReview()
	if frame := ansi.Strip(model.draftReviewView()); !strings.Contains(frame, "commit aaaaaaaa") || !strings.Contains(frame, "  a.go") || !strings.Contains(frame, "    [x] exclude me") {
		t.Fatalf("draft hierarchy:\n%s", frame)
	}
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(interactiveModel)
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(interactiveModel)
	updated, command = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(interactiveModel)
	if command == nil || model.mode != "review-edit" {
		t.Fatalf("edit mode = %q, command %#v", model.mode, command)
	}
	model.note.SetValue("edited summary\nedited rationale")
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(interactiveModel)
	if model.mode != "review" || model.draftBatches[0].drafts[1].Summary != "edited summary" || model.draftBatches[0].drafts[1].Rationale != "edited rationale" {
		t.Fatalf("edited draft = mode %q, %#v", model.mode, model.draftBatches[0].drafts[1])
	}
	updated, command = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if command == nil || model.mode != "writing" || len(writes) != 0 {
		t.Fatalf("confirmation preparation = mode %q, command %#v, writes %d", model.mode, command, len(writes))
	}
	prepared := interactiveCommandMessage[interactiveWritePrepared](t, command)
	updated, command = model.Update(prepared)
	model = updated.(interactiveModel)
	if command == nil || verifications != 2 || len(writes) != 0 {
		t.Fatalf("write preparation = command %#v, verifications %d, writes %d", command, verifications, len(writes))
	}
	firstResult := command()
	updated, command = model.Update(firstResult)
	model = updated.(interactiveModel)
	if command == nil || len(writes) != 1 {
		t.Fatalf("first write = command %#v, writes %d", command, len(writes))
	}
	secondResult := command()
	updated, command = model.Update(secondResult)
	model = updated.(interactiveModel)
	if command != nil || model.mode != "results" || len(writes) != 2 {
		t.Fatalf("final write = mode %q, command %#v, writes %d", model.mode, command, len(writes))
	}
	if len(writes[0].drafts) != 1 || writes[0].drafts[0].Summary != "edited summary" {
		t.Fatalf("excluded or edited drafts were not honored: %#v", writes[0].drafts)
	}
	results := ansi.Strip(model.writeResultsView())
	if !strings.Contains(results, "aaaaaaaa  1 note(s) saved") || !strings.Contains(results, "bbbbbbbb  second store failed") {
		t.Fatalf("per-commit results:\n%s", results)
	}
}

func TestInteractiveDraftReviewValidatesEveryCommitBeforeWriting(t *testing.T) {
	previousDiscover := interactiveDiscoverProviders
	previousVerify := interactiveVerifyNoteSnapshot
	previousWrite := interactiveWriteNoteComparison
	t.Cleanup(func() {
		interactiveDiscoverProviders = previousDiscover
		interactiveVerifyNoteSnapshot = previousVerify
		interactiveWriteNoteComparison = previousWrite
	})
	interactiveDiscoverProviders = func(string) (provider.Discovery, error) {
		return provider.Discovery{Providers: []provider.LoadedManifest{{Manifest: provider.Manifest{
			Name: "writer", Actions: map[string]providerlib.Action{provider.ActionNotesCreate: {}},
		}}}}, nil
	}
	checked := 0
	interactiveVerifyNoteSnapshot = func(source.Spec, noteSnapshot, string) error {
		checked++
		if checked == 2 {
			return errors.New("comparison moved")
		}
		return nil
	}
	writes := 0
	interactiveWriteNoteComparison = func(context.Context, noteGeneratedComparison, provider.LoadedManifest, time.Duration) ([]provider.Note, error) {
		writes++
		return nil, nil
	}
	model := newInteractiveModel("/repo", workspaceOptions{}, appconfig.Default(), workspaceview.Store{})
	for _, oid := range []string{strings.Repeat("a", 40), strings.Repeat("b", 40)} {
		model.draftBatches = append(model.draftBatches, noteGeneratedComparison{
			commit: oid, snapshot: noteSnapshot{head: oid},
			drafts: []provider.NoteDraft{{Summary: "draft", Anchor: provider.NoteAnchor{Path: "main.go"}}},
		})
	}
	model.beginDraftReview()
	updated, command := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	prepared := interactiveCommandMessage[interactiveWritePrepared](t, command)
	updated, command = model.Update(prepared)
	model = updated.(interactiveModel)
	if command != nil || model.mode != "review" || !strings.Contains(model.message, "comparison moved") || checked != 2 || writes != 0 {
		t.Fatalf("pre-write validation = mode %q, message %q, checked %d, writes %d, command %#v", model.mode, model.message, checked, writes, command)
	}
}

func interactiveCommandMessage[T any](t *testing.T, command tea.Cmd) T {
	t.Helper()
	var zero T
	if command == nil {
		t.Fatal("interactive command is nil")
	}
	message := command()
	if value, ok := message.(T); ok {
		return value
	}
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("interactive command returned %T, want %T", message, zero)
	}
	for _, child := range batch {
		if value, found := child().(T); found {
			return value
		}
	}
	t.Fatalf("interactive batch omitted %T", zero)
	return zero
}

func TestInteractiveReusesNotesOnlyForSameComparison(t *testing.T) {
	working := workspaceview.Snapshot{Comparison: workspaceview.Comparison{Kind: "working"}}
	commit := workspaceview.Snapshot{Comparison: workspaceview.Comparison{Kind: "commit", To: "abc123"}}
	if !sameWorkspaceComparison(working, workspaceOptions{view: "working"}) {
		t.Fatal("same working comparison was rejected")
	}
	if sameWorkspaceComparison(working, workspaceOptions{view: "staged"}) {
		t.Fatal("notes crossed working and staged comparisons")
	}
	if !sameWorkspaceComparison(commit, workspaceOptions{view: "commit", commit: "abc123"}) {
		t.Fatal("same commit comparison was rejected")
	}
	if sameWorkspaceComparison(commit, workspaceOptions{view: "commit", commit: "def456"}) {
		t.Fatal("notes crossed commit comparisons")
	}
}

func TestWorkspaceOptionsRejectInvalidInteractiveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("interactive:\n  dock: right\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseWorkspaceOptions([]string{"--config", path}, true); err == nil || !strings.Contains(err.Error(), "interactive.dock") {
		t.Fatalf("invalid interactive config error = %v", err)
	}
}

func TestInteractiveCommandPalettePreservesSpaces(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.mode = "command"
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("layout")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("side-by-side")},
	} {
		updated, _ := model.handleKey(key)
		model = updated.(interactiveModel)
	}
	if model.command != "layout side-by-side" {
		t.Fatalf("command = %q", model.command)
	}
}

func TestInteractiveCommandPaletteCompletesCommandsAndValues(t *testing.T) {
	command, message := completeInteractiveCommand("lay")
	if command != "layout " || message != "" {
		t.Fatalf("command completion = %q, %q", command, message)
	}
	command, message = completeInteractiveCommand("layout side")
	if command != "layout side-by-side" || message != "" {
		t.Fatalf("value completion = %q, %q", command, message)
	}
	command, message = completeInteractiveCommand("n")
	if command != "n" || !strings.Contains(message, "navigator") || !strings.Contains(message, "note") {
		t.Fatalf("ambiguous completion = %q, %q", command, message)
	}
}

func TestInteractiveCommandPaletteRejectsAmbiguousPrefix(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{view: "working", commit: "HEAD", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	updated, command := model.runPaletteCommand("n")
	model = updated.(interactiveModel)
	if command != nil || !strings.Contains(model.message, "ambiguous command n") {
		t.Fatalf("ambiguous command result = %q, %#v", model.message, command)
	}
}

func TestInteractiveNotePathsUseChangesNoteAddArgv(t *testing.T) {
	previousExecutable, previousCommand := interactiveExecutable, interactiveCommand
	t.Cleanup(func() { interactiveExecutable, interactiveCommand = previousExecutable, previousCommand })
	interactiveExecutable = func() (string, error) { return "/bin/changes", nil }
	captured := [][]string{}
	interactiveCommand = func(name string, arguments ...string) *exec.Cmd {
		captured = append(captured, append([]string{name}, arguments...))
		return exec.Command("true")
	}
	model := newInteractiveModel("/repo", workspaceOptions{configPath: "/config.yaml", view: "commit", commit: "abc123", layout: "unified", historyLimit: 5}, appconfig.Default(), workspaceview.Store{})
	model.setSnapshot(workspaceview.Snapshot{
		Version: workspaceview.SnapshotVersion, Repository: workspaceview.Repository{Root: "/repo"},
		Files:   []workspaceview.File{{Path: "main.go", Hunks: []workspaceview.Hunk{{Lines: []workspaceview.Line{{Kind: "added", NewLine: 7}}}}}},
		History: []workspaceview.HistoryEntry{}, Groups: []workspaceview.Group{}, Notes: []provider.Note{}, Threads: []workspaceview.NoteThread{}, Failures: []workspaceview.Failure{},
	})
	model.selected = 1
	_ = model.noteCommand("Summary\nRationale", false)
	_ = model.noteCommand("", true)
	if len(captured) != 2 {
		t.Fatalf("captured commands = %#v", captured)
	}
	for _, arguments := range captured {
		joined := strings.Join(arguments, "\x00")
		for _, expected := range []string{"/bin/changes", "note", "add", "--file", "main.go", "--line", "7", "--commit", "abc123", "--config", "/config.yaml"} {
			if !strings.Contains(joined, expected) {
				t.Fatalf("argv omitted %q: %#v", expected, arguments)
			}
		}
	}
	if !slices.Contains(captured[0], "--message") || slices.Contains(captured[1], "--message") {
		t.Fatalf("popup and editor argv = %#v", captured)
	}
}

func TestInteractiveProcessRestoresAlternateScreen(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "before\n"})
	head := strings.TrimSpace(gitWorkspaceOutput(t, repository, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--", "interactive", "--no-notes", "--no-symbols", "--no-calls", "--no-groups", "--quiet")
	command.Dir = repository
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(t.TempDir(), "config"),
		"XDG_DATA_HOME="+filepath.Join(t.TempDir(), "data"),
		"XDG_DATA_DIRS="+t.TempDir(),
		"XDG_STATE_HOME="+filepath.Join(t.TempDir(), "state"),
	)
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 30, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	outputChannel := make(chan []byte, 1)
	loadedChannel := make(chan struct{}, 1)
	historyChannel := make(chan struct{}, 1)
	go func() {
		var output bytes.Buffer
		buffer := make([]byte, 4096)
		loadedSeen := false
		historySeen := false
		for {
			count, readErr := terminal.Read(buffer)
			if count > 0 {
				chunk := buffer[:count]
				_, _ = output.Write(chunk)
				if bytes.Contains(chunk, []byte("\x1b]11;?")) {
					_, _ = terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
				}
				if bytes.Contains(chunk, []byte("\x1b[6n")) {
					_, _ = terminal.Write([]byte("\x1b[1;1R"))
				}
				if !loadedSeen && bytes.Contains(output.Bytes(), []byte("main.go")) && bytes.Contains(output.Bytes(), []byte("after")) {
					loadedSeen = true
					loadedChannel <- struct{}{}
				}
				historyRow := []byte("[ ] " + head[:8] + " fixture")
				if !historySeen && bytes.Contains(output.Bytes(), historyRow) {
					historySeen = true
					historyChannel <- struct{}{}
				}
			}
			if readErr != nil {
				break
			}
		}
		outputBytes := append([]byte(nil), output.Bytes()...)
		outputChannel <- outputBytes
	}()
	select {
	case <-loadedChannel:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = terminal.Close()
		output := <-outputChannel
		t.Fatalf("interactive process did not load the known file and diff: %q", output)
	}
	// Column 10, row 3 is the History tab in the loaded left-dock layout.
	if _, err := terminal.Write([]byte("\x1b[<0;10;3M")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-historyChannel:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = terminal.Close()
		output := <-outputChannel
		t.Fatalf("encoded mouse click did not render the known History row: %q", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("interactive process: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("interactive process did not exit")
	}
	_ = terminal.Close()
	output := <-outputChannel
	if !bytes.Contains(output, []byte("\x1b[?1049h")) || !bytes.Contains(output, []byte("\x1b[?1049l")) {
		t.Fatalf("alternate-screen restoration was not visible: %q", output)
	}
	for _, sequence := range [][]byte{
		[]byte("\x1b[?1002h"),
		[]byte("\x1b[?1006h"),
		[]byte("\x1b[?1002l"),
		[]byte("\x1b[?1006l"),
	} {
		if !bytes.Contains(output, sequence) {
			t.Fatalf("mouse capture or restoration omitted %q: %q", sequence, output)
		}
	}
	plain := ansi.Strip(string(output))
	for _, expected := range []string{"explorer", "changes", "main.go", "after", "[ ] " + head[:8] + " fixture"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("interactive process omitted %q: %q", expected, plain)
		}
	}
}

func gitWorkspaceTest(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func gitWorkspaceOutput(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return string(output)
}

func workspaceHelperCommand(t *testing.T, repository string, arguments ...string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestMainHelperProcess", "--"}, arguments...)...)
	command.Dir = repository
	command.Env = workspaceHelperEnvironment(t)
	return command
}

func workspaceHelperEnvironment(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"CHANGES_PROVIDERS_DIRECTORY="+t.TempDir(),
		"XDG_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(t.TempDir(), "config"),
		"XDG_DATA_HOME="+filepath.Join(t.TempDir(), "data"),
		"XDG_DATA_DIRS="+t.TempDir(),
		"XDG_STATE_HOME="+filepath.Join(t.TempDir(), "state"),
	)
}
