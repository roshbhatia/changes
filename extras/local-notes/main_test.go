package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roshbhatia/changes/internal/provider"
)

func TestCreateListAndReanchorNote(t *testing.T) {
	repository, request := localFixture(t)
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("first\nanchor\nlast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Fingerprint = "first"
	request.Note = &provider.NoteDraft{
		Summary: "Keep this invariant", Rationale: "The caller depends on it.",
		Author: "agent", Origin: provider.NoteOriginAgent, Session: "session-1",
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, StartSide: provider.NoteSideRight,
			StartLine: 1, Line: 2, Base: "base",
			Fingerprint: request.Fingerprint, Context: "anchor", Target: provider.NoteTargetWorking,
		},
	}
	created, err := create(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Notes) != 1 || created.Notes[0].Anchor.Line != 2 || created.Notes[0].Session != "session-1" ||
		created.Notes[0].Placement.Path != "main.go" {
		t.Fatalf("created note = %+v", created.Notes)
	}

	if err := os.WriteFile(path, []byte("new\nfirst\nanchor\nlast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	request.Fingerprint = "second"
	listed, err := list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 1 {
		t.Fatalf("notes = %+v", listed.Notes)
	}
	note := listed.Notes[0]
	if note.Anchor.StartLine != 1 || note.Anchor.Line != 2 || note.Placement.StartSide != "" ||
		note.Placement.StartLine != 0 || note.Placement.Line != 3 || note.Placement.Quality != provider.PlacementContext {
		t.Fatalf("reanchored note = %+v", note)
	}

	request.Staged = true
	listed, err = list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 0 {
		t.Fatalf("working note leaked into index comparison: %+v", listed.Notes)
	}
}

func TestDecodeRequestRejectsUnknownFields(t *testing.T) {
	_, err := decodeRequest(bytes.NewBufferString(`{"version":"changes.provider/v1","validaton":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeRequestRejectsOversizedTrailingData(t *testing.T) {
	_, err := decodeRequestWithLimit(strings.NewReader("{}  {}"), 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestStagedBlankAnchorDoesNotReadWorkingTree(t *testing.T) {
	repository, request := localFixture(t)
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\nunstaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Staged = true
	request.Note = &provider.NoteDraft{
		Summary: "blank line", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2, Base: request.Base,
			Fingerprint: request.Fingerprint, Target: provider.NoteTargetIndex,
		},
	}
	response, err := create(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Notes[0].Anchor.Context != "" || response.Notes[0].Anchor != request.Note.Anchor {
		t.Fatalf("created anchor = %+v", response.Notes[0].Anchor)
	}
}

func TestLegacyNoteCompatibilityAndPlacementFallbacks(t *testing.T) {
	repository, request := localFixture(t)
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("same\nother\nsame\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rationale := "legacy detail"
	legacy := storedNote{
		ID: "legacy", File: path, Line: 2, Summary: "Legacy note", Rationale: &rationale,
		Author: "human", Origin: provider.NoteOriginUser, Anchor: "missing", State: "answered",
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := publish(recordFile(request), document{Version: 1, Notes: []json.RawMessage{raw}}); err != nil {
		t.Fatal(err)
	}
	response, err := list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 1 {
		t.Fatalf("legacy notes = %+v", response.Notes)
	}
	note := response.Notes[0]
	if note.State != provider.NoteStateResolved || note.Authority != provider.NoteAuthorityOwner ||
		note.Placement.Quality != provider.PlacementFile || note.Rationale != rationale {
		t.Fatalf("legacy note = %+v", note)
	}
	request.To = "head"
	request.Head = "head"
	response, err = list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 0 {
		t.Fatalf("legacy note leaked into committed comparison: %+v", response.Notes)
	}
	request.To = ""
	request.Head = ""

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	response, err = list(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Notes[0].Placement.Quality != provider.PlacementOrphan {
		t.Fatalf("removed-file placement = %+v", response.Notes[0].Placement)
	}
}

func TestConcurrentCreatesPreserveEveryRecord(t *testing.T) {
	repository, request := localFixture(t)
	const count = 12
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(strings.Repeat("line\n", count)), 0o600); err != nil {
		t.Fatal(err)
	}
	errorsFound := make(chan error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			one := request
			one.Note = &provider.NoteDraft{
				Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
				Anchor: provider.NoteAnchor{
					Path: "main.go", Side: provider.NoteSideRight, Line: index + 1,
					Base: "base", Target: provider.NoteTargetWorking,
				},
			}
			_, err := create(one)
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	doc, err := readDocument(recordFile(request))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Notes) != count {
		t.Fatalf("stored notes = %d, want %d", len(doc.Notes), count)
	}
}

func TestCommittedNotesRequireExactEndpoints(t *testing.T) {
	repository, request := localFixture(t)
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Base, request.Head, request.To = "base-a", "head-a", "head-a"
	request.Fingerprint = "identical-patch"
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Target: provider.NoteTargetCommits,
		},
	}
	if _, err := create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	request.Base, request.Head, request.To = "base-b", "head-b", "head-b"
	response, err := list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 0 {
		t.Fatalf("note leaked across commit endpoints: %+v", response.Notes)
	}
}

func TestIndexNotesDoNotProjectThroughWorkingTree(t *testing.T) {
	repository, request := localFixture(t)
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("first\nanchor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Staged = true
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2, Base: request.Base,
			Fingerprint: request.Fingerprint, Context: "anchor", Target: provider.NoteTargetIndex,
		},
	}
	if _, err := create(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("inserted\nfirst\nanchor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	response, err := list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 1 || response.Notes[0].Placement.Line != 2 ||
		response.Notes[0].Placement.Quality != provider.PlacementExact {
		t.Fatalf("index note was projected through the working tree: %+v", response.Notes)
	}
	request.Fingerprint = "changed-index"
	response, err = list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 0 {
		t.Fatalf("index note survived a different index fingerprint: %+v", response.Notes)
	}
}

func TestBlankWorkingAnchorDegradesAfterComparisonChanges(t *testing.T) {
	repository, request := localFixture(t)
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("first\n\nlast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Fingerprint = "blank"
	request.Note = &provider.NoteDraft{
		Summary: "blank line", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2, Base: request.Base,
			Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	if _, err := create(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("first\nreplacement\nlast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	request.Fingerprint = "replacement"
	response, err := list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 1 || response.Notes[0].Placement.Quality != provider.PlacementFile ||
		response.Notes[0].Placement.Line != 0 {
		t.Fatalf("blank anchor stayed on replacement text: %+v", response.Notes)
	}
}

func TestCreatePreservesUnknownEnvelopeFields(t *testing.T) {
	repository, request := localFixture(t)
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"notes":[],"owner":{"name":"sysinit"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1, Base: request.Base,
			Target: provider.NoteTargetWorking,
		},
	}
	if _, err := create(request); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	var owner struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(envelope["owner"], &owner); err != nil || owner.Name != "sysinit" {
		t.Fatalf("owner extension = %s, %v", envelope["owner"], err)
	}
}

func TestCreateRejectsMalformedExistingRecordWithoutWriting(t *testing.T) {
	_, request := localFixture(t)
	request.Note = &provider.NoteDraft{
		Summary: "valid", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Base: request.Base, Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte("{\"version\":1,\"notes\":[{}]}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := create(request); err == nil {
		t.Fatal("create accepted a malformed existing record")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed create changed the store: %q", after)
	}
}

func TestCreateRejectsInvalidStoredComparisonWithoutWriting(t *testing.T) {
	_, request := localFixture(t)
	request.Note = &provider.NoteDraft{
		Summary: "valid", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Base: request.Base, Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"version":1,"notes":[{"id":"old","file":"main.go","line":1,"summary":"old","author":"agent","origin":"agent","comparison":{"path":"../outside","side":"RIGHT","line":1,"target":"working"}}]}` + "\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := create(request); err == nil || !strings.Contains(err.Error(), "invalid comparison") {
		t.Fatalf("create error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed create changed the store: %q", after)
	}
}

func TestCreateRejectsDuplicateStoredIDsWithoutWriting(t *testing.T) {
	_, request := localFixture(t)
	request.Note = &provider.NoteDraft{
		Summary: "valid", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Base: request.Base, Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"version":1,"notes":[{"id":"same","file":"main.go","line":1,"summary":"one","author":"agent","origin":"agent"},{"id":"same","file":"main.go","line":2,"summary":"two","author":"agent","origin":"agent"}]}` + "\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := create(request); err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("create error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed create changed the store: %q", after)
	}
}

func TestStoreRejectsControlOnlyContentAndInvalidState(t *testing.T) {
	_, request := localFixture(t)
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, record := range map[string]string{
		"blank id":        `{"id":"   ","file":"main.go","line":1,"summary":"old","author":"agent","origin":"agent"}`,
		"control summary": `{"id":"old","file":"main.go","line":1,"summary":"\u0007","author":"agent","origin":"agent"}`,
		"blank author":    `{"id":"old","file":"main.go","line":1,"summary":"old","author":" \t ","origin":"agent"}`,
		"invalid state":   `{"id":"old","file":"main.go","line":1,"summary":"old","author":"agent","origin":"agent","state":"bogus"}`,
	} {
		t.Run(name, func(t *testing.T) {
			before := []byte(`{"version":1,"notes":[` + record + `]}` + "\n")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := list(request); err == nil {
				t.Fatal("list accepted an invalid stored record")
			}
			request.Note = &provider.NoteDraft{
				Summary: "valid", Author: "agent", Origin: provider.NoteOriginAgent,
				Anchor: provider.NoteAnchor{
					Path: "main.go", Side: provider.NoteSideRight, Line: 1,
					Base: request.Base, Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
				},
			}
			if _, err := create(request); err == nil {
				t.Fatal("create accepted an invalid stored record")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("failed create changed the store: %q", after)
			}
		})
	}
}

func TestCreateRejectsContentBlankAfterSanitizingWithoutWriting(t *testing.T) {
	_, request := localFixture(t)
	for name, edit := range map[string]func(*provider.NoteDraft){
		"summary": func(draft *provider.NoteDraft) { draft.Summary = " \a " },
		"author":  func(draft *provider.NoteDraft) { draft.Author = " \a " },
	} {
		t.Run(name, func(t *testing.T) {
			draft := provider.NoteDraft{
				Summary: "valid", Author: "agent", Origin: provider.NoteOriginAgent,
				Anchor: provider.NoteAnchor{
					Path: "main.go", Side: provider.NoteSideRight, Line: 1,
					Base: request.Base, Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
				},
			}
			edit(&draft)
			request.Note = &draft
			if _, err := create(request); err == nil || !strings.Contains(err.Error(), "remain non-empty") {
				t.Fatalf("create error = %v", err)
			}
			if _, err := os.Stat(recordFile(request)); !os.IsNotExist(err) {
				t.Fatalf("failed create wrote the store: %v", err)
			}
		})
	}
}

func TestStoreFailsClosedForMalformedDataAndSymlinks(t *testing.T) {
	_, request := localFixture(t)
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := list(request); err == nil {
		t.Fatal("malformed store was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte(`{"version":1,"notes":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := list(request); err == nil {
		t.Fatal("symlinked store was accepted")
	}
}

func TestStoreRejectsSymlinkedStoreDirectory(t *testing.T) {
	_, request := localFixture(t)
	path := recordFile(request)
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := list(request); err == nil {
		t.Fatal("symlinked store directory was accepted")
	}
}

func TestStoreRejectsSymlinkedStoreAncestor(t *testing.T) {
	_, request := localFixture(t)
	path := recordFile(request)
	state := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(external, "diff-notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(state, "agents")); err != nil {
		t.Fatal(err)
	}
	if _, err := list(request); err == nil {
		t.Fatal("symlinked store ancestor was accepted")
	}
}

func TestCreateRejectsDanglingPathBelowEscapingSymlink(t *testing.T) {
	repository, request := localFixture(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(repository, "link")); err != nil {
		t.Fatal(err)
	}
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "link/missing.go", Side: provider.NoteSideRight, Line: 1,
			Base: request.Base, Fingerprint: request.Fingerprint, Context: "missing",
			Target: provider.NoteTargetWorking,
		},
	}
	if _, err := create(request); err == nil {
		t.Fatal("dangling path below an escaping symlink was accepted")
	}
	if _, err := os.Stat(recordFile(request)); !os.IsNotExist(err) {
		t.Fatalf("failed create changed the store: %v", err)
	}
}

func TestCreateSupportsTrackedSymlinkAnchor(t *testing.T) {
	repository, request := localFixture(t)
	target := filepath.Join(repository, "target.go")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.go", filepath.Join(repository, "link.go")); err != nil {
		t.Fatal(err)
	}
	request.Files = []string{"link.go"}
	request.Note = &provider.NoteDraft{
		Summary: "symlink target", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "link.go", Side: provider.NoteSideRight, Line: 1, Base: request.Base,
			Fingerprint: request.Fingerprint, Context: "target.go", Target: provider.NoteTargetWorking,
		},
	}
	response, err := create(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Notes) != 1 || response.Notes[0].Anchor.Path != "link.go" {
		t.Fatalf("created symlink note = %+v", response.Notes)
	}
	doc, err := readDocument(recordFile(request))
	if err != nil {
		t.Fatal(err)
	}
	var stored storedNote
	if err := json.Unmarshal(doc.Notes[0], &stored); err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if stored.File != resolvedTarget {
		t.Fatalf("compatibility path = %q, want %q", stored.File, resolvedTarget)
	}
}

