// changes-provider-local-notes reads and writes the version 1 XDG note record.
// It preserves unknown envelope and record fields for compatible updates.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/go-utils/paths"
)

const (
	providerName = "local-notes"
	maxStoreSize = 4 << 20
	maxAnchor    = 200
	lockAttempts = 50
	lockInterval = 10 * time.Millisecond
)

type lockOwner struct {
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type document struct {
	Version int                        `json:"version"`
	Notes   []json.RawMessage          `json:"notes"`
	Extra   map[string]json.RawMessage `json:"-"`
}

func (doc *document) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["version"]; ok {
		if err := json.Unmarshal(raw, &doc.Version); err != nil {
			return err
		}
	}
	if raw, ok := fields["notes"]; ok {
		if err := json.Unmarshal(raw, &doc.Notes); err != nil {
			return err
		}
	}
	delete(fields, "version")
	delete(fields, "notes")
	doc.Extra = fields
	return nil
}

func (doc document) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(doc.Extra)+2)
	for name, value := range doc.Extra {
		fields[name] = value
	}
	version, err := json.Marshal(doc.Version)
	if err != nil {
		return nil, err
	}
	notes, err := json.Marshal(doc.Notes)
	if err != nil {
		return nil, err
	}
	fields["version"] = version
	fields["notes"] = notes
	return json.Marshal(fields)
}

type storedNote struct {
	ID         string                  `json:"id"`
	Key        string                  `json:"key,omitempty"`
	File       string                  `json:"file"`
	Line       int                     `json:"line"`
	Summary    string                  `json:"summary"`
	Rationale  *string                 `json:"rationale"`
	Author     string                  `json:"author"`
	Origin     string                  `json:"origin"`
	Anchor     string                  `json:"anchor,omitempty"`
	State      string                  `json:"state,omitempty"`
	ReplyTo    string                  `json:"reply_to,omitempty"`
	Session    string                  `json:"session,omitempty"`
	CreatedAt  string                  `json:"created_at,omitempty"`
	Provenance provider.NoteProvenance `json:"provenance,omitempty"`
	Comparison *provider.NoteAnchor    `json:"comparison,omitempty"`
}

