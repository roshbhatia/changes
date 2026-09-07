package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
)

const (
	providerName    = "git-notes"
	notesRef        = "refs/notes/changes"
	maxStoreSize    = 4 << 20
	maxGitOutput    = 64 << 10
	lockAttempts    = 1000
	lockInterval    = 10 * time.Millisecond
	refReadAttempts = 8
	updateAttempts  = 8
)

var errNotesRefMoved = errors.New("Git notes ref changed during update")

var blockedGitEnvironment = map[string]bool{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_NAMESPACE":                    true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_QUARANTINE_PATH":              true,
	"GIT_REPLACE_REF_BASE":             true,
	"GIT_SHALLOW_FILE":                 true,
	"GIT_WORK_TREE":                    true,
}

type storedNote struct {
	Version    int                     `json:"version"`
	ID         string                  `json:"id"`
	Key        string                  `json:"key,omitempty"`
	Summary    string                  `json:"summary"`
	Rationale  string                  `json:"rationale,omitempty"`
	Author     string                  `json:"author"`
	Origin     string                  `json:"origin"`
	Session    string                  `json:"session,omitempty"`
	Provenance provider.NoteProvenance `json:"provenance,omitempty"`
	Anchor     provider.NoteAnchor     `json:"anchor"`
}

type gitRunner interface {
	Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error)
}

type commandGit struct{}

type commandError struct {
	Arguments []string
	Code      int
	Stderr    string
}

func (err *commandError) Error() string {
	detail := strings.TrimSpace(err.Stderr)
	if detail == "" {
		detail = fmt.Sprintf("exit status %d", err.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(err.Arguments, " "), detail)
}

type cappedWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (writer *cappedWriter) Write(data []byte) (int, error) {
	available := writer.limit - writer.buffer.Len()
	if available > 0 {
		if available > len(data) {
			available = len(data)
		}
		_, _ = writer.buffer.Write(data[:available])
	}
	return len(data), nil
}

func (commandGit) Run(directory string, input []byte, limit int64, environment map[string]string, arguments ...string) ([]byte, error) {
	argv := append([]string{
		"--no-pager", "-C", directory,
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
	}, arguments...)
	command := exec.Command("git", argv...)
	command.Env = gitEnvironment(environment)
	command.Stdin = bytes.NewReader(input)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &cappedWriter{limit: maxGitOutput}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, readErr
	}
	if int64(len(output)) > limit {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("git output exceeds %d bytes", limit)
	}
	waitErr := command.Wait()
	if waitErr == nil {
		return output, nil
	}
	code := -1
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code = exitErr.ExitCode()
	}
	return output, &commandError{Arguments: arguments, Code: code, Stderr: stderr.buffer.String()}
}

func gitEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found && !blockedGitEnvironment[name] {
			values[name] = value
		}
	}
	for name, value := range overrides {
		values[name] = value
	}
	values["GIT_NO_LAZY_FETCH"] = "1"
	values["GIT_NO_REPLACE_OBJECTS"] = "1"
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

type noteStore struct {
	git gitRunner
}

func main() {
	request, err := decodeRequest(os.Stdin)
	if err != nil {
		die("decode request: %v", err)
	}
	if request.Version != provider.ProtocolVersion {
		die("version must be %q", provider.ProtocolVersion)
	}
	store := noteStore{git: commandGit{}}
	var response provider.Response
	switch request.Action {
	case provider.ActionNotes:
		response, err = store.list(request)
	case provider.ActionNotesCreate:
		response, err = store.create(request)
	default:
		err = fmt.Errorf("action must be %q or %q", provider.ActionNotes, provider.ActionNotesCreate)
	}
	if err != nil {
		die("%v", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		die("encode response: %v", err)
	}
}

func decodeRequest(reader io.Reader) (provider.Request, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxStoreSize+1))
	if err != nil {
		return provider.Request{}, err
	}
	if len(data) > maxStoreSize {
		return provider.Request{}, fmt.Errorf("request exceeds %d bytes", maxStoreSize)
	}
	request := provider.Request{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return provider.Request{}, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return provider.Request{}, errors.New("request must contain one JSON value")
	}
	return request, nil
}

