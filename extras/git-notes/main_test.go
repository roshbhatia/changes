package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
)

func TestCreateAndListUseOnlyChangesRef(t *testing.T) {
	repository, request := gitFixture(t)
	runGitTest(t, repository, nil, "notes", "add", "-m", "default note", request.Head)
	before := runGitTest(t, repository, nil, "notes", "show", request.Head)

	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "manual", "Keep the return value")
	created, err := store.create(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Notes) != 1 || created.Notes[0].Source != providerName || created.Notes[0].Anchor != request.Note.Anchor {
		t.Fatalf("created notes = %+v", created.Notes)
	}
	if after := runGitTest(t, repository, nil, "notes", "show", request.Head); after != before {
		t.Fatalf("default notes ref changed: %q", after)
	}
	document := runGitTest(t, repository, nil, "notes", "--ref="+notesRef, "show", request.Head)
	if !strings.Contains(document, `"summary":"Keep the return value"`) || !strings.HasSuffix(document, "\n") {
		t.Fatalf("Changes note document = %q", document)
	}

	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 1 || listed.Notes[0] != created.Notes[0] {
		t.Fatalf("listed notes = %+v, created = %+v", listed.Notes, created.Notes)
	}
}

func TestSynchronizedNoteSurvivesDifferentDiffConfiguration(t *testing.T) {
	seed, comparison := gitFixture(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	runGitTest(t, root, nil, "clone", "--quiet", "--bare", seed, origin)
	one := filepath.Join(root, "one")
	two := filepath.Join(root, "two")
	runGitTest(t, root, nil, "clone", "--quiet", origin, one)
	runGitTest(t, root, nil, "clone", "--quiet", origin, two)
	for _, repository := range []string{one, two} {
		runGitTest(t, repository, nil, "config", "user.name", "Changes test")
		runGitTest(t, repository, nil, "config", "user.email", "changes@example.invalid")
	}
	for name, value := range map[string]string{
		"diff.algorithm": "minimal", "diff.context": "1", "diff.indentHeuristic": "true",
		"diff.interHunkContext": "9", "diff.mnemonicPrefix": "true",
	} {
		runGitTest(t, one, nil, "config", name, value)
	}
	for name, value := range map[string]string{
		"diff.algorithm": "histogram", "diff.context": "8", "diff.indentHeuristic": "false",
		"diff.interHunkContext": "0", "diff.mnemonicPrefix": "false",
	} {
		runGitTest(t, two, nil, "config", name, value)
	}
	identity := func(directory string) string {
		t.Helper()
		value, err := (source.Spec{
			Dir: directory, From: comparison.Base, To: comparison.Head,
		}).NoteDiff()
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
	}
	oneFingerprint := identity(one)
	twoFingerprint := identity(two)
	if oneFingerprint != twoFingerprint {
		t.Fatalf("note fingerprints differ: %s and %s", oneFingerprint, twoFingerprint)
	}

	request := comparison
	request.Directory = one
	request.Fingerprint = oneFingerprint
	request.Note = noteDraft(request, "sync", "Synchronized note")
	store := noteStore{git: commandGit{}}
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, one, nil, "push", "--quiet", "origin", notesRef)
	runGitTest(t, two, nil, "fetch", "--quiet", "origin", notesRef+":"+notesRef+"-incoming")
	runGitTest(t, two, nil, "notes", "--ref="+notesRef, "merge", "-s", "cat_sort_uniq", notesRef+"-incoming")

	request.Directory = two
	request.Fingerprint = twoFingerprint
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 1 || listed.Notes[0].Summary != "Synchronized note" {
		t.Fatalf("synchronized notes = %+v", listed.Notes)
	}
}

func TestCreateAndListRejectMismatchedCanonicalFingerprintBeforeLocking(t *testing.T) {
	repository, request := gitFixture(t)
	request.Fingerprint = strings.Repeat("f", 64)
	request.Note = noteDraft(request, "wrong-fingerprint", "Reject this note")
	store := noteStore{git: commandGit{}}
	for name, operation := range map[string]func() error{
		"create": func() error {
			_, err := store.create(request)
			return err
		},
		"list": func() error {
			_, err := store.list(request)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if err == nil || !strings.Contains(err.Error(), "canonical committed comparison") {
				t.Fatalf("%s error = %v", name, err)
			}
			if refExists(repository, notesRef) {
				t.Fatalf("%s created %s", name, notesRef)
			}
			common := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", "--git-common-dir"))
			if !filepath.IsAbs(common) {
				common = filepath.Join(repository, common)
			}
			if _, statErr := os.Stat(filepath.Join(common, "changes-notes.lock")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s reached the notes lock: %v", name, statErr)
			}
		})
	}
}