func main() {
	request, err := decodeRequest(os.Stdin)
	if err != nil {
		die("decode request: %v", err)
	}
	if request.Version != provider.ProtocolVersion {
		die("version must be %q", provider.ProtocolVersion)
	}

	var response provider.Response
	switch request.Action {
	case provider.ActionNotes:
		response, err = list(request)
	case provider.ActionNotesCreate:
		response, err = create(request)
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
	return decodeRequestWithLimit(reader, maxStoreSize)
}

func decodeRequestWithLimit(reader io.Reader, limit int64) (provider.Request, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return provider.Request{}, err
	}
	if int64(len(data)) > limit {
		return provider.Request{}, fmt.Errorf("request exceeds %d bytes", limit)
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

func list(request provider.Request) (provider.Response, error) {
	if request.Validation {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{validationNote()}}, nil
	}
	if len(request.Files) == 0 {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
	}
	doc, err := readDocument(recordFile(request))
	if err != nil {
		return provider.Response{}, err
	}
	if err := validateStoredNotes(doc.Notes); err != nil {
		return provider.Response{}, err
	}
	allowed := make(map[string]bool, len(request.Files))
	for _, path := range request.Files {
		allowed[filepath.ToSlash(filepath.Clean(path))] = true
	}
	notes := make([]provider.Note, 0, len(doc.Notes))
	for _, raw := range doc.Notes {
		var stored storedNote
		if err := decodeOne(raw, &stored); err != nil {
			return provider.Response{}, fmt.Errorf("the note store contains an invalid record: %w", err)
		}
		note, ok, err := normalize(request, stored, allowed)
		if err != nil {
			return provider.Response{}, err
		}
		if ok {
			notes = append(notes, note)
		}
	}
	return provider.Response{Version: provider.ProtocolVersion, Notes: notes}, nil
}

func create(request provider.Request) (provider.Response, error) {
	drafts := request.Notes
	if request.Note != nil {
		drafts = []provider.NoteDraft{*request.Note}
	}
	if len(drafts) == 0 {
		return provider.Response{}, errors.New("create requires note or notes")
	}
	path := recordFile(request)
	release, err := lock(path)
	if err != nil {
		return provider.Response{}, err
	}
	defer release()
	doc, err := readDocument(path)
	if err != nil {
		return provider.Response{}, err
	}
	if err := validateStoredNotes(doc.Notes); err != nil {
		return provider.Response{}, err
	}
	byKey := make(map[string]storedNote, len(doc.Notes))
	for _, raw := range doc.Notes {
		var stored storedNote
		if err := decodeOne(raw, &stored); err != nil {
			return provider.Response{}, fmt.Errorf("the note store contains an invalid record: %w", err)
		}
		if stored.Key != "" {
			byKey[stored.Key] = stored
		}
	}
	created := make([]provider.Note, 0, len(drafts))
	changed := false
	now := time.Now().UTC().Format(time.RFC3339)
	for _, draft := range drafts {
		if existing, ok := byKey[draft.Key]; draft.Key != "" && ok {
			if !sameDraft(existing, draft) {
				return provider.Response{}, fmt.Errorf("note key %q already names different content", draft.Key)
			}
			note, matches, err := normalize(request, existing, nil)
			if err != nil {
				return provider.Response{}, err
			}
			if !matches {
				return provider.Response{}, fmt.Errorf("note key %q belongs to another comparison", draft.Key)
			}
			created = append(created, note)
			continue
		}
		stored, raw, note, err := prepareStoredNote(request, draft, now)
		if err != nil {
			return provider.Response{}, err
		}
		doc.Notes = append(doc.Notes, raw)
		if stored.Key != "" {
			byKey[stored.Key] = stored
		}
		created = append(created, note)
		changed = true
	}
	if changed {
		if err := publish(path, doc); err != nil {
			return provider.Response{}, err
		}
	}
	return provider.Response{Version: provider.ProtocolVersion, Notes: created}, nil
}

func prepareStoredNote(request provider.Request, draft provider.NoteDraft, now string) (storedNote, json.RawMessage, provider.Note, error) {
	anchorFile, err := storedPath(request.Directory, draft.Anchor.Path)
	if err != nil {
		return storedNote{}, nil, provider.Note{}, err
	}
	compatibilityFile, err := compatibilityStoredPath(request.Directory, anchorFile)
	if err != nil {
		return storedNote{}, nil, provider.Note{}, err
	}
	id, err := newID()
	if err != nil {
		return storedNote{}, nil, provider.Note{}, err
	}
	comparison := draft.Anchor
	stored := storedNote{
		ID: id, Key: draft.Key, File: compatibilityFile, Line: draft.Anchor.Line,
		Summary: strings.TrimSpace(cleanOneLine(draft.Summary)),
		Author:  strings.TrimSpace(cleanOneLine(draft.Author)), Origin: draft.Origin,
		Anchor: cleanOneLine(draft.Anchor.Context), Session: cleanOneLine(draft.Session),
		CreatedAt: now, Provenance: draft.Provenance, Comparison: &comparison,
	}
	if stored.Summary == "" || stored.Author == "" {
		return storedNote{}, nil, provider.Note{}, errors.New("summary and author must remain non-empty after removing control bytes")
	}
	if rationale := cleanText(draft.Rationale); rationale != "" {
		stored.Rationale = &rationale
	}
	if stored.Origin == provider.NoteOriginUser {
		stored.State = "open"
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return storedNote{}, nil, provider.Note{}, err
	}
	note, ok, err := normalize(request, stored, nil)
	if err != nil {
		return storedNote{}, nil, provider.Note{}, err
	}
	if !ok {
		return storedNote{}, nil, provider.Note{}, errors.New("created note did not match its comparison")
	}
	return stored, raw, note, nil
}

func sameDraft(stored storedNote, draft provider.NoteDraft) bool {
	rationale := ""
	if stored.Rationale != nil {
		rationale = *stored.Rationale
	}
	return stored.Key == draft.Key && stored.Summary == strings.TrimSpace(cleanOneLine(draft.Summary)) &&
		rationale == cleanText(draft.Rationale) && stored.Author == strings.TrimSpace(cleanOneLine(draft.Author)) &&
		stored.Origin == draft.Origin && stored.Session == cleanOneLine(draft.Session) &&
		stored.Provenance == draft.Provenance &&
		stored.Comparison != nil && *stored.Comparison == draft.Anchor
}

func validateStoredNotes(records []json.RawMessage) error {
	seen := make(map[string]bool, len(records))
	seenKeys := make(map[string]bool, len(records))
	for _, raw := range records {
		var stored storedNote
		if err := decodeOne(raw, &stored); err != nil {
			return fmt.Errorf("the note store contains an invalid record: %w", err)
		}
		if strings.TrimSpace(cleanOneLine(stored.ID)) == "" || stored.File == "" || strings.TrimSpace(cleanOneLine(stored.Summary)) == "" ||
			strings.TrimSpace(cleanOneLine(stored.Author)) == "" || stored.Line < 0 {
			return errors.New("the note store contains an incomplete record")
		}
		if seen[stored.ID] {
			return fmt.Errorf("the note store contains duplicate id %q", stored.ID)
		}
		seen[stored.ID] = true
		if stored.Key != "" {
			if strings.IndexFunc(stored.Key, unicode.IsControl) >= 0 || len([]rune(stored.Key)) > 200 {
				return errors.New("the note store contains an invalid idempotency key")
			}
			if seenKeys[stored.Key] {
				return fmt.Errorf("the note store contains duplicate key %q", stored.Key)
			}
			seenKeys[stored.Key] = true
		}
		if stored.Origin != provider.NoteOriginAgent && stored.Origin != provider.NoteOriginUser {
			return fmt.Errorf("the note store contains an invalid origin %q", stored.Origin)
		}
		if stored.State != "" && stored.State != "open" && stored.State != "answered" && stored.State != "resolved" {
			return fmt.Errorf("the note store contains an invalid state %q", stored.State)
		}
		if stored.CreatedAt != "" {
			if _, err := time.Parse(time.RFC3339, stored.CreatedAt); err != nil {
				return fmt.Errorf("the note store contains an invalid timestamp: %w", err)
			}
		}
		if stored.Comparison != nil {
			if err := provider.ValidateNoteAnchor(*stored.Comparison); err != nil {
				return fmt.Errorf("the note store contains an invalid comparison: %w", err)
			}
		}
	}
	return nil
}

func normalize(request provider.Request, stored storedNote, allowed map[string]bool) (provider.Note, bool, error) {
	if stored.ID == "" || stored.File == "" || stored.Summary == "" || stored.Author == "" || stored.Line < 0 {
		return provider.Note{}, false, errors.New("note identity, content, or line is incomplete")
	}
	storedRelative, ok := relativeStoredPath(request.Directory, stored.File)
	if !ok {
		return provider.Note{}, false, nil
	}
	relative := filepath.ToSlash(storedRelative)
	projectionFile := stored.File
	if stored.Comparison != nil {
		var err error
		relative, projectionFile, err = comparisonStoredPath(request.Directory, stored.Comparison.Path)
		if err != nil {
			return provider.Note{}, false, err
		}
	}
	if len(allowed) > 0 && !allowed[relative] {
		return provider.Note{}, false, nil
	}
	if stored.Comparison != nil {
		if !sameComparison(*stored.Comparison, request) {
			return provider.Note{}, false, nil
		}
	} else if requestTarget(request) != provider.NoteTargetWorking {
		return provider.Note{}, false, nil
	}

	anchor := provider.NoteAnchor{
		Path: relative, Side: provider.NoteSideRight, Line: stored.Line, Context: stored.Anchor,
		Base: request.Base, Head: request.Head, Target: requestTarget(request),
	}
	if stored.Comparison != nil {
		anchor = *stored.Comparison
		anchor.Path = relative
		if anchor.Context == "" {
			anchor.Context = stored.Anchor
		}
	}
	placement := currentPlacement(request, relative, anchor)
	if stored.Comparison == nil || requestTarget(request) == provider.NoteTargetWorking {
		placement = project(request, relative, projectionFile, anchor)
	}
	state := provider.NoteStateOpen
	if stored.State == "answered" || stored.State == "resolved" {
		state = provider.NoteStateResolved
	}
	authority := provider.NoteAuthorityAdvisory
	if stored.Origin == provider.NoteOriginUser {
		authority = provider.NoteAuthorityOwner
	} else if stored.Origin != provider.NoteOriginAgent {
		return provider.Note{}, false, fmt.Errorf("note %s has invalid origin %q", stored.ID, stored.Origin)
	}
	rationale := ""
	if stored.Rationale != nil {
		rationale = *stored.Rationale
	}
	return provider.Note{
		ID: providerName + ":" + stored.ID, Source: providerName, SourceID: stored.ID,
		ReplyTo: stored.ReplyTo, Summary: cleanOneLine(stored.Summary), Rationale: cleanText(rationale),
		Author: cleanOneLine(stored.Author), Origin: stored.Origin, Authority: authority,
		State: state, Session: cleanOneLine(stored.Session), CreatedAt: stored.CreatedAt, Provenance: stored.Provenance,
		Anchor: anchor, Placement: placement,
	}, true, nil
}

func relativeStoredPath(repository, storedFile string) (string, bool) {
	root, ok := repositoryRootForPath(repository, storedFile)
	if !ok {
		return "", false
	}
	if err := validatePathComponents(root, storedFile, true); err != nil {
		return "", false
	}
	return relativeInside(root, storedFile)
}

func comparisonStoredPath(repository, value string) (string, string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if value == "" || filepath.IsAbs(value) || cleaned != value || cleaned == "." || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") {
		return "", "", errors.New("stored comparison path must be clean and repository-relative")
	}
	absolute, err := storedPath(repository, cleaned)
	if err != nil {
		return "", "", err
	}
	return cleaned, absolute, nil
}