func (store noteStore) list(request provider.Request) (provider.Response, error) {
	if request.Validation {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{validationNote()}}, nil
	}
	if requestTarget(request) != provider.NoteTargetCommits {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
	}
	if err := store.validateComparison(request); err != nil {
		return provider.Response{}, err
	}
	release, err := store.lock(request.Directory)
	if err != nil {
		return provider.Response{}, err
	}
	defer release()
	snapshot, err := store.read(request.Directory, request.Head)
	if err != nil {
		return provider.Response{}, err
	}
	allowed, err := allowedPaths(request.Files)
	if err != nil {
		return provider.Response{}, err
	}
	notes := make([]provider.Note, 0, len(snapshot.records))
	for _, record := range snapshot.records {
		if !sameComparison(record.Anchor, request) {
			continue
		}
		if len(allowed) > 0 && !allowed[record.Anchor.Path] {
			continue
		}
		notes = append(notes, normalize(record, request))
	}
	return provider.Response{Version: provider.ProtocolVersion, Notes: notes}, nil
}

func (store noteStore) create(request provider.Request) (provider.Response, error) {
	if request.Validation {
		return validationCreateResponse(request)
	}
	if requestTarget(request) != provider.NoteTargetCommits {
		return provider.Response{}, errors.New("git-notes stores only committed comparisons; use --commit or --from with --to")
	}
	if err := store.validateComparison(request); err != nil {
		return provider.Response{}, err
	}
	drafts := request.Notes
	if request.Note != nil {
		drafts = []provider.NoteDraft{*request.Note}
	}
	if len(drafts) == 0 {
		return provider.Response{}, errors.New("create requires note or notes")
	}
	release, err := store.lock(request.Directory)
	if err != nil {
		return provider.Response{}, err
	}
	defer release()
	for _, draft := range drafts {
		if err := validateDraft(draft, request); err != nil {
			return provider.Response{}, err
		}
	}
	for range updateAttempts {
		snapshot, err := store.read(request.Directory, request.Head)
		if err != nil {
			return provider.Response{}, err
		}
		records := append([]storedNote(nil), snapshot.records...)
		byKey := make(map[string]storedNote, len(records))
		for _, record := range records {
			if record.Key != "" {
				byKey[comparisonNoteKey(record.Anchor, record.Key)] = record
			}
		}
		created := make([]provider.Note, 0, len(drafts))
		changed := false
		for _, draft := range drafts {
			key := comparisonNoteKey(draft.Anchor, draft.Key)
			if existing, ok := byKey[key]; draft.Key != "" && ok {
				if !sameDraft(existing, draft) {
					return provider.Response{}, fmt.Errorf("note key %q already names different content", draft.Key)
				}
				created = append(created, normalize(existing, request))
				continue
			}
			record, err := newStoredNote(draft)
			if err != nil {
				return provider.Response{}, err
			}
			records = append(records, record)
			if record.Key != "" {
				byKey[key] = record
			}
			created = append(created, normalize(record, request))
			changed = true
		}
		if !changed {
			return provider.Response{Version: provider.ProtocolVersion, Notes: created}, nil
		}
		if err := store.publish(request.Directory, request.Head, snapshot.ref, records); err != nil {
			if errors.Is(err, errNotesRefMoved) {
				continue
			}
			return provider.Response{}, err
		}
		return provider.Response{Version: provider.ProtocolVersion, Notes: created}, nil
	}
	return provider.Response{}, errors.New("Git notes ref kept changing; retry the note creation")
}

func (store noteStore) validateComparison(request provider.Request) error {
	if strings.TrimSpace(request.Base) == "" || strings.TrimSpace(request.Head) == "" || strings.TrimSpace(request.Fingerprint) == "" {
		return errors.New("git-notes requires exact base, head, and fingerprint values")
	}
	for _, endpoint := range []struct {
		name     string
		revision string
	}{
		{name: "base", revision: request.Base},
		{name: "head", revision: request.Head},
	} {
		if err := store.validateCanonicalCommit(request.Directory, endpoint.name, endpoint.revision); err != nil {
			return err
		}
	}
	comparison, err := (source.Spec{
		Dir: request.Directory, From: request.Base, To: request.Head,
	}).NoteDiff()
	if err != nil {
		return fmt.Errorf("identify committed comparison: %w", err)
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(comparison)))
	if request.Fingerprint != expected {
		return errors.New("fingerprint must match the canonical committed comparison")
	}
	return nil
}