func TestSymbolicChangesRefIsRejected(t *testing.T) {
	for _, targetKind := range []string{"branch", "default-notes"} {
		t.Run(targetKind, func(t *testing.T) {
			repository, request := gitFixture(t)
			target := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", "HEAD"))
			if targetKind == "default-notes" {
				runGitTest(t, repository, nil, "notes", "add", "-m", "human note", request.Head)
				target = "refs/notes/commits"
			}
			before := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target))
			runGitTest(t, repository, nil, "symbolic-ref", notesRef, target)
			store := noteStore{git: commandGit{}}
			if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "must not be a symbolic ref") {
				t.Fatalf("list error = %v", err)
			}
			request.Note = noteDraft(request, "symbolic", "Symbolic")
			if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "must not be a symbolic ref") {
				t.Fatalf("create error = %v", err)
			}
			if after := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target)); after != before {
				t.Fatalf("symbolic target moved from %s to %s", before, after)
			}
			if resolved := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", notesRef)); resolved != target {
				t.Fatalf("symbolic ref target = %s", resolved)
			}
		})
	}
}

func TestDanglingSymbolicChangesRefIsRejected(t *testing.T) {
	repository, request := gitFixture(t)
	runGitTest(t, repository, nil, "symbolic-ref", notesRef, "refs/heads/missing")
	store := noteStore{git: commandGit{}}
	if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "must not be a symbolic ref") {
		t.Fatalf("list error = %v", err)
	}
	request.Note = noteDraft(request, "dangling", "Dangling")
	if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "must not be a symbolic ref") {
		t.Fatalf("create error = %v", err)
	}
	if target := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", notesRef)); target != "refs/heads/missing" {
		t.Fatalf("symbolic target = %q", target)
	}
}

func TestDescendantNotesRefDoesNotImpersonateChangesRef(t *testing.T) {
	repository, request := gitFixture(t)
	descendant := notesRef + "/archive"
	runGitTest(t, repository, nil, "update-ref", descendant, request.Head)
	store := noteStore{git: commandGit{}}
	listed, err := store.list(request)
	if err != nil || len(listed.Notes) != 0 {
		t.Fatalf("list with descendant ref = %+v, %v", listed.Notes, err)
	}
	before := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", descendant))
	request.Note = noteDraft(request, "exact-ref", "Exact ref")
	if _, err := store.create(request); err == nil {
		t.Fatal("provider created a parent ref over its descendant")
	}
	if after := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", descendant)); after != before {
		t.Fatalf("descendant ref moved from %s to %s", before, after)
	}
	if refExists(repository, notesRef) {
		t.Fatal("provider created the exact Changes notes ref despite the namespace conflict")
	}
}

func TestWorkingAndIndexComparisonsDoNotUseGitNotes(t *testing.T) {
	_, committed := gitFixture(t)
	store := noteStore{git: commandGit{}}
	for name, request := range map[string]provider.Request{
		"working": {Directory: committed.Directory, Files: []string{"main.go"}, Base: committed.Base, Fingerprint: "working"},
		"index":   {Directory: committed.Directory, Files: []string{"main.go"}, Base: committed.Base, Fingerprint: "index", Staged: true},
	} {
		t.Run(name, func(t *testing.T) {
			listed, err := store.list(request)
			if err != nil || len(listed.Notes) != 0 {
				t.Fatalf("list = %+v, %v", listed.Notes, err)
			}
			request.Note = noteDraft(request, "", "working note")
			request.Note.Anchor.Target = requestTarget(request)
			if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "committed comparisons") {
				t.Fatalf("create error = %v", err)
			}
		})
	}
}

