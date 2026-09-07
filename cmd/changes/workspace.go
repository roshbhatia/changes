package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/progress"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/changes/internal/workspaceview"
	"github.com/roshbhatia/go-utils/completion"
	"github.com/roshbhatia/go-utils/diffview"
)

type workspaceOptions struct {
	configPath   string
	color        string
	view         string
	commit       string
	layout       string
	width        int
	historyLimit int
	refresh      bool
	watch        bool
	interval     time.Duration
	quiet        bool
	noCalls      bool
	noGroups     bool
	noNotes      bool
	noSymbols    bool
}

func workspaceCommandFlags(interactive bool) []completion.Flag {
	flags := []completion.Flag{
		{Name: "commit", Description: "Commit to compare with its first parent", Value: true, CompletionCommand: completionValuesInvocation("repository")},
		{Name: "config", Description: "YAML configuration file", Value: true},
		{Name: "history-limit", Description: "Commit history limit", Value: true},
		{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
		{Name: "no-calls", Description: "Skip call analysis"},
		{Name: "no-groups", Description: "Skip logical change grouping"},
		{Name: "no-notes", Description: "Skip diff notes"},
		{Name: "no-symbols", Description: "Skip symbol analysis"},
		{Name: "quiet", Description: "Disable progress output"},
		{Name: "refresh", Description: "Bypass cached workspace and provider results"},
		{Name: "view", Description: "Git comparison view", Value: true, Values: []string{"working", "staged", "commit"}},
		{Name: "width", Description: "Render width", Value: true},
	}
	if !interactive {
		flags = append(flags,
			completion.Flag{Name: "interval", Description: "Watch interval", Value: true},
			completion.Flag{Name: "watch", Description: "Emit JSON Lines refresh events"},
		)
	}
	return flags
}

func parseWorkspaceOptions(args []string, interactive bool) (workspaceOptions, appconfig.Config, error) {
	metadataName := "workspace"
	if interactive {
		metadataName = "interactive"
	}
	metadata := subcommandMetadata(metadataName)
	flags := flag.NewFlagSet("changes "+metadataName, flag.ContinueOnError)
	configPath := flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		return workspaceOptions{}, appconfig.Config{}, err
	}
	options := workspaceOptions{configPath: *configPath, color: "never"}
	if interactive {
		options.color = "always"
	}
	flags.StringVar(&options.view, "view", "working", flagDescription(metadata, "view"))
	flags.StringVar(&options.commit, "commit", "HEAD", flagDescription(metadata, "commit"))
	flags.StringVar(&options.layout, "layout", configured.Diff.Layout, flagDescription(metadata, "layout"))
	flags.IntVar(&options.width, "width", 0, flagDescription(metadata, "width"))
	flags.IntVar(&options.historyLimit, "history-limit", configured.Interactive.HistoryLimit, flagDescription(metadata, "history-limit"))
	flags.BoolVar(&options.refresh, "refresh", false, flagDescription(metadata, "refresh"))
	flags.BoolVar(&options.quiet, "quiet", false, flagDescription(metadata, "quiet"))
	flags.BoolVar(&options.noCalls, "no-calls", false, flagDescription(metadata, "no-calls"))
	flags.BoolVar(&options.noGroups, "no-groups", false, flagDescription(metadata, "no-groups"))
	flags.BoolVar(&options.noNotes, "no-notes", false, flagDescription(metadata, "no-notes"))
	flags.BoolVar(&options.noSymbols, "no-symbols", false, flagDescription(metadata, "no-symbols"))
	if !interactive {
		flags.BoolVar(&options.watch, "watch", false, flagDescription(metadata, "watch"))
		flags.DurationVar(&options.interval, "interval", configured.Notes.RefreshInterval.Duration(), flagDescription(metadata, "interval"))
	}
	flags.Usage = func() { printCommandHelp(flags.Output(), "changes "+metadataName+" [flags]", metadata, flags) }
	if err := flags.Parse(args); err != nil {
		return workspaceOptions{}, appconfig.Config{}, err
	}
	if flags.NArg() != 0 {
		return workspaceOptions{}, appconfig.Config{}, fmt.Errorf("%s accepts only flags", metadataName)
	}
	if options.view != "working" && options.view != "staged" && options.view != "commit" {
		return workspaceOptions{}, appconfig.Config{}, errors.New("--view must be working, staged, or commit")
	}
	if options.layout != "unified" && options.layout != "side-by-side" {
		return workspaceOptions{}, appconfig.Config{}, errors.New("--layout must be unified or side-by-side")
	}
	if options.historyLimit <= 0 {
		return workspaceOptions{}, appconfig.Config{}, errors.New("--history-limit must be greater than zero")
	}
	if options.width < 0 {
		return workspaceOptions{}, appconfig.Config{}, errors.New("--width must not be negative")
	}
	if configured.Interactive.Dock != "left" && configured.Interactive.Dock != "bottom" {
		return workspaceOptions{}, appconfig.Config{}, errors.New("interactive.dock must be left or bottom")
	}
	if configured.Interactive.Navigator != "tree" && configured.Interactive.Navigator != "list" {
		return workspaceOptions{}, appconfig.Config{}, errors.New("interactive.navigator must be tree or list")
	}
	if configured.Interactive.NoteInput != "popup" && configured.Interactive.NoteInput != "editor" {
		return workspaceOptions{}, appconfig.Config{}, errors.New("interactive.noteInput must be popup or editor")
	}
	if configured.Interactive.CacheMaxEntries < 0 || configured.Interactive.CacheTTL.Duration() < 0 {
		return workspaceOptions{}, appconfig.Config{}, errors.New("interactive cache bounds must not be negative")
	}
	if options.watch && options.interval <= 0 {
		return workspaceOptions{}, appconfig.Config{}, errors.New("--interval must be greater than zero in watch mode")
	}
	return options, configured, nil
}

