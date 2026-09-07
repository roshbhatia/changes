package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/roshbhatia/changes/internal/appconfig"
	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/workspaceview"
)

type workspaceLoaded struct {
	snapshot workspaceview.Snapshot
	err      error
}

type noteWritten struct {
	output string
	err    error
}

type workspaceRefreshTick time.Time

type navItem struct {
	label string
	path  string
	oid   string
}

type interactiveModel struct {
	root        string
	options     workspaceOptions
	configured  appconfig.Config
	store       workspaceview.Store
	snapshot    workspaceview.Snapshot
	viewport    viewport.Model
	spinner     spinner.Model
	note        textarea.Model
	width       int
	height      int
	focus       string
	mode        string
	command     string
	commands    []string
	message     string
	tab         string
	restorePath string
	restoreOID  string
	selected    int
	line        int
	loading     bool
	showHelp    bool
}

var (
	interactiveAccent     = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	interactiveMuted      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	interactiveActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	interactiveError      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	interactivePlain      = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	interactiveRule       = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8"))
	interactiveExecutable = os.Executable
	interactiveCommand    = exec.Command
)

func runInteractive(args []string) {
	// Pin ANSI slots because Bubble Tea v1 can parse terminal color replies as input.
	lipgloss.SetColorProfile(termenv.ANSI)
	lipgloss.SetHasDarkBackground(true)

	options, configured, err := parseWorkspaceOptions(args, true)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fail(errors.New("interactive mode requires a terminal; use changes workspace for external clients"))
	}
	root, err := currentRepositoryRoot()
	if err != nil {
		fail(err)
	}
	store, err := workspaceview.DefaultStore(configured.Interactive.CacheMaxEntries)
	if err != nil {
		fail(err)
	}
	state, found, stateErr := store.LoadState(root)
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "changes: load workspace state: %v\n", stateErr)
	}
	if found {
		if !argumentPresent(args, "layout") {
			options.layout = state.Layout
		}
		if !argumentPresent(args, "view") {
			options.view = state.View
			if state.SelectedOID != "" {
				options.commit = state.SelectedOID
			}
		}
	}
	model := newInteractiveModel(root, options, configured, store)
	if found {
		model.restoreState(state)
	}
	if !options.refresh {
		cached, cachedFound, cacheErr := store.LoadSnapshot(root, workspaceSlot(options))
		if cacheErr != nil {
			model.message = "cache: " + cacheErr.Error()
		} else if cachedFound && snapshotWithinTTL(cached, configured.Interactive.CacheTTL.Duration()) {
			cached.Freshness.State = "refreshing"
			model.setSnapshot(cached)
		}
	}
	if width, height, sizeErr := term.GetSize(int(os.Stdout.Fd())); sizeErr == nil {
		model.width, model.height = width, height
		model.resize()
		model.options.width = model.diffWidth()
	}
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	result, err := program.Run()
	if err != nil {
		fail(err)
	}
	if final, ok := result.(interactiveModel); ok {
		if err := final.persistState(); err != nil {
			fmt.Fprintf(os.Stderr, "changes: save workspace state: %v\n", err)
		}
	}
}

func argumentPresent(args []string, name string) bool {
	for _, argument := range args {
		if argument == "--"+name || argument == "-"+name || strings.HasPrefix(argument, "--"+name+"=") || strings.HasPrefix(argument, "-"+name+"=") {
			return true
		}
	}
	return false
}

func newInteractiveModel(root string, options workspaceOptions, configured appconfig.Config, store workspaceview.Store) interactiveModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	input := textarea.New()
	input.Placeholder = "Summary on the first line; optional rationale below"
	input.CharLimit = maxNoteMessageBytes
	input.SetWidth(72)
	input.SetHeight(6)
	return interactiveModel{
		root: root, options: options, configured: configured, store: store,
		viewport: viewport.New(80, 20), spinner: spin, note: input,
		focus: "main", mode: "normal", tab: "files", loading: true,
	}
}

func (model interactiveModel) Init() tea.Cmd {
	return tea.Batch(model.spinner.Tick, model.refreshCommand(), model.refreshTick())
}

