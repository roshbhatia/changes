package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-shellwords"
	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/completion"
	gitutil "github.com/roshbhatia/go-utils/git"
)

const (
	maxNoteMessageBytes = 1 << 20
	maxNoteFileBytes    = 16 << 20
)

type noteDocument struct {
	Version string          `json:"version"`
	Notes   []provider.Note `json:"notes"`
}

type noteComparisonFlags struct {
	commit string
	from   string
	to     string
	staged bool
}

func runNote(args []string) {
	metadata := subcommandMetadata("note")
	if len(args) == 1 && isHelp(args[0]) {
		flags := flag.NewFlagSet("changes note", flag.ContinueOnError)
		printCommandHelp(flags.Output(), "changes note <command>", metadata, flags)
		return
	}
	if len(args) == 0 || (args[0] != "add" && args[0] != "generate" && args[0] != "list") {
		fail(errors.New("note requires add, generate, or list"))
	}
	if args[0] == "add" {
		runNoteAdd(args[1:])
		return
	}
	if args[0] == "generate" {
		runNoteGenerate(args[1:])
		return
	}
	runNoteList(args[1:])
}

func runNoteGenerate(args []string) {
	metadata := subcommandMetadata("note", "generate")
	flags := flag.NewFlagSet("changes note generate", flag.ContinueOnError)
	configPath := flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	providerName := flags.String("provider", "", flagDescription(metadata, "provider"))
	storeName := flags.String("store", "", flagDescription(metadata, "store"))
	session := flags.String("session", "", flagDescription(metadata, "session"))
	asJSON := flags.Bool("json", false, flagDescription(metadata, "json"))
	comparison := addNoteComparisonFlags(flags, metadata)
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes note generate [flags]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(errors.New("note generate accepts only flags"))
	}
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	if configured.Notes.GeneratorTimeout.Duration() <= 0 {
		fail(errors.New("notes.generatorTimeout must be greater than zero"))
	}
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	spec, _, err := noteSpec(cwd, "", *comparison)
	if err != nil {
		fail(err)
	}
	snapshot, err := stableNoteSnapshot(spec)
	if err != nil {
		fail(err)
	}
	if len(snapshot.files) == 0 {
		if *asJSON {
			fmt.Println(`{"version":"changes.notes/v1","notes":[]}`)
		}
		return
	}
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	generators, err := selectOneNoteProvider(discovery.Providers, provider.ActionNotesGenerate, *providerName, "generator", "provider")
	if err != nil {
		fail(err)
	}
	writers, err := selectOneNoteProvider(discovery.Providers, provider.ActionNotesCreate, *storeName, "store", "store")
	if err != nil {
		fail(err)
	}
	request := noteRequestWithIDs(spec, snapshot.files, snapshot.patch, snapshot.base, snapshot.head)
	request.Patch = snapshot.placementPatch
	ctx, cancel := context.WithTimeout(context.Background(), configured.Notes.GeneratorTimeout.Duration())
	response, err := provider.Run(ctx, generators[0], provider.ActionNotesGenerate, request, provider.CachePolicy{})
	cancel()
	if err != nil {
		fail(err)
	}
	allowed := make(map[string]bool, len(snapshot.files))
	for _, path := range snapshot.files {
		allowed[filepath.ToSlash(filepath.Clean(path))] = true
	}
	for index := range response.Notes {
		note := &response.Notes[index]
		if !allowed[note.Anchor.Path] {
			fail(fmt.Errorf("provider %s generated a note for unchanged path %q", generators[0].Manifest.Name, note.Anchor.Path))
		}
		if err := validateNoteVisibility(snapshot.placementPatch, note); err != nil {
			fail(fmt.Errorf("provider %s: %w", generators[0].Manifest.Name, err))
		}
		if note.Placement.Quality != provider.PlacementExact {
			fail(fmt.Errorf("provider %s generated note %q without an exact placement", generators[0].Manifest.Name, note.ID))
		}
		contextLine, err := noteRangeInFilePatch(
			snapshot.placementPatch, note.Anchor.Path, note.Anchor.Side, note.Anchor.StartLine, note.Anchor.Line,
		)
		if err != nil {
			fail(fmt.Errorf("provider %s: %w", generators[0].Manifest.Name, err))
		}
		note.Anchor.Context = contextLine
	}
	after, err := stableNoteSnapshot(spec)
	if err != nil {
		fail(err)
	}
	if after.patch != snapshot.patch || after.placementPatch != snapshot.placementPatch ||
		after.base != snapshot.base || after.head != snapshot.head ||
		!slices.Equal(after.files, snapshot.files) {
		fail(errors.New("selected comparison changed while generating notes"))
	}
	drafts := make([]provider.NoteDraft, 0, len(response.Notes))
	for _, generated := range response.Notes {
		provenance := generated.Provenance
		if provenance.SessionID == "" {
			provenance.SessionID = *session
		}
		if provenance.WorkingDirectory == "" {
			provenance.WorkingDirectory = spec.Dir
		}
		drafts = append(drafts, provider.NoteDraft{
			Key:     generated.ID,
			Summary: generated.Summary, Rationale: generated.Rationale, Author: generated.Author,
			Origin: provider.NoteOriginAgent, Session: *session, Provenance: provenance, Anchor: generated.Anchor,
		})
	}
	created := []provider.Note{}
	if len(drafts) > 0 {
		writeRequest := noteRequestWithIDs(spec, snapshot.files, snapshot.patch, snapshot.base, snapshot.head)
		writeRequest.Notes = drafts
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(configured.Providers.Timeout))
		written, writeErr := provider.Run(ctx, writers[0], provider.ActionNotesCreate, writeRequest, provider.CachePolicy{})
		cancel()
		if writeErr != nil {
			fail(writeErr)
		}
		created = written.Notes
	}
	if *asJSON {
		data, err := json.Marshal(noteDocument{Version: "changes.notes/v1", Notes: created})
		if err != nil {
			fail(err)
		}
		fmt.Println(string(data))
		return
	}
	for _, note := range created {
		fmt.Printf("note: %s %s\n", cleanNoteOneLine(note.ID), noteLocation(note))
	}
}

