package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
)

func TestNoteLineInPatchFindsBothSidesAndRejectsHiddenLines(t *testing.T) {
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,2 +2,2 @@\n-old\n+new\n same\n"
	for _, test := range []struct {
		side string
		line int
		want string
	}{
		{side: provider.NoteSideLeft, line: 2, want: "old"},
		{side: provider.NoteSideRight, line: 2, want: "new"},
		{side: provider.NoteSideLeft, line: 3, want: "same"},
		{side: provider.NoteSideRight, line: 3, want: "same"},
	} {
		got, err := noteLineInPatch(patch, test.side, test.line)
		if err != nil || got != test.want {
			t.Fatalf("%s:%d = %q, %v; want %q", test.side, test.line, got, err, test.want)
		}
	}
	if _, err := noteLineInPatch(patch, provider.NoteSideRight, 1); err == nil {
		t.Fatal("line outside the rendered diff was accepted")
	}
	if _, err := noteRangeInPatch(patch, provider.NoteSideRight, 1, 2); err == nil {
		t.Fatal("range with a hidden start line was accepted")
	}
}

func TestNoteRangeInFilePatchUsesTheSelectedFileAndSide(t *testing.T) {
	patch := "diff --git a/first.go b/first.go\n--- a/first.go\n+++ b/first.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -10 +10 @@\n-before\n+after\n"
	if _, err := noteRangeInFilePatch(patch, "main.go", provider.NoteSideRight, 0, 1); err == nil {
		t.Fatal("a line visible only in another file was accepted")
	}
	got, err := noteRangeInFilePatch(patch, "main.go", provider.NoteSideRight, 0, 10)
	if err != nil || got != "after" {
		t.Fatalf("main.go:10 = %q, %v", got, err)
	}
	deleted := "diff --git a/old.go b/old.go\n--- a/old.go\n+++ /dev/null\n@@ -1 +0,0 @@\n--- not-a-header\n"
	got, err = noteRangeInFilePatch(deleted, "old.go", provider.NoteSideLeft, 0, 1)
	if err != nil || got != "-- not-a-header" {
		t.Fatalf("deleted old.go:1 = %q, %v", got, err)
	}
	quoted := "diff --git quoted\n--- \"a/space\\nname.go\"\n+++ \"b/space\\nname.go\"\n@@ -1 +1 @@\n-old\n+new\n"
	got, err = noteRangeInFilePatch(quoted, "space\nname.go", provider.NoteSideRight, 0, 1)
	if err != nil || got != "new" {
		t.Fatalf("quoted path:1 = %q, %v", got, err)
	}
	for name, patch := range map[string]string{
		"binary":       "diff --git a/blob.bin b/blob.bin\nindex 8352675..b842680 100644\nBinary files a/blob.bin and b/blob.bin differ\n",
		"binary space": "diff --git a/blob data.bin b/blob data.bin\nindex 8352675..b842680 100644\nBinary files a/blob data.bin and b/blob data.bin differ\n",
		"mode":         "diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\n",
		"rename":       "diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n",
	} {
		path := "blob.bin"
		if name == "binary space" {
			path = "blob data.bin"
		} else if name == "mode" {
			path = "script.sh"
		} else if name == "rename" {
			path = "new.go"
		}
		if _, err := noteRangeInFilePatch(patch, path, provider.NoteSideRight, 0, 0); err != nil {
			t.Fatalf("%s file note: %v", name, err)
		}
	}
	combined := "diff --cc conflict.go\nindex 1111111,2222222..3333333\n--- a/conflict.go\n+++ b/conflict.go\n@@@ -1,1 -1,1 +1,1 @@@\n++merged\n"
	if _, err := noteRangeInFilePatch(combined, "conflict.go", provider.NoteSideRight, 0, 0); err != nil {
		t.Fatalf("combined file note: %v", err)
	}
	if _, err := noteRangeInFilePatch(combined, "conflict.go", provider.NoteSideRight, 0, 1); err == nil || !strings.Contains(err.Error(), "file notes only") {
		t.Fatalf("combined line note error = %v", err)
	}
}