func (model interactiveModel) refreshTick() tea.Cmd {
	interval := model.configured.Notes.RefreshInterval.Duration()
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(at time.Time) tea.Msg { return workspaceRefreshTick(at) })
}

func (model interactiveModel) refreshCommand() tea.Cmd {
	root, options, configured := model.root, model.options, model.configured
	previous := []provider.Note{}
	if sameWorkspaceComparison(model.snapshot, options) {
		previous = append(previous, model.snapshot.Notes...)
	}
	return func() tea.Msg {
		snapshot, err := buildWorkspaceSnapshotWithNotes(root, options, configured, previous)
		return workspaceLoaded{snapshot: snapshot, err: err}
	}
}

func sameWorkspaceComparison(snapshot workspaceview.Snapshot, options workspaceOptions) bool {
	if snapshot.Comparison.Kind != options.view {
		return false
	}
	return options.view != "commit" || snapshot.Comparison.To == options.commit
}

func (model interactiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch value := message.(type) {
	case tea.WindowSizeMsg:
		previousWidth := model.diffWidth()
		model.width, model.height = value.Width, value.Height
		model.resize()
		if width := model.diffWidth(); width > 0 && width != previousWidth {
			model.options.width = width
			model.loading = true
			return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
		}
		return model, nil
	case spinner.TickMsg:
		if !model.loading {
			return model, nil
		}
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(value)
		return model, command
	case workspaceLoaded:
		model.loading = false
		if value.err != nil {
			model.message = value.err.Error()
			return model, nil
		}
		model.message = ""
		model.setSnapshot(value.snapshot)
		if err := model.store.SaveSnapshot(workspaceSlot(model.options), value.snapshot); err != nil {
			model.message = "cache: " + err.Error()
		}
		return model, nil
	case workspaceRefreshTick:
		command := model.refreshTick()
		if model.loading || model.mode != "normal" {
			return model, command
		}
		model.loading = true
		return model, tea.Batch(command, model.spinner.Tick, model.refreshCommand())
	case noteWritten:
		model.loading = false
		if value.err != nil {
			model.message = value.err.Error()
			return model, nil
		}
		model.message = strings.TrimSpace(value.output)
		model.options.refresh = true
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case tea.KeyMsg:
		return model.handleKey(value)
	case tea.MouseMsg:
		return model.handleMouse(value)
	}
	return model, nil
}