func (store noteStore) validateCanonicalCommit(directory, name, revision string) error {
	output, err := store.git.Run(directory, nil, maxGitOutput, nil, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve %s commit: %w", name, err)
	}
	if strings.TrimSpace(string(output)) != revision {
		return fmt.Errorf("%s commit must be its canonical full object id", name)
	}
	return nil
}

func validateDraft(draft provider.NoteDraft, request provider.Request) error {
	if err := provider.ValidateNoteAnchor(draft.Anchor); err != nil {
		return err
	}
	if !sameComparison(draft.Anchor, request) {
		return errors.New("note anchor must exactly match the committed comparison")
	}
	if strings.TrimSpace(cleanOneLine(draft.Summary)) == "" || strings.TrimSpace(cleanOneLine(draft.Author)) == "" {
		return errors.New("summary and author must remain non-empty after removing control bytes")
	}
	if draft.Origin != provider.NoteOriginAgent && draft.Origin != provider.NoteOriginUser {
		return errors.New("note origin must be agent or user")
	}
	if strings.IndexFunc(draft.Key, unicode.IsControl) >= 0 || len([]rune(draft.Key)) > 200 {
		return errors.New("note key must be at most 200 characters without control bytes")
	}
	return nil
}

func newStoredNote(draft provider.NoteDraft) (storedNote, error) {
	id := ""
	if draft.Key != "" {
		id = keyedNoteID(draft.Anchor, draft.Key)
	} else {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return storedNote{}, err
		}
		id = hex.EncodeToString(raw)
	}
	return storedNote{
		Version: 1, ID: id, Key: draft.Key,
		Summary: strings.TrimSpace(cleanOneLine(draft.Summary)), Rationale: cleanText(draft.Rationale),
		Author: strings.TrimSpace(cleanOneLine(draft.Author)), Origin: draft.Origin,
		Session: cleanOneLine(draft.Session), Provenance: draft.Provenance, Anchor: draft.Anchor,
	}, nil
}

type noteSnapshot struct {
	ref     string
	records []storedNote
}

func (store noteStore) read(directory, head string) (noteSnapshot, error) {
	ref, err := store.currentRef(directory)
	if err != nil {
		return noteSnapshot{}, err
	}
	if ref == "" {
		return noteSnapshot{records: []storedNote{}}, nil
	}
	object, err := store.noteObject(directory, ref, head)
	if err != nil {
		return noteSnapshot{}, err
	}
	if object == "" {
		return noteSnapshot{ref: ref, records: []storedNote{}}, nil
	}
	data, err := store.git.Run(directory, nil, maxStoreSize, nil, "cat-file", "blob", object)
	if err != nil {
		return noteSnapshot{}, fmt.Errorf("read git note document: %w", err)
	}
	records, err := decodeRecords(data)
	if err != nil {
		return noteSnapshot{}, err
	}
	for _, record := range records {
		if record.Anchor.Head != head {
			return noteSnapshot{}, fmt.Errorf("git note document contains a record for head %s instead of %s", record.Anchor.Head, head)
		}
	}
	if err := store.validateStoredCommits(directory, head, records); err != nil {
		return noteSnapshot{}, err
	}
	return noteSnapshot{ref: ref, records: records}, nil
}

func (store noteStore) validateStoredCommits(directory, head string, records []storedNote) error {
	commits := map[string]bool{head: true}
	for _, record := range records {
		commits[record.Anchor.Base] = true
		commits[record.Anchor.Head] = true
	}
	values := make([]string, 0, len(commits))
	for revision := range commits {
		if len(revision) != len(head) || revision != strings.ToLower(revision) {
			return fmt.Errorf("git note document: stored commit must be its canonical full object id")
		}
		decoded, err := hex.DecodeString(revision)
		if err != nil || len(decoded)*2 != len(revision) {
			return fmt.Errorf("git note document: stored commit must be its canonical full object id")
		}
		values = append(values, revision)
	}
	sort.Strings(values)
	input := []byte(strings.Join(values, "\n") + "\n")
	output, err := store.git.Run(
		directory, input, maxStoreSize, nil,
		"cat-file", "--batch-check=%(objectname) %(objecttype)",
	)
	if err != nil {
		return fmt.Errorf("validate stored commits: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != len(values) {
		return errors.New("git note document: Git returned incomplete stored commit metadata")
	}
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != values[index] || fields[1] != "commit" {
			return fmt.Errorf("git note document: stored object %s is not a commit", values[index])
		}
	}
	return nil
}