func runWorkspace(args []string) {
	options, configured, err := parseWorkspaceOptions(args, false)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	root, err := currentRepositoryRoot()
	if err != nil {
		fail(err)
	}
	store, err := workspaceview.DefaultStore(configured.Interactive.CacheMaxEntries)
	if err != nil {
		fail(err)
	}
	if !options.watch {
		indicator := progress.Start(os.Stderr, "building workspace", configured.Interactive.Progress && !options.quiet)
		snapshot, buildErr := buildWorkspaceSnapshot(root, options, configured)
		indicator.Stop()
		if buildErr != nil {
			fail(buildErr)
		}
		if err := store.SaveSnapshot(workspaceSlot(options), snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "changes: cache workspace: %v\n", err)
		}
		writeJSON(os.Stdout, snapshot)
		return
	}
	lastNotes := []provider.Note{}
	if !options.refresh {
		if cached, found, cacheErr := store.LoadSnapshot(root, workspaceSlot(options)); cacheErr != nil {
			fmt.Fprintf(os.Stderr, "changes: load workspace cache: %v\n", cacheErr)
		} else if found && snapshotWithinTTL(cached, configured.Interactive.CacheTTL.Duration()) {
			cached.Freshness.State = "refreshing"
			writeWorkspaceEvent(workspaceview.Event{Type: "snapshot", Snapshot: &cached})
			lastNotes = append([]provider.Note(nil), cached.Notes...)
		}
	}
	for {
		writeWorkspaceEvent(workspaceview.Event{Type: "refreshing"})
		snapshot, buildErr := buildWorkspaceSnapshotWithNotes(root, options, configured, lastNotes)
		if buildErr != nil {
			writeWorkspaceEvent(workspaceview.Event{Type: "error", Error: buildErr.Error()})
		} else {
			lastNotes = append([]provider.Note(nil), snapshot.Notes...)
			_ = store.SaveSnapshot(workspaceSlot(options), snapshot)
			writeWorkspaceEvent(workspaceview.Event{Type: "snapshot", Snapshot: &snapshot})
		}
		time.Sleep(options.interval)
	}
}

func currentRepositoryRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return source.Root(cwd)
}

func workspaceSlot(options workspaceOptions) string {
	return strings.Join([]string{options.view, options.commit, options.layout, options.color}, "\x00")
}

func snapshotWithinTTL(snapshot workspaceview.Snapshot, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	generated, err := time.Parse(time.RFC3339Nano, snapshot.Freshness.GeneratedAt)
	return err == nil && time.Since(generated) <= ttl
}