func (model interactiveModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isInteractiveTerminalReply(key.String()) {
		return model, nil
	}
	if key.String() == "ctrl+c" {
		_ = model.persistState()
		return model, tea.Quit
	}
	if model.showHelp {
		if key.String() == "?" || key.String() == "esc" {
			model.showHelp = false
		}
		return model, nil
	}
	if model.mode == "note" {
		if key.String() == "esc" {
			model.mode = "normal"
			model.note.Blur()
			return model, nil
		}
		if key.String() == "ctrl+s" {
			message := strings.TrimSpace(model.note.Value())
			if message == "" {
				model.message = "note message is empty"
				return model, nil
			}
			model.mode = "normal"
			model.note.Blur()
			model.loading = true
			return model, tea.Batch(model.spinner.Tick, model.noteCommand(message, false))
		}
		var command tea.Cmd
		model.note, command = model.note.Update(key)
		return model, command
	}
	if model.mode == "command" {
		switch key.String() {
		case "esc":
			model.mode, model.command, model.message = "normal", "", ""
			return model, nil
		case "enter":
			command := model.command
			model.mode, model.command = "normal", ""
			if strings.TrimSpace(command) != "" {
				model.commands = append(model.commands, command)
				if len(model.commands) > 20 {
					model.commands = model.commands[len(model.commands)-20:]
				}
			}
			return model.runPaletteCommand(command)
		case "up":
			if len(model.commands) > 0 {
				model.command = model.commands[len(model.commands)-1]
			}
			return model, nil
		case "tab":
			model.command, model.message = completeInteractiveCommand(model.command)
			return model, nil
		case "backspace":
			_, size := utf8.DecodeLastRuneInString(model.command)
			if size > 0 {
				model.command = model.command[:len(model.command)-size]
			}
			model.message = ""
			return model, nil
		case " ":
			model.command += " "
			model.message = ""
			return model, nil
		default:
			if key.Type == tea.KeyRunes {
				model.command += string(key.Runes)
				model.message = ""
			}
			return model, nil
		}
	}
	switch key.String() {
	case "q":
		_ = model.persistState()
		return model, tea.Quit
	case "?":
		model.showHelp = !model.showHelp
	case ":":
		model.mode, model.message = "command", ""
	case "tab":
		if model.focus == "main" {
			model.focus = "navigator"
		} else {
			model.focus = "main"
		}
	case "ctrl+h", "ctrl+j":
		model.focus = "navigator"
	case "ctrl+l", "ctrl+k":
		model.focus = "main"
	case "f":
		model.toggleTab()
	case "t":
		if model.configured.Interactive.Navigator == "tree" {
			model.configured.Interactive.Navigator = "list"
		} else {
			model.configured.Interactive.Navigator = "tree"
		}
	case "d":
		if model.configured.Interactive.Dock == "left" {
			model.configured.Interactive.Dock = "bottom"
		} else {
			model.configured.Interactive.Dock = "left"
		}
		model.resize()
		model.options.width = model.diffWidth()
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case "s":
		if model.options.layout == "unified" {
			model.options.layout = "side-by-side"
		} else {
			model.options.layout = "unified"
		}
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case "v":
		model.cycleView()
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case "r":
		model.options.refresh = true
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case "n":
		if model.selectedFile() == "" {
			model.message = "select a changed file before adding a note"
			return model, nil
		}
		model.mode = "note"
		model.note.Reset()
		model.note.Focus()
		return model, textarea.Blink
	case "a":
		if model.configured.Interactive.NoteInput == "editor" {
			if model.selectedFile() == "" {
				model.message = "select a changed file before adding a note"
				return model, nil
			}
			model.loading = true
			return model, tea.Batch(model.spinner.Tick, model.noteCommand("", true))
		}
		if model.selectedFile() == "" {
			model.message = "select a changed file before adding a note"
			return model, nil
		}
		model.mode = "note"
		model.note.Reset()
		model.note.Focus()
		return model, textarea.Blink
	case "e":
		if model.selectedFile() == "" {
			model.message = "select a changed file before adding a note"
			return model, nil
		}
		model.loading = true
		return model, tea.Batch(model.spinner.Tick, model.noteCommand("", true))
	case "up", "k":
		if model.focus == "navigator" {
			model.moveSelection(-1)
			return model, nil
		}
	case "down", "j":
		if model.focus == "navigator" {
			model.moveSelection(1)
			return model, nil
		}
	case "enter":
		if model.focus == "navigator" {
			return model.activateSelection()
		}
	case "[":
		model.moveLine(-1)
	case "]":
		model.moveLine(1)
	}
	var command tea.Cmd
	model.viewport, command = model.viewport.Update(key)
	return model, command
}

func (model interactiveModel) handleMouse(mouse tea.MouseMsg) (tea.Model, tea.Cmd) {
	if model.showHelp || model.mode != "normal" || mouse.Y < 1 || mouse.Y >= model.height-1 {
		return model, nil
	}
	inNavigator := false
	navigatorRow := -1
	if model.effectiveDock() == "left" {
		inNavigator = mouse.X < model.navigatorWidth()
		navigatorRow = mouse.Y - 2
	} else {
		mainHeight := model.bodyHeight() - model.navigatorHeight()
		inNavigator = mouse.Y >= 1+mainHeight
		navigatorRow = mouse.Y - (1 + mainHeight) - 1
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		if inNavigator {
			model.focus = "navigator"
			if navigatorRow > 0 {
				start, end := model.navigatorWindow()
				index := start + navigatorRow - 1
				if index >= start && index < end {
					model.selected = index
					model.line = 0
				}
			}
		} else {
			model.focus = "main"
		}
		return model, nil
	}
	if mouse.Button != tea.MouseButtonWheelUp && mouse.Button != tea.MouseButtonWheelDown {
		return model, nil
	}
	if inNavigator {
		model.focus = "navigator"
		delta := 1
		if mouse.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		model.moveSelection(delta)
		return model, nil
	}
	model.focus = "main"
	var command tea.Cmd
	model.viewport, command = model.viewport.Update(mouse)
	return model, command
}