func runNoteAdd(args []string) {
	metadata := subcommandMetadata("note", "add")
	flags := flag.NewFlagSet("changes note add", flag.ContinueOnError)
	configPath := flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	file := flags.String("file", "", flagDescription(metadata, "file"))
	line := flags.Int("line", 0, flagDescription(metadata, "line"))
	startLine := flags.Int("start-line", 0, flagDescription(metadata, "start-line"))
	side := flags.String("side", "right", flagDescription(metadata, "side"))
	message := flags.String("message", "", flagDescription(metadata, "message"))
	messageFile := flags.String("message-file", "", flagDescription(metadata, "message-file"))
	author := flags.String("author", "", flagDescription(metadata, "author"))
	origin := flags.String("origin", provider.NoteOriginUser, flagDescription(metadata, "origin"))
	session := flags.String("session", "", flagDescription(metadata, "session"))
	providerName := flags.String("provider", "", flagDescription(metadata, "provider"))
	expectedFileSHA256 := flags.String("expected-file-sha256", "", flagDescription(metadata, "expected-file-sha256"))
	asJSON := flags.Bool("json", false, flagDescription(metadata, "json"))
	comparison := addNoteComparisonFlags(flags, metadata)
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes note add [flags]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(errors.New("note add accepts only flags"))
	}
	if *file == "" {
		fail(errors.New("note add requires --file"))
	}
	if *line < 0 || *startLine < 0 || (*line == 0 && *startLine != 0) || (*line > 0 && *startLine > *line) {
		fail(errors.New("note range must use non-negative lines with start-line before line"))
	}
	messageSet := false
	messageFileSet := false
	flags.Visit(func(found *flag.Flag) {
		messageSet = messageSet || found.Name == "message"
		messageFileSet = messageFileSet || found.Name == "message-file"
	})
	resolvedSide, err := noteSide(*side)
	if err != nil {
		fail(err)
	}
	if messageSet && messageFileSet {
		fail(errors.New("--message and --message-file are mutually exclusive"))
	}
	if *expectedFileSHA256 != "" {
		decoded, decodeErr := hex.DecodeString(*expectedFileSHA256)
		if decodeErr != nil || len(decoded) != sha256.Size {
			fail(errors.New("--expected-file-sha256 must be a 64-character SHA-256 digest"))
		}
	}
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	interactive := !messageSet && !messageFileSet
	rawMessage := *message
	if messageFileSet {
		rawMessage, err = readNoteMessage(*messageFile)
		if err != nil {
			fail(err)
		}
	}
	if interactive {
		rawMessage, err = editNote(configured.Notes.Editor)
		if err != nil {
			fail(err)
		}
		if rawMessage == "" {
			return
		}
	}
	summary, rationale := splitNoteMessage(rawMessage, interactive)
	if summary == "" {
		fail(errors.New("note message has no summary"))
	}
	if *author == "" {
		*author = defaultNoteAuthor()
	}
	if *origin != provider.NoteOriginAgent && *origin != provider.NoteOriginUser {
		fail(errors.New("--origin must be agent or user"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	spec, relative, err := noteSpec(cwd, *file, *comparison)
	if err != nil {
		fail(err)
	}
	comparisonPatch, placementPatch, base, head, err := stableNoteComparison(
		spec, relative, resolvedSide, *expectedFileSHA256,
	)
	if err != nil {
		fail(err)
	}
	contextLine, err := noteRangeInFilePatch(placementPatch, relative, resolvedSide, *startLine, *line)
	if err != nil {
		fail(fmt.Errorf("%s: %w", relative, err))
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(comparisonPatch)))
	provenanceKind := "manual"
	if *origin == provider.NoteOriginAgent {
		provenanceKind = "harness"
	}
	draft := &provider.NoteDraft{
		Summary: summary, Rationale: rationale, Author: *author, Origin: *origin, Session: *session,
		Provenance: provider.NoteProvenance{Kind: provenanceKind, Tool: "changes", SessionID: *session, WorkingDirectory: cwd},
		Anchor: provider.NoteAnchor{
			Path: relative, Side: resolvedSide, StartSide: rangeStartSide(resolvedSide, *startLine),
			StartLine: *startLine, Line: *line,
			Base: base, Head: head, Fingerprint: fingerprint, Context: contextLine,
			Target: noteTarget(spec),
		},
	}
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	writable, err := selectOneNoteProvider(discovery.Providers, provider.ActionNotesCreate, *providerName, "writer", "provider")
	if err != nil {
		fail(err)
	}
	selected := writable[0]
	request := noteRequestWithIDs(spec, []string{relative}, comparisonPatch, base, head)
	request.Note = draft
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(configured.Providers.Timeout))
	response, err := provider.Run(ctx, selected, provider.ActionNotesCreate, request, provider.CachePolicy{})
	cancel()
	if err != nil {
		fail(err)
	}
	created := response.Notes[0]
	if *asJSON {
		data, err := json.Marshal(created)
		if err != nil {
			fail(err)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Printf("note: %s %s\n", cleanNoteOneLine(created.ID), noteLocation(created))
}

func runNoteList(args []string) {
	metadata := subcommandMetadata("note", "list")
	flags := flag.NewFlagSet("changes note list", flag.ContinueOnError)
	configPath := flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	providerName := flags.String("provider", "", flagDescription(metadata, "provider"))
	asJSON := flags.Bool("json", false, flagDescription(metadata, "json"))
	comparison := addNoteComparisonFlags(flags, metadata)
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes note list [flags]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(errors.New("note list accepts only flags"))
	}
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	spec, _, err := noteSpec(cwd, "", *comparison)
	if err != nil {
		fail(err)
	}
	snapshot, err := stableNoteSnapshot(spec)
	if err != nil {
		fail(err)
	}
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	readers, err := selectNoteProviders(discovery.Providers, provider.ActionNotes, *providerName)
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(configured.Providers.Timeout))
	result := readNotes(
		ctx, spec, snapshot.files, snapshot.patch, snapshot.placementPatch, snapshot.base, snapshot.head,
		readers,
	)
	cancel()
	if len(result.failures) > 0 {
		fail(errors.Join(result.failures...))
	}
	notes := result.notes
	if *asJSON {
		if notes == nil {
			notes = []provider.Note{}
		}
		data, err := json.Marshal(noteDocument{Version: "changes.notes/v1", Notes: notes})
		if err != nil {
			fail(err)
		}
		fmt.Println(string(data))
		return
	}
	if output := renderNoteRows(notes, 100); output != "" {
		fmt.Println(output)
	}
}

