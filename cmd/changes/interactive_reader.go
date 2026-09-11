package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/roshbhatia/changes/internal/workspaceview"
)

type readerFinished struct{ err error }
type readerPrepared struct {
	command *exec.Cmd
	err     error
}
type scopePreviewLoaded struct {
	path, rendered string
	epoch          uint64
	err            error
	freshness      workspaceview.Freshness
	failures       []workspaceview.Failure
	previewEpoch   uint64
}

func (model interactiveModel) currentRefreshEpoch() uint64 {
	if model.refreshEpoch == nil {
		return 0
	}
	return model.refreshEpoch.Load()
}

func (model interactiveModel) scopePreviewCommand() tea.Cmd {
	selectedPath, epoch := model.previewPath, model.currentRefreshEpoch()
	previewEpoch := uint64(0)
	if model.previewEpoch != nil {
		previewEpoch = model.previewEpoch.Add(1)
	}
	return func() tea.Msg {
		var paths []string
		if selectedPath != "" {
			paths = []string{":(literal)" + selectedPath}
		}
		snapshot, err := buildWorkspaceScopedSnapshot(model.root, model.options, model.configured, model.snapshot.Notes, paths)
		return scopePreviewLoaded{path: selectedPath, epoch: epoch, previewEpoch: previewEpoch, rendered: snapshot.Rendered, err: err, freshness: snapshot.Freshness, failures: snapshot.Failures}
	}
}

func patchReader(root string, options workspaceOptions, paths, argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("configure interactive.reader as a command array")
	}
	spec, err := workspaceSource(root, options)
	if err != nil {
		return nil, err
	}
	patch, err := spec.RawDiff()
	if err != nil {
		return nil, err
	}
	patch = scopePatch(patch, paths)
	if strings.TrimSpace(patch) == "" {
		return nil, nil
	}
	command := exec.Command(argv[0], argv[1:]...)
	if command.Err != nil {
		return nil, command.Err
	}
	command.Dir = root
	command.Stdin = strings.NewReader(patch)
	return command, nil
}

func scopePatch(patch string, paths []string) string {
	if len(paths) == 0 {
		return patch
	}
	var selected strings.Builder
	for _, section := range splitPatchFiles(patch) {
		matched := false
		for _, scope := range paths {
			scope = strings.TrimPrefix(scope, ":(literal)")
			for _, candidate := range []string{section.oldPath, section.newPath} {
				candidate = displayDiffPath(candidate)
				if candidate == scope || strings.HasSuffix(scope, "/") && strings.HasPrefix(candidate, scope) {
					matched = true
				}
			}
		}
		if matched {
			selected.WriteString(section.patch)
		}
	}
	return selected.String()
}

func (model interactiveModel) readerCommand() tea.Cmd {
	options := model.options
	var paths []string
	items := model.navigatorItems()
	if model.focus == "navigator" && model.selected < len(items) {
		item := items[model.selected]
		if item.oid != "" {
			options.view, options.commit = "commit", item.oid
		} else if item.path != "" {
			paths = []string{":(literal)" + item.path}
		}
	} else if model.previewActive && model.previewPath != "" {
		paths = []string{":(literal)" + model.previewPath}
	}
	argv := append([]string(nil), model.configured.Interactive.Reader...)
	return func() tea.Msg {
		command, err := patchReader(model.root, options, paths, argv)
		return readerPrepared{command: command, err: err}
	}
}

func (model interactiveModel) directoryItems() []navItem {
	if len(model.snapshot.Files) == 0 {
		return nil
	}
	entries := map[string]navItem{}
	for _, file := range model.snapshot.Files {
		label := path.Base(file.Path) + fmt.Sprintf(" +%d -%d", file.Added, file.Deleted)
		if file.NoteCount > 0 {
			label += fmt.Sprintf(" (%d notes)", file.NoteCount)
		}
		entries[file.Path] = navItem{path: file.Path, label: label}
		for directory := path.Dir(file.Path); directory != "."; directory = path.Dir(directory) {
			entries[directory+"/"] = navItem{path: directory + "/", directory: true, label: path.Base(directory) + "/"}
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := []navItem{{directory: true, label: "./ (all changes)"}}
	if model.collapsed[""] {
		return items
	}
	for _, key := range keys {
		hidden := false
		for parent := path.Dir(strings.TrimSuffix(key, "/")); parent != "."; parent = path.Dir(parent) {
			if model.collapsed[parent+"/"] {
				hidden = true
				break
			}
		}
		if hidden {
			continue
		}
		item := entries[key]
		marker := "  "
		if item.directory {
			marker = "- "
			if model.collapsed[item.path] {
				marker = "+ "
			}
		}
		item.label = strings.Repeat("  ", strings.Count(strings.TrimSuffix(key, "/"), "/")) + marker + item.label
		items = append(items, item)
	}
	return items
}

func (model *interactiveModel) setNavigator(kind string) {
	items := model.navigatorItems()
	var selected navItem
	if model.selected < len(items) {
		selected = items[model.selected]
	}
	model.configured.Interactive.Navigator = kind
	model.selected = 0
	for index, item := range model.navigatorItems() {
		if item.path == selected.path || selected.directory && strings.HasPrefix(item.path, selected.path) {
			model.selected = index
			break
		}
	}
}