func (model interactiveModel) runPaletteCommand(command string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return model, nil
	}
	commands := interactivePaletteCommands()
	exact := slices.Contains(commands, fields[0]) || fields[0] == "q"
	if !exact {
		matches := []string{}
		for _, candidate := range commands {
			if strings.HasPrefix(candidate, fields[0]) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) > 1 {
			model.message = "ambiguous command " + fields[0] + ": " + strings.Join(matches, ", ")
			return model, nil
		}
		if len(matches) == 1 {
			fields[0] = matches[0]
		}
	}
	switch fields[0] {
	case "quit", "q":
		_ = model.persistState()
		return model, tea.Quit
	case "refresh":
		model.options.refresh, model.loading = true, true
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	case "dock":
		if len(fields) == 2 && (fields[1] == "left" || fields[1] == "bottom") {
			model.configured.Interactive.Dock = fields[1]
			model.resize()
			model.options.width, model.loading = model.diffWidth(), true
			return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
		}
	case "layout":
		if len(fields) == 2 && (fields[1] == "unified" || fields[1] == "side-by-side") {
			model.options.layout, model.loading = fields[1], true
			return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
		}
	case "navigator":
		if len(fields) == 2 && (fields[1] == "tree" || fields[1] == "list") {
			model.configured.Interactive.Navigator = fields[1]
			return model, nil
		}
	case "tab":
		if len(fields) == 2 && (fields[1] == "files" || fields[1] == "history") {
			model.tab = fields[1]
			model.selected = 0
			model.line = 0
			return model, nil
		}
	case "view":
		if len(fields) >= 2 && (fields[1] == "working" || fields[1] == "staged" || fields[1] == "commit") {
			model.options.view = fields[1]
			if fields[1] == "commit" && len(fields) == 3 {
				model.options.commit = fields[2]
			}
			model.loading = true
			return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
		}
	case "note":
		if len(fields) == 2 && fields[1] == "popup" {
			model.mode = "note"
			model.note.Reset()
			model.note.Focus()
			return model, textarea.Blink
		}
		if len(fields) == 2 && fields[1] == "editor" {
			model.loading = true
			return model, tea.Batch(model.spinner.Tick, model.noteCommand("", true))
		}
	}
	model.message = "unknown command: " + command
	return model, nil
}

func completeInteractiveCommand(command string) (string, string) {
	trailingSpace := strings.HasSuffix(command, " ")
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command, strings.Join(interactivePaletteCommands(), "  ")
	}
	if len(fields) == 1 && !trailingSpace {
		matches := matchingInteractiveValues(interactivePaletteCommands(), fields[0])
		if len(matches) == 1 {
			return matches[0] + " ", ""
		}
		return command, strings.Join(matches, "  ")
	}
	values := interactivePaletteValues[fields[0]]
	if len(values) == 0 || len(fields) > 2 || len(fields) == 2 && trailingSpace {
		return command, ""
	}
	prefix := ""
	if len(fields) == 2 {
		prefix = fields[1]
	}
	matches := matchingInteractiveValues(values, prefix)
	if len(matches) == 1 {
		return fields[0] + " " + matches[0], ""
	}
	return command, strings.Join(matches, "  ")
}

func matchingInteractiveValues(values []string, prefix string) []string {
	matches := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			matches = append(matches, value)
		}
	}
	return matches
}

