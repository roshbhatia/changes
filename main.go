// changes prints a diff the way a reviewer reads one: an eza shaped tree of
// the touched files, each file's hunks grouped under the outline symbol that
// owns them, and each symbol annotated with the call edges the edit added or
// removed. It is the same renderer traces draws in its inspector, over git
// instead of over a trace.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/completion"
	"github.com/roshbhatia/go-utils/diffview"
	"github.com/roshbhatia/go-utils/workspace"
)

const usage = `changes [flags] [<from> [<to>]] [-- <path>...]

  Refs follow git diff: none is HEAD against the working tree, one is that ref
  against the working tree, two compare the trees. A from of the form a..b is
  split into two refs.

  -r reads every repository under the workspace, which is $SYSINIT_WORKSPACE
  when the working directory sits inside it, then the git top level, then the
  working directory. Each repository's files hang under its own name.

Flags:
`

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "completion" {
		generateCompletion(os.Args[2:])
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
	staged := flag.Bool("staged", false, "compare the index rather than the working tree")
	since := flag.String("since", "", "compare against the tree as of a time or a revision, e.g. \"2 hours ago\" or HEAD~3")
	watch := flag.Bool("watch", false, "reprint whenever the diff changes")
	flag.BoolVar(watch, "w", false, "shorthand for -watch")
	every := flag.Duration("interval", 700*time.Millisecond, "how often -watch re-reads the diff")
	width := flag.Int("width", 0, "render at this width (default: the terminal's)")
	noCalls := flag.Bool("no-calls", false, "skip calldiff, which is the slow layer")
	noSyms := flag.Bool("no-symbols", false, "skip the ast-grep outline layer")
	budget := flag.Duration("budget", 20*time.Second, "how long the outline and call layers may take")
	recurse := flag.Bool("recursive", false, "read every repository under the workspace")
	flag.BoolVar(recurse, "r", false, "shorthand for -recursive")
	scan := flag.String("root", "", "scan from here with -r, instead of the workspace")
	stat := flag.Bool("stat", false, "draw the tree and the churn, without the hunks")
	flag.BoolVar(stat, "s", false, "shorthand for -stat")
	color := flag.String("color", "auto", "color output: auto, always, or never")
	diffEngine := flag.String("engine", envOr("CHANGES_DIFF_ENGINE", "git"), "display engine: git, delta, difftastic, diff-so-fancy, internal, or command")
	engineCommand := flag.String("engine-command", os.Getenv("CHANGES_DIFF_COMMAND"), "executable used by the command display engine")
	layout := flag.String("layout", envOr("CHANGES_DIFF_LAYOUT", "unified"), "diff layout: unified or side-by-side")
	flag.Usage = func() {
		_, _ = fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	resolvedColor := resolveColor(*color)
	switch resolvedColor {
	case "auto":
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	default:
		fail(fmt.Errorf("-color must be auto, always, or never"))
	}
	engineOptions := engine.Options{
		Color:   resolvedColor,
		Command: *engineCommand,
		Layout:  *layout,
		Width:   columns(*width),
	}
	if err := engine.Validate(*diffEngine, engineOptions); err != nil {
		fail(err)
	}

	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if *scan != "" {
		dir = *scan
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
	from, to, paths := split(root, flag.Args())
	if *since != "" {
		if from != "" {
			fail(fmt.Errorf("-since and a revision argument both name the left side"))
		}
		from = source.Revision(root, *since)
		if from == "" {
			fail(fmt.Errorf("-since %q resolves to no commit before HEAD: it is not a revision, and git read it as now", *since))
		}
	}
	// git resolves a pathspec against the process cwd, and every command here
	// runs at the repository root, so a relative path from a subdirectory would
	// silently match nothing.
	for i, p := range paths {
		if abs, err := filepath.Abs(filepath.Join(dir, p)); err == nil {
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

// split reads the git shaped positional arguments. flag.Parse consumes the --
// separator, so the refs and the paths arrive in one list and the first
// argument git does not know as a tree starts the paths.
func split(root string, args []string) (from, to string, paths []string) {
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
			if _, err := os.Stat(a); err != nil {
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
}

func (r renderer) render() (string, error) {
	patches, err := r.patches()
	if err != nil {
		return "", err
	}
	return r.renderPatches(patches)
}

func (r renderer) renderPatches(patches []string) (string, error) {
	if r.stat || r.engine == "internal" {
		return r.draw(patches, false), nil
	}
	body, err := r.display(patches)
	if err != nil {
		return "", err
	}
	summary := r.draw(patches, true)
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
	if r.engine == "git" || r.engine == "difftastic" {
		for _, spec := range r.specs {
			var output string
			var err error
			if r.engine == "git" {
				output, err = spec.DisplayDiff(r.color)
			} else {
				output, err = spec.Difftastic(r.engineOptions.Layout, r.color)
			}
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
	opts := diffview.Options{
		Width:   r.columns(),
		Symbols: map[string][]diffview.Symbol{},
		Edges:   map[string][]diffview.Edge{},
		Pins:    map[string]bool{},
		Stat:    r.stat,
		Summary: summary,
	}
	for i, patch := range patches {
		files := diffview.Parse(patch)
		if len(files) == 0 {
			continue
		}
		spec := r.specs[i]
		under := r.prefix(spec.Dir)
		if under != "" {
			opts.Pins[strings.TrimSuffix(under, "/")] = true
		}
		syms, edges := r.layers(spec, files)
		for j := range files {
			at := under + files[j].Path
			opts.Symbols[at] = syms[files[j].Path]
			opts.Edges[at] = edges[files[j].Path]
			files[j].Path = at
			opts.Files = append(opts.Files, files[j])
		}
	}
	return diffview.Render(opts)
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

func (r renderer) layers(spec source.Spec, files []diffview.File) (map[string][]diffview.Symbol, map[string][]diffview.Edge) {
	touched := make([]string, 0, len(files))
	for _, f := range files {
		touched = append(touched, f.Path)
	}
	sort.Strings(touched)

	syms := map[string][]diffview.Symbol{}
	if r.syms {
		dir, kept, done := spec.Tree(touched)
		defer done()
		syms = source.Outline(dir, kept, r.budget)
	}
	edges := map[string][]diffview.Edge{}
	if r.calls {
		edges = source.Calls(spec.Dir, spec.From, spec.To, touched, r.budget)
	}
	return syms, edges
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

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func runDifftool(args []string) {
	flags := flag.NewFlagSet("changes difftool", flag.ContinueOnError)
	diffEngine := flags.String("engine", envOr("CHANGES_DIFF_ENGINE", "git"), "display engine")
	engineCommand := flags.String("engine-command", os.Getenv("CHANGES_DIFF_COMMAND"), "command display executable")
	layout := flags.String("layout", envOr("CHANGES_DIFF_LAYOUT", "unified"), "unified or side-by-side")
	color := flags.String("color", "auto", "auto, always, or never")
	width := flags.Int("width", 0, "render width")
	if err := flags.Parse(args); err != nil {
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
		Color:   resolveColor(*color),
		Command: *engineCommand,
		Label:   label,
		Layout:  *layout,
		Width:   columns(*width),
	}
	if err := engine.Validate(*diffEngine, options); err != nil {
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
	flags := flag.NewFlagSet("changes render", flag.ContinueOnError)
	diffEngine := flags.String("engine", envOr("CHANGES_DIFF_ENGINE", "git"), "display engine")
	engineCommand := flags.String("engine-command", os.Getenv("CHANGES_DIFF_COMMAND"), "command display executable")
	layout := flags.String("layout", envOr("CHANGES_DIFF_LAYOUT", "unified"), "unified or side-by-side")
	color := flags.String("color", envOr("CHANGES_DIFF_COLOR", "auto"), "auto, always, or never")
	width := flags.Int("width", envInt("CHANGES_DIFF_WIDTH"), "render width")
	if err := flags.Parse(args); err != nil {
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
		Color:   resolveColor(*color),
		Command: *engineCommand,
		Layout:  *layout,
		Width:   columns(*width),
	}
	if err := engine.Validate(*diffEngine, options); err != nil {
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

func generateCompletion(args []string) {
	if len(args) != 1 {
		fail(fmt.Errorf("completion requires bash, zsh, fish, or nu"))
	}
	out, err := completion.Generate(args[0], completion.Command{
		Name:        "changes",
		Description: "Render Git changes with symbol and call analysis",
		Flags: []completion.Flag{
			{Name: "budget", Description: "Analysis time budget", Value: true},
			{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
			{Name: "engine", Description: "Diff display engine", Value: true, Values: engine.Names},
			{Name: "engine-command", Description: "Command display executable", Value: true},
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
		},
		Subcommands: []completion.Command{
			{
				Name:        "difftool",
				Description: "Compare Git difftool LOCAL and REMOTE files",
				Flags: []completion.Flag{
					{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
					{Name: "engine", Description: "Diff display engine", Value: true, Values: engine.Names},
					{Name: "engine-command", Description: "Command display executable", Value: true},
					{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
					{Name: "width", Description: "Render width", Value: true},
				},
			},
			{
				Name:        "render",
				Description: "Render a patch from standard input",
				Flags: []completion.Flag{
					{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
					{Name: "engine", Description: "Diff display engine", Value: true, Values: engine.Names},
					{Name: "engine-command", Description: "Command display executable", Value: true},
					{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
					{Name: "width", Description: "Render width", Value: true},
				},
			},
		},
	})
	if err != nil {
		fail(err)
	}
	fmt.Println(out)
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
