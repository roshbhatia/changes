package workspaceview

import (
	"github.com/roshbhatia/changes/internal/provider"
	sharedconfig "github.com/roshbhatia/go-utils/config"
)

const (
	SnapshotVersion = "changes.workspace/v1"
	EventVersion    = "changes.workspace.event/v1"
)

type Snapshot struct {
	Version    string          `json:"version" jsonschema:"required,const=changes.workspace/v1"`
	Repository Repository      `json:"repository" jsonschema:"required"`
	Comparison Comparison      `json:"comparison" jsonschema:"required"`
	Freshness  Freshness       `json:"freshness" jsonschema:"required"`
	History    []HistoryEntry  `json:"history" jsonschema:"required"`
	Groups     []Group         `json:"groups" jsonschema:"required"`
	Files      []File          `json:"files" jsonschema:"required"`
	Notes      []provider.Note `json:"notes" jsonschema:"required"`
	Threads    []NoteThread    `json:"threads" jsonschema:"required"`
	Failures   []Failure       `json:"failures" jsonschema:"required"`
	Rendered   string          `json:"rendered" jsonschema:"required"`
}

type Repository struct {
	Root   string `json:"root" jsonschema:"required"`
	Name   string `json:"name" jsonschema:"required"`
	Branch string `json:"branch,omitempty"`
	Head   string `json:"head,omitempty"`
}

type Comparison struct {
	Kind        string `json:"kind" jsonschema:"required,enum=working,enum=staged,enum=commit"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Base        string `json:"base,omitempty"`
	Head        string `json:"head,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Fingerprint string `json:"fingerprint" jsonschema:"required"`
	Layout      string `json:"layout" jsonschema:"required,enum=unified,enum=side-by-side"`
	FirstParent bool   `json:"firstParent,omitempty"`
}

type Freshness struct {
	State       string `json:"state" jsonschema:"required,enum=fresh,enum=cached,enum=stale,enum=refreshing"`
	GeneratedAt string `json:"generatedAt" jsonschema:"required,format=date-time"`
	RefreshedAt string `json:"refreshedAt,omitempty" jsonschema:"format=date-time"`
	CacheHit    bool   `json:"cacheHit,omitempty"`
}

type HistoryEntry struct {
	OID         string   `json:"oid" jsonschema:"required"`
	Parent      string   `json:"parent,omitempty"`
	Summary     string   `json:"summary" jsonschema:"required"`
	Author      string   `json:"author" jsonschema:"required"`
	AuthoredAt  string   `json:"authoredAt" jsonschema:"required,format=date-time"`
	NoteCount   int      `json:"noteCount,omitempty" jsonschema:"minimum=0"`
	NoteAuthors []string `json:"noteAuthors,omitempty"`
}

type Group struct {
	ID       string   `json:"id" jsonschema:"required"`
	Title    string   `json:"title" jsonschema:"required"`
	Summary  string   `json:"summary,omitempty"`
	ParentID string   `json:"parentId,omitempty"`
	Order    int      `json:"order,omitempty"`
	Files    []string `json:"files" jsonschema:"required"`
}

type File struct {
	Path        string   `json:"path" jsonschema:"required"`
	Added       int      `json:"added" jsonschema:"required,minimum=0"`
	Deleted     int      `json:"deleted" jsonschema:"required,minimum=0"`
	NoteCount   int      `json:"noteCount,omitempty" jsonschema:"minimum=0"`
	NoteAuthors []string `json:"noteAuthors,omitempty"`
	Hunks       []Hunk   `json:"hunks" jsonschema:"required"`
}

type Hunk struct {
	OldStart int    `json:"oldStart" jsonschema:"required,minimum=0"`
	NewStart int    `json:"newStart" jsonschema:"required,minimum=0"`
	Symbol   string `json:"symbol,omitempty"`
	Lines    []Line `json:"lines" jsonschema:"required"`
}

type Line struct {
	Kind    string   `json:"kind" jsonschema:"required,enum=context,enum=added,enum=deleted"`
	Text    string   `json:"text" jsonschema:"required"`
	OldLine int      `json:"oldLine,omitempty" jsonschema:"minimum=0"`
	NewLine int      `json:"newLine,omitempty" jsonschema:"minimum=0"`
	NoteIDs []string `json:"noteIds,omitempty"`
}

type NoteThread struct {
	ID        string          `json:"id" jsonschema:"required"`
	Path      string          `json:"path" jsonschema:"required"`
	Side      string          `json:"side,omitempty"`
	StartLine int             `json:"startLine,omitempty" jsonschema:"minimum=0"`
	Line      int             `json:"line,omitempty" jsonschema:"minimum=0"`
	State     string          `json:"state" jsonschema:"required"`
	Comments  []provider.Note `json:"comments" jsonschema:"required"`
}

type Failure struct {
	Source  string `json:"source,omitempty"`
	Message string `json:"message" jsonschema:"required"`
	Stale   bool   `json:"stale,omitempty"`
}

type Event struct {
	Version  string    `json:"version" jsonschema:"required,const=changes.workspace.event/v1"`
	Type     string    `json:"type" jsonschema:"required,enum=snapshot,enum=refreshing,enum=error"`
	At       string    `json:"at" jsonschema:"required,format=date-time"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type State struct {
	Version      string   `json:"version"`
	Repository   string   `json:"repository"`
	Dock         string   `json:"dock"`
	Navigator    string   `json:"navigator"`
	Tab          string   `json:"tab"`
	Layout       string   `json:"layout"`
	View         string   `json:"view"`
	SelectedPath string   `json:"selectedPath,omitempty"`
	SelectedLine int      `json:"selectedLine,omitempty"`
	SelectedOID  string   `json:"selectedOid,omitempty"`
	Commands     []string `json:"commands,omitempty"`
}

// Schema emits the public snapshot contract from the same Go types used at runtime.
func Schema() ([]byte, error) {
	return sharedconfig.Schema[Snapshot]("Changes workspace snapshot")
}
