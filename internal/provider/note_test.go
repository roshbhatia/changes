package provider

import (
	"strings"
	"testing"
)

func TestValidateNoteAcceptsLineAndFileNotes(t *testing.T) {
	for _, note := range []Note{
		validNote(),
		func() Note {
			note := validNote()
			note.Anchor.StartSide = ""
			note.Anchor.StartLine = 0
			note.Anchor.Line = 0
			note.Placement.StartSide = ""
			note.Placement.StartLine = 0
			note.Placement.Line = 0
			note.Placement.Quality = PlacementFile
			return note
		}(),
		func() Note {
			note := validNote()
			note.Placement = NotePlacement{Target: NoteTargetWorking, Quality: PlacementOrphan}
			return note
		}(),
	} {
		if err := validateNote(note); err != nil {
			t.Fatalf("note rejected: %+v: %v", note, err)
		}
	}
}

func TestValidateNoteAcceptsMixedSideRangeCoordinates(t *testing.T) {
	note := validNote()
	note.Anchor.StartSide = NoteSideLeft
	note.Anchor.StartLine = 10
	note.Anchor.Line = 2
	note.Placement.StartSide = NoteSideLeft
	note.Placement.StartLine = 10
	note.Placement.Line = 2
	if err := validateNote(note); err != nil {
		t.Fatalf("mixed-side range was rejected: %v", err)
	}
}

func TestValidateNoteRejectsInvalidContractFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Note)
		want string
	}{
		{name: "identity", edit: func(note *Note) { note.ID = "" }, want: "id is required"},
		{name: "source", edit: func(note *Note) { note.Source = "" }, want: "source is required"},
		{name: "origin", edit: func(note *Note) { note.Origin = "bot" }, want: "invalid origin"},
		{name: "authority", edit: func(note *Note) { note.Authority = "admin" }, want: "invalid authority"},
		{name: "state", edit: func(note *Note) { note.State = "dismissed" }, want: "invalid state"},
		{name: "side", edit: func(note *Note) { note.Anchor.Side = "right" }, want: "side must be"},
		{name: "start side", edit: func(note *Note) { note.Anchor.StartSide = "" }, want: "start side"},
		{name: "range", edit: func(note *Note) { note.Anchor.StartLine = 10; note.Anchor.Line = 5 }, want: "start line"},
		{name: "anchor path", edit: func(note *Note) { note.Anchor.Path = "../main.go" }, want: "repository-relative"},
		{name: "placement path", edit: func(note *Note) { note.Placement.Path = "a/../main.go" }, want: "repository-relative"},
		{name: "unrelated placement path", edit: func(note *Note) { note.Placement.Path = "other.go" }, want: "must match"},
		{name: "target", edit: func(note *Note) { note.Anchor.Target = "tree" }, want: "target is invalid"},
		{name: "empty target", edit: func(note *Note) { note.Anchor.Target = "" }, want: "target is invalid"},
		{name: "placement", edit: func(note *Note) { note.Placement.Quality = "maybe" }, want: "placement quality"},
		{name: "file placement line", edit: func(note *Note) { note.Placement.Quality = PlacementFile }, want: "must not contain a line"},
		{name: "timestamp", edit: func(note *Note) { note.CreatedAt = "yesterday" }, want: "not RFC3339"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			note := validNote()
			test.edit(&note)
			if err := validateNote(note); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateProviderResponseQualifiesAndDeduplicatesNotes(t *testing.T) {
	note := validNote()
	for _, test := range []struct {
		name     string
		action   string
		response Response
		want     string
	}{
		{name: "source", action: ActionNotes, response: Response{Notes: []Note{func() Note { copy := note; copy.Source = "other"; return copy }()}}, want: "source must be provider name"},
		{name: "duplicate", action: ActionNotes, response: Response{Notes: []Note{note, note}}, want: "duplicate note id"},
		{name: "empty create", action: ActionNotesCreate, response: Response{}, want: "exactly one note"},
		{name: "many create", action: ActionNotesCreate, response: Response{Notes: []Note{note, func() Note { copy := note; copy.ID = "local:2"; copy.SourceID = "2"; return copy }()}}, want: "exactly one note"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateProviderResponse("local", test.action, test.response); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateNoteDraftRestrictsWritableOrigins(t *testing.T) {
	draft := &NoteDraft{
		Summary: "summary", Author: "author", Origin: NoteOriginExternal,
		Anchor: NoteAnchor{Path: "main.go", Side: NoteSideRight, Line: 1},
	}
	if err := validateNoteDraft(draft); err == nil || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("error = %v", err)
	}
}

func TestQualifyNoteIDsPreservesNativeSourceIDs(t *testing.T) {
	response := Response{Notes: []Note{{
		ID: "comment", SourceID: "comment", ThreadID: "thread", ReplyTo: "parent",
	}}}
	qualifyNoteIDs("github", &response)
	note := response.Notes[0]
	if note.ID != "github:comment" || note.ThreadID != "github:thread" ||
		note.ReplyTo != "github:parent" || note.SourceID != "comment" {
		t.Fatalf("qualified note = %+v", note)
	}
	qualifyNoteIDs("github", &response)
	if response.Notes[0] != note {
		t.Fatalf("qualification was not idempotent: %+v", response.Notes[0])
	}
}

func TestValidateCreateResponseRequiresRequestedAnchorAndPlacementPath(t *testing.T) {
	note := validNote()
	request := Request{
		Action: ActionNotesCreate, Files: []string{"main.go"},
		Note: &NoteDraft{Anchor: note.Anchor},
	}
	if err := validateCreateResponse(request, Response{Notes: []Note{note}}); err != nil {
		t.Fatal(err)
	}
	note.Anchor.Line++
	if err := validateCreateResponse(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("anchor error = %v", err)
	}
	note = validNote()
	note.Placement.Path = "other.go"
	if err := validateCreateResponse(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "exactly match") {
		t.Fatalf("placement error = %v", err)
	}
}

func TestValidateResponseComparisonRejectsForeignPlacement(t *testing.T) {
	note := validNote()
	request := Request{Action: ActionNotes, Fingerprint: "current"}
	note.Placement.Fingerprint = request.Fingerprint
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err != nil {
		t.Fatal(err)
	}
	note.Placement.Target = NoteTargetCommits
	note.Placement.Base = "foreign-base"
	note.Placement.Head = "foreign-head"
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "comparison") {
		t.Fatalf("comparison error = %v", err)
	}
	note = validNote()
	note.Placement.Fingerprint = request.Fingerprint
	note.Anchor.Target = NoteTargetCommits
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "anchor target") {
		t.Fatalf("anchor target error = %v", err)
	}
	note.Anchor.Target = NoteTargetWorking
	note.Anchor.Base = "foreign"
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "anchor endpoints") {
		t.Fatalf("anchor endpoint error = %v", err)
	}
}

func TestValidateResponseComparisonAcceptsHistoricalCommittedAnchor(t *testing.T) {
	note := validNote()
	request := Request{Action: ActionNotes, Base: "current-base", Head: "current-head", To: "current-head", Fingerprint: "current"}
	note.Anchor.Target = NoteTargetCommits
	note.Anchor.Base = "original-base"
	note.Anchor.Head = "original-head"
	note.Placement.Target = NoteTargetCommits
	note.Placement.Base = request.Base
	note.Placement.Head = request.Head
	note.Placement.Fingerprint = request.Fingerprint
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err != nil {
		t.Fatalf("historical anchor rejected: %v", err)
	}
	note.Anchor.Base = ""
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err != nil {
		t.Fatalf("anchor with unknown original base rejected: %v", err)
	}
	note.Anchor.Head = ""
	if err := validateResponseComparison(request, Response{Notes: []Note{note}}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing committed endpoint error = %v", err)
	}
}

func TestValidationErrorsQuoteControlledIdentifiers(t *testing.T) {
	note := validNote()
	note.ID = "\x1b]8;;https://example.test\ax\x1b]8;;\a"
	note.Origin = "invalid"
	err := validateNote(note)
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\a") {
		t.Fatalf("unsafe validation error = %q", err)
	}
}

func validNote() Note {
	return Note{
		ID: "local:1", Source: "local", SourceID: "1", Summary: "summary", Author: "author",
		Origin: NoteOriginAgent, Authority: NoteAuthorityAdvisory, State: NoteStateOpen,
		CreatedAt: "2026-09-05T01:02:03Z",
		Anchor: NoteAnchor{
			Path: "main.go", Side: NoteSideRight, StartSide: NoteSideRight, StartLine: 2, Line: 3,
			Target: NoteTargetWorking,
		},
		Placement: NotePlacement{
			Path: "main.go", Side: NoteSideRight, StartSide: NoteSideRight,
			StartLine: 2, Line: 3, Target: NoteTargetWorking, Quality: PlacementExact,
		},
	}
}
