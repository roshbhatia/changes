// Command changes prints a review-oriented diff. It groups touched files in a tree,
// nests hunks under their enclosing symbols, and annotates changed call edges.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/progress"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/changes/internal/workspaceview"
	"github.com/roshbhatia/go-utils/completion"
	"github.com/roshbhatia/go-utils/diffview"
	gitutil "github.com/roshbhatia/go-utils/git"
	"github.com/roshbhatia/go-utils/workspace"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__values" {
		runCompletionValues(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "completion" {
		runCompletion(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "generate" {
		runGenerate(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "provider" {
		runProvider(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "note" {
		runNote(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "difftool" {
		runDifftool(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "render" {
		runRender(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "workspace" {
		runWorkspace(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "interactive" {
		runInteractive(os.Args[2:])
		return
	}
	configPath := argumentValue(os.Args[1:], "config")
	configured, err := appconfig.Load(configPath)
	if err != nil {
		fail(err)
	}
	metadata := commandMetadata()
	showVersion := flag.Bool("version", false, flagDescription(metadata, "version"))
	staged := flag.Bool("staged", false, flagDescription(metadata, "staged"))
	flag.String("config", configPath, flagDescription(metadata, "config"))
	since := flag.String("since", "", flagDescription(metadata, "since"))
	watch := flag.Bool("watch", false, flagDescription(metadata, "watch"))
	flag.BoolVar(watch, "w", false, shortFlagDescription(metadata, "w"))
	every := flag.Duration("interval", 700*time.Millisecond, flagDescription(metadata, "interval"))
	width := flag.Int("width", 0, flagDescription(metadata, "width"))
	noCalls := flag.Bool("no-calls", false, flagDescription(metadata, "no-calls"))
	noGroups := flag.Bool("no-groups", false, flagDescription(metadata, "no-groups"))
	noNotes := flag.Bool("no-notes", false, flagDescription(metadata, "no-notes"))
	noSyms := flag.Bool("no-symbols", false, flagDescription(metadata, "no-symbols"))
	quiet := flag.Bool("quiet", false, flagDescription(metadata, "quiet"))
	budget := flag.Duration("budget", time.Duration(configured.Providers.Timeout), flagDescription(metadata, "budget"))
	recurse := flag.Bool("recursive", false, flagDescription(metadata, "recursive"))
	flag.BoolVar(recurse, "r", false, shortFlagDescription(metadata, "r"))
	scan := flag.String("root", "", flagDescription(metadata, "root"))
	stat := flag.Bool("stat", false, flagDescription(metadata, "stat"))
	flag.BoolVar(stat, "s", false, shortFlagDescription(metadata, "s"))
	color := flag.String("color", configured.Color, flagDescription(metadata, "color"))
	diffEngine := flag.String("engine", configured.Diff.Engine, flagDescription(metadata, "engine"))
	filter := flag.String("filter", "", flagDescription(metadata, "filter"))
	groupProvider := flag.String("group-provider", configured.Providers.Group, flagDescription(metadata, "group-provider"))
	layout := flag.String("layout", configured.Diff.Layout, flagDescription(metadata, "layout"))
	flag.Usage = func() {
		printCommandHelp(
			flag.CommandLine.Output(),
			"changes [flags] [<from> [<to>]] [-- <path>...]",
			metadata,
			flag.CommandLine,
		)
	}
	rawArguments := append([]string(nil), os.Args[1:]...)
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *watch && *every <= 0 {
		fail(errors.New("--interval must be greater than zero in watch mode"))
	}
	if *budget <= 0 {
		fail(errors.New("--budget must be greater than zero"))
	}
	if *watch && !*noNotes && configured.Notes.RefreshInterval.Duration() <= 0 {
		fail(errors.New("notes.refreshInterval must be greater than zero in watch mode"))
	}
	resolvedColor := configureColor(*color)
	engineOptions := engine.Options{
		Color:  resolvedColor,
		Filter: commandValue(configured.Diff.Filter, *filter),
		Layout: *layout,
		Width:  columns(*width),
	}
	if err := engine.ValidatePatch(*diffEngine, engineOptions); err != nil {
		fail(err)
	}
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	for _, diagnostic := range discovery.Diagnostics {
		fmt.Fprintf(os.Stderr, "changes: skipped provider %s: %s\n", diagnostic.Manifest.Name, diagnostic.Problem)
	}
	providers := discovery.Providers
	if err := validateGroupSelection(*diffEngine, !*noGroups, *groupProvider, providers); err != nil {
		fail(err)
	}

	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if *scan != "" {
		dir, err = resolveFrom(dir, *scan)
		if err != nil {
			fail(fmt.Errorf("resolve -root %q: %w", *scan, err))
		}
	}

	var roots []string
	if *recurse {
		roots, err = workspace.Roots(dir)
		if err != nil {
			fail(err)
		}
		if len(roots) == 0 {
			fail(fmt.Errorf("no git repository under %s", workspace.Root(dir)))
		}
	} else {
		one, err := source.Root(dir)
		if err != nil {
			fail(err)
		}
		roots = []string{one}
	}
	// Every positional is read against the first repository, because a ref has
	// to mean one thing across the whole render.
	root := roots[0]
	from, to, paths := split(root, dir, restorePathSeparator(rawArguments, flag.Args()))
	if *since != "" {
		if from != "" {
			fail(fmt.Errorf("-since and a revision argument both name the left side"))
		}
		from = source.Revision(root, *since)
		if from == "" {
			fail(fmt.Errorf("-since %q resolves to no commit before HEAD: it is not a revision, and git read it as now", *since))
		}
	}
	if *staged && to != "" {
		fail(fmt.Errorf("--staged accepts at most one revision; compare two revisions without --staged"))
	}
	// git resolves a pathspec against the process cwd, and every command here
	// runs at the repository root, so a relative path from a subdirectory would
	// silently match nothing.
	for i, p := range paths {
		if abs, err := resolveFrom(dir, p); err == nil {
			paths[i] = abs
		}
	}

	specs := make([]source.Spec, 0, len(roots))
	for _, one := range roots {
		specs = append(specs, source.Spec{
			Dir:    one,
			From:   from,
			To:     to,
			Staged: *staged,
			Paths:  paths,
		})
	}
	view := renderer{
		specs:         specs,
		under:         workspace.Root(dir),
		named:         *recurse,
		stat:          *stat,
		width:         *width,
		syms:          !*noSyms && !*stat,
		calls:         !*noCalls && !*stat,
		groups:        !*noGroups && *diffEngine == "builtin",
		groupProvider: *groupProvider,
		notes:         !*noNotes,
		noteInterval:  configured.Notes.RefreshInterval.Duration(),
		budget:        *budget,
		engine:        *diffEngine,
		engineOptions: engineOptions,
		providers:     providers,
		providerCache: provider.CachePolicy{
			TTL: time.Duration(configured.Providers.CacheTTL), MaxEntries: configured.Providers.CacheMaxEntries,
		},
	}

	if !*watch {
		indicator := progress.Start(os.Stderr, "reading changes", configured.Interactive.Progress && !*quiet)
		out, err := view.render()
		indicator.Stop()
		if err != nil {
			fail(err)
		}
		if out == "" {
			fmt.Fprintln(os.Stderr, "changes: nothing changed")
			return
		}
		fmt.Println(out)
		return
	}
	fail(view.follow(*every))
}

// split reads Git-shaped positional arguments. An explicit separator always
// wins. Without one, the first argument Git does not know as an object starts
// the path list.
func split(root, pathRoot string, args []string) (from, to string, paths []string) {
	refs := args
	for i, a := range args {
		if a == "--" {
			refs, paths = args[:i], args[i+1:]
			break
		}
	}
	if len(refs) > 0 {
		if a, b, ok := strings.Cut(refs[0], ".."); ok {
			return a, b, append(refs[1:], paths...)
		}
	}
	for i, a := range refs {
		if !source.IsRev(root, a) {
			// An argument that is neither a tree nor a path is a typo, and
			// read as a pathspec it matches nothing and prints "nothing
			// changed", which reads as a clean tree.
			if _, err := os.Stat(filepath.Join(pathRoot, a)); err != nil {
				fail(fmt.Errorf("%s is not a revision or a path", a))
			}
			return from, to, append(refs[i:], paths...)
		}
		switch i {
		case 0:
			from = a
		case 1:
			to = a
		default:
			// git takes two trees and no more, so a third would silently
			// change which comparison ran.
			fail(fmt.Errorf("too many revisions: %s", strings.Join(refs, " ")))
		}
	}
	return from, to, paths
}

func resolveFrom(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Abs(path)
}

// restorePathSeparator keeps an explicit Git path boundary after flag.Parse.
// The standard flag package removes a leading --, but leaves one that follows
// a positional argument. Raw arguments tell these two cases apart, including a
// path whose literal name is --.
func restorePathSeparator(raw, parsed []string) []string {
	separator := -1
	for index, argument := range raw {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return parsed
	}
	paths := raw[separator+1:]
	if len(paths) > len(parsed) {
		return parsed
	}
	pathStart := len(parsed) - len(paths)
	if pathStart > 0 && parsed[pathStart-1] == "--" {
		return parsed
	}
	withSeparator := make([]string, 0, len(parsed)+1)
	withSeparator = append(withSeparator, parsed[:pathStart]...)
	withSeparator = append(withSeparator, "--")
	return append(withSeparator, parsed[pathStart:]...)
}

type renderer struct {
	specs         []source.Spec
	under         string
	named         bool
	stat          bool
	width         int
	syms          bool
	calls         bool
	groups        bool
	groupProvider string
	notes         bool
	noteInterval  time.Duration
	budget        time.Duration
	engine        string
	engineOptions engine.Options
	providers     []provider.LoadedManifest
	providerCache provider.CachePolicy
}

type noteLayer struct {
	values []provider.Note
	stale  bool
}

type noteRead struct {
	values        []provider.Note
	complete      bool
	failedSources map[string]bool
}

type diffAnalysis struct {
	options diffview.Options
	groups  []provider.ChangeGroup
	aliases map[string]string
}

func (r renderer) render() (string, error) {
	patches, err := r.patches()
	if err != nil {
		return "", err
	}
	return r.renderPatches(patches)
}

func (r renderer) renderPatches(patches []string) (string, error) {
	output, _, err := r.renderPatchesWithNotes(patches, noteLayer{}, r.notes)
	return output, err
}

func (r renderer) renderPatchesWithNotes(patches []string, notes noteLayer, refreshNotes bool) (string, noteLayer, error) {
	output, notes, _, err := r.renderPatchesWithAnalysis(patches, notes, refreshNotes)
	return output, notes, err
}

func (r renderer) renderPatchesWithAnalysis(patches []string, notes noteLayer, refreshNotes bool) (string, noteLayer, diffAnalysis, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.budget)
	defer cancel()
	if refreshNotes {
		notes = refreshNoteLayer(notes, r.noteContext(ctx, patches))
	}
	analysis := r.diffAnalysisContext(ctx, patches, false)
	notes = remapNotePaths(notes, analysis.aliases)
	options := analysis.options
	if r.stat || r.engine == "builtin" {
		body, err := renderTreeWithGroups(options, analysis.groups, notes, r.columns())
		return body, notes, analysis, err
	}
	body, err := r.display(patches)
	if err != nil {
		return "", notes, analysis, err
	}
	summary := semanticTreeRows(options, notes, r.columns())
	if summary == "" {
		return body, notes, analysis, nil
	}
	if body == "" {
		return summary, notes, analysis, nil
	}
	return summary + "\n\n" + body, notes, analysis, nil
}

func (r renderer) noteContext(ctx context.Context, patches []string) noteRead {
	if !r.notes {
		return noteRead{complete: true}
	}
	readers, err := selectNoteProviders(r.providers, provider.ActionNotes, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "changes: select note providers: %v\n", err)
		return noteRead{}
	}
	if len(readers) == 0 {
		return noteRead{complete: true}
	}
	all := []provider.Note{}
	failedSources := map[string]bool{}
	complete := true
	for index, spec := range r.specs {
		snapshot, err := stableNoteSnapshot(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "changes: read note comparison for %s: %v\n", spec.Dir, err)
			complete = false
			failedSources["*"] = true
			continue
		}
		if len(snapshot.files) == 0 {
			continue
		}
		currentDisplayed, err := spec.Diff()
		if err != nil {
			fmt.Fprintf(os.Stderr, "changes: verify note comparison for %s: %v\n", spec.Dir, err)
			complete = false
			failedSources["*"] = true
			continue
		}
		if snapshot.placementPatch != patches[index] || currentDisplayed != patches[index] {
			fmt.Fprintf(os.Stderr, "changes: note comparison changed before rendering %s\n", spec.Dir)
			complete = false
			failedSources["*"] = true
			continue
		}
		displayed := notePathsInPatch(patches[index])
		result := readNotes(
			ctx, spec, displayed, snapshot.patch, snapshot.placementPatch, snapshot.base, snapshot.head,
			readers,
		)
		for _, failure := range result.failures {
			fmt.Fprintf(os.Stderr, "changes: %v\n", failure)
		}
		complete = complete && len(result.failures) == 0
		for source := range result.failedSources {
			failedSources[source] = true
		}
		under := r.prefix(spec.Dir)
		for _, note := range result.notes {
			note.Anchor.Path = under + note.Anchor.Path
			if note.Placement.Path != "" {
				note.Placement.Path = under + note.Placement.Path
			}
			all = append(all, note)
		}
	}
	sortNotes(all)
	return noteRead{values: all, complete: complete, failedSources: failedSources}
}

func refreshNoteLayer(previous noteLayer, current noteRead) noteLayer {
	if current.complete || len(previous.values) == 0 {
		return noteLayer{values: current.values, stale: !current.complete}
	}
	values := append([]provider.Note(nil), current.values...)
	seen := make(map[string]bool, len(values))
	for _, note := range values {
		seen[note.ID] = true
	}
	for _, note := range previous.values {
		if !seen[note.ID] && (current.failedSources["*"] || current.failedSources[note.Source]) {
			values = append(values, note)
			seen[note.ID] = true
		}
	}
	sortNotes(values)
	return noteLayer{values: values, stale: true}
}

func (r renderer) display(patches []string) (string, error) {
	return engine.Patch(r.engine, strings.Join(patches, "\n"), r.engineOptions)
}

// patches reads one repository per spec, concurrently, because git is the cheap
// layer and a workspace holds a handful of repositories.
//
// A revision one repository does not carry is normal across a workspace, so
// that repository is named on stderr and left out rather than failing the whole
// render. A single repository still fails, because there is nothing left to
// draw.
func (r renderer) patches() ([]string, error) {
	out, errs := make([]string, len(r.specs)), make([]error, len(r.specs))
	var wg sync.WaitGroup
	for i, spec := range r.specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i], errs[i] = spec.Diff()
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil {
			continue
		}
		if len(r.specs) == 1 {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "changes: skipped %s: %v\n", r.specs[i].Dir, err)
	}
	return out, nil
}

func (r renderer) diffOptions(patches []string, summary bool) diffview.Options {
	return r.diffAnalysisContext(context.Background(), patches, summary).options
}

func (r renderer) diffAnalysisContext(ctx context.Context, patches []string, summary bool) diffAnalysis {
	opts := diffview.Options{
		Width:   r.columns(),
		Symbols: map[string][]diffview.Symbol{},
		Edges:   map[string][]diffview.Edge{},
		Pins:    map[string]bool{},
		Stat:    r.stat,
		Summary: summary,
		Unified: r.engineOptions.Layout == "unified",
	}
	groups := []provider.ChangeGroup{}
	aliases := map[string]string{}
	for i, patch := range patches {
		files, rawPaths := parseDiffFiles(patch)
		if len(files) == 0 {
			continue
		}
		spec := r.specs[i]
		under := r.prefix(spec.Dir)
		for oldPath, newPath := range diffPathAliases(patch) {
			aliases[displayDiffPath(under+oldPath)] = displayDiffPath(under + newPath)
		}
		if under != "" {
			opts.Pins[strings.TrimSuffix(under, "/")] = true
		}
		touched, err := spec.Files()
		if err != nil {
			touched = make([]string, 0, len(files))
			for _, file := range files {
				touched = append(touched, rawPaths[file.Path])
			}
		}
		syms, edges, repoGroups := r.layers(ctx, spec, touched, patches[i])
		groupPrefix := fmt.Sprintf("repo-%d:", i)
		for _, group := range repoGroups {
			group.ID = groupPrefix + group.ID
			if group.ParentID != "" {
				group.ParentID = groupPrefix + group.ParentID
			}
			for anchorIndex := range group.Anchors {
				group.Anchors[anchorIndex].Path = under + group.Anchors[anchorIndex].Path
			}
			groups = append(groups, group)
		}
		for j := range files {
			rawPath := rawPaths[files[j].Path]
			at := displayDiffPath(under + rawPath)
			opts.Symbols[at] = syms[rawPath]
			opts.Edges[at] = edges[rawPath]
			files[j].Path = at
			opts.Files = append(opts.Files, files[j])
		}
	}
	return diffAnalysis{options: opts, groups: groups, aliases: aliases}
}

var (
	contextTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	contextPath  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	contextKind  = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8"))
	contextAdd   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	contextDel   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

type semanticNode struct {
	label    string
	children []semanticNode
}

func semanticTreeRows(opts diffview.Options, notes noteLayer, width int) string {
	files := make([]semanticNode, 0, len(opts.Files))
	for index := range opts.Files {
		file := &opts.Files[index]
		fileNotes := notesForPath(notes.values, file.Path)
		claimed := make([]bool, len(fileNotes))
		children := []semanticNode{}
		for _, symbol := range opts.Symbols[file.Path] {
			add, del, up, down, changed := semanticCounts(file, symbol, opts.Edges[file.Path])
			symbolNotes := []semanticNode{}
			for noteIndex, note := range fileNotes {
				placement := noteDisplayPlacement(note)
				if placement.Side == provider.NoteSideRight && placement.Line >= symbol.From && placement.Line <= symbol.To {
					claimed[noteIndex] = true
					symbolNotes = append(symbolNotes, semanticNoteNode(note, notes.stale))
				}
			}
			if !changed && len(symbolNotes) == 0 {
				continue
			}
			kind := cleanNoteOneLine(symbol.Kind)
			name := cleanNoteOneLine(symbol.Name)
			label := contextKind.Render(kind+" ") + name + semanticChurn(add, del, up, down)
			children = append(children, semanticNode{label: label, children: symbolNotes})
		}
		if edges := unmatchedEdges(opts.Symbols[file.Path], opts.Edges[file.Path]); len(edges) > 0 {
			up, down := edgeCounts(edges)
			children = append(children, semanticNode{label: "file call changes" + semanticChurn(0, 0, up, down)})
		}
		for noteIndex, note := range fileNotes {
			if !claimed[noteIndex] {
				children = append(children, semanticNoteNode(note, notes.stale))
			}
		}
		if len(children) > 0 {
			files = append(files, semanticNode{label: contextPath.Render(file.Path), children: children})
		}
	}
	if len(files) == 0 {
		return ""
	}
	rows := []string{contextTitle.Render("change context")}
	appendSemanticNodes(&rows, files, "", width)
	return strings.Join(rows, "\n")
}

func appendSemanticNodes(rows *[]string, nodes []semanticNode, prefix string, width int) {
	for index, node := range nodes {
		last := index == len(nodes)-1
		connector := "├── "
		continuation := "│   "
		if last {
			connector = "└── "
			continuation = "    "
		}
		*rows = append(*rows, prefix+contextKind.Render(connector)+truncateTreeLabel(node.label, width-lipgloss.Width(prefix+connector)))
		appendSemanticNodes(rows, node.children, prefix+contextKind.Render(continuation), width)
	}
}

func semanticNoteNode(note provider.Note, stale bool) semanticNode {
	children := []semanticNode{{label: cleanNoteOneLine(note.Summary)}}
	if rationale := cleanNoteOneLine(note.Rationale); rationale != "" {
		children = append(children, semanticNode{label: contextKind.Render(rationale)})
	}
	return semanticNode{label: noteTreeLabel(note, stale), children: children}
}

func truncateTreeLabel(label string, width int) string {
	if width < 8 {
		width = 8
	}
	return ansi.Truncate(label, width, "…")
}

type noteInsertion struct {
	marker string
	prefix string
	side   string
	notes  []provider.Note
}

type changeGroupPart struct {
	path    []string
	options diffview.Options
	notes   []provider.Note
}

func renderTreeWithGroups(opts diffview.Options, groups []provider.ChangeGroup, notes noteLayer, width int) (string, error) {
	parts := partitionChangeGroups(opts, groups, notes.values)
	if len(parts) == 0 {
		return renderTreeWithNotes(opts, notes, width)
	}
	combined := opts
	combined.Files = nil
	combined.Symbols = map[string][]diffview.Symbol{}
	combined.Edges = map[string][]diffview.Edge{}
	combined.Pins = map[string]bool{}
	insertions := []noteInsertion{}
	for _, part := range parts {
		marked, partInsertions, err := markNoteRows(part.options, part.notes)
		if err != nil {
			return "", err
		}
		prefix := strings.Join(part.path, "/") + "/"
		groupPath := ""
		for _, component := range part.path {
			if groupPath == "" {
				groupPath = component
			} else {
				groupPath += "/" + component
			}
			combined.Pins[groupPath] = true
		}
		for _, file := range marked.Files {
			oldPath := file.Path
			file.Path = prefix + oldPath
			combined.Files = append(combined.Files, file)
			combined.Symbols[file.Path] = marked.Symbols[oldPath]
			combined.Edges[file.Path] = marked.Edges[oldPath]
		}
		insertions = append(insertions, partInsertions...)
	}
	body, err := renderMarkedTree(combined, insertions, notes, width)
	if err != nil {
		return "", err
	}
	return fitTreeWidth(replaceTreeSummary(body, opts), width), nil
}

func partitionChangeGroups(opts diffview.Options, groups []provider.ChangeGroup, notes []provider.Note) []changeGroupPart {
	if len(groups) == 0 {
		return nil
	}
	ordered := append([]provider.ChangeGroup(nil), groups...)
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Order != ordered[right].Order {
			return ordered[left].Order < ordered[right].Order
		}
		return ordered[left].ID < ordered[right].ID
	})
	byID := make(map[string]provider.ChangeGroup, len(ordered))
	for _, group := range ordered {
		byID[group.ID] = group
	}
	parts := make([]changeGroupPart, len(ordered)+1)
	for index, group := range ordered {
		parts[index] = changeGroupPart{path: changeGroupPath(group, byID), options: emptyDiffOptions(opts)}
	}
	parts[len(ordered)] = changeGroupPart{path: []string{"other changes"}, options: emptyDiffOptions(opts)}
	for _, file := range opts.Files {
		assigned := make([][]diffview.Hunk, len(parts))
		for _, hunk := range file.Hunks {
			part := len(ordered)
			for index, group := range ordered {
				if groupClaimsHunk(group, file.Path, hunk) {
					part = index
					break
				}
			}
			assigned[part] = append(assigned[part], hunk)
		}
		if len(file.Hunks) == 0 {
			part := len(ordered)
			for index, group := range ordered {
				if groupClaimsFile(group, file.Path) {
					part = index
					break
				}
			}
			assigned[part] = []diffview.Hunk{}
			addGroupedFile(&parts[part].options, file, assigned[part], opts)
			continue
		}
		for index, hunks := range assigned {
			if len(hunks) > 0 {
				addGroupedFile(&parts[index].options, file, hunks, opts)
			}
		}
	}
	for _, note := range notes {
		part := len(ordered)
		placement := noteDisplayPlacement(note)
		for index, group := range ordered {
			if groupClaimsPlacement(group, placement) {
				part = index
				break
			}
		}
		parts[part].notes = append(parts[part].notes, note)
	}
	visible := parts[:0]
	for _, part := range parts {
		if len(part.options.Files) > 0 || len(part.notes) > 0 {
			visible = append(visible, part)
		}
	}
	return visible
}

func emptyDiffOptions(opts diffview.Options) diffview.Options {
	opts.Files = nil
	opts.Symbols = map[string][]diffview.Symbol{}
	opts.Edges = map[string][]diffview.Edge{}
	opts.Pins = map[string]bool{}
	return opts
}

func addGroupedFile(target *diffview.Options, file diffview.File, hunks []diffview.Hunk, source diffview.Options) {
	file.Hunks = append([]diffview.Hunk(nil), hunks...)
	file.Add, file.Del = 0, 0
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Kind {
			case '+':
				file.Add++
			case '-':
				file.Del++
			}
		}
	}
	target.Files = append(target.Files, file)
	for _, symbol := range source.Symbols[file.Path] {
		for _, hunk := range hunks {
			if hunkContainsRange(hunk, provider.NoteSideRight, symbol.From, symbol.To) {
				target.Symbols[file.Path] = append(target.Symbols[file.Path], symbol)
				break
			}
		}
	}
	for _, edge := range source.Edges[file.Path] {
		for _, hunk := range hunks {
			if hunkContainsRange(hunk, provider.NoteSideRight, edge.Line, edge.Line) {
				target.Edges[file.Path] = append(target.Edges[file.Path], edge)
				break
			}
		}
	}
}

func changeGroupPath(group provider.ChangeGroup, groups map[string]provider.ChangeGroup) []string {
	path := []string{cleanNoteOneLine(group.Title)}
	for group.ParentID != "" {
		parent, exists := groups[group.ParentID]
		if !exists {
			break
		}
		path = append([]string{cleanNoteOneLine(parent.Title)}, path...)
		group = parent
	}
	return path
}

func groupClaimsFile(group provider.ChangeGroup, path string) bool {
	for _, anchor := range group.Anchors {
		if anchor.Line == 0 && sameDiffPath(anchor.Path, path) {
			return true
		}
	}
	return false
}

func groupClaimsHunk(group provider.ChangeGroup, path string, hunk diffview.Hunk) bool {
	for _, anchor := range group.Anchors {
		if !sameDiffPath(anchor.Path, path) {
			continue
		}
		if anchor.Line == 0 || hunkContainsRange(hunk, anchor.Side, anchor.StartLine, anchor.Line) {
			return true
		}
	}
	return false
}

func groupClaimsPlacement(group provider.ChangeGroup, placement provider.NotePlacement) bool {
	for _, anchor := range group.Anchors {
		if !sameDiffPath(anchor.Path, placement.Path) {
			continue
		}
		if anchor.Line == 0 || anchor.Side == placement.Side && rangesOverlap(anchor.StartLine, anchor.Line, placement.StartLine, placement.Line) {
			return true
		}
	}
	return false
}

func hunkContainsRange(hunk diffview.Hunk, side string, start, end int) bool {
	oldLine, newLine := hunk.OldAt, hunk.NewAt
	for _, line := range hunk.Lines {
		candidate := newLine
		present := line.Kind != '-'
		if side == provider.NoteSideLeft {
			candidate, present = oldLine, line.Kind != '+'
		}
		if present && rangesOverlap(start, end, candidate, candidate) {
			return true
		}
		switch line.Kind {
		case '+':
			newLine++
		case '-':
			oldLine++
		default:
			oldLine++
			newLine++
		}
	}
	return false
}

func rangesOverlap(leftStart, leftEnd, rightStart, rightEnd int) bool {
	if leftStart == 0 {
		leftStart = leftEnd
	}
	if rightStart == 0 {
		rightStart = rightEnd
	}
	return leftStart <= rightEnd && rightStart <= leftEnd
}

func renderTreeWithNotes(opts diffview.Options, notes noteLayer, width int) (string, error) {
	marked, insertions, err := markNoteRows(opts, notes.values)
	if err != nil {
		return "", err
	}
	return renderMarkedTree(marked, insertions, notes, width)
}

func renderMarkedTree(marked diffview.Options, insertions []noteInsertion, notes noteLayer, width int) (string, error) {
	body := diffview.Render(marked)
	if len(insertions) == 0 {
		return fitTreeWidth(body, width), nil
	}
	lines := strings.Split(body, "\n")
	byRow := map[int][]noteInsertion{}
	for _, insertion := range insertions {
		found := false
		for row := range lines {
			position := strings.Index(lines[row], insertion.marker)
			if insertion.side == provider.NoteSideRight {
				position = strings.LastIndex(lines[row], insertion.marker)
			}
			if position < 0 {
				continue
			}
			found = true
			prefix := lineNotePrefix(lines[row][:position])
			if insertion.prefix == "file" {
				prefix = fileNotePrefix(lines[row][:position])
			}
			lines[row] = strings.ReplaceAll(lines[row], insertion.marker, "")
			insertion.prefix = prefix
			byRow[row] = append(byRow[row], insertion)
			break
		}
		if !found {
			return "", errors.New("rendered diff omitted a note anchor")
		}
	}
	output := make([]string, 0, len(lines)+len(notes.values)*3)
	for row, line := range lines {
		output = append(output, line)
		for _, insertion := range byRow[row] {
			output = append(output, embeddedNoteLines(insertion.prefix, insertion.notes, notes.stale, width)...)
		}
	}
	return fitTreeWidth(strings.Join(output, "\n"), width), nil
}

func replaceTreeSummary(body string, original diffview.Options) string {
	if body == "" {
		return body
	}
	add, del := 0, 0
	for _, file := range original.Files {
		add += file.Add
		del += file.Del
	}
	word := "files"
	if len(original.Files) == 1 {
		word = "file"
	}
	summary := contextTitle.Render(fmt.Sprintf("%d %s", len(original.Files), word)) + "   " +
		contextAdd.Render(fmt.Sprintf("+%d", add)) + "  " + contextDel.Render(fmt.Sprintf("-%d", del))
	if _, rest, found := strings.Cut(body, "\n"); found {
		return summary + "\n" + rest
	}
	return summary
}

func markNoteRows(opts diffview.Options, notes []provider.Note) (diffview.Options, []noteInsertion, error) {
	marked := cloneDiffOptions(opts)
	insertions := []noteInsertion{}
	matched := make([]bool, len(notes))
	markerCount := 0
	nextMarker := func() string {
		markerCount++
		return noteMarker(markerCount)
	}
	for fileIndex := range marked.Files {
		file := &marked.Files[fileIndex]
		for hunkIndex := range file.Hunks {
			hunk := &file.Hunks[hunkIndex]
			oldLine, newLine := hunk.OldAt, hunk.NewAt
			for lineIndex := range hunk.Lines {
				line := &hunk.Lines[lineIndex]
				for _, side := range []string{provider.NoteSideLeft, provider.NoteSideRight} {
					group := []provider.Note{}
					for noteIndex, note := range notes {
						if matched[noteIndex] || opts.Stat || !sameDiffPath(noteDisplayPlacement(note).Path, file.Path) {
							continue
						}
						placement := noteDisplayPlacement(note)
						if placement.Side == side && placement.Quality == provider.PlacementExact &&
							noteLineMatches(placement, line.Kind, oldLine, newLine) {
							matched[noteIndex] = true
							group = append(group, note)
						}
					}
					if len(group) > 0 {
						marker := nextMarker()
						line.Text = marker + line.Text
						insertions = append(insertions, noteInsertion{marker: marker, side: side, notes: group})
					}
				}
				switch line.Kind {
				case '+':
					newLine++
				case '-':
					oldLine++
				default:
					oldLine++
					newLine++
				}
			}
		}
		fallback := []provider.Note{}
		for noteIndex, note := range notes {
			if !matched[noteIndex] && sameDiffPath(noteDisplayPlacement(note).Path, file.Path) {
				matched[noteIndex] = true
				fallback = append(fallback, note)
			}
		}
		if len(fallback) > 0 {
			marker := nextMarker()
			oldPath := file.Path
			file.Path = markFilePath(file.Path, marker)
			marked.Symbols[file.Path] = marked.Symbols[oldPath]
			marked.Edges[file.Path] = marked.Edges[oldPath]
			delete(marked.Symbols, oldPath)
			delete(marked.Edges, oldPath)
			insertions = append(insertions, noteInsertion{marker: marker, prefix: "file", notes: fallback})
		}
	}
	for noteIndex, note := range notes {
		if matched[noteIndex] {
			continue
		}
		marker := nextMarker()
		path := noteDisplayPlacement(note).Path
		marked.Files = append(marked.Files, diffview.File{Path: markFilePath(displayDiffPath(path), marker)})
		insertions = append(insertions, noteInsertion{marker: marker, prefix: "file", notes: []provider.Note{note}})
	}
	return marked, insertions, nil
}

func cloneDiffOptions(opts diffview.Options) diffview.Options {
	clone := opts
	clone.Files = append([]diffview.File(nil), opts.Files...)
	clone.Symbols = make(map[string][]diffview.Symbol, len(opts.Symbols))
	clone.Edges = make(map[string][]diffview.Edge, len(opts.Edges))
	for path, symbols := range opts.Symbols {
		clone.Symbols[path] = append([]diffview.Symbol(nil), symbols...)
	}
	for path, edges := range opts.Edges {
		clone.Edges[path] = append([]diffview.Edge(nil), edges...)
	}
	for fileIndex := range clone.Files {
		clone.Files[fileIndex].Hunks = append([]diffview.Hunk(nil), opts.Files[fileIndex].Hunks...)
		for hunkIndex := range clone.Files[fileIndex].Hunks {
			original := opts.Files[fileIndex].Hunks[hunkIndex].Lines
			clone.Files[fileIndex].Hunks[hunkIndex].Lines = append([]diffview.Line(nil), original...)
		}
	}
	return clone
}

func noteLineMatches(placement provider.NotePlacement, kind byte, oldLine, newLine int) bool {
	switch placement.Side {
	case provider.NoteSideLeft:
		return kind != '+' && placement.Line == oldLine
	case provider.NoteSideRight:
		return kind != '-' && placement.Line == newLine
	default:
		return false
	}
}

func noteMarker(index int) string {
	// Git forbids NUL in paths and file contents. The two zero-width variation
	// selectors make each marker distinct without affecting terminal layout.
	index--
	return "\x00" + string(rune(0xe0100+index/240)) + string(rune(0xe0100+index%240))
}

func markFilePath(path, marker string) string {
	index := strings.LastIndexByte(path, '/') + 1
	return path[:index] + marker + path[index:]
}

func lineNotePrefix(beforeMarker string) string {
	plain := ansi.Strip(stripNoteMarkers(beforeMarker))
	if strings.Contains(plain, " │ ") {
		return trimASCIISuffix(plain, 5) + strings.Repeat(" ", 5)
	}
	return trimASCIISuffix(plain, 7) + strings.Repeat(" ", 5)
}

func fileNotePrefix(beforeMarker string) string {
	plain := ansi.Strip(stripNoteMarkers(beforeMarker))
	for _, branch := range []struct{ connector, guide string }{{"├── ", "│   "}, {"└── ", "    "}} {
		if strings.HasSuffix(plain, branch.connector) {
			return strings.TrimSuffix(plain, branch.connector) + branch.guide + "│ "
		}
	}
	return "│ "
}

func stripNoteMarkers(value string) string {
	return strings.Map(func(character rune) rune {
		if character == 0 || character >= 0xe0100 && character <= 0xe01ef {
			return -1
		}
		return character
	}, value)
}

func trimASCIISuffix(value string, count int) string {
	if count > len(value) {
		return ""
	}
	return value[:len(value)-count]
}

func fitTreeWidth(body string, width int) string {
	width = max(20, width)
	lines := strings.Split(body, "\n")
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], width, "…")
	}
	return strings.Join(lines, "\n")
}

func embeddedNoteLines(prefix string, notes []provider.Note, stale bool, width int) []string {
	rows := []string{}
	groups := noteRenderGroups(notes)
	for index, group := range groups {
		last := index == len(groups)-1
		connector := "├── "
		continuation := "│   "
		if last {
			connector = "└── "
			continuation = "    "
		}
		if group.threadID != "" {
			headPrefix := prefix + contextKind.Render(connector)
			label := contextTitle.Render("●") + " " + contextPath.Render(noteLocation(group.notes[0])) + " · " +
				contextKind.Render(fmt.Sprintf("thread · %d comments", len(group.notes)))
			rows = append(rows, headPrefix+truncateTreeLabel(label, width-lipgloss.Width(ansi.Strip(headPrefix))))
			bodyPrefix := prefix + contextKind.Render(continuation)
			rows = append(rows, embeddedThreadLines(bodyPrefix, group.notes, stale, width)...)
			continue
		}
		note := group.notes[0]
		headPrefix := prefix + contextKind.Render(connector)
		rows = append(rows, headPrefix+truncateTreeLabel(noteTreeLabel(note, stale), width-lipgloss.Width(ansi.Strip(headPrefix))))
		bodyPrefix := prefix + contextKind.Render(continuation)
		rows = append(rows, bodyPrefix+truncateTreeLabel(cleanNoteOneLine(note.Summary), width-lipgloss.Width(ansi.Strip(bodyPrefix))))
		if rationale := cleanNoteOneLine(note.Rationale); rationale != "" {
			rows = append(rows, bodyPrefix+contextKind.Render(truncateTreeLabel(rationale, width-lipgloss.Width(ansi.Strip(bodyPrefix)))))
		}
		if provenance := noteProvenanceLabel(note); provenance != "" {
			rows = append(rows, bodyPrefix+contextKind.Render(truncateTreeLabel(provenance, width-lipgloss.Width(ansi.Strip(bodyPrefix)))))
		}
	}
	return rows
}

type noteRenderGroup struct {
	threadID string
	notes    []provider.Note
}

func noteRenderGroups(notes []provider.Note) []noteRenderGroup {
	groups := []noteRenderGroup{}
	threads := map[string]int{}
	for _, note := range notes {
		if note.ThreadID == "" {
			groups = append(groups, noteRenderGroup{notes: []provider.Note{note}})
			continue
		}
		if index, ok := threads[note.ThreadID]; ok {
			groups[index].notes = append(groups[index].notes, note)
			continue
		}
		threads[note.ThreadID] = len(groups)
		groups = append(groups, noteRenderGroup{threadID: note.ThreadID, notes: []provider.Note{note}})
	}
	return groups
}

func embeddedThreadLines(prefix string, notes []provider.Note, stale bool, width int) []string {
	rows := []string{}
	for index, note := range notes {
		last := index == len(notes)-1
		connector, continuation := "├── ", "│   "
		if last {
			connector, continuation = "└── ", "    "
		}
		headPrefix := prefix + contextKind.Render(connector)
		rows = append(rows, headPrefix+truncateTreeLabel(noteTreeLabel(note, stale), width-lipgloss.Width(ansi.Strip(headPrefix))))
		bodyPrefix := prefix + contextKind.Render(continuation)
		rows = append(rows, bodyPrefix+truncateTreeLabel(cleanNoteOneLine(note.Summary), width-lipgloss.Width(ansi.Strip(bodyPrefix))))
		if rationale := cleanNoteOneLine(note.Rationale); rationale != "" {
			rows = append(rows, bodyPrefix+contextKind.Render(truncateTreeLabel(rationale, width-lipgloss.Width(ansi.Strip(bodyPrefix)))))
		}
		if provenance := noteProvenanceLabel(note); provenance != "" {
			rows = append(rows, bodyPrefix+contextKind.Render(truncateTreeLabel(provenance, width-lipgloss.Width(ansi.Strip(bodyPrefix)))))
		}
	}
	return rows
}

func noteProvenanceLabel(note provider.Note) string {
	parts := []string{}
	if note.Provenance.Kind != "" {
		parts = append(parts, note.Provenance.Kind)
	}
	if note.Provenance.Tool != "" {
		parts = append(parts, "tool "+note.Provenance.Tool)
	}
	if note.Provenance.SessionID != "" {
		parts = append(parts, "session "+note.Provenance.SessionID)
	}
	if note.Provenance.WorkingDirectory != "" {
		parts = append(parts, "from "+note.Provenance.WorkingDirectory)
	}
	url := note.Provenance.URL
	if url == "" {
		url = note.URL
	}
	if url != "" {
		parts = append(parts, url)
	}
	return strings.Join(parts, " · ")
}

func noteTreeLabel(note provider.Note, stale bool) string {
	placement := noteDisplayPlacement(note)
	state := note.State
	if placement.Quality != provider.PlacementExact {
		state += "/" + placement.Quality
	}
	if stale {
		state += "/refresh-pending"
	}
	location := "file"
	if placement.Line > 0 {
		location = fmt.Sprintf("line %d@%s", placement.Line, strings.ToLower(placement.Side))
		if placement.StartLine > 0 {
			location = fmt.Sprintf("lines %d@%s-%s", placement.StartLine, strings.ToLower(placement.StartSide), strings.TrimPrefix(location, "line "))
		}
	}
	return contextTitle.Render("●") + " " + contextPath.Render(location) + " · " +
		contextKind.Render(state+" ") + cleanNoteOneLine(note.Author) + " via " + cleanNoteOneLine(note.Source)
}

func noteDisplayPlacement(note provider.Note) provider.NotePlacement {
	if note.Placement.Quality != provider.PlacementOrphan {
		return note.Placement
	}
	return provider.NotePlacement{
		Path: note.Anchor.Path, Side: note.Anchor.Side, StartSide: note.Anchor.StartSide,
		StartLine: note.Anchor.StartLine, Line: note.Anchor.Line, Quality: provider.PlacementOrphan,
	}
}

func notesForPath(notes []provider.Note, path string) []provider.Note {
	selected := []provider.Note{}
	for _, note := range notes {
		notePath, _ := notePosition(note)
		if sameDiffPath(notePath, path) {
			selected = append(selected, note)
		}
	}
	return selected
}

func diffFileForPath(files []diffview.File, path string) (diffview.File, bool) {
	for _, file := range files {
		if sameDiffPath(path, file.Path) {
			return file, true
		}
	}
	return diffview.File{}, false
}

func sameDiffPath(left, right string) bool {
	decode := func(path string) string {
		if strings.HasPrefix(path, `"`) && strings.HasSuffix(path, `"`) {
			if decoded, err := strconv.Unquote(path); err == nil {
				path = strings.TrimPrefix(decoded, "b/")
			}
		}
		return filepath.ToSlash(filepath.Clean(path))
	}
	return decode(left) == decode(right)
}

func semanticCounts(file *diffview.File, symbol diffview.Symbol, edges []diffview.Edge) (add, del, up, down int, changed bool) {
	for _, hunk := range file.Hunks {
		lineNumber := hunk.NewAt
		for _, line := range hunk.Lines {
			switch line.Kind {
			case '+':
				if lineNumber >= symbol.From && lineNumber <= symbol.To {
					add++
					changed = true
				}
				lineNumber++
			case '-':
				if lineNumber >= symbol.From && lineNumber <= symbol.To {
					del++
					changed = true
				}
			default:
				lineNumber++
			}
		}
	}
	for _, edge := range edges {
		if edge.Line < symbol.From || edge.Line > symbol.To {
			continue
		}
		if edge.Added {
			up++
		} else {
			down++
		}
	}
	return add, del, up, down, changed
}

func edgeCounts(edges []diffview.Edge) (up, down int) {
	for _, edge := range edges {
		if edge.Added {
			up++
		} else {
			down++
		}
	}
	return up, down
}

func unmatchedEdges(symbols []diffview.Symbol, edges []diffview.Edge) []diffview.Edge {
	unmatched := make([]diffview.Edge, 0, len(edges))
	for _, edge := range edges {
		matched := false
		for _, symbol := range symbols {
			if edge.Line >= symbol.From && edge.Line <= symbol.To {
				matched = true
				break
			}
		}
		if !matched {
			unmatched = append(unmatched, edge)
		}
	}
	return unmatched
}

func semanticChurn(add, del, up, down int) string {
	parts := []string{}
	if add > 0 {
		parts = append(parts, contextAdd.Render(fmt.Sprintf("+%d", add)))
	}
	if del > 0 {
		parts = append(parts, contextDel.Render(fmt.Sprintf("-%d", del)))
	}
	if up > 0 {
		parts = append(parts, contextAdd.Render(fmt.Sprintf("+%d %s", up, countLabel(up, "call"))))
	}
	if down > 0 {
		parts = append(parts, contextDel.Render(fmt.Sprintf("-%d %s", down, countLabel(down, "call"))))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

func normalizeDiffPaths(files []diffview.File) ([]diffview.File, map[string]string) {
	rawPaths := make(map[string]string, len(files))
	for index := range files {
		rawPath := files[index].Path
		if strings.HasPrefix(rawPath, `"`) && strings.HasSuffix(rawPath, `"`) {
			if decoded, err := strconv.Unquote(rawPath); err == nil {
				rawPath = strings.TrimPrefix(decoded, "b/")
			}
		}
		displayPath := displayDiffPath(rawPath)
		files[index].Path = displayPath
		rawPaths[displayPath] = rawPath
	}
	return files, rawPaths
}

func parseDiffFiles(patch string) ([]diffview.File, map[string]string) {
	sections := splitPatchFiles(patch)
	if len(sections) == 0 {
		files, paths := normalizeDiffPaths(diffview.Parse(patch))
		return sanitizeDiffFiles(files), paths
	}
	files := make([]diffview.File, 0, len(sections))
	rawPaths := make(map[string]string, len(sections))
	for _, section := range sections {
		rawPath := section.newPath
		if rawPath == "" {
			rawPath = section.oldPath
		}
		if rawPath == "" {
			continue
		}
		parsed := diffview.Parse(section.patch)
		file := diffview.File{}
		if len(parsed) > 0 {
			file = parsed[0]
		}
		file = sanitizeDiffFiles([]diffview.File{file})[0]
		displayPath := displayDiffPath(rawPath)
		file.Path = displayPath
		files = append(files, file)
		rawPaths[displayPath] = rawPath
	}
	return files, rawPaths
}

func sanitizeDiffFiles(files []diffview.File) []diffview.File {
	for fileIndex := range files {
		for hunkIndex := range files[fileIndex].Hunks {
			for lineIndex := range files[fileIndex].Hunks[hunkIndex].Lines {
				line := &files[fileIndex].Hunks[hunkIndex].Lines[lineIndex]
				line.Text = sanitizeDiffText(line.Text)
			}
		}
	}
	return files
}

func sanitizeDiffText(value string) string {
	return strings.Map(func(character rune) rune {
		if character != '\t' && unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
}

func notePathsInPatch(patch string) []string {
	seen := map[string]bool{}
	paths := []string{}
	for _, section := range splitPatchFiles(patch) {
		for _, path := range []string{section.oldPath, section.newPath} {
			if path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func diffPathAliases(patch string) map[string]string {
	aliases := map[string]string{}
	for _, section := range splitPatchFiles(patch) {
		if section.oldPath != "" && section.newPath != "" {
			aliases[section.oldPath] = section.newPath
		}
	}
	return aliases
}

func remapNotePaths(layer noteLayer, aliases map[string]string) noteLayer {
	if len(aliases) == 0 || len(layer.values) == 0 {
		return layer
	}
	values := append([]provider.Note(nil), layer.values...)
	remap := func(path string) string {
		for oldPath, newPath := range aliases {
			if sameDiffPath(path, oldPath) {
				return newPath
			}
		}
		return path
	}
	for index := range values {
		values[index].Anchor.Path = remap(values[index].Anchor.Path)
		if values[index].Placement.Path != "" {
			values[index].Placement.Path = remap(values[index].Placement.Path)
		}
	}
	layer.values = values
	return layer
}

func displayDiffPath(path string) string {
	for _, character := range path {
		if character < 0x20 || character > 0x7e || character == '"' || character == '\\' {
			return strconv.Quote(path)
		}
	}
	return path
}

// One repository renders under its own paths, so a single repository reads
// exactly as it did before -r existed.
func (r renderer) prefix(dir string) string {
	if !r.named {
		return ""
	}
	rel, err := filepath.Rel(r.under, dir)
	if err != nil || rel == "." {
		return filepath.Base(dir) + "/"
	}
	return rel + "/"
}

func (r renderer) layers(ctx context.Context, spec source.Spec, touched []string, patch string) (map[string][]diffview.Symbol, map[string][]diffview.Edge, []provider.ChangeGroup) {
	touched = append([]string(nil), touched...)
	sort.Strings(touched)

	syms := map[string][]diffview.Symbol{}
	edges := map[string][]diffview.Edge{}
	groups := []provider.ChangeGroup{}
	request := provider.Request{
		Directory:   spec.Dir,
		Files:       touched,
		Fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(patch))),
		From:        spec.From,
		Staged:      spec.Staged,
		To:          spec.To,
	}
	for _, configured := range r.providers {
		if r.syms && provider.Supports(configured.Manifest, provider.ActionSymbols) {
			response, err := provider.Run(ctx, configured, provider.ActionSymbols, request, r.providerCache)
			if err != nil {
				fmt.Fprintf(os.Stderr, "changes: %v\n", err)
			} else {
				mergeSymbols(syms, response.Symbols)
			}
		}
		if r.calls && provider.Supports(configured.Manifest, provider.ActionCalls) {
			response, err := provider.Run(ctx, configured, provider.ActionCalls, request, r.providerCache)
			if err != nil {
				fmt.Fprintf(os.Stderr, "changes: %v\n", err)
			} else {
				mergeEdges(edges, response.Edges)
			}
		}
	}
	if r.groups {
		configured, found, err := selectGroupProvider(r.providers, r.groupProvider)
		if err != nil {
			fmt.Fprintf(os.Stderr, "changes: %v\n", err)
		} else if found {
			groupRequest := request
			groupRequest.Patch = patch
			response, runErr := provider.Run(ctx, configured, provider.ActionGroups, groupRequest, r.providerCache)
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "changes: %v\n", runErr)
			} else {
				groups = response.Groups
			}
		}
	}
	return syms, edges, groups
}

func selectGroupProvider(providers []provider.LoadedManifest, name string) (provider.LoadedManifest, bool, error) {
	for _, configured := range providers {
		if !provider.Supports(configured.Manifest, provider.ActionGroups) {
			continue
		}
		if name == "" || configured.Manifest.Name == name {
			return configured, true, nil
		}
	}
	if name != "" {
		return provider.LoadedManifest{}, false, fmt.Errorf("group provider %q was not found", name)
	}
	return provider.LoadedManifest{}, false, nil
}

func validateGroupSelection(engineName string, enabled bool, name string, providers []provider.LoadedManifest) error {
	if !enabled || name == "" {
		return nil
	}
	if engineName != "builtin" {
		return errors.New("logical grouping requires the builtin diff engine")
	}
	_, _, err := selectGroupProvider(providers, name)
	return err
}

func mergeSymbols(target, source map[string][]diffview.Symbol) {
	for path, values := range source {
		target[path] = append(target[path], values...)
	}
}

func mergeEdges(target, source map[string][]diffview.Edge) {
	for path, values := range source {
		target[path] = append(target[path], values...)
	}
}

// The renderer pads every row to the width it is given, so a width taken from
// a pipe would print a wall of trailing spaces. 100 is the width the side by
// side view was tuned against.
func (r renderer) columns() int { return columns(r.width) }

// follow reprints only when the complete frame changes. It checks patches on
// the watch interval and notes on their slower refresh interval, which keeps
// remote providers below the local file polling rate.
func (r renderer) follow(every time.Duration) error {
	lastPatches := ""
	lastFrame := ""
	lastNoteRead := time.Time{}
	notes := noteLayer{}
	initialized := false
	for {
		patches, err := r.patches()
		if err != nil {
			return err
		}
		currentPatches := strings.Join(patches, "\x00")
		patchChanged := !initialized || currentPatches != lastPatches
		notesDue := noteRefreshDue(r.notes, lastNoteRead, time.Now(), r.noteInterval)
		if patchChanged || notesDue {
			notesForFrame := noteLayerForFrame(notes, patchChanged, notesDue)
			out, refreshedNotes, err := r.renderPatchesWithNotes(patches, notesForFrame, notesDue)
			if err != nil {
				return err
			}
			if notesDue {
				notes = refreshedNotes
			}
			if out == "" {
				out = "changes: nothing changed"
			}
			if !initialized || out != lastFrame {
				// Home the cursor and clear forward, so the frame lands in the
				// scrollback the reader already has rather than in an alt screen
				// they cannot scroll.
				fmt.Print("\x1b[H\x1b[2J", out, "\n")
			}
			lastFrame = out
			if notesDue {
				lastNoteRead = time.Now()
			}
		}
		lastPatches = currentPatches
		initialized = true
		time.Sleep(every)
	}
}

func noteLayerForFrame(notes noteLayer, patchChanged, notesDue bool) noteLayer {
	if patchChanged && !notesDue {
		return noteLayer{}
	}
	return notes
}

func noteRefreshDue(enabled bool, lastRead, now time.Time, interval time.Duration) bool {
	return enabled && now.Sub(lastRead) >= interval
}

func columns(width int) int {
	if width > 0 {
		return width
	}
	if value, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && value > 0 {
		return value
	}
	return 100
}

func resolveColor(value string) string {
	if value == "auto" {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return "always"
		}
		return "never"
	}
	return value
}

func configureColor(value string) string {
	resolved := resolveColor(value)
	switch resolved {
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	default:
		fail(fmt.Errorf("-color must be auto, always, or never"))
	}
	return resolved
}

func runDifftool(args []string) {
	configured, err := appconfig.Load(argumentValue(args, "config"))
	if err != nil {
		fail(err)
	}
	metadata := subcommandMetadata("difftool")
	flags := flag.NewFlagSet("changes difftool", flag.ContinueOnError)
	flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	diffEngine := flags.String("engine", fileEngine(configured.Diff.Difftool), flagDescription(metadata, "engine"))
	difftool := flags.String("difftool", "", flagDescription(metadata, "difftool"))
	layout := flags.String("layout", configured.Diff.Layout, flagDescription(metadata, "layout"))
	color := flags.String("color", configured.Color, flagDescription(metadata, "color"))
	width := flags.Int("width", 0, flagDescription(metadata, "width"))
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes difftool [flags] LOCAL REMOTE [MERGED]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() < 2 || flags.NArg() > 3 {
		fail(fmt.Errorf("difftool requires LOCAL and REMOTE files, with an optional MERGED path"))
	}
	label := os.Getenv("MERGED")
	if flags.NArg() == 3 {
		label = flags.Arg(2)
	}
	options := engine.Options{
		Color:    configureColor(*color),
		Difftool: commandValue(configured.Diff.Difftool, *difftool),
		Label:    label,
		Layout:   *layout,
		Width:    columns(*width),
	}
	if err := engine.ValidateFiles(*diffEngine, options); err != nil {
		fail(err)
	}
	out, err := engine.Files(*diffEngine, flags.Arg(0), flags.Arg(1), options)
	if err != nil {
		fail(err)
	}
	if out != "" {
		fmt.Println(out)
	}
}

func runRender(args []string) {
	configured, err := appconfig.Load(argumentValue(args, "config"))
	if err != nil {
		fail(err)
	}
	metadata := subcommandMetadata("render")
	flags := flag.NewFlagSet("changes render", flag.ContinueOnError)
	flags.String("config", argumentValue(args, "config"), flagDescription(metadata, "config"))
	diffEngine := flags.String("engine", configured.Diff.Engine, flagDescription(metadata, "engine"))
	filter := flags.String("filter", "", flagDescription(metadata, "filter"))
	layout := flags.String("layout", configured.Diff.Layout, flagDescription(metadata, "layout"))
	color := flags.String("color", configured.Color, flagDescription(metadata, "color"))
	width := flags.Int("width", envInt("CHANGES_DIFF_WIDTH"), flagDescription(metadata, "width"))
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes render [flags]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("render reads a patch from standard input"))
	}
	patch, err := readPatch(os.Stdin, source.MaxPatchBytes)
	if err != nil {
		fail(err)
	}
	options := engine.Options{
		Color:  configureColor(*color),
		Filter: commandValue(configured.Diff.Filter, *filter),
		Layout: *layout,
		Width:  columns(*width),
	}
	if err := engine.ValidatePatch(*diffEngine, options); err != nil {
		fail(err)
	}
	out, err := engine.Patch(*diffEngine, string(patch), options)
	if err != nil {
		fail(err)
	}
	if out != "" {
		fmt.Println(out)
	}
}

func readPatch(reader io.Reader, limit int64) ([]byte, error) {
	patch, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(patch)) > limit {
		return nil, fmt.Errorf("input patch exceeds %d bytes", limit)
	}
	return patch, nil
}

func runCompletion(args []string) {
	metadata := subcommandMetadata("completion")
	if len(args) == 1 && isHelp(args[0]) {
		flags := flag.NewFlagSet("changes completion", flag.ContinueOnError)
		printCommandHelp(flags.Output(), "changes completion <bash|zsh|fish|nu>", metadata, flags)
		return
	}
	if len(args) != 1 {
		fail(fmt.Errorf("completion requires bash, zsh, fish, or nu"))
	}
	out, err := completion.Generate(args[0], completionGeneratorMetadata())
	if err != nil {
		fail(err)
	}
	fmt.Println(out)
}

func runCompletionValues(args []string) {
	if len(args) < 1 || len(args) > 2 {
		fail(errors.New("__values requires one value set and optional command context"))
	}
	context := ""
	if len(args) == 2 {
		context = args[1]
	}
	var values []string
	switch args[0] {
	case "repository":
		values = append(gitLineCompletionValues("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes", "refs/tags"), "HEAD")
		values = append(values, gitCompletionValues("ls-files", "-z", "--cached", "--others", "--exclude-standard")...)
	case "paths":
		values = gitCompletionValues("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	case "providers", "group-providers", "note-generators", "note-readers", "note-writers":
		configured, err := appconfig.Load(argumentValue(splitCompletionContext(context), "config"))
		if err != nil {
			return
		}
		discovery, err := provider.Discover(configured.Providers.Directory)
		if err != nil {
			return
		}
		for _, candidate := range discovery.Providers {
			action := ""
			if args[0] == "group-providers" {
				action = provider.ActionGroups
			} else if args[0] == "note-readers" {
				action = provider.ActionNotes
			} else if args[0] == "note-generators" {
				action = provider.ActionNotesGenerate
			} else if args[0] == "note-writers" {
				action = provider.ActionNotesCreate
			}
			if action == "" || provider.Supports(candidate.Manifest, action) {
				values = append(values, candidate.Manifest.Name)
			}
		}
	case "shells":
		values = []string{"bash", "zsh", "fish", "nu"}
	default:
		fail(fmt.Errorf("unknown completion value set %q", args[0]))
	}
	writeCompletionValues(os.Stdout, values)
}

func splitCompletionContext(value string) []string {
	var fields []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, character := range value {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if unicode.IsSpace(character) {
			flush()
			continue
		}
		current.WriteRune(character)
	}
	if escaped {
		current.WriteRune('\\')
	}
	flush()
	return fields
}

func writeCompletionValues(output io.Writer, values []string) {
	sort.Strings(values)
	last := ""
	for _, value := range values {
		if value == "" || value == last || strings.ContainsAny(value, "\r\n") {
			continue
		}
		_, _ = fmt.Fprintln(output, value)
		last = value
	}
}

func gitCompletionValues(args ...string) []string {
	output, ok := gitCompletionOutput(args...)
	if !ok {
		return nil
	}
	records := bytes.Split(output, []byte{0})
	values := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) != 0 {
			values = append(values, string(record))
		}
	}
	return values
}

func gitLineCompletionValues(args ...string) []string {
	output, ok := gitCompletionOutput(args...)
	if !ok {
		return nil
	}
	return strings.Fields(string(output))
}

func gitCompletionOutput(args ...string) ([]byte, bool) {
	command := exec.Command("git", args...)
	command.Env = gitutil.CleanEnv()
	output, err := command.Output()
	if err != nil {
		return nil, false
	}
	return output, true
}

func commandMetadata() completion.Command {
	return completion.Command{
		Name:              "changes",
		Synopsis:          "Render Git changes with logical groups, symbols, calls, and notes",
		CompletionCommand: completionValuesInvocation("repository"),
		LongDescription: `Refs follow git diff: none is the index against the working tree, one is that ref
against the working tree, and two compare the trees. A from of the form a..b is
split into two refs.

-r reads every repository under the workspace. Use -root to select its
boundary. Without -root, Changes uses the Git top level, then the working
directory. Each repository's files hang under its own name.`,
		Flags: []completion.Flag{
			{Name: "budget", Description: "Analysis time budget", Value: true},
			{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
			{Name: "config", Description: "YAML configuration file", Value: true},
			{Name: "engine", Description: "Patch display engine", Value: true, Values: engine.PatchNames},
			{Name: "filter", Description: "Standard-input patch filter", Value: true},
			{Name: "group-provider", Description: "Logical change-group provider", Value: true, CompletionCommand: contextualCompletionValuesInvocation("group-providers")},
			{Name: "interval", Description: "Watch interval", Value: true},
			{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
			{Name: "no-calls", Description: "Skip call analysis"},
			{Name: "no-groups", Description: "Skip logical change grouping"},
			{Name: "no-notes", Description: "Skip diff notes"},
			{Name: "no-symbols", Description: "Skip symbol analysis"},
			{Name: "quiet", Description: "Disable progress output"},
			{Name: "recursive", Short: "r", Description: "Read all workspace repositories"},
			{Name: "root", Description: "Workspace scan root", Value: true},
			{Name: "since", Description: "Left revision or time", Value: true},
			{Name: "staged", Description: "Compare the index"},
			{Name: "stat", Short: "s", Description: "Show change summary"},
			{Name: "watch", Short: "w", Description: "Watch for changes"},
			{Name: "width", Description: "Render width", Value: true},
			{Name: "version", Description: "Print the Changes version"},
		},
		Subcommands: []completion.Command{
			{Name: "interactive", Synopsis: "Review changes in an interactive workspace", Flags: workspaceCommandFlags(true)},
			{Name: "workspace", Synopsis: "Emit the versioned workspace snapshot", Flags: workspaceCommandFlags(false)},
			{
				Name:              "completion",
				Synopsis:          "Generate shell completions",
				CompletionCommand: completionValuesInvocation("shells"),
			},
			{
				Name:              "difftool",
				Synopsis:          "Compare Git difftool LOCAL and REMOTE files",
				CompletionCommand: completionValuesInvocation("paths"),
				Flags: []completion.Flag{
					{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
					{Name: "config", Description: "YAML configuration file", Value: true},
					{Name: "engine", Description: "File comparison engine", Value: true, Values: engine.FileNames},
					{Name: "difftool", Description: "Git-compatible difftool executable", Value: true},
					{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
					{Name: "width", Description: "Render width", Value: true},
				},
			},
			{
				Name:     "render",
				Synopsis: "Render a patch from standard input",
				Flags: []completion.Flag{
					{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
					{Name: "config", Description: "YAML configuration file", Value: true},
					{Name: "engine", Description: "Patch display engine", Value: true, Values: engine.PatchNames},
					{Name: "filter", Description: "Standard-input patch filter", Value: true},
					{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
					{Name: "width", Description: "Render width", Value: true},
				},
			},
			{
				Name:     "generate",
				Synopsis: "Generate README command docs and JSON Schema",
				Flags: []completion.Flag{
					{Name: "check", Description: "Fail when generated files are stale"},
				},
			},
			{
				Name:     "note",
				Synopsis: "Create and inspect diff notes",
				Subcommands: []completion.Command{
					{
						Name:     "add",
						Synopsis: "Create a note on the selected diff",
						Flags: []completion.Flag{
							{Name: "author", Description: "Note author", Value: true},
							{Name: "commit", Description: "First-parent commit comparison", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "config", Description: "YAML configuration file", Value: true},
							{Name: "expected-file-sha256", Description: "Require the selected file side to match this SHA-256 digest", Value: true},
							{Name: "file", Description: "Repository file to annotate", Value: true, CompletionCommand: completionValuesInvocation("paths")},
							{Name: "from", Description: "Left revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "json", Description: "Print the created note as JSON"},
							{Name: "line", Description: "Last line of the note range", Value: true},
							{Name: "message", Description: "Summary and optional rationale", Value: true},
							{Name: "message-file", Description: "Read note text from a file or standard input", Value: true},
							{Name: "origin", Description: "Author kind", Value: true, Values: []string{"agent", "user"}},
							{Name: "provider", Description: "Writable note provider", Value: true, CompletionCommand: contextualCompletionValuesInvocation("note-writers")},
							{Name: "session", Description: "Harness session identifier", Value: true},
							{Name: "side", Description: "Diff side", Value: true, Values: []string{"left", "right"}},
							{Name: "staged", Description: "Compare the index"},
							{Name: "start-line", Description: "First line of a multi-line range", Value: true},
							{Name: "to", Description: "Right revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
						},
					},
					{
						Name:     "generate",
						Synopsis: "Generate notes with a provider and save them",
						Flags: []completion.Flag{
							{Name: "commit", Description: "First-parent commit comparison", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "config", Description: "YAML configuration file", Value: true},
							{Name: "from", Description: "Left revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "json", Description: "Print generated notes as JSON"},
							{Name: "provider", Description: "Note generator provider", Value: true, CompletionCommand: contextualCompletionValuesInvocation("note-generators")},
							{Name: "session", Description: "Harness session identifier", Value: true},
							{Name: "staged", Description: "Compare the index"},
							{Name: "store", Description: "Writable note provider", Value: true, CompletionCommand: contextualCompletionValuesInvocation("note-writers")},
							{Name: "to", Description: "Right revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
						},
					},
					{
						Name:     "list",
						Synopsis: "List notes on the selected diff",
						Flags: []completion.Flag{
							{Name: "commit", Description: "First-parent commit comparison", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "config", Description: "YAML configuration file", Value: true},
							{Name: "from", Description: "Left revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
							{Name: "json", Description: "Print JSON"},
							{Name: "provider", Description: "Note provider", Value: true, CompletionCommand: contextualCompletionValuesInvocation("note-readers")},
							{Name: "staged", Description: "Compare the index"},
							{Name: "to", Description: "Right revision", Value: true, CompletionCommand: completionValuesInvocation("repository")},
						},
					},
				},
			},
			{
				Name:     "provider",
				Synopsis: "Inspect and validate context providers",
				Subcommands: []completion.Command{
					{
						Name:              "list",
						Synopsis:          "List configured analysis providers",
						Flags:             providerCommandFlags(),
						CompletionCommand: contextualCompletionValuesInvocation("providers"),
					},
					{
						Name:              "validate",
						Synopsis:          "Validate provider commands and JSON behavior",
						Flags:             providerCommandFlags(),
						CompletionCommand: contextualCompletionValuesInvocation("providers"),
					},
				},
			},
		},
	}
}

func completionValuesInvocation(kind string) []string {
	return []string{"changes", "__values", kind}
}

func contextualCompletionValuesInvocation(kind string) []string {
	return []string{"changes", "__values", kind, completion.ContextPlaceholder}
}

// the shared generator adds completion itself, so omit the explicit entry.
func completionGeneratorMetadata() completion.Command {
	command := commandMetadata()
	subcommands := make([]completion.Command, 0, len(command.Subcommands)-1)
	for _, subcommand := range command.Subcommands {
		if subcommand.Name != "completion" {
			subcommands = append(subcommands, subcommand)
		}
	}
	command.Subcommands = subcommands
	return command
}

func providerCommandFlags() []completion.Flag {
	return []completion.Flag{
		{Name: "config", Description: "YAML configuration file", Value: true},
		{Name: "json", Description: "Print JSON"},
	}
}

func subcommandMetadata(path ...string) completion.Command {
	command := commandMetadata()
	for _, name := range path {
		found := false
		for _, candidate := range command.Subcommands {
			if candidate.Name == name {
				command = candidate
				found = true
				break
			}
		}
		if !found {
			panic("missing command metadata for " + strings.Join(path, " "))
		}
	}
	return command
}

func flagDescription(command completion.Command, name string) string {
	for _, option := range command.Flags {
		if option.Name == name {
			return option.Description
		}
	}
	panic("missing flag metadata for --" + name)
}

func shortFlagDescription(command completion.Command, name string) string {
	for _, option := range command.Flags {
		if option.Short == name {
			return "shorthand for --" + option.Name
		}
	}
	panic("missing flag metadata for -" + name)
}

func printCommandHelp(output io.Writer, usage string, command completion.Command, flags *flag.FlagSet) {
	synopsis := command.Synopsis
	if synopsis == "" {
		synopsis = command.Description
	}
	_, _ = fmt.Fprintf(output, "Usage:\n  %s\n\n%s\n", usage, synopsis)
	if command.LongDescription != "" {
		_, _ = fmt.Fprintf(output, "\n%s\n", command.LongDescription)
	}
	if hasVisibleFlags(flags) {
		_, _ = fmt.Fprintln(output, "\nFlags:")
		flags.PrintDefaults()
	}
	if len(command.Subcommands) > 0 {
		_, _ = fmt.Fprintln(output, "\nCommands:")
		for _, subcommand := range command.Subcommands {
			synopsis := subcommand.Synopsis
			if synopsis == "" {
				synopsis = subcommand.Description
			}
			_, _ = fmt.Fprintf(output, "  %-12s %s\n", subcommand.Name, synopsis)
		}
	}
}

func hasVisibleFlags(flags *flag.FlagSet) bool {
	visible := false
	flags.VisitAll(func(_ *flag.Flag) { visible = true })
	return visible
}

func isHelp(argument string) bool {
	return argument == "-h" || argument == "--help"
}

func runProvider(args []string) {
	metadata := subcommandMetadata("provider")
	if len(args) == 1 && isHelp(args[0]) {
		flags := flag.NewFlagSet("changes provider", flag.ContinueOnError)
		printCommandHelp(flags.Output(), "changes provider <command>", metadata, flags)
		return
	}
	if len(args) == 0 || (args[0] != "list" && args[0] != "validate") {
		fail(fmt.Errorf("provider requires list or validate"))
	}
	action := args[0]
	metadata = subcommandMetadata("provider", action)
	flags := flag.NewFlagSet("changes provider "+action, flag.ContinueOnError)
	configPath := flags.String("config", "", flagDescription(metadata, "config"))
	asJSON := flags.Bool("json", false, flagDescription(metadata, "json"))
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes provider "+action+" [flags] [provider-name]", metadata, flags)
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() > 1 {
		fail(fmt.Errorf("provider %s accepts at most one provider name", action))
	}
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	discovery, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	providers := discovery.All()
	if flags.NArg() == 1 {
		name := flags.Arg(0)
		selected := []provider.LoadedManifest{}
		for _, manifest := range discovery.Providers {
			if manifest.Manifest.Name == name {
				selected = append(selected, manifest)
			}
		}
		for _, manifest := range discovery.Diagnostics {
			if manifest.Manifest.Name == name {
				selected = append(selected, manifest)
			}
		}
		if len(selected) == 0 {
			fail(fmt.Errorf("unknown provider %q", name))
		}
		providers = selected
	}
	if action == "list" {
		if *asJSON {
			data, _ := json.Marshal(providers)
			fmt.Println(string(data))
			return
		}
		if len(providers) == 0 {
			fmt.Println("No analysis providers are configured.")
			return
		}
		output, err := provider.RenderList(providers)
		if err != nil {
			fail(err)
		}
		if output != "" {
			fmt.Println(output)
		}
		return
	}
	if len(providers) == 0 {
		if *asJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No analysis providers are configured.")
		}
		return
	}
	results := make([]provider.Validation, 0, len(providers))
	failed := false
	for _, manifest := range providers {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(configured.Providers.Timeout))
		result := provider.Validate(ctx, manifest)
		cancel()
		results = append(results, result)
		failed = failed || !result.OK()
	}
	if *asJSON {
		data, _ := json.Marshal(results)
		fmt.Println(string(data))
	} else {
		output, err := provider.RenderValidations(results)
		if err != nil {
			fail(err)
		}
		if output != "" {
			fmt.Println(output)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func runGenerate(args []string) {
	metadata := subcommandMetadata("generate")
	flags := flag.NewFlagSet("changes generate", flag.ContinueOnError)
	check := flags.Bool("check", false, flagDescription(metadata, "check"))
	flags.Usage = func() {
		printCommandHelp(flags.Output(), "changes generate [flags]", metadata, flags)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("generate accepts only flags"))
	}
	schema, err := appconfig.Schema()
	if err != nil {
		fail(err)
	}
	providerSchema, err := provider.Schema()
	if err != nil {
		fail(err)
	}
	workspaceSchema, err := workspaceview.Schema()
	if err != nil {
		fail(err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		fail(fmt.Errorf("read README.md: %w", err))
	}
	generated, err := completion.ReplaceSection(string(readme), "cli", completion.Markdown(commandMetadata()))
	if err != nil {
		fail(err)
	}
	outputs := map[string][]byte{
		"README.md":                    []byte(generated),
		"schema/changes.schema.json":   schema,
		"schema/provider.schema.json":  providerSchema,
		"schema/workspace.schema.json": workspaceSchema,
	}
	for shell, path := range map[string]string{
		"bash": "completions/changes.bash",
		"fish": "completions/changes.fish",
		"nu":   "completions/changes.nu",
		"zsh":  "completions/changes.zsh",
	} {
		generatedCompletion, generateErr := completion.Generate(shell, completionGeneratorMetadata())
		if generateErr != nil {
			fail(generateErr)
		}
		outputs[path] = []byte(generatedCompletion + "\n")
	}
	for path, data := range outputs {
		if *check {
			current, readErr := os.ReadFile(path)
			if readErr != nil || string(current) != string(data) {
				fail(fmt.Errorf("%s is stale; run changes generate", path))
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(err)
		}
	}
}

func argumentValue(args []string, name string) string {
	long := "--" + name
	short := "-" + name
	for index, argument := range args {
		if value, ok := strings.CutPrefix(argument, long+"="); ok {
			return value
		}
		if value, ok := strings.CutPrefix(argument, short+"="); ok {
			return value
		}
		if (argument == long || argument == short) && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func commandValue(configured []string, override string) []string {
	if strings.TrimSpace(override) == "" {
		return configured
	}
	return strings.Fields(override)
}

func fileEngine(difftool []string) string {
	if len(difftool) > 0 {
		return "difftool"
	}
	return "builtin"
}

func envInt(name string) int {
	value := os.Getenv(name)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		fail(fmt.Errorf("%s must be an integer", name))
	}
	return parsed
}

func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "changes: %v\n", err)
	os.Exit(1)
}
