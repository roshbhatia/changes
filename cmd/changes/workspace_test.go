package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/workspaceview"
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
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(interactiveModel)
	if model.activeTab() != "history" || len(model.navigatorItems()) != 1 {
		t.Fatalf("history navigator = %#v", model.navigatorItems())
	}
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
	frameChannel := make(chan struct{}, 1)
	go func() {
		var output bytes.Buffer
		buffer := make([]byte, 4096)
		frameSeen := false
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
				if !frameSeen && bytes.Contains(output.Bytes(), []byte("changes")) {
					frameSeen = true
					frameChannel <- struct{}{}
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
	case <-frameChannel:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = terminal.Close()
		output := <-outputChannel
		t.Fatalf("interactive process did not render: %q", output)
	}
	time.Sleep(100 * time.Millisecond)
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