func (store noteStore) noteObject(directory, ref, head string) (string, error) {
	arguments := []string{"ls-tree", "-r", "-z", "--full-tree", ref, "--"}
	arguments = append(arguments, notePaths(head)...)
	output, err := store.git.Run(directory, nil, maxGitOutput, nil, arguments...)
	if err != nil {
		return "", fmt.Errorf("find Git note mapping: %w", err)
	}
	object := ""
	for _, entry := range bytes.Split(output, []byte{0}) {
		tab := bytes.IndexByte(entry, '\t')
		if tab < 0 || strings.ReplaceAll(string(entry[tab+1:]), "/", "") != head {
			continue
		}
		fields := strings.Fields(string(entry[:tab]))
		if len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" || fields[2] == "" {
			return "", errors.New("Git notes tree contains an invalid object mapping")
		}
		if object != "" {
			return "", errors.New("Git notes tree contains duplicate object mappings")
		}
		object = fields[2]
	}
	return object, nil
}

func notePaths(object string) []string {
	paths := []string{object}
	for prefix := 2; prefix < len(object); prefix += 2 {
		parts := make([]string, 0, prefix/2+1)
		for index := 0; index < prefix; index += 2 {
			parts = append(parts, object[index:index+2])
		}
		parts = append(parts, object[prefix:])
		paths = append(paths, strings.Join(parts, "/"))
	}
	return paths
}

func (store noteStore) currentRef(directory string) (string, error) {
	for range refReadAttempts {
		output, err := store.git.Run(
			directory, nil, maxGitOutput, nil,
			"for-each-ref", "--count=2", "--sort=refname", "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(symref)", notesRef,
		)
		if err != nil {
			return "", fmt.Errorf("inspect Git notes ref: %w", err)
		}
		for _, line := range bytes.Split(bytes.TrimSuffix(output, []byte{'\n'}), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			fields := bytes.Split(line, []byte{0})
			if len(fields) != 4 {
				return "", errors.New("Git notes ref returned invalid metadata")
			}
			if string(fields[0]) != notesRef {
				continue
			}
			if len(fields[3]) != 0 {
				return "", fmt.Errorf("%s must not be a symbolic ref", notesRef)
			}
			if string(fields[2]) != "commit" {
				return "", fmt.Errorf("%s must point to a commit", notesRef)
			}
			ref := string(fields[1])
			if ref == "" || strings.ContainsAny(ref, " \t\r\n") {
				return "", errors.New("Git notes ref returned invalid metadata")
			}
			return ref, nil
		}

		if _, err := store.git.Run(directory, nil, maxGitOutput, nil, "show-ref", "--exists", notesRef); commandFailedWith(err, 2) {
			return "", nil
		} else if err != nil {
			return "", fmt.Errorf("inspect Git notes ref existence: %w", err)
		}
		if _, err := store.git.Run(directory, nil, maxGitOutput, nil, "symbolic-ref", "--quiet", notesRef); err == nil {
			return "", fmt.Errorf("%s must not be a symbolic ref", notesRef)
		} else if !commandFailedWith(err, 1) {
			return "", fmt.Errorf("inspect dangling Git notes ref: %w", err)
		}
	}
	return "", errors.New("Git notes ref changed during inspection")
}

func commandFailedWith(err error, code int) bool {
	var commandErr *commandError
	return errors.As(err, &commandErr) && commandErr.Code == code
}

