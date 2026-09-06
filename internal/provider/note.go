package provider

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	NoteOriginAgent    = "agent"
	NoteOriginExternal = "external"
	NoteOriginUser     = "user"

	NoteAuthorityAdvisory = "advisory"
	NoteAuthorityExternal = "external"
	NoteAuthorityOwner    = "owner"

	NoteStateOpen     = "open"
	NoteStateResolved = "resolved"

	NoteSideLeft  = "LEFT"
	NoteSideRight = "RIGHT"

	PlacementExact    = "exact"
	PlacementContext  = "context"
	PlacementFile     = "file"
	PlacementOutdated = "outdated"
	PlacementOrphan   = "orphan"

	NoteTargetCommits = "commits"
	NoteTargetIndex   = "index"
	NoteTargetWorking = "working"
)

// NoteDraft is the source-neutral input to a writable note provider.
type NoteDraft struct {
	Summary   string     `json:"summary"`
	Rationale string     `json:"rationale,omitempty"`
	Author    string     `json:"author"`
	Origin    string     `json:"origin"`
	Session   string     `json:"session,omitempty"`
	Anchor    NoteAnchor `json:"anchor"`
}

// Note is one normalized annotation returned by any provider.
type Note struct {
	ID        string        `json:"id"`
	Source    string        `json:"source"`
	SourceID  string        `json:"sourceId"`
	ThreadID  string        `json:"threadId,omitempty"`
	ReplyTo   string        `json:"replyTo,omitempty"`
	Summary   string        `json:"summary"`
	Rationale string        `json:"rationale,omitempty"`
	Author    string        `json:"author"`
	Origin    string        `json:"origin"`
	Authority string        `json:"authority"`
	State     string        `json:"state"`
	Session   string        `json:"session,omitempty"`
	CreatedAt string        `json:"createdAt,omitempty"`
	UpdatedAt string        `json:"updatedAt,omitempty"`
	URL       string        `json:"url,omitempty"`
	Anchor    NoteAnchor    `json:"anchor"`
	Placement NotePlacement `json:"placement"`
}

// NoteAnchor is the immutable location recorded by the note source.
type NoteAnchor struct {
	Path        string `json:"path"`
	Side        string `json:"side"`
	StartSide   string `json:"startSide,omitempty"`
	StartLine   int    `json:"startLine,omitempty"`
	Line        int    `json:"line,omitempty"`
	Base        string `json:"base,omitempty"`
	Head        string `json:"head,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Context     string `json:"context,omitempty"`
	Target      string `json:"target,omitempty"`
}

// NotePlacement is a provider's current projection of the immutable anchor.
type NotePlacement struct {
	Path        string `json:"path,omitempty"`
	Side        string `json:"side,omitempty"`
	StartSide   string `json:"startSide,omitempty"`
	StartLine   int    `json:"startLine,omitempty"`
	Line        int    `json:"line,omitempty"`
	Base        string `json:"base,omitempty"`
	Head        string `json:"head,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Target      string `json:"target"`
	Quality     string `json:"quality"`
}

func validateNoteDraft(note *NoteDraft) error {
	if note == nil {
		return errors.New("request note is required")
	}
	if strings.TrimSpace(note.Summary) == "" {
		return errors.New("note summary is required")
	}
	if strings.TrimSpace(note.Author) == "" {
		return errors.New("note author is required")
	}
	if !oneOf(note.Origin, NoteOriginAgent, NoteOriginUser) {
		return fmt.Errorf("note origin must be %q or %q", NoteOriginAgent, NoteOriginUser)
	}
	return validateAnchor(note.Anchor)
}