func (model interactiveModel) noteCommand(message string, editor bool) tea.Cmd {
	executable, err := interactiveExecutable()
	if err != nil {
		return func() tea.Msg { return noteWritten{err: err} }
	}
	args := []string{"note", "add", "--file", model.selectedFile(), "--side", "right"}
	if line := model.selectedLine(); line > 0 {
		args = append(args, "--line", fmt.Sprint(line))
	}
	if model.options.configPath != "" {
		args = append(args, "--config", model.options.configPath)
	}
	switch model.options.view {
	case "staged":
		args = append(args, "--staged")
	case "commit":
		args = append(args, "--commit", model.options.commit)
	}
	if !editor {
		args = append(args, "--message", message)
	}
	command := interactiveCommand(executable, args...)
	command.Dir = model.root
	if editor {
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		return tea.ExecProcess(command, func(err error) tea.Msg { return noteWritten{err: err} })
	}
	return func() tea.Msg {
		output, err := command.CombinedOutput()
		if err != nil {
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				detail = err.Error()
			}
			return noteWritten{err: fmt.Errorf("add note: %s", detail)}
		}
		return noteWritten{output: string(output)}
	}
}

func (model *interactiveModel) setSnapshot(snapshot workspaceview.Snapshot) {
	model.snapshot = snapshot
	model.viewport.SetContent(snapshot.Rendered)
	items := model.navigatorItems()
	if model.selected >= len(items) {
		model.selected = max(0, len(items)-1)
	}
	for index, item := range items {
		if model.restorePath != "" && item.path == model.restorePath || model.restoreOID != "" && item.oid == model.restoreOID {
			model.selected = index
			model.restorePath, model.restoreOID = "", ""
			break
		}
	}
}

func (model *interactiveModel) resize() {
	model.viewport.Width = max(1, model.diffWidth())
	model.viewport.Height = max(1, model.diffHeight())
	model.note.SetWidth(max(20, min(72, model.diffWidth()-2)))
	model.note.SetHeight(max(3, min(6, model.diffHeight()-3)))
}