func addNoteComparisonFlags(flags *flag.FlagSet, metadata completion.Command) *noteComparisonFlags {
	comparison := &noteComparisonFlags{}
	flags.StringVar(&comparison.commit, "commit", "", flagDescription(metadata, "commit"))
	flags.StringVar(&comparison.from, "from", "", flagDescription(metadata, "from"))
	flags.StringVar(&comparison.to, "to", "", flagDescription(metadata, "to"))
	flags.BoolVar(&comparison.staged, "staged", false, flagDescription(metadata, "staged"))
	return comparison
}

func noteSpec(cwd, file string, flags noteComparisonFlags) (source.Spec, string, error) {
	root, err := source.Root(cwd)
	if err != nil {
		return source.Spec{}, "", err
	}
	if flags.commit != "" && (flags.from != "" || flags.to != "" || flags.staged) {
		return source.Spec{}, "", errors.New("--commit cannot be combined with --from, --to, or --staged")
	}
	if flags.to != "" && flags.from == "" {
		return source.Spec{}, "", errors.New("--to requires --from")
	}
	if flags.staged && flags.to != "" {
		return source.Spec{}, "", errors.New("--staged cannot be combined with --to")
	}
	spec := source.Spec{Dir: root, From: flags.from, To: flags.to, Staged: flags.staged}
	if flags.commit != "" {
		head, err := noteGitOutput(root, "rev-parse", "--verify", flags.commit+"^{commit}")
		if err != nil {
			return source.Spec{}, "", fmt.Errorf("resolve --commit %q: %w", flags.commit, err)
		}
		base, err := noteGitOutput(root, "rev-parse", "--verify", strings.TrimSpace(head)+"^1")
		if err != nil {
			return source.Spec{}, "", fmt.Errorf("--commit %q has no first parent; use --from and --to", flags.commit)
		}
		spec.From, spec.To = strings.TrimSpace(base), strings.TrimSpace(head)
	}
	if file == "" {
		return spec, "", nil
	}
	absolute := file
	if !filepath.IsAbs(absolute) {
		resolvedCWD, resolveErr := filepath.EvalSymlinks(cwd)
		if resolveErr != nil {
			return source.Spec{}, "", resolveErr
		}
		absolute = filepath.Join(resolvedCWD, absolute)
	} else {
		resolvedParent, resolveErr := filepath.EvalSymlinks(filepath.Dir(absolute))
		if resolveErr != nil {
			return source.Spec{}, "", resolveErr
		}
		absolute = filepath.Join(resolvedParent, filepath.Base(absolute))
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return source.Spec{}, "", fmt.Errorf("--file %q must be inside %s", file, root)
	}
	relative = filepath.ToSlash(relative)
	spec.Paths = []string{absolute}
	return spec, relative, nil
}