func TestCreateRejectsFileSymlinkOutsideRepository(t *testing.T) {
	repository, request := localFixture(t)
	target := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(target, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(repository, "linked.go")); err != nil {
		t.Fatal(err)
	}
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "linked.go", Side: provider.NoteSideRight, Line: 1, Base: request.Base,
			Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	if _, err := create(request); err == nil {
		t.Fatal("file symlink outside the repository was accepted")
	}
}

func TestCreateRejectsDanglingFileSymlinkOutsideRepository(t *testing.T) {
	repository, request := localFixture(t)
	target := filepath.Join(t.TempDir(), "missing.go")
	if err := os.Symlink(target, filepath.Join(repository, "linked.go")); err != nil {
		t.Fatal(err)
	}
	request.Note = &provider.NoteDraft{
		Summary: "note", Author: "agent", Origin: provider.NoteOriginAgent,
		Anchor: provider.NoteAnchor{
			Path: "linked.go", Side: provider.NoteSideRight, Line: 1, Base: request.Base,
			Fingerprint: request.Fingerprint, Target: provider.NoteTargetWorking,
		},
	}
	if _, err := create(request); err == nil {
		t.Fatal("dangling file symlink outside the repository was accepted")
	}
}

func TestLockCanBeReacquiredAfterRelease(t *testing.T) {
	repository, request := localFixture(t)
	request.Validation = true
	path := recordFile(request)
	first, err := lock(path)
	if err != nil {
		t.Fatal(err)
	}
	lockInfo, err := os.Stat(path + ".lock")
	if err != nil || !lockInfo.Mode().IsRegular() {
		t.Fatalf("Changes lock is not a regular file: %v, %+v", err, lockInfo)
	}
	if _, err := lock(path); err == nil {
		t.Fatal("concurrent lock acquisition succeeded")
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path+".lock", old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := lock(path); err == nil {
		t.Fatal("active lock was reclaimed because of its age")
	}
	if err := os.Mkdir(path+".lock", 0o700); !os.IsExist(err) {
		t.Fatalf("directory-lock peer did not observe the lock: %v", err)
	}
	first()
	if err := os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatalf("directory-lock peer could not acquire after release: %v", err)
	}
	if _, err := lock(path); err == nil {
		t.Fatal("active directory lock was reclaimed")
	}
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	second, err := lock(path)
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	second()
	if _, err := os.Stat(filepath.Join(repository, ".changes-provider-validation", "notes.json.lock")); !os.IsNotExist(err) {
		t.Fatalf("released lock remains: %v", err)
	}
}