func TestStableKeysAreScopedToTheExactComparison(t *testing.T) {
	_, first := gitFixture(t)
	store := noteStore{git: commandGit{}}
	first.Note = noteDraft(first, "review:same", "First comparison")
	createdFirst, err := store.create(first)
	if err != nil {
		t.Fatal(err)
	}

	second := first
	if err := os.WriteFile(filepath.Join(second.Directory, "main.go"), []byte("package main\n\nfunc value() int { return 3 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, second.Directory, nil, "add", "main.go")
	runGitTest(t, second.Directory, nil, "commit", "--quiet", "-m", "second head")
	second.Head = strings.TrimSpace(runGitTest(t, second.Directory, nil, "rev-parse", "HEAD"))
	second.To = second.Head
	comparison, err := (source.Spec{Dir: second.Directory, From: second.Base, To: second.Head}).NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	second.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(comparison)))
	second.Note = noteDraft(second, "review:same", "Second comparison")
	createdSecond, err := store.create(second)
	if err != nil {
		t.Fatal(err)
	}
	if createdFirst.Notes[0].ID == createdSecond.Notes[0].ID {
		t.Fatal("separate comparisons received the same stable note id")
	}
	for name, test := range map[string]struct {
		request provider.Request
		want    string
	}{
		"first":  {request: first, want: "First comparison"},
		"second": {request: second, want: "Second comparison"},
	} {
		request := test.request
		request.Note = nil
		listed, listErr := store.list(request)
		if listErr != nil || len(listed.Notes) != 1 || listed.Notes[0].Summary != test.want {
			t.Fatalf("%s comparison notes = %+v, %v", name, listed.Notes, listErr)
		}
	}
}

func TestComparisonRejectsNoncanonicalObjectIDs(t *testing.T) {
	_, request := gitFixture(t)
	request.Base = strings.ToUpper(request.Base)
	request.Head = strings.ToUpper(request.Head)
	request.Note = noteDraft(request, "uppercase", "Uppercase")
	store := noteStore{git: commandGit{}}
	if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "canonical full object id") {
		t.Fatalf("noncanonical comparison error = %v", err)
	}
}

func TestBatchCreateIsAtomicAndIdempotent(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	valid := *noteDraft(request, "generator:one", "First")
	invalid := *noteDraft(request, "generator:two", "Second")
	invalid.Anchor.Path = "../outside.go"
	request.Notes = []provider.NoteDraft{valid, invalid}
	if _, err := store.create(request); err == nil {
		t.Fatal("invalid batch was accepted")
	}
	if refExists(repository, notesRef) {
		t.Fatal("invalid batch created the notes ref")
	}

	second := *noteDraft(request, "generator:two", "Second")
	request.Notes = []provider.NoteDraft{valid, second}
	firstResponse, err := store.create(request)
	if err != nil {
		t.Fatal(err)
	}
	secondResponse, err := store.create(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstResponse.Notes) != 2 || len(secondResponse.Notes) != 2 {
		t.Fatalf("responses = %+v, %+v", firstResponse.Notes, secondResponse.Notes)
	}
	for index := range firstResponse.Notes {
		if firstResponse.Notes[index] != secondResponse.Notes[index] {
			t.Fatalf("retry changed note %d: %+v then %+v", index, firstResponse.Notes[index], secondResponse.Notes[index])
		}
	}
	request.Notes = nil
	listed, err := store.list(request)
	if err != nil || len(listed.Notes) != 2 {
		t.Fatalf("stored notes = %+v, %v", listed.Notes, err)
	}
}

func TestConcurrentCreatesPreserveEveryRecord(t *testing.T) {
	_, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	const count = 12
	errorsFound := make(chan error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			one := request
			one.Note = noteDraft(one, fmt.Sprintf("parallel:%02d", index), fmt.Sprintf("Note %02d", index))
			_, err := store.create(one)
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
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != count {
		t.Fatalf("stored notes = %d, want %d", len(listed.Notes), count)
	}
}

func TestExternalRefUpdateIsMergedBeforeCompareAndSwap(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "provider:a", "Provider A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}

	external, err := newStoredNote(*noteDraft(request, "external:b", "External B"))
	if err != nil {
		t.Fatal(err)
	}
	interleaved := &interleavingGit{next: commandGit{}}
	interleaved.beforeUpdate = func() {
		document := []byte(runGitTest(t, repository, nil, "notes", "--ref="+notesRef, "show", request.Head))
		records, decodeErr := decodeRecords(document)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		records = append(records, external)
		document, encodeErr := encodeRecords(records)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		writeNoteDocument(t, repository, request.Head, document)
	}
	store.git = interleaved
	request.Note = noteDraft(request, "provider:c", "Provider C")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 3 {
		t.Fatalf("interleaved notes = %+v", listed.Notes)
	}
}

func TestSymbolicTransitionBeforePublishFailsClosed(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "direct:a", "Direct A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	target := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", "HEAD"))
	before := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target))
	hook := &hookGit{next: commandGit{}, match: func(arguments []string) bool {
		return len(arguments) > 1 && arguments[0] == "update-ref" && arguments[1] == "--no-deref"
	}}
	hook.before = func() {
		runGitTest(t, repository, nil, "update-ref", "-d", notesRef)
		runGitTest(t, repository, nil, "symbolic-ref", notesRef, target)
	}
	store.git = hook
	request.Note = noteDraft(request, "direct:b", "Direct B")
	if _, err := store.create(request); err == nil {
		t.Fatal("create succeeded after the notes ref became symbolic")
	}
	if after := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target)); after != before {
		t.Fatalf("symbolic target moved from %s to %s", before, after)
	}
	if resolved := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", notesRef)); resolved != target {
		t.Fatalf("symbolic ref target = %s", resolved)
	}
}