func decodeRecords(data []byte) ([]storedNote, error) {
	if len(data) > maxStoreSize {
		return nil, fmt.Errorf("git note document exceeds %d bytes", maxStoreSize)
	}
	if len(data) == 0 {
		return []storedNote{}, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("git note document must end with a newline")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	records := make([]storedNote, 0, len(lines))
	seenIDs := make(map[string]bool, len(lines))
	seenKeys := make(map[string]bool, len(lines))
	previous := []byte(nil)
	for _, line := range lines {
		if len(line) == 0 {
			return nil, errors.New("git note document contains a blank record")
		}
		if previous != nil && bytes.Compare(previous, line) >= 0 {
			return nil, errors.New("git note records must be sorted and unique")
		}
		var record storedNote
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("git note document contains malformed JSON: %w", err)
		}
		var extra json.RawMessage
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, errors.New("git note record contains trailing data")
		}
		canonical, err := json.Marshal(record)
		if err != nil || !bytes.Equal(canonical, line) {
			return nil, errors.New("git note record is not canonical JSON")
		}
		if err := validateStored(record); err != nil {
			return nil, err
		}
		if record.Key != "" {
			key := comparisonNoteKey(record.Anchor, record.Key)
			if seenKeys[key] {
				return nil, fmt.Errorf("git note document contains duplicate key %q", record.Key)
			}
			seenKeys[key] = true
		}
		if seenIDs[record.ID] {
			return nil, fmt.Errorf("git note document contains duplicate id %q", record.ID)
		}
		seenIDs[record.ID] = true
		records = append(records, record)
		previous = line
	}
	return records, nil
}

func validateStored(record storedNote) error {
	if record.Version != 1 {
		return errors.New("git note document contains an invalid record version")
	}
	if strings.TrimSpace(cleanOneLine(record.Summary)) == "" || strings.TrimSpace(cleanOneLine(record.Author)) == "" {
		return errors.New("git note document contains empty content")
	}
	if record.Summary != strings.TrimSpace(cleanOneLine(record.Summary)) || record.Author != strings.TrimSpace(cleanOneLine(record.Author)) ||
		record.Rationale != cleanText(record.Rationale) || record.Session != cleanOneLine(record.Session) {
		return errors.New("git note document contains unsafe text")
	}
	if record.Origin != provider.NoteOriginAgent && record.Origin != provider.NoteOriginUser {
		return errors.New("git note document contains an invalid origin")
	}
	if strings.IndexFunc(record.Key, unicode.IsControl) >= 0 || len([]rune(record.Key)) > 200 {
		return errors.New("git note document contains an invalid key")
	}
	if err := provider.ValidateNoteAnchor(record.Anchor); err != nil {
		return fmt.Errorf("git note document contains an invalid anchor: %w", err)
	}
	if record.Anchor.Target != provider.NoteTargetCommits || record.Anchor.Base == "" || record.Anchor.Head == "" || record.Anchor.Fingerprint == "" {
		return errors.New("git note document contains a non-committed anchor")
	}
	if record.Key != "" {
		if record.ID != keyedNoteID(record.Anchor, record.Key) {
			return errors.New("git note document contains an invalid keyed id")
		}
	} else if !canonicalRandomNoteID(record.ID) {
		return errors.New("git note document contains an invalid random id")
	}
	return nil
}

func keyedNoteID(anchor provider.NoteAnchor, key string) string {
	digest := sha256.Sum256([]byte(comparisonNoteKey(anchor, key)))
	return hex.EncodeToString(digest[:16])
}

func canonicalRandomNoteID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && id == hex.EncodeToString(decoded)
}