func (model interactiveModel) View() string {
	if model.width < 40 || model.height < 10 {
		return model.tooSmallView()
	}
	status := interactiveFit(model.statusLine(), model.width)
	if model.showHelp {
		body := interactiveBox("keys", model.width, model.bodyHeight(), model.helpView(), true)
		return status + "\n" + body + "\n" + interactiveFit(interactiveMuted.Render("esc close"), model.width)
	}
	navigator := model.navigatorView()
	main := model.mainView()
	var body string
	if model.effectiveDock() == "bottom" {
		mainHeight := model.bodyHeight() - model.navigatorHeight()
		body = interactiveBox("changes", model.width, mainHeight, main, model.focus == "main") + "\n" +
			interactiveBox("explorer", model.width, model.navigatorHeight(), navigator, model.focus == "navigator")
	} else {
		navWidth := model.navigatorWidth()
		left := interactiveBox("explorer", navWidth, model.bodyHeight(), navigator, model.focus == "navigator")
		right := interactiveBox("changes", model.width-navWidth, model.bodyHeight(), main, model.focus == "main")
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return status + "\n" + body + "\n" + interactiveFit(model.footerView(), model.width)
}

func (model interactiveModel) statusLine() string {
	branch := model.snapshot.Repository.Branch
	if branch == "" {
		branch = "detached"
	}
	freshness := model.snapshot.Freshness.State
	if freshness == "" {
		freshness = "loading"
	}
	loading := ""
	if model.loading {
		loading = " " + model.spinner.View()
	}
	view := model.options.view
	if model.snapshot.Comparison.Kind == "commit" {
		from, to := model.snapshot.Comparison.From, model.snapshot.Comparison.To
		if len(from) > 8 {
			from = from[:8]
		}
		if len(to) > 8 {
			to = to[:8]
		}
		view += " " + from + ".." + to
	}
	return interactiveAccent.Render("changes") + " " + model.rootName() + " · " + branch + " · " +
		view + " · " + model.options.layout + " · " + freshness + loading
}

func (model interactiveModel) rootName() string {
	if model.snapshot.Repository.Name != "" {
		return model.snapshot.Repository.Name
	}
	return filepath.Base(model.root)
}

func (model interactiveModel) navigatorView() string {
	tab := model.activeTab()
	files, history := "Files", "History"
	if tab == "files" {
		files = interactiveActive.Render(files)
	} else {
		history = interactiveActive.Render(history)
	}
	rows := []string{files + "  " + history + "  " + interactiveMuted.Render(model.configured.Interactive.Navigator)}
	items := model.navigatorItems()
	start, end := model.navigatorWindow()
	for index := start; index < end; index++ {
		item := items[index]
		prefix := "  "
		if index == model.selected {
			prefix = "› "
		}
		row := prefix + item.label
		if index == model.selected && model.focus == "navigator" {
			row = interactiveActive.Render(row)
		}
		rows = append(rows, row)
	}
	if len(items) == 0 {
		if model.activeTab() == "history" {
			rows = append(rows, interactiveMuted.Render("No commit history"))
		} else {
			rows = append(rows, interactiveMuted.Render("No changed files"))
		}
	}
	return strings.Join(rows, "\n")
}

func (model interactiveModel) navigatorItems() []navItem {
	if model.activeTab() == "history" {
		items := make([]navItem, 0, len(model.snapshot.History))
		for _, commit := range model.snapshot.History {
			oid := commit.OID
			if len(oid) > 8 {
				oid = oid[:8]
			}
			marker := "○"
			if commit.NoteCount > 0 {
				marker = "◆"
			}
			items = append(items, navItem{oid: commit.OID, label: marker + " " + oid + " " + commit.Summary})
		}
		return items
	}
	items := make([]navItem, 0, len(model.snapshot.Files))
	for _, file := range model.snapshot.Files {
		marker := "○"
		if file.NoteCount > 0 {
			marker = "◆"
		}
		label := marker + " " + file.Path + fmt.Sprintf(" +%d -%d", file.Added, file.Deleted)
		if model.configured.Interactive.Navigator == "tree" {
			depth := strings.Count(file.Path, "/")
			label = strings.Repeat("│  ", depth) + "└─ " + marker + " " + filepath.Base(file.Path)
		}
		items = append(items, navItem{path: file.Path, label: label})
	}
	return items
}

func (model interactiveModel) activeTab() string {
	if model.tab == "history" {
		return "history"
	}
	return "files"
}

func (model *interactiveModel) toggleTab() {
	if model.activeTab() == "files" {
		model.tab = "history"
	} else {
		model.tab = "files"
	}
	model.selected = 0
	model.line = 0
}

func (model *interactiveModel) cycleView() {
	switch model.options.view {
	case "working":
		model.options.view = "staged"
	case "staged":
		model.options.view = "commit"
	default:
		model.options.view = "working"
	}
}

func (model *interactiveModel) moveSelection(delta int) {
	count := len(model.navigatorItems())
	if count == 0 {
		model.selected = 0
		return
	}
	model.selected = (model.selected + delta + count) % count
	model.line = 0
}

func (model *interactiveModel) moveLine(delta int) {
	lines := model.changedLines()
	if len(lines) == 0 {
		model.line = 0
		return
	}
	index := 0
	for candidate, line := range lines {
		if line == model.line {
			index = candidate
			break
		}
	}
	model.line = lines[(index+delta+len(lines))%len(lines)]
	model.message = fmt.Sprintf("annotation target %s:%d", model.selectedFile(), model.line)
}

func (model interactiveModel) activateSelection() (tea.Model, tea.Cmd) {
	items := model.navigatorItems()
	if len(items) == 0 || model.selected >= len(items) {
		return model, nil
	}
	item := items[model.selected]
	if item.oid != "" {
		model.options.view, model.options.commit, model.loading = "commit", item.oid, true
		model.focus = "main"
		return model, tea.Batch(model.spinner.Tick, model.refreshCommand())
	}
	if item.path != "" {
		lines := strings.Split(model.snapshot.Rendered, "\n")
		for index, line := range lines {
			if strings.Contains(line, item.path) || strings.Contains(line, filepath.Base(item.path)) {
				model.viewport.SetYOffset(index)
				break
			}
		}
		model.focus = "main"
	}
	return model, nil
}

func (model interactiveModel) selectedFile() string {
	if model.activeTab() != "files" {
		return ""
	}
	items := model.navigatorItems()
	if len(items) == 0 || model.selected >= len(items) {
		return ""
	}
	return items[model.selected].path
}

func (model interactiveModel) selectedLine() int {
	if model.selectedFile() == "" {
		return 0
	}
	if model.line > 0 {
		return model.line
	}
	lines := model.changedLines()
	if len(lines) > 0 {
		return lines[0]
	}
	return 0
}

func (model interactiveModel) changedLines() []int {
	path := model.selectedFile()
	lines := []int{}
	for _, file := range model.snapshot.Files {
		if file.Path != path {
			continue
		}
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if line.Kind == "added" && line.NewLine > 0 {
					lines = append(lines, line.NewLine)
				}
			}
		}
	}
	return lines
}

