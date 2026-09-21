package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/workspaceview"
	"github.com/roshbhatia/go-utils/diffview"
)

func TestFileLocationsSurviveGroupingNotesAndRepeatedBasenames(t *testing.T) {
	files := []diffview.File{
		{Path: "one/same.go", Add: 1, Hunks: []diffview.Hunk{{NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "first"}}}}},
		{Path: "two/same.go", Add: 1, Hunks: []diffview.Hunk{{NewAt: 1, Lines: []diffview.Line{{Kind: '+', Text: "second"}}}}},
	}
	for _, width := range []int{30, 100} {
		rows := map[string]int{}
		body, err := renderTreeWithGroups(diffview.Options{Files: files, Width: width, Unified: true}, []provider.ChangeGroup{{ID: "both", Title: "Related changes", Anchors: []provider.GroupAnchor{{Path: "one/same.go"}, {Path: "two/same.go"}}}}, noteLayer{values: []provider.Note{{State: provider.NoteStateOpen, Author: "reviewer", Source: "test", Placement: provider.NotePlacement{Path: "one/same.go", Side: provider.NoteSideRight, Line: 1, Quality: provider.PlacementExact}}}}, width, rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[files[0].Path] >= rows[files[1].Path] || strings.Contains(body, "changes-file=") {
			t.Fatalf("invalid anchors %v in %q", rows, body)
		}
		for _, file := range files {
			if !strings.Contains(ansi.Strip(strings.Split(body, "\n")[rows[file.Path]]), "same.go") {
				t.Fatalf("anchor missed %s", file.Path)
			}
		}
	}
}

func TestPageKeysAndClickScopes(t *testing.T) {
	model := newInteractiveModel("/repo", workspaceOptions{}, appconfig.Default(), workspaceview.Store{})
	model.width, model.height = 140, 30
	model.resize()
	row := 25
	model.snapshot = workspaceview.Snapshot{Rendered: strings.Repeat("line\n", 100), Files: []workspaceview.File{{Path: "file.go", RenderedLine: &row}}}
	model.viewport.SetContent(model.snapshot.Rendered)
	model.focus = "navigator"
	updated, _ := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(interactiveModel)
	if model.viewport.YOffset == 0 || model.configured.Interactive.Dock != "left" {
		t.Fatal("d must scroll without moving sidebar")
	}
	updated, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	model = updated.(interactiveModel)
	if model.viewport.YOffset != 0 {
		t.Fatal("u must scroll up")
	}
	model.configured.Interactive.Navigator = "list"
	region := model.navigatorRegion()
	click := tea.MouseMsg{X: region.X + 2, Y: region.Y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	updated, command := model.handleMouse(click)
	model = updated.(interactiveModel)
	if command != nil || model.previewActive || model.viewport.YOffset != row {
		t.Fatalf("click did not jump in overview: %d %v", model.viewport.YOffset, model.previewActive)
	}
	click.Shift = true
	updated, command = model.handleMouse(click)
	model = updated.(interactiveModel)
	if command == nil || !model.previewActive || model.previewPath != "file.go" {
		t.Fatal("shift-click did not scope the file")
	}
	click.Shift = false
	updated, _ = model.handleMouse(click)
	model = updated.(interactiveModel)
	if model.previewActive || model.viewport.YOffset != row {
		t.Fatal("click did not leave the scoped view")
	}
}

func TestRecordedAuthorshipDistinguishesAssistanceAndUncommittedLines(t *testing.T) {
	root := t.TempDir()
	prepareRepository(t, root, map[string]string{"a.txt": "before\n"})
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "Change\n\nAssisted-by: Codex\nCo-authored-by: Human <human@example.invalid>")
	options := workspaceOptions{view: "commit", commit: "HEAD", layout: "unified", historyLimit: 5, noCalls: true, noGroups: true, noNotes: true, noSymbols: true}
	configured := appconfig.Default()
	configured.Providers.Directory = t.TempDir()
	snapshot, err := buildWorkspaceSnapshot(root, options, configured)
	if err != nil {
		t.Fatal(err)
	}
	if contributorBadge(snapshot.History[0].Contributors) == "" {
		t.Fatal("declared assistance missing")
	}
	body := lineAttribution(context.Background(), root, options, snapshot, "a.txt")
	if !strings.Contains(body, "assistant: Codex") || !strings.Contains(body, "co-author: Human") {
		t.Fatalf("missing attribution: %s", body)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("uncommitted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	options.view = "working"
	snapshot, err = buildWorkspaceSnapshot(root, options, configured)
	if err != nil {
		t.Fatal(err)
	}
	body = lineAttribution(context.Background(), root, options, snapshot, "a.txt")
	if !strings.Contains(body, "+ 1 uncommitted or unavailable; authorship unknown") {
		t.Fatalf("invented author for working line: %s", body)
	}
	if !strings.Contains(body, "- 1 Changes test") {
		t.Fatalf("previous committed author missing: %s", body)
	}
}