func noteGitEnvironment() []string {
	environment := gitutil.CleanEnv()
	kept := environment[:0]
	for _, entry := range environment {
		if strings.HasPrefix(entry, "GIT_NO_LAZY_FETCH=") ||
			strings.HasPrefix(entry, "GIT_NO_REPLACE_OBJECTS=") ||
			strings.HasPrefix(entry, "GIT_REPLACE_REF_BASE=") {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1")
}

func noteGitCommand(directory string, arguments ...string) *exec.Cmd {
	arguments = append([]string{"-c", "core.fsmonitor=false"}, arguments...)
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = noteGitEnvironment()
	return command
}

func noteGitOutput(directory string, arguments ...string) (string, error) {
	command := noteGitCommand(directory, arguments...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func validateExpectedNoteFile(spec source.Spec, relative, side, expected string) error {
	if expected == "" {
		return nil
	}
	if err := validateExpectedNoteFileDomain(spec, relative, side); err != nil {
		return err
	}
	content, err := noteComparisonFile(spec, relative, side)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(content)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return errors.New("target file does not match the selected diff side")
	}
	return nil
}

func validateExpectedNoteFileDomain(spec source.Spec, relative, side string) error {
	path := filepath.Join(spec.Dir, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("target path must be a regular file")
	}
	if side != provider.NoteSideRight || spec.Staged || spec.To != "" {
		return nil
	}
	command := noteGitCommand(spec.Dir, "check-attr", "-z", "filter", "working-tree-encoding", "--", relative)
	output := &noteFileBuffer{limit: maxNoteFileBytes}
	var stderr bytes.Buffer
	command.Stdout = output
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("inspect Git attributes for %s: %s: %w", relative, strings.TrimSpace(stderr.String()), err)
	}
	if output.exceeded {
		return errors.New("Git attributes output exceeds the annotation limit")
	}
	fields := bytes.Split(output.Bytes(), []byte{0})
	for index := 0; index+2 < len(fields); index += 3 {
		attribute, value := string(fields[index+1]), string(fields[index+2])
		if value != "" && value != "unspecified" && value != "unset" {
			return fmt.Errorf("target file uses unsupported Git %s conversion", attribute)
		}
	}
	return nil
}

func noteComparisonFile(spec source.Spec, relative, side string) ([]byte, error) {
	object := ""
	if side == provider.NoteSideRight {
		switch {
		case spec.Staged:
			object = ":./" + relative
		case spec.To != "":
			object = spec.To + ":" + relative
		default:
			return readBoundedNoteFile(filepath.Join(spec.Dir, filepath.FromSlash(relative)))
		}
	} else {
		switch {
		case spec.Staged && spec.From != "":
			object = spec.From + ":" + relative
		case spec.Staged:
			object = "HEAD:" + relative
		case spec.From != "":
			object = spec.From + ":" + relative
		default:
			object = ":./" + relative
		}
	}
	command := noteGitCommand(spec.Dir, "show", object)
	output := &noteFileBuffer{limit: maxNoteFileBytes}
	var stderr bytes.Buffer
	command.Stdout = output
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("read selected diff side for %s: %s: %w", relative, strings.TrimSpace(stderr.String()), err)
	}
	if output.exceeded {
		return nil, fmt.Errorf("target file exceeds %d bytes", maxNoteFileBytes)
	}
	return output.Bytes(), nil
}

func readBoundedNoteFile(path string) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, errors.New("target path must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return nil, errors.New("target path changed while opening it")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxNoteFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxNoteFileBytes {
		return nil, fmt.Errorf("target file exceeds %d bytes", maxNoteFileBytes)
	}
	return content, nil
}

type noteFileBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *noteFileBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - buffer.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = buffer.Buffer.Write(value[:remaining])
	}
	if remaining < len(value) {
		buffer.exceeded = true
	}
	return written, nil
}

func noteRequestWithIDs(spec source.Spec, files []string, patch, base, head string) provider.Request {
	return provider.Request{
		Base: base, Head: head, Directory: spec.Dir, Files: files,
		From: spec.From, To: spec.To, Staged: spec.Staged,
		Fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(patch))),
	}
}

func stableNoteComparison(spec source.Spec, relative, side, expected string) (
	patch, placementPatch, base, head string,
	err error,
) {
	comparisonSpec := spec
	comparisonSpec.Paths = nil
	for range 3 {
		comparison, resolveErr := resolveNoteComparison(comparisonSpec)
		if resolveErr != nil {
			return "", "", "", "", resolveErr
		}
		base, head = comparison.base, comparison.head
		placementSpec := comparison.spec
		placementSpec.Paths = spec.Paths
		patch, placementPatch, err = notePatches(comparison.spec, placementSpec)
		if err != nil {
			return "", "", "", "", err
		}
		digest := ""
		if expected != "" {
			digest, err = noteFileDigest(comparison.spec, relative, side)
			if err != nil {
				return "", "", "", "", err
			}
		}
		afterPatch, afterPlacementPatch, afterErr := notePatches(comparison.spec, placementSpec)
		if afterErr != nil {
			return "", "", "", "", afterErr
		}
		afterDigest := ""
		if expected != "" {
			afterDigest, err = noteFileDigest(comparison.spec, relative, side)
			if err != nil {
				return "", "", "", "", err
			}
		}
		afterComparison, afterResolveErr := resolveNoteComparison(comparisonSpec)
		if afterResolveErr != nil {
			return "", "", "", "", afterResolveErr
		}
		if base == afterComparison.base && head == afterComparison.head &&
			patch == afterPatch && placementPatch == afterPlacementPatch && digest == afterDigest {
			if expected != "" && !strings.EqualFold(digest, expected) {
				return "", "", "", "", errors.New("target file does not match the selected diff side")
			}
			return patch, placementPatch, base, head, nil
		}
	}
	return "", "", "", "", errors.New("selected comparison changed while capturing the note")
}

func notePatches(comparison, placement source.Spec) (patch, placementPatch string, err error) {
	placementPatch, err = placement.Diff()
	if err != nil {
		return "", "", err
	}
	if noteTarget(comparison) != provider.NoteTargetCommits {
		if slices.Equal(comparison.Paths, placement.Paths) {
			return placementPatch, placementPatch, nil
		}
		patch, err = comparison.Diff()
		return patch, placementPatch, err
	}
	patch, err = comparison.NoteDiff()
	return patch, placementPatch, err
}