func (model interactiveModel) footerView() string {
	if model.mode == "note" {
		return model.note.View() + "\n" + interactiveMuted.Render("ctrl+s save · esc cancel")
	}
	if model.mode == "command" {
		line := interactiveAccent.Render(":"+model.command) + interactiveActive.Render(" ")
		if model.message != "" {
			line += interactiveMuted.Render("  " + model.message)
		}
		return line
	}
	if model.message != "" {
		if strings.Contains(model.message, "error") || strings.Contains(model.message, "failed") {
			return interactiveError.Render(model.message)
		}
		return interactiveMuted.Render(model.message)
	}
	if len(model.snapshot.Failures) > 0 {
		return interactiveError.Render(model.snapshot.Failures[0].Message)
	}
	ids := []string{"focus", "move", "activate", "refresh", "command", "help", "quit"}
	if model.width >= 110 {
		ids = []string{"focus", "move", "activate", "tab", "layout", "view", "refresh", "command", "help", "quit"}
	}
	name := "changes"
	if model.focus == "navigator" {
		name = "explorer"
	}
	return interactiveActive.Render(name) + interactiveMuted.Render("   "+interactiveBindingHints(ids...))
}

func (model interactiveModel) helpView() string {
	keyWidth := min(23, max(12, model.width/4))
	lines := make([]string, 0, len(interactiveBindings))
	for _, binding := range interactiveBindings {
		key := interactiveFit(binding.keys, keyWidth)
		lines = append(lines, interactiveAccent.Render(key)+interactivePlain.Render(binding.description))
	}
	return strings.Join(lines, "\n")
}

func (model interactiveModel) mainView() string {
	if model.snapshot.Version == "" {
		if model.loading {
			return model.spinner.View() + " loading workspace"
		}
		return interactiveMuted.Render("No workspace snapshot")
	}
	if len(model.snapshot.Files) == 0 || strings.TrimSpace(model.snapshot.Rendered) == "" {
		label := "No changes in this comparison"
		switch model.options.view {
		case "working":
			label = "Working tree clean"
		case "staged":
			label = "No staged changes"
		case "commit":
			label = "This commit has no first-parent changes"
		}
		return interactiveActive.Render(label) + "\n\n" +
			interactiveMuted.Render(interactiveBindingHint("view")+"   "+interactiveBindingHint("refresh"))
	}
	return model.viewport.View()
}

func (model interactiveModel) bodyHeight() int {
	return max(1, model.height-2)
}

func (model interactiveModel) effectiveDock() string {
	if model.configured.Interactive.Dock == "left" && model.width >= 72 {
		return "left"
	}
	return "bottom"
}

func (model interactiveModel) navigatorWidth() int {
	return min(44, max(28, model.width/3))
}

func (model interactiveModel) navigatorHeight() int {
	return min(10, max(5, model.bodyHeight()/3))
}

func (model interactiveModel) diffWidth() int {
	if model.width < 1 {
		return 0
	}
	if model.effectiveDock() == "left" {
		return max(1, model.width-model.navigatorWidth()-2)
	}
	return max(1, model.width-2)
}

func (model interactiveModel) diffHeight() int {
	if model.height < 1 {
		return 0
	}
	if model.effectiveDock() == "bottom" {
		return max(1, model.bodyHeight()-model.navigatorHeight()-2)
	}
	return max(1, model.bodyHeight()-2)
}