func buildWorkspaceSnapshot(root string, options workspaceOptions, configured appconfig.Config) (workspaceview.Snapshot, error) {
	return buildWorkspaceSnapshotWithNotes(root, options, configured, nil)
}

func buildWorkspaceSnapshotWithNotes(root string, options workspaceOptions, configured appconfig.Config, previous []provider.Note) (workspaceview.Snapshot, error) {
	spec := source.Spec{Dir: root}
	comparison := workspaceview.Comparison{Kind: options.view, Layout: options.layout}
	if options.view == "staged" {
		spec.Staged = true
	}
	if options.view == "commit" {
		resolved, commit, err := source.CommitComparison(root, options.commit)
		if err != nil {
			return workspaceview.Snapshot{}, err
		}
		spec = resolved
		comparison.From, comparison.To = commit.Parent, commit.OID
		comparison.Parent, comparison.FirstParent = commit.Parent, true
	}
	base, head := spec.ComparisonIDs()
	comparison.Base, comparison.Head = base, head
	patch, err := spec.Diff()
	if err != nil {
		return workspaceview.Snapshot{}, err
	}
	fingerprint := sha256.Sum256([]byte(patch))
	comparison.Fingerprint = hex.EncodeToString(fingerprint[:])
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		return workspaceview.Snapshot{}, err
	}
	for _, diagnostic := range discovery.Diagnostics {
		fmt.Fprintf(os.Stderr, "changes: skipped provider %s: %s\n", diagnostic.Manifest.Name, diagnostic.Problem)
	}
	width := options.width
	if width <= 0 {
		width = 100
	}
	renderColor := options.color
	if renderColor == "" {
		renderColor = "never"
	}
	view := renderer{
		specs: []source.Spec{spec}, under: root, width: width,
		syms: !options.noSymbols, calls: !options.noCalls, groups: !options.noGroups,
		groupProvider: configured.Providers.Group, notes: !options.noNotes,
		noteInterval: configured.Notes.RefreshInterval.Duration(), budget: time.Duration(configured.Providers.Timeout),
		engine: "builtin", engineOptions: engine.Options{Color: renderColor, Layout: options.layout, Width: width},
		providers: discovery.Providers, providerCache: provider.CachePolicy{TTL: time.Duration(configured.Providers.CacheTTL), MaxEntries: configured.Providers.CacheMaxEntries},
	}
	if options.refresh {
		view.providerCache = provider.CachePolicy{}
	}
	rendered, noteValues, analysis, err := view.renderPatchesWithAnalysis([]string{patch}, noteLayer{values: append([]provider.Note(nil), previous...)}, view.notes)
	if err != nil {
		return workspaceview.Snapshot{}, err
	}
	branch, repositoryHead, err := source.RepositoryIdentity(root)
	if err != nil {
		return workspaceview.Snapshot{}, err
	}
	history, err := source.History(root, options.historyLimit)
	if err != nil {
		return workspaceview.Snapshot{}, err
	}
	historyEntries := workspaceHistory(history)
	if comparison.Kind == "commit" {
		for index := range historyEntries {
			if historyEntries[index].OID != comparison.To {
				continue
			}
			authors := map[string]bool{}
			for _, note := range noteValues.values {
				authors[note.Author] = true
			}
			historyEntries[index].NoteCount = len(noteValues.values)
			historyEntries[index].NoteAuthors = sortedKeys(authors)
		}
	}
	snapshot := workspaceview.Snapshot{
		Version:    workspaceview.SnapshotVersion,
		Repository: workspaceview.Repository{Root: root, Name: filepath.Base(root), Branch: branch, Head: repositoryHead},
		Comparison: comparison,
		Freshness:  workspaceview.Freshness{State: "fresh", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), RefreshedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		History:    historyEntries, Groups: workspaceGroups(analysis.groups),
		Files: workspaceFiles(analysis.options, noteValues.values), Notes: append([]provider.Note(nil), noteValues.values...),
		Threads: workspaceThreads(noteValues.values), Failures: []workspaceview.Failure{}, Rendered: rendered,
	}
	if noteValues.stale {
		snapshot.Freshness.State = "stale"
		snapshot.Failures = append(snapshot.Failures, workspaceview.Failure{Message: "one or more note providers failed; retained notes may be stale", Stale: true})
	}
	return snapshot, nil
}