func TestRefSnapshotSurvivesDirectToSymbolicTransition(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "direct:a", "Direct A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	target := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", "HEAD"))
	targetBefore := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target))
	hook := &afterHookGit{next: commandGit{}, match: func(arguments []string) bool {
		return len(arguments) > 0 && arguments[0] == "for-each-ref"
	}}
	hook.after = func() {
		runGitTest(t, repository, nil, "update-ref", "-d", notesRef)
		runGitTest(t, repository, nil, "symbolic-ref", notesRef, target)
	}
	store.git = hook
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 1 || listed.Notes[0].Summary != "Direct A" {
		t.Fatalf("notes after direct-to-symbolic transition = %+v", listed.Notes)
	}
	if after := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", target)); after != targetBefore {
		t.Fatalf("symbolic target moved from %s to %s", targetBefore, after)
	}
	if resolved := strings.TrimSpace(runGitTest(t, repository, nil, "symbolic-ref", notesRef)); resolved != target {
		t.Fatalf("symbolic ref target = %s", resolved)
	}
}

func TestRefSnapshotRetriesDanglingToDirectTransition(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "direct:a", "Direct A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	direct := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef))
	runGitTest(t, repository, nil, "symbolic-ref", notesRef, "refs/heads/missing")
	hook := &afterHookGit{next: commandGit{}, match: func(arguments []string) bool {
		return len(arguments) > 0 && arguments[0] == "for-each-ref"
	}}
	hook.after = func() {
		runGitTest(t, repository, nil, "update-ref", "--no-deref", notesRef, direct)
	}
	store.git = hook
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 1 || listed.Notes[0].Summary != "Direct A" {
		t.Fatalf("notes after dangling-to-direct transition = %+v", listed.Notes)
	}
	if resolved := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef)); resolved != direct {
		t.Fatalf("direct ref = %s, want %s", resolved, direct)
	}
}

func TestDirectTreeChangesRefIsRejected(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "tree:a", "Tree A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef+"^{tree}"))
	runGitTest(t, repository, nil, "update-ref", notesRef, tree)

	request.Note = nil
	if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "must point to a commit") {
		t.Fatalf("list error = %v", err)
	}
	request.Note = noteDraft(request, "tree:b", "Tree B")
	if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "must point to a commit") {
		t.Fatalf("create error = %v", err)
	}
	if after := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef)); after != tree {
		t.Fatalf("tree ref moved from %s to %s", tree, after)
	}
}

func TestSnapshotReadIgnoresABAMovement(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "raw:a", "Raw A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	refA := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef))
	recordB, err := newStoredNote(*noteDraft(request, "transient:b", "Transient B"))
	if err != nil {
		t.Fatal(err)
	}
	documentB, err := encodeRecords([]storedNote{recordB})
	if err != nil {
		t.Fatal(err)
	}
	writeNoteDocument(t, repository, request.Head, documentB)
	refB := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef))
	runGitTest(t, repository, nil, "update-ref", notesRef, refA)

	hook := &hookGit{next: commandGit{}, match: func(arguments []string) bool {
		return len(arguments) > 0 && arguments[0] == "ls-tree"
	}}
	hook.before = func() {
		runGitTest(t, repository, nil, "update-ref", notesRef, refB, refA)
		runGitTest(t, repository, nil, "update-ref", notesRef, refA, refB)
	}
	store.git = hook
	request.Note = noteDraft(request, "raw:c", "Raw C")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 2 || listed.Notes[0].Summary == "Transient B" || listed.Notes[1].Summary == "Transient B" {
		t.Fatalf("notes after ABA movement = %+v", listed.Notes)
	}
}

