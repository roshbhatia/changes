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

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
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
	if len(os.Args) > 1 && os.Args[1] == "difftool" {
		runDifftool(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "render" {
		runRender(os.Args[2:])
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
	noSyms := flag.Bool("no-symbols", false, flagDescription(metadata, "no-symbols"))
	budget := flag.Duration("budget", time.Duration(configured.Providers.Timeout), flagDescription(metadata, "budget"))
	recurse := flag.Bool("recursive", false, flagDescription(metadata, "recursive"))
	flag.BoolVar(recurse, "r", false, shortFlagDescription(metadata, "r"))
	scan := flag.String("root", "", flagDescription(metadata, "root"))
	stat := flag.Bool("stat", false, flagDescription(metadata, "stat"))
	flag.BoolVar(stat, "s", false, shortFlagDescription(metadata, "s"))
	color := flag.String("color", configured.Color, flagDescription(metadata, "color"))
	diffEngine := flag.String("engine", configured.Diff.Engine, flagDescription(metadata, "engine"))
	filter := flag.String("filter", "", flagDescription(metadata, "filter"))
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
	providers := make([]provider.Manifest, 0, len(discovery.Providers))
	for _, loaded := range discovery.Providers {
		providers = append(providers, loaded.Manifest)
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
		budget:        *budget,
		color:         resolvedColor,
		engine:        *diffEngine,
		engineOptions: engineOptions,
		providers:     providers,
	}

	if !*watch {
		out, err := view.render()
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
	budget        time.Duration
	color         string
	engine        string
	engineOptions engine.Options
	providers     []provider.Manifest
}

func (r renderer) render() (string, error) {
	patches, err := r.patches()
	if err != nil {
		return "", err
	}
	return r.renderPatches(patches)
}

func (r renderer) renderPatches(patches []string) (string, error) {
	if r.stat || (r.engine == "builtin" && r.engineOptions.Layout == "side-by-side") {
		return r.draw(patches, false), nil
	}
	body, err := r.display(patches)
	if err != nil {
		return "", err
	}
	summary := r.semanticContext(patches)
	if summary == "" {
		return body, nil
	}
	if body == "" {
		return summary, nil
	}
	return summary + "\n\n" + body, nil
}

func (r renderer) display(patches []string) (string, error) {
	outputs := make([]string, 0, len(r.specs))
	if r.engine == "builtin" {
		for _, spec := range r.specs {
			output, err := spec.DisplayDiff(r.color)
			if err != nil {
				return "", err
			}
			if output != "" {
				outputs = append(outputs, output)
			}
		}
		return strings.Join(outputs, "\n\n"), nil
	}
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

// draw folds every repository into one tree. Each file keeps the path its own
// repository reported, prefixed with the repository's place under the
// workspace, so two repositories holding the same file name stay apart and the
// symbol and call layers keep their keys.
func (r renderer) draw(patches []string, summary bool) string {
	return diffview.Render(r.diffOptions(patches, summary))
}

func (r renderer) diffOptions(patches []string, summary bool) diffview.Options {
	opts := diffview.Options{
		Width:   r.columns(),
		Symbols: map[string][]diffview.Symbol{},
		Edges:   map[string][]diffview.Edge{},
		Pins:    map[string]bool{},
		Stat:    r.stat,
		Summary: summary,
		Unified: r.engineOptions.Layout == "unified",
	}
	for i, patch := range patches {
		files, rawPaths := normalizeDiffPaths(diffview.Parse(patch))
		if len(files) == 0 {
			continue
		}
		spec := r.specs[i]
		under := r.prefix(spec.Dir)
		if under != "" {
			opts.Pins[strings.TrimSuffix(under, "/")] = true
		}
		touched := make([]string, 0, len(files))
		for _, file := range files {
			touched = append(touched, rawPaths[file.Path])
		}
		syms, edges := r.layers(spec, touched, patches[i])
		for j := range files {
			rawPath := rawPaths[files[j].Path]
			at := under + files[j].Path
			opts.Symbols[at] = syms[rawPath]
			opts.Edges[at] = edges[rawPath]
			files[j].Path = at
			opts.Files = append(opts.Files, files[j])
		}
	}
	return opts
}

var (
	contextTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	contextPath  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	contextKind  = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8"))
	contextAdd   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	contextDel   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// semanticContext adds only provider-derived context above an inline Git
// patch. The patch already names every file and its churn, so repeating the
// complete file tree there adds noise without adding information.
func (r renderer) semanticContext(patches []string) string {
	return semanticRows(r.diffOptions(patches, true))
}

func semanticRows(opts diffview.Options) string {
	rows := []string{}
	for index := range opts.Files {
		file := &opts.Files[index]
		symbols := opts.Symbols[file.Path]
		edges := opts.Edges[file.Path]
		for _, symbol := range symbols {
			add, del, up, down, changed := semanticCounts(file, symbol, edges)
			if !changed {
				continue
			}
			label := contextPath.Render(fmt.Sprintf("%s:%d", file.Path, symbol.From)) + " · " +
				contextKind.Render(symbol.Kind+" ") + symbol.Name
			rows = append(rows, label+semanticChurn(add, del, up, down))
		}
		unmatched := unmatchedEdges(symbols, edges)
		if len(unmatched) > 0 {
			up, down := edgeCounts(unmatched)
			rows = append(rows, contextPath.Render(file.Path)+" · file call changes"+semanticChurn(0, 0, up, down))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return contextTitle.Render("context") + "\n" + strings.Join(rows, "\n")
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
		displayPath := files[index].Path
		rawPath := displayPath
		if strings.HasPrefix(displayPath, `"`) && strings.HasSuffix(displayPath, `"`) {
			if decoded, err := strconv.Unquote(displayPath); err == nil {
				rawPath = strings.TrimPrefix(decoded, "b/")
				displayPath = strconv.Quote(rawPath)
			}
		}
		files[index].Path = displayPath
		rawPaths[displayPath] = rawPath
	}
	return files, rawPaths
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

func (r renderer) layers(spec source.Spec, touched []string, patch string) (map[string][]diffview.Symbol, map[string][]diffview.Edge) {
	touched = append([]string(nil), touched...)
	sort.Strings(touched)

	syms := map[string][]diffview.Symbol{}
	edges := map[string][]diffview.Edge{}
	request := provider.Request{
		Directory:   spec.Dir,
		Files:       touched,
		Fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(patch))),
		From:        spec.From,
		Staged:      spec.Staged,
		To:          spec.To,
	}
	for _, configured := range r.providers {
		if r.syms && provider.Supports(configured, provider.ActionSymbols) {
			ctx, cancel := context.WithTimeout(context.Background(), r.budget)
			response, err := provider.Run(ctx, configured, provider.ActionSymbols, request)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "changes: %v\n", err)
			} else {
				mergeSymbols(syms, response.Symbols)
			}
		}
		if r.calls && provider.Supports(configured, provider.ActionCalls) {
			ctx, cancel := context.WithTimeout(context.Background(), r.budget)
			response, err := provider.Run(ctx, configured, provider.ActionCalls, request)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "changes: %v\n", err)
			} else {
				mergeEdges(edges, response.Edges)
			}
		}
	}
	return syms, edges
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

// follow reprints on change rather than on a timer, because a diff that has not
// moved redraws to the same bytes and the flicker says nothing. The raw patches
// are the change detector: they are what every layer is derived from.
func (r renderer) follow(every time.Duration) error {
	last := ""
	for {
		patches, err := r.patches()
		if err != nil {
			return err
		}
		now := strings.Join(patches, "\x00")
		if now != last {
			last = now
			out, err := r.renderPatches(patches)
			if err != nil {
				return err
			}
			if out == "" {
				out = "changes: nothing changed"
			}
			// Home the cursor and clear forward, so the frame lands in the
			// scrollback the reader already has rather than in an alt screen
			// they cannot scroll.
			fmt.Print("\x1b[H\x1b[2J", out, "\n")
		}
		time.Sleep(every)
	}
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
	patch, err := io.ReadAll(os.Stdin)
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
	if len(args) != 1 {
		fail(errors.New("__values requires one value set"))
	}
	var values []string
	switch args[0] {
	case "repository":
		values = append(gitLineCompletionValues("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes", "refs/tags"), "HEAD")
		values = append(values, gitCompletionValues("ls-files", "-z", "--cached", "--others", "--exclude-standard")...)
	case "paths":
		values = gitCompletionValues("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	case "providers":
		configured, err := appconfig.Load("")
		if err != nil {
			return
		}
		discovery, err := provider.Discover(configured.Providers.Directory)
		if err != nil {
			return
		}
		for _, candidate := range discovery.Providers {
			values = append(values, candidate.Manifest.Name)
		}
	case "shells":
		values = []string{"bash", "zsh", "fish", "nu"}
	default:
		fail(fmt.Errorf("unknown completion value set %q", args[0]))
	}
	writeCompletionValues(os.Stdout, values)
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
		Synopsis:          "Render Git changes with symbol and call analysis",
		CompletionCommand: completionValuesInvocation("repository"),
		LongDescription: `Refs follow git diff: none is HEAD against the working tree, one is that ref
against the working tree, and two compare the trees. A from of the form a..b is
split into two refs.

-r reads every repository under the workspace. The workspace is
$SYSINIT_WORKSPACE when the working directory sits inside it, then the Git top
level, then the working directory. Each repository's files hang under its own
name.`,
		Flags: []completion.Flag{
			{Name: "budget", Description: "Analysis time budget", Value: true},
			{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
			{Name: "config", Description: "YAML configuration file", Value: true},
			{Name: "engine", Description: "Patch display engine", Value: true, Values: engine.PatchNames},
			{Name: "filter", Description: "Standard-input patch filter", Value: true},
			{Name: "interval", Description: "Watch interval", Value: true},
			{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
			{Name: "no-calls", Description: "Skip call analysis"},
			{Name: "no-symbols", Description: "Skip symbol analysis"},
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
				Name:     "provider",
				Synopsis: "Inspect and validate analysis providers",
				Subcommands: []completion.Command{
					{
						Name:              "list",
						Synopsis:          "List configured analysis providers",
						Flags:             providerCommandFlags(),
						CompletionCommand: completionValuesInvocation("providers"),
					},
					{
						Name:              "validate",
						Synopsis:          "Validate provider commands and JSON behavior",
						Flags:             providerCommandFlags(),
						CompletionCommand: completionValuesInvocation("providers"),
					},
				},
			},
		},
	}
}

func completionValuesInvocation(kind string) []string {
	return []string{"changes", "__values", kind}
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
	readme, err := os.ReadFile("README.md")
	if err != nil {
		fail(fmt.Errorf("read README.md: %w", err))
	}
	generated, err := completion.ReplaceSection(string(readme), "cli", completion.Markdown(commandMetadata()))
	if err != nil {
		fail(err)
	}
	outputs := map[string][]byte{
		"README.md":                   []byte(generated),
		"schema/changes.schema.json":  schema,
		"schema/provider.schema.json": providerSchema,
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