func noteFileDigest(spec source.Spec, relative, side string) (string, error) {
	if err := validateExpectedNoteFileDomain(spec, relative, side); err != nil {
		return "", err
	}
	content, err := noteComparisonFile(spec, relative, side)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}

type resolvedNoteComparison struct {
	spec source.Spec
	base string
	head string
}

func resolveNoteComparison(spec source.Spec) (resolvedNoteComparison, error) {
	spec.NoLazyFetch = true
	base, head := spec.ComparisonIDs()
	resolved := spec
	if spec.From == "" && spec.To == "" && !spec.Staged {
		if base == "" {
			return resolvedNoteComparison{}, errors.New("resolve working-tree comparison identity")
		}
		return resolvedNoteComparison{spec: resolved, base: base}, nil
	}
	if base == "" {
		left := spec.From
		if left == "" {
			left = "HEAD"
		}
		return resolvedNoteComparison{}, fmt.Errorf("resolve comparison base %q", left)
	}
	resolved.From = base
	if spec.To != "" {
		if head == "" {
			return resolvedNoteComparison{}, fmt.Errorf("resolve comparison head %q", spec.To)
		}
		resolved.To = head
	}
	return resolvedNoteComparison{spec: resolved, base: base, head: head}, nil
}

func stableExpectedComparison(
	resolve func() (resolvedNoteComparison, error),
	readPatch func(source.Spec) (string, error),
	readDigest func(source.Spec) (string, error),
	expected string,
) (patch, base, head string, err error) {
	for range 3 {
		comparison, resolveErr := resolve()
		if resolveErr != nil {
			return "", "", "", resolveErr
		}
		base, head = comparison.base, comparison.head
		patch, err = readPatch(comparison.spec)
		if err != nil {
			return "", "", "", err
		}
		digest, digestErr := readDigest(comparison.spec)
		if digestErr != nil {
			return "", "", "", digestErr
		}
		afterPatch, afterErr := readPatch(comparison.spec)
		if afterErr != nil {
			return "", "", "", afterErr
		}
		afterDigest, afterDigestErr := readDigest(comparison.spec)
		if afterDigestErr != nil {
			return "", "", "", afterDigestErr
		}
		afterComparison, afterResolveErr := resolve()
		if afterResolveErr != nil {
			return "", "", "", afterResolveErr
		}
		if base == afterComparison.base && head == afterComparison.head && patch == afterPatch && digest == afterDigest {
			if !strings.EqualFold(digest, expected) {
				return "", "", "", errors.New("target file does not match the selected diff side")
			}
			return patch, base, head, nil
		}
	}
	return "", "", "", errors.New("selected comparison changed while capturing the note")
}

func stableComparison(
	resolve func() (resolvedNoteComparison, error),
	readPatch func(source.Spec) (string, error),
) (patch, base, head string, err error) {
	for range 3 {
		comparison, resolveErr := resolve()
		if resolveErr != nil {
			return "", "", "", resolveErr
		}
		base, head = comparison.base, comparison.head
		patch, err = readPatch(comparison.spec)
		if err != nil {
			return "", "", "", err
		}
		afterPatch, afterErr := readPatch(comparison.spec)
		if afterErr != nil {
			return "", "", "", afterErr
		}
		afterComparison, afterResolveErr := resolve()
		if afterResolveErr != nil {
			return "", "", "", afterResolveErr
		}
		if base == afterComparison.base && head == afterComparison.head && patch == afterPatch {
			return patch, base, head, nil
		}
	}
	return "", "", "", errors.New("selected comparison changed while capturing the note")
}

type noteSnapshot struct {
	patch          string
	placementPatch string
	files          []string
	base           string
	head           string
}

func stableNoteSnapshot(spec source.Spec) (noteSnapshot, error) {
	comparison := spec
	comparison.Paths = nil
	for range 3 {
		resolved, err := resolveNoteComparison(comparison)
		if err != nil {
			return noteSnapshot{}, err
		}
		placement := resolved.spec
		placement.Paths = spec.Paths
		before, beforePlacement, err := notePatches(resolved.spec, placement)
		if err != nil {
			return noteSnapshot{}, err
		}
		files, err := resolved.spec.NoteFiles()
		if err != nil {
			return noteSnapshot{}, err
		}
		after, afterPlacement, err := notePatches(resolved.spec, placement)
		if err != nil {
			return noteSnapshot{}, err
		}
		afterFiles, err := resolved.spec.NoteFiles()
		if err != nil {
			return noteSnapshot{}, err
		}
		afterResolved, err := resolveNoteComparison(comparison)
		if err != nil {
			return noteSnapshot{}, err
		}
		if resolved.base == afterResolved.base && resolved.head == afterResolved.head &&
			before == after && beforePlacement == afterPlacement && slices.Equal(files, afterFiles) {
			return noteSnapshot{
				patch: before, placementPatch: beforePlacement,
				files: files, base: resolved.base, head: resolved.head,
			}, nil
		}
	}
	return noteSnapshot{}, errors.New("selected comparison changed while reading notes")
}

func noteTarget(spec source.Spec) string {
	if spec.To != "" {
		return provider.NoteTargetCommits
	}
	if spec.Staged {
		return provider.NoteTargetIndex
	}
	return provider.NoteTargetWorking
}

type noteProviderRead struct {
	notes         []provider.Note
	failures      []error
	failedSources map[string]bool
}