func TestProcessExitReleasesLock(t *testing.T) {
	if os.Getenv("GO_WANT_LOCAL_NOTE_LOCK_CRASH") == "1" {
		if _, err := lock(os.Getenv("LOCAL_NOTE_LOCK_PATH")); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	state := t.TempDir()
	path := filepath.Join(state, "agents", "diff-notes", "notes.json")
	command := exec.Command(os.Args[0], "-test.run=TestProcessExitReleasesLock")
	command.Env = append(os.Environ(),
		"GO_WANT_LOCAL_NOTE_LOCK_CRASH=1",
		"LOCAL_NOTE_LOCK_PATH="+path,
		"XDG_STATE_HOME="+state,
		"SYSINIT_PATHS_MANIFEST="+filepath.Join(state, "missing-paths.json"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lock child: %v\n%s", err, output)
	}
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			release, err := lock(path)
			if err == nil {
				release()
			}
			errorsFound <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errorsFound; err != nil {
			t.Fatalf("concurrent lock after process exit: %v", err)
		}
	}
}

func localFixture(t *testing.T) (string, provider.Request) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("SYSINIT_PATHS_MANIFEST", filepath.Join(state, "missing-paths.json"))
	repository := t.TempDir()
	return repository, provider.Request{
		Directory: repository, Files: []string{"main.go"}, Base: "base",
		Fingerprint: "fingerprint",
	}
}