func TestLargeNotesTreeRemainsWritable(t *testing.T) {
	repository, request := gitFixture(t)
	blob := strings.TrimSpace(runGitTest(t, repository, []byte("unrelated\n"), "hash-object", "-w", "--stdin"))
	var input bytes.Buffer
	for index := 1; index <= 50000; index++ {
		fmt.Fprintf(&input, "100644 blob %s\t%040x\n", blob, index)
	}
	tree := strings.TrimSpace(runGitTest(t, repository, input.Bytes(), "mktree"))
	commit := strings.TrimSpace(runGitTest(t, repository, nil, "commit-tree", tree, "-m", "large notes tree"))
	runGitTest(t, repository, nil, "update-ref", notesRef, commit)
	request.Note = noteDraft(request, "large-tree", "Large tree")
	store := noteStore{git: commandGit{}}
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	listed, err := store.list(request)
	if err != nil || len(listed.Notes) != 1 || listed.Notes[0].Summary != "Large tree" {
		t.Fatalf("large-tree notes = %+v, %v", listed.Notes, err)
	}
}

func TestCorruptAndOversizedDocumentsFailWithoutReplacement(t *testing.T) {
	for name, document := range map[string][]byte{
		"malformed": []byte("not json\n"),
		"oversized": bytes.Repeat([]byte("x"), maxStoreSize+1),
	} {
		t.Run(name, func(t *testing.T) {
			repository, request := gitFixture(t)
			writeNoteDocument(t, repository, request.Head, document)
			before := noteObject(t, repository, request.Head)
			request.Note = noteDraft(request, "valid", "Valid")
			store := noteStore{git: commandGit{}}
			if _, err := store.create(request); err == nil {
				t.Fatal("corrupt document was accepted")
			}
			if after := noteObject(t, repository, request.Head); after != before {
				t.Fatalf("failed create replaced %s with %s", before, after)
			}
		})
	}
}

func TestStoredRecordIDsMustMatchTheirCanonicalForm(t *testing.T) {
	for name, mutate := range map[string]func(*storedNote){
		"keyed": func(record *storedNote) {
			record.ID = strings.Repeat("0", 32)
		},
		"random": func(record *storedNote) {
			record.Key = ""
			record.ID = "ABC"
		},
	} {
		t.Run(name, func(t *testing.T) {
			repository, request := gitFixture(t)
			record, err := newStoredNote(*noteDraft(request, "stable", "Stable"))
			if err != nil {
				t.Fatal(err)
			}
			mutate(&record)
			line, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeNoteDocument(t, repository, request.Head, append(line, '\n'))
			store := noteStore{git: commandGit{}}
			if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "invalid") {
				t.Fatalf("stored id error = %v", err)
			}
		})
	}
}

func TestStoredComparisonRequiresCanonicalCommitIDs(t *testing.T) {
	repository, request := gitFixture(t)
	record, err := newStoredNote(*noteDraft(request, "stored-base", "Stored base"))
	if err != nil {
		t.Fatal(err)
	}
	record.Anchor.Base = "HEAD"
	record.ID = keyedNoteID(record.Anchor, record.Key)
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeNoteDocument(t, repository, request.Head, append(line, '\n'))
	before := noteObject(t, repository, request.Head)
	store := noteStore{git: commandGit{}}
	if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "canonical full object id") {
		t.Fatalf("stored comparison error = %v", err)
	}
	request.Note = noteDraft(request, "new", "New")
	if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "canonical full object id") {
		t.Fatalf("stored comparison create error = %v", err)
	}
	if after := noteObject(t, repository, request.Head); after != before {
		t.Fatalf("failed create replaced %s with %s", before, after)
	}
}

func TestRecordForAnotherHeadFailsWithoutReplacement(t *testing.T) {
	repository, request := gitFixture(t)
	record, err := newStoredNote(*noteDraft(request, "wrong-head", "Wrong head"))
	if err != nil {
		t.Fatal(err)
	}
	record.Anchor.Head = request.Base
	record.ID = keyedNoteID(record.Anchor, record.Key)
	document, err := encodeRecords([]storedNote{record})
	if err != nil {
		t.Fatal(err)
	}
	writeNoteDocument(t, repository, request.Head, document)
	before := noteObject(t, repository, request.Head)
	store := noteStore{git: commandGit{}}
	if _, err := store.list(request); err == nil || !strings.Contains(err.Error(), "record for head") {
		t.Fatalf("list error = %v", err)
	}
	request.Note = noteDraft(request, "valid", "Valid")
	if _, err := store.create(request); err == nil || !strings.Contains(err.Error(), "record for head") {
		t.Fatalf("create error = %v", err)
	}
	if after := noteObject(t, repository, request.Head); after != before {
		t.Fatalf("failed create replaced %s with %s", before, after)
	}
}

