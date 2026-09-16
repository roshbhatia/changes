package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/workspace"
)

func interactiveRepositories(options workspaceOptions) ([]string, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	if options.root != "" {
		dir, err = resolveFrom(dir, options.root)
		if err != nil {
			return nil, "", fmt.Errorf("resolve --root %q: %w", options.root, err)
		}
	}
	if !options.recursive {
		root, err := source.Root(dir)
		return []string{root}, root, err
	}
	boundary := workspace.Root(dir)
	roots, err := workspace.Roots(dir)
	if err != nil {
		return nil, boundary, err
	}
	if len(roots) == 0 {
		return nil, boundary, fmt.Errorf("no git repository under %s", boundary)
	}
	return roots, boundary, nil
}

func (model interactiveModel) selectRepository(root string) (tea.Model, tea.Cmd) {
	if root == model.root {
		model.setTab("files")
		return model, nil
	}
	if err := model.persistState(); err != nil {
		model.message = "save workspace state: " + err.Error()
		return model, nil
	}
	options := model.options
	options.commit = model.initialCommit
	next := newInteractiveModel(root, options, model.configured, model.store)
	next.repositories, next.repositoryBoundary, next.initialCommit = model.repositories, model.repositoryBoundary, model.initialCommit
	next.refreshEpoch, next.previewEpoch = model.refreshEpoch, model.previewEpoch
	next.commands = append([]string(nil), model.commands...)
	next.width, next.height, next.focus = model.width, model.height, "navigator"
	next.resize()
	return next, tea.Batch(next.spinner.Tick, next.refreshCommand())
}