func workspaceHistory(commits []source.Commit) []workspaceview.HistoryEntry {
	entries := make([]workspaceview.HistoryEntry, 0, len(commits))
	for _, commit := range commits {
		entries = append(entries, workspaceview.HistoryEntry{OID: commit.OID, Parent: commit.Parent, Summary: commit.Summary, Author: commit.Author, AuthoredAt: commit.AuthoredAt, NoteAuthors: []string{}})
	}
	return entries
}

func workspaceGroups(groups []provider.ChangeGroup) []workspaceview.Group {
	result := make([]workspaceview.Group, 0, len(groups))
	for _, group := range groups {
		files := []string{}
		seen := map[string]bool{}
		for _, anchor := range group.Anchors {
			if !seen[anchor.Path] {
				files = append(files, anchor.Path)
				seen[anchor.Path] = true
			}
		}
		result = append(result, workspaceview.Group{ID: group.ID, Title: group.Title, Summary: group.Summary, ParentID: group.ParentID, Order: group.Order, Files: files})
	}
	return result
}

func workspaceFiles(options diffview.Options, notes []provider.Note) []workspaceview.File {
	files := make([]workspaceview.File, 0, len(options.Files))
	for _, file := range options.Files {
		authors := map[string]bool{}
		noteCount := 0
		for _, note := range notes {
			if sameDiffPath(noteDisplayPlacement(note).Path, file.Path) {
				noteCount++
				authors[note.Author] = true
			}
		}
		entry := workspaceview.File{Path: file.Path, Added: file.Add, Deleted: file.Del, NoteCount: noteCount, NoteAuthors: sortedKeys(authors), Hunks: []workspaceview.Hunk{}}
		for _, hunk := range file.Hunks {
			oldLine, newLine := hunk.OldAt, hunk.NewAt
			converted := workspaceview.Hunk{OldStart: hunk.OldAt, NewStart: hunk.NewAt, Symbol: hunk.Sym, Lines: []workspaceview.Line{}}
			for _, line := range hunk.Lines {
				kind := "context"
				structured := workspaceview.Line{Text: line.Text}
				switch line.Kind {
				case '+':
					kind, structured.NewLine = "added", newLine
					newLine++
				case '-':
					kind, structured.OldLine = "deleted", oldLine
					oldLine++
				default:
					structured.OldLine, structured.NewLine = oldLine, newLine
					oldLine++
					newLine++
				}
				structured.Kind = kind
				for _, note := range notes {
					placement := noteDisplayPlacement(note)
					if sameDiffPath(placement.Path, file.Path) && noteLineMatches(placement, line.Kind, structured.OldLine, structured.NewLine) {
						structured.NoteIDs = append(structured.NoteIDs, note.ID)
					}
				}
				converted.Lines = append(converted.Lines, structured)
			}
			entry.Hunks = append(entry.Hunks, converted)
		}
		files = append(files, entry)
	}
	return files
}

func workspaceThreads(notes []provider.Note) []workspaceview.NoteThread {
	threads := []workspaceview.NoteThread{}
	indices := map[string]int{}
	for _, note := range notes {
		if note.ThreadID == "" {
			continue
		}
		index, exists := indices[note.ThreadID]
		if !exists {
			placement := noteDisplayPlacement(note)
			index = len(threads)
			indices[note.ThreadID] = index
			threads = append(threads, workspaceview.NoteThread{ID: note.ThreadID, Path: placement.Path, Side: placement.Side, StartLine: placement.StartLine, Line: placement.Line, State: note.State, Comments: []provider.Note{}})
		}
		threads[index].Comments = append(threads[index].Comments, note)
		if note.State == provider.NoteStateOpen {
			threads[index].State = provider.NoteStateOpen
		}
	}
	return threads
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeJSON(file *os.File, value any) {
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fail(err)
	}
}

func writeWorkspaceEvent(event workspaceview.Event) {
	event.Version = workspaceview.EventVersion
	event.At = time.Now().UTC().Format(time.RFC3339Nano)
	writeJSON(os.Stdout, event)
}