func readNotes(
	ctx context.Context,
	spec source.Spec,
	files []string,
	patch string,
	placementPatch string,
	base string,
	head string,
	providers []provider.LoadedManifest,
) noteProviderRead {
	if len(files) == 0 {
		return noteProviderRead{notes: []provider.Note{}, failedSources: map[string]bool{}}
	}
	request := noteRequestWithIDs(spec, files, patch, base, head)
	allowed := make(map[string]bool, len(files))
	for _, path := range files {
		allowed[filepath.ToSlash(filepath.Clean(path))] = true
	}
	all := []provider.Note{}
	var failures []error
	failedSources := map[string]bool{}
	for _, configured := range providers {
		response, err := provider.Run(ctx, configured, provider.ActionNotes, request, provider.CachePolicy{})
		if err != nil {
			failures = append(failures, err)
			failedSources[configured.Manifest.Name] = true
			continue
		}
		for _, note := range response.Notes {
			path, _ := notePosition(note)
			if !allowed[filepath.ToSlash(filepath.Clean(path))] {
				continue
			}
			if err := validateNoteVisibility(placementPatch, &note); err != nil {
				failures = append(failures, fmt.Errorf("provider %s: %w", configured.Manifest.Name, err))
				failedSources[configured.Manifest.Name] = true
				continue
			}
			all = append(all, note)
		}
	}
	filtered := all[:0]
	for _, note := range all {
		if !failedSources[note.Source] {
			filtered = append(filtered, note)
		}
	}
	all = filtered
	sortNotes(all)
	return noteProviderRead{notes: all, failures: failures, failedSources: failedSources}
}

func validateNoteVisibility(patch string, note *provider.Note) error {
	placement := &note.Placement
	if placement.Line == 0 || placement.Quality != provider.PlacementExact && placement.Quality != provider.PlacementContext {
		return nil
	}
	if placement.StartLine > 0 && placement.StartSide != placement.Side {
		if _, err := noteRangeInFilePatch(patch, placement.Path, placement.StartSide, 0, placement.StartLine); err != nil {
			if degradeInvisiblePlacement(note) {
				return nil
			}
			return fmt.Errorf("note %q has an invisible %s range start: %w", note.ID, placement.Quality, err)
		}
		context, err := noteRangeInFilePatch(patch, placement.Path, placement.Side, 0, placement.Line)
		if err != nil {
			if degradeInvisiblePlacement(note) {
				return nil
			}
			return fmt.Errorf("note %q has an invisible %s range end: %w", note.ID, placement.Quality, err)
		}
		if !contextPlacementMatches(note, context) {
			degradeContextPlacement(placement)
		}
		return nil
	}
	context, err := noteRangeInFilePatch(patch, placement.Path, placement.Side, placement.StartLine, placement.Line)
	if err != nil {
		if degradeInvisiblePlacement(note) {
			return nil
		}
		return fmt.Errorf("note %q has an invisible %s placement: %w", note.ID, placement.Quality, err)
	}
	if !contextPlacementMatches(note, context) {
		degradeContextPlacement(placement)
	}
	return nil
}

func contextPlacementMatches(note *provider.Note, context string) bool {
	if note.Placement.Quality != provider.PlacementContext {
		return true
	}
	return normalizedNoteContext(context) != "" &&
		normalizedNoteContext(context) == normalizedNoteContext(note.Anchor.Context)
}

func normalizedNoteContext(value string) string {
	runes := []rune(strings.TrimSpace(cleanNoteOneLine(value)))
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return string(runes)
}

func degradeContextPlacement(placement *provider.NotePlacement) bool {
	if placement.Quality != provider.PlacementContext {
		return false
	}
	placement.StartSide = ""
	placement.StartLine = 0
	placement.Line = 0
	placement.Quality = provider.PlacementFile
	return true
}

func degradeInvisiblePlacement(note *provider.Note) bool {
	if degradeContextPlacement(&note.Placement) {
		return true
	}
	if note.Placement.Quality != provider.PlacementExact ||
		note.Anchor.Target != provider.NoteTargetCommits ||
		note.Placement.Target != provider.NoteTargetCommits {
		return false
	}
	note.Placement.StartSide = ""
	note.Placement.StartLine = 0
	note.Placement.Line = 0
	note.Placement.Quality = provider.PlacementFile
	return true
}

func selectNoteProviders(
	configured []provider.LoadedManifest,
	action string,
	name string,
) ([]provider.LoadedManifest, error) {
	selected := []provider.LoadedManifest{}
	known := false
	for _, candidate := range configured {
		if name != "" && candidate.Manifest.Name == name {
			known = true
		}
		if (name == "" || candidate.Manifest.Name == name) && provider.Supports(candidate.Manifest, action) {
			selected = append(selected, candidate)
		}
	}
	if name != "" && !known {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	if name != "" && len(selected) == 0 {
		return nil, fmt.Errorf("provider %q does not implement %s", name, action)
	}
	return selected, nil
}

func selectOneNoteProvider(
	configured []provider.LoadedManifest,
	action string,
	name string,
	role string,
	selectionFlag string,
) ([]provider.LoadedManifest, error) {
	selected, err := selectNoteProviders(configured, action, name)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no configured provider implements %s", action)
	}
	if len(selected) > 1 {
		names := make([]string, 0, len(selected))
		for _, candidate := range selected {
			names = append(names, candidate.Manifest.Name)
		}
		return nil, fmt.Errorf("multiple note %ss implement %s (%s); select one with --%s", role, action, strings.Join(names, ", "), selectionFlag)
	}
	return selected, nil
}

func noteLineInPatch(patch, side string, wanted int) (string, error) {
	return noteRangeInPatch(patch, side, 0, wanted)
}