func compatibilityStoredPath(repository, anchorFile string) (string, error) {
	resolved, err := filepath.EvalSymlinks(anchorFile)
	if err != nil {
		info, lstatErr := os.Lstat(anchorFile)
		if lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
			return anchorFile, nil
		}
		target, readErr := os.Readlink(anchorFile)
		if readErr != nil {
			return "", readErr
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(anchorFile), target)
		}
		resolved, err = resolveMissingPath(target)
		if err != nil {
			return "", err
		}
	}
	root, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return "", err
	}
	if _, ok := relativeInside(root, resolved); !ok {
		return "", errors.New("note path resolves outside the repository")
	}
	if _, err := filepath.EvalSymlinks(anchorFile); err != nil {
		return anchorFile, nil
	}
	return resolved, nil
}

func resolveMissingPath(value string) (string, error) {
	return resolveMissingPathAt(value, 0)
}

func resolveMissingPathAt(value string, depth int) (string, error) {
	if depth > 32 {
		return "", errors.New("note path contains too many symbolic links")
	}
	info, err := os.Lstat(value)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(value)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(value), target)
		}
		return resolveMissingPathAt(target, depth+1)
	}
	if err == nil {
		return filepath.EvalSymlinks(value)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(value)
	if parent == value {
		return "", err
	}
	resolvedParent, err := resolveMissingPathAt(parent, depth+1)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(value)), nil
}