func (store noteStore) publish(directory, head, expectedRef string, records []storedNote) error {
	document, err := encodeRecords(records)
	if err != nil {
		return err
	}
	blob, err := store.git.Run(directory, document, maxGitOutput, nil, "hash-object", "-w", "--stdin")
	if err != nil {
		return fmt.Errorf("write git note blob: %w", err)
	}
	object := strings.TrimSpace(string(blob))
	if object == "" {
		return errors.New("git hash-object returned an empty object id")
	}
	common, err := store.commonDirectory(directory)
	if err != nil {
		return err
	}
	index, err := os.CreateTemp(common, "changes-notes-index-")
	if err != nil {
		return fmt.Errorf("create temporary Git index: %w", err)
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		_ = os.Remove(indexPath)
		return err
	}
	if err := os.Remove(indexPath); err != nil {
		return err
	}
	defer os.Remove(indexPath)
	environment := map[string]string{"GIT_INDEX_FILE": indexPath}
	if expectedRef == "" {
		if _, err := store.git.Run(directory, nil, maxGitOutput, environment, "read-tree", "--empty"); err != nil {
			return fmt.Errorf("initialize Git notes tree: %w", err)
		}
	} else if _, err := store.git.Run(directory, nil, maxGitOutput, environment, "read-tree", expectedRef+"^{tree}"); err != nil {
		return fmt.Errorf("read Git notes tree: %w", err)
	}
	removeArguments := []string{"update-index", "--force-remove", "--"}
	removeArguments = append(removeArguments, notePaths(head)...)
	if _, err := store.git.Run(directory, nil, maxGitOutput, environment, removeArguments...); err != nil {
		return fmt.Errorf("remove old Git note mapping: %w", err)
	}
	cacheInfo := "100644," + object + "," + head
	if _, err := store.git.Run(directory, nil, maxGitOutput, environment, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return fmt.Errorf("add Git note mapping: %w", err)
	}
	treeOutput, err := store.git.Run(directory, nil, maxGitOutput, environment, "write-tree")
	if err != nil {
		return fmt.Errorf("write Git notes tree: %w", err)
	}
	tree := strings.TrimSpace(string(treeOutput))
	commitArguments := []string{"commit-tree", tree, "-m", "changes notes"}
	if expectedRef != "" {
		commitArguments = append(commitArguments, "-p", expectedRef)
	}
	commitOutput, err := store.git.Run(directory, nil, maxGitOutput, nil, commitArguments...)
	if err != nil {
		return fmt.Errorf("write Git notes commit: %w", err)
	}
	commit := strings.TrimSpace(string(commitOutput))
	if commit == "" {
		return errors.New("git commit-tree returned an empty object id")
	}
	if _, err := store.git.Run(directory, nil, maxGitOutput, nil, "update-ref", "--no-deref", notesRef, commit, expectedRef); err != nil {
		current, currentErr := store.currentRef(directory)
		if currentErr == nil && current != expectedRef {
			return errNotesRefMoved
		}
		return fmt.Errorf("update Git notes ref: %w", err)
	}
	return nil
}

func encodeRecords(records []storedNote) ([]byte, error) {
	lines := make([][]byte, len(records))
	for index, record := range records {
		if err := validateStored(record); err != nil {
			return nil, err
		}
		line, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		lines[index] = line
	}
	sort.Slice(lines, func(one, two int) bool { return bytes.Compare(lines[one], lines[two]) < 0 })
	document := append(bytes.Join(lines, []byte{'\n'}), '\n')
	if len(document) > maxStoreSize {
		return nil, fmt.Errorf("git note document exceeds %d bytes", maxStoreSize)
	}
	return document, nil
}

func (store noteStore) lock(directory string) (func(), error) {
	common, err := store.commonDirectory(directory)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(common, "changes-notes.lock")
	fd, err := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Git notes lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), lockPath)
	for range lockAttempts {
		if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			released := false
			return func() {
				if !released {
					released = true
					_ = syscall.Flock(fd, syscall.LOCK_UN)
					_ = file.Close()
				}
			}, nil
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("lock Git notes store: %w", err)
		}
		time.Sleep(lockInterval)
	}
	_ = file.Close()
	return nil, errors.New("another process holds the Git notes store lock")
}

func (store noteStore) commonDirectory(directory string) (string, error) {
	output, err := store.git.Run(directory, nil, maxGitOutput, nil, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("find Git common directory: %w", err)
	}
	common := strings.TrimSpace(string(output))
	if !filepath.IsAbs(common) {
		common = filepath.Join(directory, common)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return "", err
	}
	return common, nil
}

func sameComparison(anchor provider.NoteAnchor, request provider.Request) bool {
	return anchor.Target == provider.NoteTargetCommits && anchor.Base == request.Base && anchor.Head == request.Head &&
		anchor.Fingerprint == request.Fingerprint
}

func comparisonNoteKey(anchor provider.NoteAnchor, key string) string {
	return anchor.Base + "\x00" + anchor.Head + "\x00" + anchor.Fingerprint + "\x00" + key
}