func noteRangeInPatch(patch, side string, start, end int) (string, error) {
	if end == 0 {
		return "", nil
	}
	visible := map[int]string{}
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			oldLine = patchHunkStart(line, '-')
			newLine = patchHunkStart(line, '+')
			inHunk = true
		case !inHunk || line == "" || strings.HasPrefix(line, "\\ No newline"):
			continue
		case line[0] == '-':
			if side == provider.NoteSideLeft {
				visible[oldLine] = line[1:]
			}
			oldLine++
		case line[0] == '+':
			if side == provider.NoteSideRight {
				visible[newLine] = line[1:]
			}
			newLine++
		case line[0] == ' ':
			if side == provider.NoteSideLeft {
				visible[oldLine] = line[1:]
			} else {
				visible[newLine] = line[1:]
			}
			oldLine++
			newLine++
		}
	}
	first := start
	if first == 0 {
		first = end
	}
	if end-first+1 > len(visible) {
		return "", fmt.Errorf("range %d-%d is not fully visible on the %s side of the selected diff", first, end, strings.ToLower(side))
	}
	for line := first; line <= end; line++ {
		if _, ok := visible[line]; !ok {
			return "", fmt.Errorf("line %d is not visible on the %s side of the selected diff", line, strings.ToLower(side))
		}
	}
	return visible[end], nil
}

func patchHunkStart(header string, marker byte) int {
	for _, field := range strings.Fields(header) {
		if len(field) < 2 || field[0] != marker {
			continue
		}
		value := strings.TrimPrefix(field, string(marker))
		if before, _, found := strings.Cut(value, ","); found {
			value = before
		}
		line, err := strconv.Atoi(value)
		if err == nil {
			return line
		}
	}
	return 0
}

func noteRangeInFilePatch(patch, path, side string, start, end int) (string, error) {
	for _, section := range splitPatchFiles(patch) {
		candidate := section.newPath
		if side == provider.NoteSideLeft {
			candidate = section.oldPath
		}
		if candidate == path || sectionHeaderMatches(section.header, path, side) {
			if end > 0 && (strings.HasPrefix(section.header, "diff --cc ") || strings.HasPrefix(section.header, "diff --combined ")) {
				return "", errors.New("combined diffs support file notes only")
			}
			return noteRangeInPatch(section.patch, side, start, end)
		}
	}
	return "", errors.New("file is not changed in the selected comparison")
}

type patchFile struct {
	oldPath string
	newPath string
	header  string
	patch   string
}

func splitPatchFiles(patch string) []patchFile {
	files := []patchFile{}
	current := patchFile{}
	inHeaders := false
	var section strings.Builder
	flush := func() {
		if section.Len() == 0 {
			return
		}
		current.patch = section.String()
		files = append(files, current)
		current = patchFile{}
		section.Reset()
	}
	for _, line := range strings.SplitAfter(patch, "\n") {
		if prefix := combinedDiffPrefix(line); prefix != "" {
			flush()
			inHeaders = true
			path := patchHeaderPath(line, prefix, "")
			current.oldPath, current.newPath = path, path
			current.header = strings.TrimSuffix(line, "\n")
		} else if strings.HasPrefix(line, "diff --git ") {
			flush()
			inHeaders = true
			current.oldPath, current.newPath = diffHeaderPaths(line)
			current.header = strings.TrimSuffix(line, "\n")
		}
		section.WriteString(line)
		if strings.HasPrefix(line, "@@") {
			inHeaders = false
		} else if inHeaders && strings.HasPrefix(line, "--- ") {
			current.oldPath = patchHeaderPath(line, "--- ", "a/")
		} else if inHeaders && strings.HasPrefix(line, "+++ ") {
			current.newPath = patchHeaderPath(line, "+++ ", "b/")
		}
	}
	flush()
	return files
}

func sectionHeaderMatches(header, path, side string) bool {
	if header == "diff --cc "+path || header == "diff --combined "+path {
		return true
	}
	if side == provider.NoteSideLeft {
		return strings.HasPrefix(header, "diff --git a/"+path+" ")
	}
	return strings.HasSuffix(header, " b/"+path)
}

func combinedDiffPrefix(line string) string {
	for _, prefix := range []string{"diff --cc ", "diff --combined "} {
		if strings.HasPrefix(line, prefix) {
			return prefix
		}
	}
	return ""
}

func diffHeaderPaths(line string) (string, string) {
	tokens := diffHeaderTokens(strings.TrimSuffix(strings.TrimPrefix(line, "diff --git "), "\n"))
	if len(tokens) != 2 {
		return "", ""
	}
	return filepath.ToSlash(strings.TrimPrefix(tokens[0], "a/")),
		filepath.ToSlash(strings.TrimPrefix(tokens[1], "b/"))
}

func diffHeaderTokens(value string) []string {
	tokens := []string{}
	for index := 0; index < len(value); {
		for index < len(value) && value[index] == ' ' {
			index++
		}
		if index == len(value) {
			break
		}
		start := index
		if value[index] == '"' {
			index++
			escaped := false
			for index < len(value) {
				character := value[index]
				index++
				if escaped {
					escaped = false
					continue
				}
				if character == '\\' {
					escaped = true
					continue
				}
				if character == '"' {
					break
				}
			}
			decoded, err := strconv.Unquote(value[start:index])
			if err != nil {
				return nil
			}
			tokens = append(tokens, decoded)
			continue
		}
		for index < len(value) && value[index] != ' ' {
			index++
		}
		tokens = append(tokens, value[start:index])
	}
	return tokens
}