func TestListWaitsForProviderWriteLock(t *testing.T) {
	_, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	release, err := store.lock(request.Directory)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := store.list(request)
		finished <- err
	}()
	select {
	case err := <-finished:
		release()
		t.Fatalf("list completed while write lock was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("list did not complete after write lock release")
	}
}

func TestMergedRecordsRejectConflictingKeys(t *testing.T) {
	_, request := gitFixture(t)
	one, err := newStoredNote(*noteDraft(request, "same", "One"))
	if err != nil {
		t.Fatal(err)
	}
	two, err := newStoredNote(*noteDraft(request, "same", "Two"))
	if err != nil {
		t.Fatal(err)
	}
	lines := make([][]byte, 2)
	lines[0], _ = jsonMarshal(one)
	lines[1], _ = jsonMarshal(two)
	if bytes.Compare(lines[0], lines[1]) > 0 {
		lines[0], lines[1] = lines[1], lines[0]
	}
	document := append(bytes.Join(lines, []byte{'\n'}), '\n')
	if _, err := decodeRecords(document); err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("decode error = %v", err)
	}
}

func TestProviderNeverRunsRemoteOrConfigurationCommands(t *testing.T) {
	repository, request := gitFixture(t)
	runGitTest(t, repository, nil, "remote", "add", "origin", "ssh://invalid.example/repository")
	recorder := &recordingGit{next: commandGit{}}
	store := noteStore{git: recorder}
	request.Note = noteDraft(request, "audit", "Audit")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	if _, err := store.list(request); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range recorder.commands {
		forbidden := map[string]bool{"fetch": true, "merge": true, "pull": true, "push": true, "config": true}
		if len(arguments) > 0 && forbidden[arguments[0]] {
			t.Fatalf("provider ran forbidden Git command: %v", arguments)
		}
	}
}

func TestProviderGitCommandsDoNotRunRepositoryHooks(t *testing.T) {
	repository, request := gitFixture(t)
	hooks := filepath.Join(repository, ".git", "hooks")
	marker := filepath.Join(t.TempDir(), "reference-transaction-ran")
	hook := filepath.Join(hooks, "reference-transaction")
	content := []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$1\" >> \"${CHANGES_HOOK_MARKER:?}\"\n")
	if err := os.WriteFile(hook, content, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, nil, "config", "core.hooksPath", hooks)
	t.Setenv("CHANGES_HOOK_MARKER", marker)
	runGitTest(t, repository, nil, "update-ref", "refs/test/hook-probe", request.Head)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("reference-transaction hook probe: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "hooks", "Hooks")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider ran reference-transaction hook: %v", err)
	}
}

func TestProviderGitCommandsDoNotRunConfiguredFSMonitor(t *testing.T) {
	repository, request := gitFixture(t)
	hook := filepath.Join(t.TempDir(), "fsmonitor")
	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	content := []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$*\" >> \"${CHANGES_FSMONITOR_MARKER:?}\"\nprintf 'token\\000'\n")
	if err := os.WriteFile(hook, content, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, nil, "config", "core.fsmonitor", hook)
	runGitTest(t, repository, nil, "config", "core.fsmonitorHookVersion", "2")
	t.Setenv("CHANGES_FSMONITOR_MARKER", marker)

	runGitTest(t, repository, nil, "ls-files")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("configured fsmonitor probe: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "fsmonitor", "FSMonitor")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	if _, err := store.list(request); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider ran configured fsmonitor: %v", err)
	}
}

func TestGitEnvironmentDisablesLazyFetch(t *testing.T) {
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	environment := gitEnvironment(map[string]string{"GIT_NO_LAZY_FETCH": "also-wrong"})
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	if values["GIT_NO_LAZY_FETCH"] != "1" {
		t.Fatalf("GIT_NO_LAZY_FETCH = %q", values["GIT_NO_LAZY_FETCH"])
	}
	if values["GIT_NO_REPLACE_OBJECTS"] != "1" {
		t.Fatalf("GIT_NO_REPLACE_OBJECTS = %q", values["GIT_NO_REPLACE_OBJECTS"])
	}
	if _, found := values["GIT_REPLACE_REF_BASE"]; found {
		t.Fatal("GIT_REPLACE_REF_BASE was preserved")
	}
}