func TestNoteSpecUsesCanonicalRepositoryPath(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	spec, relative, err := noteSpec(repository, "main.go", noteComparisonFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if relative != "main.go" || len(spec.Paths) != 1 {
		t.Fatalf("spec = %+v, relative = %q", spec, relative)
	}
}

func TestStableComparisonRetriesMovingEndpoints(t *testing.T) {
	identities := [][2]string{{"base-a", "head-a"}, {"base-b", "head-b"}, {"base-b", "head-b"}, {"base-b", "head-b"}}
	reads := 0
	patches := 0
	patch, base, head, err := stableComparison(func() (string, string) {
		identity := identities[reads]
		reads++
		return identity[0], identity[1]
	}, func() (string, error) {
		patches++
		return fmt.Sprintf("patch-%d", patches), nil
	})
	if err != nil || patch != "patch-2" || base != "base-b" || head != "head-b" {
		t.Fatalf("stable comparison = %q %q %q, %v", patch, base, head, err)
	}

	reads = 0
	_, _, _, err = stableComparison(func() (string, string) {
		reads++
		return fmt.Sprintf("base-%d", reads), "head"
	}, func() (string, error) { return "patch", nil })
	if err == nil || !strings.Contains(err.Error(), "changed while capturing") {
		t.Fatalf("moving comparison error = %v", err)
	}
}

func TestSplitNoteMessageOnlyStripsEditorComments(t *testing.T) {
	message := "Summary\n\nDetail\n# literal"
	summary, rationale := splitNoteMessage(message, false)
	if summary != "Summary" || rationale != "Detail\n# literal" {
		t.Fatalf("literal message = %q / %q", summary, rationale)
	}
	summary, rationale = splitNoteMessage(message, true)
	if summary != "Summary" || rationale != "Detail" {
		t.Fatalf("editor message = %q / %q", summary, rationale)
	}
}

func TestEditNoteUsesConfiguredFilePlaceholder(t *testing.T) {
	script := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Edited summary\\n\\nEdited detail\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	message, err := editNote([]string{script, "$FILE"})
	if err != nil {
		t.Fatal(err)
	}
	summary, rationale := splitNoteMessage(message, true)
	if summary != "Edited summary" || rationale != "Edited detail" {
		t.Fatalf("edited message = %q / %q", summary, rationale)
	}
}

func TestEditNoteParsesQuotedEnvironmentEditor(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "editor with spaces")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "editor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Environment editor\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", fmt.Sprintf("%q", script))
	t.Setenv("EDITOR", "")
	message, err := editNote(nil)
	if err != nil {
		t.Fatal(err)
	}
	if message != "Environment editor\n" {
		t.Fatalf("message = %q", message)
	}
}

func TestExplicitEmptyMessageFileDoesNotOpenEditor(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"note", "add", "--file", "main.go", "--message-file", empty)
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1", "VISUAL=false", "EDITOR=false")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "note message has no summary") {
		t.Fatalf("error = %v, output = %s", err, output)
	}
	command = exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"note", "add", "--file", "main.go", "--message", "")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1", "VISUAL=false", "EDITOR=false")
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "note message has no summary") {
		t.Fatalf("empty --message error = %v, output = %s", err, output)
	}
}

func TestStableNoteSnapshotIgnoresDisplayPathFilter(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"first.go": "old\n", "second.go": "old\n"})
	for _, name := range []string{"first.go", "second.go"} {
		if err := os.WriteFile(filepath.Join(repository, name), []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	spec := source.Spec{Dir: repository, Paths: []string{filepath.Join(repository, "first.go")}}
	displayed, err := spec.Diff()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := stableNoteSnapshot(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(displayed, "second.go") || !strings.Contains(snapshot.patch, "second.go") {
		t.Fatalf("displayed = %q, comparison = %q", displayed, snapshot.patch)
	}
}

func TestValidateNoteVisibilityRejectsInvisibleExactLine(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:note",
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 999,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementExact,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err == nil {
		t.Fatal("invisible exact placement was accepted")
	}
	note.Placement.Line = 1
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatalf("visible exact placement: %v", err)
	}
}

func TestValidateNoteVisibilityChecksMixedSideRangeStart(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": ""})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:range",
		Placement: provider.NotePlacement{
			Path: "main.go", StartSide: provider.NoteSideLeft, StartLine: 1,
			Side: provider.NoteSideRight, Line: 2,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementExact,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err == nil {
		t.Fatal("mixed-side range with invisible start was accepted")
	}
}

func TestValidateNoteVisibilityDegradesInvisibleContextToFile(t *testing.T) {
	repository := t.TempDir()
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	prepareRepository(t, repository, map[string]string{"main.go": before})
	after := strings.Replace(before, "ten\n", "changed\n", 1)
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:context",
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementContext,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatal(err)
	}
	if note.Placement.Quality != provider.PlacementFile || note.Placement.Line != 0 {
		t.Fatalf("degraded placement = %+v", note.Placement)
	}
}

func TestValidateNoteVisibilityDegradesMismatchedContextToFile(t *testing.T) {
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+visible-a\n"
	note := provider.Note{
		ID:     "fake:context",
		Anchor: provider.NoteAnchor{Context: "target"},
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementContext,
		},
	}
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatal(err)
	}
	if note.Placement.Quality != provider.PlacementFile || note.Placement.Line != 0 {
		t.Fatalf("mismatched context placement = %+v", note.Placement)
	}
}