func sameDraft(record storedNote, draft provider.NoteDraft) bool {
	return record.Key == draft.Key && record.Summary == strings.TrimSpace(cleanOneLine(draft.Summary)) &&
		record.Rationale == cleanText(draft.Rationale) && record.Author == strings.TrimSpace(cleanOneLine(draft.Author)) &&
		record.Origin == draft.Origin && record.Session == cleanOneLine(draft.Session) &&
		record.Provenance == draft.Provenance && record.Anchor == draft.Anchor
}

func normalize(record storedNote, request provider.Request) provider.Note {
	authority := provider.NoteAuthorityAdvisory
	if record.Origin == provider.NoteOriginUser {
		authority = provider.NoteAuthorityOwner
	}
	return provider.Note{
		ID: record.ID, Source: providerName, SourceID: record.ID,
		Summary: record.Summary, Rationale: record.Rationale, Author: record.Author,
		Origin: record.Origin, Authority: authority, State: provider.NoteStateOpen,
		Session: record.Session, Provenance: record.Provenance, Anchor: record.Anchor,
		Placement: provider.NotePlacement{
			Path: record.Anchor.Path, Side: record.Anchor.Side, StartSide: record.Anchor.StartSide,
			StartLine: record.Anchor.StartLine, Line: record.Anchor.Line,
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Target: provider.NoteTargetCommits, Quality: provider.PlacementExact,
		},
	}
}

func allowedPaths(paths []string) (map[string]bool, error) {
	allowed := make(map[string]bool, len(paths))
	for _, value := range paths {
		cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
		if value == "" || filepath.IsAbs(value) || cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("request file %q must be repository-relative", value)
		}
		allowed[value] = true
	}
	return allowed, nil
}

func requestTarget(request provider.Request) string {
	if request.To != "" || request.Head != "" {
		return provider.NoteTargetCommits
	}
	if request.Staged {
		return provider.NoteTargetIndex
	}
	return provider.NoteTargetWorking
}

func validationCreateResponse(request provider.Request) (provider.Response, error) {
	drafts := request.Notes
	if request.Note != nil {
		drafts = []provider.NoteDraft{*request.Note}
	}
	if len(drafts) == 0 {
		return provider.Response{}, errors.New("create requires note or notes")
	}
	notes := make([]provider.Note, 0, len(drafts))
	for index, draft := range drafts {
		record := storedNote{
			Version: 1, ID: fmt.Sprintf("validation-%d", index+1), Summary: draft.Summary,
			Rationale: draft.Rationale, Author: draft.Author, Origin: draft.Origin,
			Session: draft.Session, Provenance: draft.Provenance, Anchor: draft.Anchor,
		}
		note := normalize(record, request)
		note.Placement.Target = draft.Anchor.Target
		note.Placement.Base = request.Base
		note.Placement.Head = request.Head
		note.Placement.Fingerprint = request.Fingerprint
		notes = append(notes, note)
	}
	return provider.Response{Version: provider.ProtocolVersion, Notes: notes}, nil
}

func validationNote() provider.Note {
	anchor := provider.NoteAnchor{
		Path: "main.ts", Side: provider.NoteSideRight, Line: 2,
		Fingerprint: "provider-validation", Context: "return ready()", Target: provider.NoteTargetWorking,
	}
	return provider.Note{
		ID: "validation", Source: providerName, SourceID: "validation",
		Summary: "Explain the ready change", Author: "provider-validation",
		Origin: provider.NoteOriginAgent, Authority: provider.NoteAuthorityAdvisory,
		State: provider.NoteStateOpen, Anchor: anchor,
		Placement: provider.NotePlacement{
			Path: "main.ts", Side: provider.NoteSideRight, Line: 2,
			Fingerprint: "provider-validation", Target: provider.NoteTargetWorking,
			Quality: provider.PlacementExact,
		},
	}
}

func cleanOneLine(text string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' {
			return ' '
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, text)
}

func cleanText(text string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' {
			return character
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, text)
}

func die(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "changes-provider-git-notes: "+format+"\n", arguments...)
	os.Exit(1)
}