func repositoryRootForPath(repository, file string) (string, bool) {
	root, err := filepath.Abs(repository)
	if err == nil {
		if _, ok := relativeInside(root, file); ok {
			return root, true
		}
	}
	root, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", false
	}
	if _, ok := relativeInside(root, file); !ok {
		return "", false
	}
	return root, true
}

func relativeInside(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

func sameComparison(anchor provider.NoteAnchor, request provider.Request) bool {
	target := requestTarget(request)
	if anchor.Target != "" && anchor.Target != target {
		return false
	}
	if target == provider.NoteTargetCommits {
		return anchor.Base != "" && anchor.Base == request.Base &&
			anchor.Head != "" && anchor.Head == request.Head
	}
	if target == provider.NoteTargetIndex {
		return anchor.Base == request.Base && anchor.Head == request.Head &&
			anchor.Fingerprint != "" && anchor.Fingerprint == request.Fingerprint
	}
	return anchor.Base == request.Base && anchor.Head == request.Head
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

func currentPlacement(request provider.Request, path string, anchor provider.NoteAnchor) provider.NotePlacement {
	return provider.NotePlacement{
		Path: path, Side: anchor.Side, StartSide: anchor.StartSide,
		StartLine: anchor.StartLine, Line: anchor.Line,
		Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
		Target: requestTarget(request), Quality: provider.PlacementExact,
	}
}

func project(request provider.Request, relative, storedFile string, anchor provider.NoteAnchor) provider.NotePlacement {
	placement := currentPlacement(request, relative, anchor)
	if anchor.Line == 0 || anchor.Fingerprint != "" && anchor.Fingerprint == request.Fingerprint {
		return placement
	}
	context := anchorText(anchor.Context)
	if anchor.Side == provider.NoteSideLeft || context == "" {
		placement.StartSide, placement.StartLine, placement.Line = "", 0, 0
		placement.Quality = provider.PlacementFile
		return placement
	}
	lines, err := repositoryFileLines(request.Directory, storedFile)
	if err != nil {
		return provider.NotePlacement{
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Target: requestTarget(request), Quality: provider.PlacementOrphan,
		}
	}
	if anchor.Line <= len(lines) && anchorText(lines[anchor.Line-1]) == context {
		placement.StartSide = ""
		placement.StartLine = 0
		placement.Quality = provider.PlacementContext
		return placement
	}
	line := uniqueLine(lines, context)
	if line > 0 {
		placement.Line = line
		placement.StartSide = ""
		placement.StartLine = 0
		placement.Quality = provider.PlacementContext
		return placement
	}
	placement.StartSide, placement.StartLine, placement.Line = "", 0, 0
	placement.Quality = provider.PlacementFile
	return placement
}

func validationNote() provider.Note {
	anchor := provider.NoteAnchor{
		Path: "main.ts", Side: provider.NoteSideRight, Line: 2,
		Fingerprint: "provider-validation", Context: "return ready()", Target: provider.NoteTargetWorking,
	}
	return provider.Note{
		ID: providerName + ":validation", Source: providerName, SourceID: "validation",
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

func recordFile(request provider.Request) string {
	if request.Validation {
		return filepath.Join(request.Directory, ".changes-provider-validation", "notes.json")
	}
	return filepath.Join(paths.AgentDiffNotes(), "notes.json")
}

func storedPath(root, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", errors.New("note path must be repository-relative")
	}
	cleaned := filepath.Clean(filepath.FromSlash(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("note path must stay inside the repository")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	absolute := filepath.Join(resolvedRoot, cleaned)
	relative, err := filepath.Rel(resolvedRoot, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("note path resolves outside the repository")
	}
	if err := validatePathComponents(resolvedRoot, absolute, true); err != nil {
		return "", err
	}
	return absolute, nil
}

func readDocument(path string) (document, error) {
	if err := validateStorePath(path); err != nil {
		return document{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return document{Version: 1, Notes: []json.RawMessage{}}, nil
	}
	if err != nil {
		return document{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStoreSize+1))
	if err != nil {
		return document{}, err
	}
	if len(data) > maxStoreSize {
		return document{}, fmt.Errorf("note store exceeds %d bytes", maxStoreSize)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return document{Version: 1, Notes: []json.RawMessage{}}, nil
	}
	var doc document
	if err := decodeOne(data, &doc); err != nil {
		return document{}, fmt.Errorf("note store is malformed: %w", err)
	}
	if doc.Version != 1 || doc.Notes == nil {
		return document{}, errors.New("note store must contain version 1 and a notes array")
	}
	return doc, nil
}

func publish(path string, doc document) error {
	if err := validateStorePath(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxStoreSize {
		return fmt.Errorf("note store exceeds %d bytes", maxStoreSize)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func lock(path string) (func(), error) {
	if err := validateStorePath(path); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	if err := validateStorePath(lockPath); err != nil {
		return nil, err
	}
	token, err := newID()
	if err != nil {
		return nil, err
	}
	ownerData, err := json.Marshal(lockOwner{Token: token, PID: os.Getpid()})
	if err != nil {
		return nil, err
	}
	for range lockAttempts {
		held, acquired, err := linkLock(lockPath, ownerData)
		if err != nil {
			return nil, err
		}
		if acquired {
			released := false
			return func() {
				if !released {
					released = true
					heldInfo, heldErr := held.Stat()
					pathInfo, pathErr := os.Lstat(lockPath)
					if current, err := readLockOwner(lockPath); err == nil && current.Token == token &&
						heldErr == nil && pathErr == nil && os.SameFile(heldInfo, pathInfo) {
						_ = os.Remove(lockPath)
					}
					_ = syscall.Flock(int(held.Fd()), syscall.LOCK_UN)
					_ = held.Close()
				}
			}, nil
		}
		reaped, err := reapAbandonedLock(lockPath)
		if err != nil {
			return nil, err
		}
		if reaped {
			continue
		}
		time.Sleep(lockInterval)
	}
	return nil, fmt.Errorf("another process holds the note store lock: %s", lockPath)
}

func linkLock(path string, data []byte) (*os.File, bool, error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".owner-*")
	if err != nil {
		return nil, false, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, false, err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return nil, false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, false, err
	}
	if err := syscall.Flock(int(temporary.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = temporary.Close()
		return nil, false, err
	}
	if err := os.Link(name, path); err != nil {
		_ = syscall.Flock(int(temporary.Fd()), syscall.LOCK_UN)
		_ = temporary.Close()
		if os.IsExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return temporary, true, nil
}

func reapAbandonedLock(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, nil
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("note store lock must be a regular file or directory")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	owner, err := readLockOwner(path)
	if err != nil {
		return false, nil
	}
	if owner.Token == "" || owner.PID <= 0 {
		return false, nil
	}
	heldInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	pathInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(heldInfo, pathInfo) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

func readLockOwner(path string) (lockOwner, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return lockOwner{}, err
	}
	var owner lockOwner
	if err := decodeOne(data, &owner); err != nil {
		return lockOwner{}, err
	}
	return owner, nil
}

func validateStorePath(path string) error {
	root := trustedStoreRoot(path)
	return validatePathComponents(root, path, false)
}

func trustedStoreRoot(path string) string {
	state, err := filepath.Abs(paths.StateHome())
	if err == nil {
		if _, ok := relativeInside(state, path); ok {
			return state
		}
	}
	marker := string(filepath.Separator) + ".changes-provider-validation" + string(filepath.Separator)
	if index := strings.Index(path, marker); index > 0 {
		return path[:index]
	}
	return filepath.Dir(filepath.Dir(path))
}

func validatePathComponents(root, target string, allowFinalSymlink bool) error {
	relative, ok := relativeInside(root, target)
	if !ok {
		return errors.New("path must stay below its trusted root")
	}
	components := strings.Split(relative, string(filepath.Separator))
	candidate := root
	for index, component := range components {
		candidate = filepath.Join(candidate, component)
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 && !(allowFinalSymlink && index == len(components)-1) {
			return fmt.Errorf("path component is a symlink: %s", candidate)
		}
	}
	return nil
}

func decodeOne(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON value has trailing data")
	}
	return nil
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func repositoryFileLines(root, path string) ([]string, error) {
	containedRoot, ok := repositoryRootForPath(root, path)
	if !ok {
		return nil, errors.New("note path is outside the repository")
	}
	if err := validatePathComponents(containedRoot, path, true); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		return []string{target}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), nil
}

func uniqueLine(lines []string, anchor string) int {
	found := 0
	for index, line := range lines {
		if anchorText(line) != anchor {
			continue
		}
		if found != 0 {
			return 0
		}
		found = index + 1
	}
	return found
}

func anchorText(text string) string {
	runes := []rune(cleanOneLine(strings.TrimSpace(text)))
	if len(runes) > maxAnchor {
		runes = runes[:maxAnchor]
	}
	return string(runes)
}

func cleanOneLine(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func cleanText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func die(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "changes-provider-local-notes: "+format+"\n", arguments...)
	os.Exit(1)
}