func TestReplacementRefsCannotChangeNotesSnapshot(t *testing.T) {
	repository, request := gitFixture(t)
	store := noteStore{git: commandGit{}}
	request.Note = noteDraft(request, "raw:a", "Raw A")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	rawRef := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef))
	replacement, err := newStoredNote(*noteDraft(request, "replacement:b", "Replacement B"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := encodeRecords([]storedNote{replacement})
	if err != nil {
		t.Fatal(err)
	}
	writeNoteDocument(t, repository, request.Head, document)
	replacementRef := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", notesRef))
	runGitTest(t, repository, nil, "update-ref", notesRef, rawRef)
	runGitTest(t, repository, nil, "update-ref", "refs/replace/"+rawRef, replacementRef)
	t.Setenv("GIT_REPLACE_REF_BASE", "refs/replace/")
	request.Note = noteDraft(request, "raw:c", "Raw C")
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	request.Note = nil
	listed, err := store.list(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 2 || listed.Notes[0].Summary == "Replacement B" || listed.Notes[1].Summary == "Replacement B" {
		t.Fatalf("notes with replacement ref = %+v", listed.Notes)
	}
}

func TestInheritedGitEnvironmentCannotRedirectRepository(t *testing.T) {
	repository, request := gitFixture(t)
	redirected := filepath.Join(t.TempDir(), "redirected")
	runGitTest(t, t.TempDir(), nil, "clone", "--quiet", repository, redirected)
	runGitTest(t, redirected, nil, "config", "user.name", "Changes test")
	runGitTest(t, redirected, nil, "config", "user.email", "changes@example.invalid")
	t.Setenv("GIT_DIR", filepath.Join(redirected, ".git"))
	t.Setenv("GIT_WORK_TREE", redirected)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(redirected, ".git", "index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(redirected, ".git", "objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(redirected, ".git", "objects"))
	t.Setenv("GIT_COMMON_DIR", filepath.Join(redirected, ".git"))
	t.Setenv("GIT_NAMESPACE", "redirected")
	request.Note = noteDraft(request, "environment", "Environment")
	store := noteStore{git: commandGit{}}
	if _, err := store.create(request); err != nil {
		t.Fatal(err)
	}
	if !refExists(repository, notesRef) {
		t.Fatal("selected repository has no Changes notes ref")
	}
	if refExists(redirected, notesRef) {
		t.Fatal("inherited Git environment redirected the Changes notes ref")
	}
}

func TestExplicitFetchMergeAndPushPreserveIndependentNotes(t *testing.T) {
	source, request := gitFixture(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	cloneOne := filepath.Join(root, "one")
	cloneTwo := filepath.Join(root, "two")
	runGitTest(t, root, nil, "clone", "--quiet", "--bare", source, origin)
	runGitTest(t, root, nil, "clone", "--quiet", origin, cloneOne)
	runGitTest(t, root, nil, "clone", "--quiet", origin, cloneTwo)
	for _, clone := range []string{cloneOne, cloneTwo} {
		runGitTest(t, clone, nil, "config", "user.name", "Changes test")
		runGitTest(t, clone, nil, "config", "user.email", "changes@example.invalid")
	}

	store := noteStore{git: commandGit{}}
	one := request
	one.Directory = cloneOne
	one.Note = noteDraft(one, "clone:one", "First clone")
	if _, err := store.create(one); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, cloneOne, nil, "push", "--quiet", "origin", "HEAD:refs/heads/main")
	if refExists(origin, notesRef) {
		t.Fatal("ordinary branch push copied the Changes notes ref")
	}

	two := request
	two.Directory = cloneTwo
	two.Note = noteDraft(two, "clone:two", "Second clone")
	if _, err := store.create(two); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, cloneOne, nil, "push", "--quiet", "origin", notesRef)
	runGitTest(t, cloneTwo, nil, "fetch", "--quiet", "origin", notesRef+":"+notesRef+"-incoming")
	runGitTest(t, cloneTwo, nil, "notes", "--ref="+notesRef, "merge", "-s", "cat_sort_uniq", notesRef+"-incoming")

	two.Note = nil
	listed, err := store.list(two)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Notes) != 2 {
		t.Fatalf("merged notes = %+v", listed.Notes)
	}
}