func TestNoteLocationIncludesSideAndRange(t *testing.T) {
	note := provider.Note{Placement: provider.NotePlacement{
		Path: "main.go", Side: provider.NoteSideLeft, Line: 8, Quality: provider.PlacementExact,
	}}
	if got := noteLocation(note); got != "main.go:8@left" {
		t.Fatalf("single-line location = %q", got)
	}
	note.Placement.StartSide = provider.NoteSideRight
	note.Placement.StartLine = 4
	if got := noteLocation(note); got != "main.go:4@right-8@left" {
		t.Fatalf("range location = %q", got)
	}
}

func TestNoteCLIAuthorsListsAndRendersThroughProvider(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\ntwo\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\nchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes:
    description: read notes
  changes.notes.create:
    description: create notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runNoteCLI(t, repository, "note", "add", "--config", config, "--provider", "fake",
		"--file", "main.go", "--line", "2", "--message", "Harness context\nWhy it changed.",
		"--author", "harness", "--origin", "agent", "--session", "session-7")
	if !strings.Contains(out, "note: fake:created main.go:2") {
		t.Fatalf("note add output = %s", out)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var request provider.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if request.Action != provider.ActionNotesCreate || request.Note == nil ||
		request.Note.Session != "session-7" || request.Note.Anchor.Context != "changed" ||
		request.Note.Anchor.Target != provider.NoteTargetWorking || request.Base == "" || request.Head != "" {
		t.Fatalf("create request = %+v", request)
	}

	out = runNoteCLI(t, repository, "note", "list", "--config", config, "--provider", "fake")
	for _, want := range []string{"notes", "main.go:2", "Remember this context", "reviewer via fake"} {
		if !strings.Contains(out, want) {
			t.Fatalf("note list omitted %q:\n%s", want, out)
		}
	}
	out = runNoteCLI(t, repository, "--config", config, "--color", "never", "--no-symbols", "--no-calls")
	for _, want := range []string{"notes", "Remember this context", "diff --git a/main.go b/main.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render omitted %q:\n%s", want, out)
		}
	}
}

func TestNoteProviderCompletionUsesConfigAndAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config folder")
	providers := filepath.Join(root, "providers")
	if err := os.MkdirAll(providers, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, action := range map[string]string{
		"reader": provider.ActionNotes,
		"writer": provider.ActionNotesCreate,
	} {
		manifest := fmt.Sprintf("version: provider/v1\nname: %s\ndescription: test\ncommand: [true]\nactions:\n  %s:\n    description: test\n", name, action)
		if err := os.WriteFile(filepath.Join(providers, name+".yaml"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(root, "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	context := fmt.Sprintf(`changes note add --config %q --provider `, config)
	output := runNoteCLI(t, t.TempDir(), "__values", "note-writers", context)
	if output != "writer\n" {
		t.Fatalf("writer completion = %q", output)
	}
	context = fmt.Sprintf(`changes provider validate --config %q `, config)
	output = runNoteCLI(t, t.TempDir(), "__values", "providers", context)
	if output != "reader\nwriter\n" {
		t.Fatalf("provider completion = %q", output)
	}
}

func TestFakeNoteProviderProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	var request provider.Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Args[separator+1], payload, 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "read", Source: "fake", SourceID: "read", Summary: "Remember this context",
		Rationale: "It explains the changed line.", Author: "reviewer",
		Origin: provider.NoteOriginExternal, Authority: provider.NoteAuthorityExternal,
		State: provider.NoteStateOpen,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2,
			Base: request.Base, Head: request.Head, Target: provider.NoteTargetWorking,
		},
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2,
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Target: requestNoteTarget(request), Quality: provider.PlacementExact,
		},
	}
	if request.Action == provider.ActionNotesCreate {
		note = provider.Note{
			ID: "created", Source: "fake", SourceID: "created", Summary: request.Note.Summary,
			Author: request.Note.Author, Origin: request.Note.Origin, Authority: provider.NoteAuthorityAdvisory,
			State: provider.NoteStateOpen, Anchor: request.Note.Anchor,
			Placement: provider.NotePlacement{
				Path: request.Note.Anchor.Path, Side: request.Note.Anchor.Side,
				StartSide: request.Note.Anchor.StartSide, StartLine: request.Note.Anchor.StartLine,
				Line: request.Note.Anchor.Line, Base: request.Base, Head: request.Head,
				Fingerprint: request.Fingerprint, Target: requestNoteTarget(request),
				Quality: provider.PlacementExact,
			},
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(provider.Response{
		Version: provider.ProtocolVersion, Notes: []provider.Note{note},
	}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func requestNoteTarget(request provider.Request) string {
	if request.To != "" || request.Head != "" {
		return provider.NoteTargetCommits
	}
	if request.Staged {
		return provider.NoteTargetIndex
	}
	return provider.NoteTargetWorking
}

func runNoteCLI(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestMainHelperProcess", "--"}, arguments...)...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("changes %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