func validateNote(note Note) error {
	for name, value := range map[string]string{
		"id": note.ID, "source": note.Source, "sourceId": note.SourceID,
		"summary": note.Summary, "author": note.Author,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("note %s is required", name)
		}
	}
	if !oneOf(note.Origin, NoteOriginAgent, NoteOriginExternal, NoteOriginUser) {
		return fmt.Errorf("note %q has invalid origin %q", note.ID, note.Origin)
	}
	if !oneOf(note.Authority, NoteAuthorityAdvisory, NoteAuthorityExternal, NoteAuthorityOwner) {
		return fmt.Errorf("note %q has invalid authority %q", note.ID, note.Authority)
	}
	if !oneOf(note.State, NoteStateOpen, NoteStateResolved) {
		return fmt.Errorf("note %q has invalid state %q", note.ID, note.State)
	}
	if err := validateAnchor(note.Anchor); err != nil {
		return fmt.Errorf("note %q: %w", note.ID, err)
	}
	if !oneOf(note.Placement.Quality, PlacementExact, PlacementContext, PlacementFile, PlacementOutdated, PlacementOrphan) {
		return fmt.Errorf("note %q has invalid placement quality %q", note.ID, note.Placement.Quality)
	}
	if !oneOf(note.Placement.Target, NoteTargetCommits, NoteTargetIndex, NoteTargetWorking) {
		return fmt.Errorf("note %q placement target is invalid: %q", note.ID, note.Placement.Target)
	}
	if note.Placement.Quality != PlacementOrphan {
		if err := validateNotePath(note.Placement.Path); err != nil {
			return fmt.Errorf("note %q placement: %w", note.ID, err)
		}
		if note.Anchor.Path != note.Placement.Path {
			return fmt.Errorf("note %q placement path must match its anchor path", note.ID)
		}
		if err := validateRange(note.Placement.Side, note.Placement.StartSide, note.Placement.StartLine, note.Placement.Line); err != nil {
			return fmt.Errorf("note %q placement: %w", note.ID, err)
		}
		if note.Placement.Quality == PlacementFile && note.Placement.Line != 0 {
			return fmt.Errorf("note %q file placement must not contain a line", note.ID)
		}
	}
	for name, value := range map[string]string{"createdAt": note.CreatedAt, "updatedAt": note.UpdatedAt} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("note %q %s is not RFC3339: %w", note.ID, name, err)
		}
	}
	return nil
}

func validateAnchor(anchor NoteAnchor) error {
	if err := validateNotePath(anchor.Path); err != nil {
		return fmt.Errorf("note anchor: %w", err)
	}
	if err := validateRange(anchor.Side, anchor.StartSide, anchor.StartLine, anchor.Line); err != nil {
		return fmt.Errorf("note anchor: %w", err)
	}
	if !oneOf(anchor.Target, NoteTargetCommits, NoteTargetIndex, NoteTargetWorking) {
		return fmt.Errorf("note anchor target is invalid: %q", anchor.Target)
	}
	return nil
}

// ValidateNoteAnchor checks an anchor before a provider persists or returns it.
func ValidateNoteAnchor(anchor NoteAnchor) error {
	return validateAnchor(anchor)
}

func validateNotePath(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("path is required")
	}
	if strings.HasPrefix(value, "/") || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") || path.Clean(value) != value {
		return errors.New("path must be a clean repository-relative path")
	}
	return nil
}

func validateRange(side, startSide string, start, end int) error {
	if !oneOf(side, NoteSideLeft, NoteSideRight) {
		return fmt.Errorf("side must be %q or %q", NoteSideLeft, NoteSideRight)
	}
	if start < 0 || end < 0 {
		return errors.New("lines must not be negative")
	}
	if end == 0 && start != 0 {
		return errors.New("a file note must not have a start line")
	}
	if start == 0 && startSide != "" {
		return errors.New("a note without a start line must not have a start side")
	}
	if start > 0 && !oneOf(startSide, NoteSideLeft, NoteSideRight) {
		return fmt.Errorf("start side must be %q or %q", NoteSideLeft, NoteSideRight)
	}
	if end > 0 && startSide == side && start > end {
		return errors.New("start line must not follow the end line")
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