func TestValidationDoesNotRequireARepository(t *testing.T) {
	request := provider.Request{
		Validation: true, Fingerprint: "provider-validation",
		Note: &provider.NoteDraft{
			Summary: "Explain the ready change", Author: "provider-validation", Origin: provider.NoteOriginAgent,
			Anchor: provider.NoteAnchor{
				Path: "main.ts", Side: provider.NoteSideRight, Line: 2, Context: "return ready()",
				Fingerprint: "provider-validation", Target: provider.NoteTargetWorking,
			},
		},
	}
	store := noteStore{git: commandGit{}}
	created, err := store.create(request)
	if err != nil || len(created.Notes) != 1 || created.Notes[0].Anchor != request.Note.Anchor {
		t.Fatalf("validation create = %+v, %v", created.Notes, err)
	}
	listed, err := store.list(request)
	if err != nil || len(listed.Notes) != 1 || listed.Notes[0].Anchor.Target != provider.NoteTargetWorking {
		t.Fatalf("validation list = %+v, %v", listed.Notes, err)
	}
}

type recordingGit struct {
	next     gitRunner
	commands [][]string
}

func (runner *recordingGit) Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error) {
	runner.commands = append(runner.commands, append([]string(nil), arguments...))
	return runner.next.Run(directory, input, limit, environment, arguments...)
}

type interleavingGit struct {
	next         gitRunner
	beforeUpdate func()
	once         sync.Once
}

func (runner *interleavingGit) Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error) {
	if len(arguments) > 0 && arguments[0] == "update-ref" {
		runner.once.Do(runner.beforeUpdate)
	}
	return runner.next.Run(directory, input, limit, environment, arguments...)
}

type hookGit struct {
	next   gitRunner
	match  func([]string) bool
	before func()
	once   sync.Once
}

func (runner *hookGit) Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error) {
	if runner.match(arguments) {
		runner.once.Do(runner.before)
	}
	return runner.next.Run(directory, input, limit, environment, arguments...)
}

type afterHookGit struct {
	next  gitRunner
	match func([]string) bool
	after func()
	once  sync.Once
}

func (runner *afterHookGit) Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error) {
	output, err := runner.next.Run(directory, input, limit, environment, arguments...)
	if runner.match(arguments) {
		runner.once.Do(runner.after)
	}
	return output, err
}

func gitFixture(t *testing.T) (string, provider.Request) {
	t.Helper()
	repository := t.TempDir()
	runGitTest(t, repository, nil, "init", "--quiet")
	runGitTest(t, repository, nil, "config", "user.name", "Changes test")
	runGitTest(t, repository, nil, "config", "user.email", "changes@example.invalid")
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc value() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, nil, "add", "main.go")
	runGitTest(t, repository, nil, "commit", "--quiet", "-m", "base")
	base := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", "HEAD"))
	if err := os.WriteFile(path, []byte("package main\n\nfunc value() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, nil, "add", "main.go")
	runGitTest(t, repository, nil, "commit", "--quiet", "-m", "head")
	head := strings.TrimSpace(runGitTest(t, repository, nil, "rev-parse", "HEAD"))
	comparison, err := (source.Spec{Dir: repository, From: base, To: head}).NoteDiff()
	if err != nil {
		t.Fatal(err)
	}
	return repository, provider.Request{
		Directory: repository, Files: []string{"main.go"}, From: base, To: head,
		Base: base, Head: head, Fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(comparison))),
	}
}

func noteDraft(request provider.Request, key, summary string) *provider.NoteDraft {
	return &provider.NoteDraft{
		Key: key, Summary: summary, Rationale: "Because callers rely on it.",
		Author: "agent", Origin: provider.NoteOriginAgent, Session: "session-1",
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 3,
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Context: "func value() int { return 2 }", Target: provider.NoteTargetCommits,
		},
	}
}

func runGitTest(t *testing.T, directory string, input []byte, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = gitEnvironment(nil)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func refExists(repository, ref string) bool {
	command := exec.Command("git", "-C", repository, "show-ref", "--verify", "--quiet", ref)
	command.Env = gitEnvironment(nil)
	return command.Run() == nil
}

func writeNoteDocument(t *testing.T, repository, head string, document []byte) {
	t.Helper()
	object := strings.TrimSpace(runGitTest(t, repository, document, "hash-object", "-w", "--stdin"))
	runGitTest(t, repository, nil, "notes", "--ref="+notesRef, "add", "-f", "-C", object, head)
}

func noteObject(t *testing.T, repository, head string) string {
	t.Helper()
	fields := strings.Fields(runGitTest(t, repository, nil, "notes", "--ref="+notesRef, "list", head))
	if len(fields) != 1 {
		t.Fatalf("note mapping = %v", fields)
	}
	return fields[0]
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}