func (model interactiveModel) navigatorWindow() (int, int) {
	items := model.navigatorItems()
	height := model.bodyHeight()
	if model.effectiveDock() == "bottom" {
		height = model.navigatorHeight()
	}
	limit := max(1, height-3)
	start := max(0, model.selected-limit/2)
	if start+limit > len(items) {
		start = max(0, len(items)-limit)
	}
	return start, min(len(items), start+limit)
}

func (model interactiveModel) tooSmallView() string {
	if model.width < 1 || model.height < 1 {
		return ""
	}
	lines := make([]string, model.height)
	for index := range lines {
		lines[index] = strings.Repeat(" ", model.width)
	}
	messages := []string{
		fmt.Sprintf("Changes needs 40x10; this pane is %dx%d", model.width, model.height),
		"Resize the pane or press q to quit",
	}
	for index, message := range messages {
		row := model.height/2 - 1 + index
		if row >= 0 && row < len(lines) {
			padding := max(0, (model.width-ansi.StringWidth(message))/2)
			lines[row] = interactiveFit(strings.Repeat(" ", padding)+message, model.width)
		}
	}
	return strings.Join(lines, "\n")
}

func interactiveFit(value string, width int) string {
	if width < 1 {
		return ""
	}
	value = ansi.Truncate(value, width, "…")
	if current := ansi.StringWidth(value); current < width {
		value += strings.Repeat(" ", width-current)
	}
	return value
}

func interactiveBox(name string, width, height int, body string, focused bool) string {
	if width < 4 || height < 3 {
		return interactiveFit(body, max(0, width))
	}
	innerWidth, innerHeight := width-2, height-2
	edge := interactiveRule
	if focused {
		edge = interactiveAccent
	}
	title := "─ " + name + " "
	top := edge.Render("╭" + title + strings.Repeat("─", max(0, innerWidth-ansi.StringWidth(title))) + "╮")
	lines := strings.Split(body, "\n")
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	out := make([]string, 0, height)
	out = append(out, interactiveFit(top, width))
	for _, line := range lines {
		out = append(out, edge.Render("│")+interactiveFit(line, innerWidth)+edge.Render("│"))
	}
	out = append(out, edge.Render("╰"+strings.Repeat("─", innerWidth)+"╯"))
	return strings.Join(out, "\n")
}

func isInteractiveTerminalReply(key string) bool {
	return key == "alt+]" || key == "alt+\\" ||
		strings.HasPrefix(key, "]10;") || strings.HasPrefix(key, "]11;") ||
		strings.HasPrefix(key, "10;rgb:") || strings.HasPrefix(key, "11;rgb:")
}

func (model interactiveModel) persistState() error {
	items := model.navigatorItems()
	selectedPath, selectedOID := "", ""
	if len(items) > 0 && model.selected < len(items) {
		selectedPath, selectedOID = items[model.selected].path, items[model.selected].oid
	}
	return model.store.SaveState(workspaceview.State{
		Repository: model.root, Dock: model.configured.Interactive.Dock,
		Navigator: model.configured.Interactive.Navigator, Tab: model.activeTab(),
		Layout: model.options.layout, View: model.options.view,
		SelectedPath: selectedPath, SelectedLine: model.selectedLine(), SelectedOID: selectedOID,
		Commands: append([]string(nil), model.commands...),
	})
}

func (model *interactiveModel) restoreState(state workspaceview.State) {
	if state.Dock == "left" || state.Dock == "bottom" {
		model.configured.Interactive.Dock = state.Dock
	}
	if state.Navigator == "tree" || state.Navigator == "list" {
		model.configured.Interactive.Navigator = state.Navigator
	}
	if state.Tab == "history" {
		model.tab = "history"
	} else {
		model.tab = "files"
	}
	model.commands = append([]string(nil), state.Commands...)
	model.line = state.SelectedLine
	model.restorePath, model.restoreOID = state.SelectedPath, state.SelectedOID
	items := model.navigatorItems()
	for index, item := range items {
		if item.path == state.SelectedPath || item.oid == state.SelectedOID {
			model.selected = index
			break
		}
	}
}
