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
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	interactiveExecutable = os.Executable
	interactiveCommand    = exec.Command
)

func runInteractive(args []string) {
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
	program := tea.NewProgram(model, tea.WithAltScreen())
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
	return tea.Batch(model.spinner.Tick, model.refreshCommand())
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
		model.width, model.height = value.Width, value.Height
		model.resize()
		return model, nil
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(value)
		return model, command
	case workspaceLoaded:
		model.loading = false
		if value.err != nil {
			model.message = value.err.Error()
			return model, nil
		}
		model.message = "workspace refreshed"
		model.setSnapshot(value.snapshot)
		if err := model.store.SaveSnapshot(workspaceSlot(model.options), value.snapshot); err != nil {
			model.message = "cache: " + err.Error()
		}
		return model, nil
	case noteWritten:
		model.loading = false
		if value.err != nil {
			model.message = value.err.Error()
			return model, nil
		}
		model.message = strings.TrimSpace(value.output)
		model.options.refresh = true
		model.loading = true
		return model, model.refreshCommand()
	case tea.KeyMsg:
		return model.handleKey(value)
	}
	return model, nil
}

func (model interactiveModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+c" {
		_ = model.persistState()
		return model, tea.Quit
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
			return model, model.noteCommand(message, false)
		}
		var command tea.Cmd
		model.note, command = model.note.Update(key)
		return model, command
	}
	if model.mode == "command" {
		switch key.String() {
		case "esc":
			model.mode, model.command = "normal", ""
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
		case "backspace":
			_, size := utf8.DecodeLastRuneInString(model.command)
			if size > 0 {
				model.command = model.command[:len(model.command)-size]
			}
			return model, nil
		case " ":
			model.command += " "
			return model, nil
		default:
			if key.Type == tea.KeyRunes {
				model.command += string(key.Runes)
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
		model.mode = "command"
	case "tab":
		if model.focus == "main" {
			model.focus = "navigator"
		} else {
			model.focus = "main"
		}
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
	case "s":
		if model.options.layout == "unified" {
			model.options.layout = "side-by-side"
		} else {
			model.options.layout = "unified"
		}
		model.loading = true
		return model, model.refreshCommand()
	case "v":
		model.cycleView()
		model.loading = true
		return model, model.refreshCommand()
	case "r":
		model.options.refresh = true
		model.loading = true
		return model, model.refreshCommand()
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
			return model, model.noteCommand("", true)
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
		return model, model.noteCommand("", true)
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

func (model interactiveModel) runPaletteCommand(command string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return model, nil
	}
	commands := []string{"dock", "layout", "navigator", "note", "quit", "refresh", "tab", "view"}
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
		return model, model.refreshCommand()
	case "dock":
		if len(fields) == 2 && (fields[1] == "left" || fields[1] == "bottom") {
			model.configured.Interactive.Dock = fields[1]
			model.resize()
			return model, nil
		}
	case "layout":
		if len(fields) == 2 && (fields[1] == "unified" || fields[1] == "side-by-side") {
			model.options.layout, model.loading = fields[1], true
			return model, model.refreshCommand()
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
			return model, model.refreshCommand()
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
			return model, model.noteCommand("", true)
		}
	}
	model.message = "unknown command: " + command
	return model, nil
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
	availableHeight := max(4, model.height-3)
	if model.configured.Interactive.Dock == "bottom" {
		navHeight := min(10, max(5, availableHeight/3))
		model.viewport.Width = max(20, model.width-2)
		model.viewport.Height = max(3, availableHeight-navHeight-1)
		return
	}
	navWidth := min(44, max(26, model.width/3))
	model.viewport.Width = max(20, model.width-navWidth-3)
	model.viewport.Height = availableHeight
}

func (model interactiveModel) View() string {
	status := model.statusLine()
	if model.showHelp {
		return status + "\n" + model.helpView()
	}
	navigator := model.navigatorView()
	main := model.viewport.View()
	if model.configured.Interactive.Dock == "bottom" {
		mainWidth := max(20, model.width-2)
		body := lipgloss.NewStyle().Width(mainWidth).Render(main) + "\n" + navigator
		return status + "\n" + body + "\n" + model.footerView()
	}
	navWidth := min(44, max(26, model.width/3))
	left := lipgloss.NewStyle().Width(navWidth).Render(navigator)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " │ ", main)
	return status + "\n" + body + "\n" + model.footerView()
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
	limit := max(1, model.height-5)
	if model.configured.Interactive.Dock == "bottom" {
		limit = min(9, max(3, (model.height-3)/3-1))
	}
	start := max(0, model.selected-limit/2)
	if start+limit > len(items) {
		start = max(0, len(items)-limit)
	}
	end := min(len(items), start+limit)
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
		return model, model.refreshCommand()
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
		return ":" + model.command
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
	return interactiveMuted.Render("tab focus · f files/history · t tree/list · d dock · s layout · v view · [ ] line · a configured note · n popup · e editor · r refresh · : commands · ? help · q quit")
}

func (model interactiveModel) helpView() string {
	return strings.Join([]string{
		interactiveAccent.Render("Interactive workspace"),
		"",
		"The main pane keeps logical groups, diff lines, context, and note threads together.",
		"Files and History share the navigator. Enter opens a file or first-parent commit view.",
		"",
		"Commands: layout, dock, navigator, tab, view, refresh, note popup, note editor, quit.",
		"Press ? to return.",
	}, "\n")
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
