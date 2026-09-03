// changes prints a diff the way a reviewer reads one: an eza shaped tree of
// the touched files, each file's hunks grouped under the outline symbol that
// owns them, and each symbol annotated with the call edges the edit added or
// removed. It is the same renderer traces draws in its inspector, over git
// instead of over a trace.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/engine"
	"github.com/roshbhatia/changes/internal/provider"
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
	staged := flag.Bool("staged", false, "compare the index rather than the working tree")
	flag.String("config", configPath, "configuration file (default: ~/.config/changes/config.yaml)")
	since := flag.String("since", "", "compare against the tree as of a time or a revision, e.g. \"2 hours ago\" or HEAD~3")
	watch := flag.Bool("watch", false, "reprint whenever the diff changes")
	flag.BoolVar(watch, "w", false, "shorthand for -watch")
	every := flag.Duration("interval", 700*time.Millisecond, "how often -watch re-reads the diff")
	width := flag.Int("width", 0, "render at this width (default: the terminal's)")
	noCalls := flag.Bool("no-calls", false, "skip call-edge analysis")
	noSyms := flag.Bool("no-symbols", false, "skip symbol analysis")
	budget := flag.Duration("budget", time.Duration(configured.Providers.Timeout), "how long each analysis provider may take")
	recurse := flag.Bool("recursive", false, "read every repository under the workspace")
	flag.BoolVar(recurse, "r", false, "shorthand for -recursive")
	scan := flag.String("root", "", "scan from here with -r, instead of the workspace")
	stat := flag.Bool("stat", false, "draw the tree and the churn, without the hunks")
	flag.BoolVar(stat, "s", false, "shorthand for -stat")
	color := flag.String("color", configured.Color, "color output: auto, always, or never")
	diffEngine := flag.String("engine", configured.Diff.Engine, "display engine: git, internal, or command")
	engineCommand := flag.String("engine-command", "", "override the configured patch command")
	layout := flag.String("layout", configured.Diff.Layout, "diff layout: unified or side-by-side")
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
		Command: commandValue(configured.Diff.Command, *engineCommand),
		Layout:  *layout,
		Width:   columns(*width),
	}
	if err := engine.Validate(*diffEngine, engineOptions); err != nil {
		fail(err)
	}
	loadedProviders, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	providers := make([]provider.Manifest, 0, len(loadedProviders))
	for _, loaded := range loadedProviders {
		providers = append(providers, loaded.Manifest)
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
	if r.engine == "git" {
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
		files := diffview.Parse(patch)
		if len(files) == 0 {
			continue
		}
		spec := r.specs[i]
		under := r.prefix(spec.Dir)
		if under != "" {
			opts.Pins[strings.TrimSuffix(under, "/")] = true
		}
		syms, edges := r.layers(spec, files, patches[i])
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

func (r renderer) layers(spec source.Spec, files []diffview.File, patch string) (map[string][]diffview.Symbol, map[string][]diffview.Edge) {
	touched := make([]string, 0, len(files))
	for _, f := range files {
		touched = append(touched, f.Path)
	}
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

func runDifftool(args []string) {
	configured, err := appconfig.Load(argumentValue(args, "config"))
	if err != nil {
		fail(err)
	}
	flags := flag.NewFlagSet("changes difftool", flag.ContinueOnError)
	flags.String("config", argumentValue(args, "config"), "configuration file")
	diffEngine := flags.String("engine", configured.Diff.Engine, "display engine")
	engineCommand := flags.String("engine-command", "", "override the configured command executable")
	layout := flags.String("layout", configured.Diff.Layout, "unified or side-by-side")
	color := flags.String("color", configured.Color, "auto, always, or never")
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
		Command: commandValue(configured.Diff.Command, *engineCommand),
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
	configured, err := appconfig.Load(argumentValue(args, "config"))
	if err != nil {
		fail(err)
	}
	flags := flag.NewFlagSet("changes render", flag.ContinueOnError)
	flags.String("config", argumentValue(args, "config"), "configuration file")
	diffEngine := flags.String("engine", configured.Diff.Engine, "display engine")
	engineCommand := flags.String("engine-command", "", "override the configured command executable")
	layout := flags.String("layout", configured.Diff.Layout, "unified or side-by-side")
	color := flags.String("color", configured.Color, "auto, always, or never")
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
		Command: commandValue(configured.Diff.Command, *engineCommand),
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
	out, err := completion.Generate(args[0], commandMetadata())
	if err != nil {
		fail(err)
	}
	fmt.Println(out)
}

func commandMetadata() completion.Command {
	return completion.Command{
		Name:        "changes",
		Description: "Render Git changes with symbol and call analysis",
		Flags: []completion.Flag{
			{Name: "budget", Description: "Analysis time budget", Value: true},
			{Name: "color", Description: "Color output", Value: true, Values: []string{"auto", "always", "never"}},
			{Name: "config", Description: "YAML configuration file", Value: true},
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
					{Name: "config", Description: "YAML configuration file", Value: true},
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
					{Name: "config", Description: "YAML configuration file", Value: true},
					{Name: "engine", Description: "Diff display engine", Value: true, Values: engine.Names},
					{Name: "engine-command", Description: "Command display executable", Value: true},
					{Name: "layout", Description: "Diff layout", Value: true, Values: []string{"unified", "side-by-side"}},
					{Name: "width", Description: "Render width", Value: true},
				},
			},
			{
				Name:        "generate",
				Description: "Generate README command docs and JSON Schema",
				Flags: []completion.Flag{
					{Name: "check", Description: "Fail when generated files are stale"},
				},
			},
			{
				Name:        "provider",
				Description: "Inspect and validate analysis providers",
				Subcommands: []completion.Command{
					{Name: "list", Description: "List configured analysis providers", Flags: []completion.Flag{{Name: "config", Description: "YAML configuration file", Value: true}, {Name: "json", Description: "Print JSON"}}},
					{Name: "validate", Description: "Validate provider commands and JSON behavior", Flags: []completion.Flag{{Name: "config", Description: "YAML configuration file", Value: true}, {Name: "json", Description: "Print JSON"}}},
				},
			},
		},
	}
}

func runProvider(args []string) {
	if len(args) == 0 || (args[0] != "list" && args[0] != "validate") {
		fail(fmt.Errorf("provider requires list or validate"))
	}
	action := args[0]
	flags := flag.NewFlagSet("changes provider "+action, flag.ContinueOnError)
	configPath := flags.String("config", "", "YAML configuration file")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		fail(err)
	}
	if flags.NArg() > 1 {
		fail(fmt.Errorf("provider %s accepts at most one provider name", action))
	}
	configured, err := appconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	providers, err := provider.Discover(configured.Providers.Directory)
	if err != nil {
		fail(err)
	}
	if flags.NArg() == 1 {
		name := flags.Arg(0)
		selected := []provider.LoadedManifest{}
		for _, manifest := range providers {
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
	flags := flag.NewFlagSet("changes generate", flag.ContinueOnError)
	check := flags.Bool("check", false, "fail when generated files are stale")
	if err := flags.Parse(args); err != nil {
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