func patchHeaderPath(line, marker, sidePrefix string) string {
	value := strings.TrimSuffix(strings.TrimPrefix(line, marker), "\n")
	if strings.HasPrefix(value, "\"") {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return ""
		}
		value = decoded
	}
	if value == "/dev/null" {
		return ""
	}
	return filepath.ToSlash(strings.TrimPrefix(value, sidePrefix))
}

func rangeStartSide(side string, start int) string {
	if start == 0 {
		return ""
	}
	return side
}

func noteSide(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LEFT":
		return provider.NoteSideLeft, nil
	case "RIGHT":
		return provider.NoteSideRight, nil
	default:
		return "", fmt.Errorf("--side must be left or right, got %q", value)
	}
}

func readNoteMessage(path string) (string, error) {
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxNoteMessageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxNoteMessageBytes {
		return "", fmt.Errorf("note message exceeds %d bytes", maxNoteMessageBytes)
	}
	return string(data), nil
}

func editNote(configured []string) (string, error) {
	temporary, err := os.CreateTemp("", "changes-note-*.md")
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	defer os.Remove(path)
	template := "\n# Write the summary on the first line. Remaining text is the rationale.\n# Empty or comment-only content cancels the note.\n"
	if _, err := temporary.WriteString(template); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	command := append([]string(nil), configured...)
	if len(command) == 0 {
		value := os.Getenv("VISUAL")
		if value == "" {
			value = os.Getenv("EDITOR")
		}
		if value == "" {
			value = "vi"
		}
		command, err = shellwords.Parse(value)
		if err != nil {
			return "", fmt.Errorf("parse note editor: %w", err)
		}
	}
	if len(command) == 0 || command[0] == "" {
		return "", errors.New("note editor command is empty")
	}
	found := false
	for index := range command {
		if strings.Contains(command[index], "$FILE") {
			command[index] = strings.ReplaceAll(command[index], "$FILE", path)
			found = true
		}
	}
	if !found {
		command = append(command, path)
	}
	process := exec.Command(command[0], command[1:]...)
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := process.Run(); err != nil {
		return "", fmt.Errorf("note editor %s: %w", command[0], err)
	}
	return readNoteMessage(path)
}

func splitNoteMessage(message string, comments bool) (string, string) {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	lines := strings.Split(message, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if comments && strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, cleanNoteText(line))
	}
	for index, line := range kept {
		if summary := strings.TrimSpace(line); summary != "" {
			return summary, strings.TrimSpace(strings.Join(kept[index+1:], "\n"))
		}
	}
	return "", ""
}

func defaultNoteAuthor() string {
	if value, err := gitutil.Output(".", "config", "user.email"); err == nil && value != "" {
		return value
	}
	if value, err := gitutil.Output(".", "config", "user.name"); err == nil && value != "" {
		return value
	}
	if value := os.Getenv("USER"); value != "" {
		return value
	}
	return "user"
}

func sortNotes(notes []provider.Note) {
	sort.SliceStable(notes, func(left, right int) bool {
		lpath, lline := notePosition(notes[left])
		rpath, rline := notePosition(notes[right])
		if lpath != rpath {
			return lpath < rpath
		}
		if lline != rline {
			return lline < rline
		}
		if notes[left].CreatedAt != notes[right].CreatedAt {
			return notes[left].CreatedAt < notes[right].CreatedAt
		}
		return notes[left].ID < notes[right].ID
	})
}

func notePosition(note provider.Note) (string, int) {
	if note.Placement.Quality == provider.PlacementOrphan {
		return note.Anchor.Path, note.Anchor.Line
	}
	return note.Placement.Path, note.Placement.Line
}

func renderNoteRows(notes []provider.Note, width int) string {
	if len(notes) == 0 {
		return ""
	}
	rows := make([]string, 0, len(notes)*2+1)
	rows = append(rows, contextTitle.Render("notes"))
	for _, note := range notes {
		location := noteLocation(note)
		state := note.State
		if note.Placement.Quality != provider.PlacementExact {
			state += "/" + note.Placement.Quality
		}
		header := contextPath.Render(location) + " · " + contextKind.Render(state+" ") +
			cleanNoteOneLine(note.Author) + " via " + cleanNoteOneLine(note.Source)
		rows = append(rows, header)
		body := cleanNoteOneLine(note.Summary)
		if rationale := cleanNoteOneLine(note.Rationale); rationale != "" {
			body += " - " + rationale
		}
		if width > 4 && len([]rune(body)) > width-4 {
			body = string([]rune(body)[:width-5]) + "…"
		}
		rows = append(rows, "  "+body)
	}
	return strings.Join(rows, "\n")
}

func noteLocation(note provider.Note) string {
	location := note.Placement
	if location.Quality == provider.PlacementOrphan {
		location = provider.NotePlacement{
			Path: note.Anchor.Path, Side: note.Anchor.Side, StartSide: note.Anchor.StartSide,
			StartLine: note.Anchor.StartLine, Line: note.Anchor.Line,
		}
	}
	value := cleanNoteOneLine(location.Path)
	if location.Line == 0 {
		return value
	}
	end := strconv.Itoa(location.Line) + "@" + strings.ToLower(location.Side)
	if location.StartLine == 0 {
		return value + ":" + end
	}
	start := strconv.Itoa(location.StartLine) + "@" + strings.ToLower(location.StartSide)
	return value + ":" + start + "-" + end
}

func cleanNoteOneLine(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func cleanNoteText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}
