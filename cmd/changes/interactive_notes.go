package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/go-utils/panes"
)

func (model interactiveModel) startDraftGeneration() (tea.Model, tea.Cmd) {
	if model.focus != "navigator" || model.activeTab() != "history" {
		model.message = "open History and select a commit before generating notes"
		return model, nil
	}
	commits := model.selectedCommitsForGeneration()
	if len(commits) == 0 {
		model.message = "select a commit before generating notes"
		return model, nil
	}
	if model.configured.Notes.GeneratorTimeout.Duration() <= 0 {
		model.message = "notes.generatorTimeout must be greater than zero"
		return model, nil
	}
	model.mode, model.loading, model.message = "generating", true, ""
	return model, tea.Batch(model.spinner.Tick, model.prepareDraftGenerationCommand(commits))
}

func (model interactiveModel) prepareDraftGenerationCommand(commits []string) tea.Cmd {
	root := model.root
	configured := model.configured
	commits = append([]string(nil), commits...)
	return func() tea.Msg {
		specs, err := noteGenerationSpecs(root, noteComparisonFlags{}, noteCommitFlags(commits))
		if err != nil {
			return interactiveGenerationPrepared{err: err}
		}
		discovery, err := interactiveDiscoverProviders(configured.Providers.Directory)
		if err != nil {
			return interactiveGenerationPrepared{err: err}
		}
		generators, err := selectOneNoteProvider(
			discovery.Providers, provider.ActionNotesGenerate, configured.Notes.Generator, "generator", "provider",
		)
		if err != nil {
			return interactiveGenerationPrepared{err: err}
		}
		return interactiveGenerationPrepared{specs: specs, generator: generators[0]}
	}
}

func (model interactiveModel) generateNextDraftCommand() tea.Cmd {
	index := len(model.draftBatches)
	if index >= len(model.generationSpecs) {
		return nil
	}
	selected := model.generationSpecs[index]
	generator := model.generator
	timeout := model.configured.Notes.GeneratorTimeout.Duration()
	return func() tea.Msg {
		batch, err := interactiveGenerateNoteComparison(
			context.Background(), selected.commit, selected.spec, "", generator, timeout,
		)
		return interactiveDraftGenerated{batch: batch, err: err}
	}
}

func (model *interactiveModel) beginDraftReview() {
	model.mode, model.loading, model.message = "review", false, ""
	model.reviewCursor = 0
	model.reviewIncluded = make([][]bool, len(model.draftBatches))
	for batch := range model.draftBatches {
		model.reviewIncluded[batch] = make([]bool, len(model.draftBatches[batch].drafts))
		for draft := range model.reviewIncluded[batch] {
			model.reviewIncluded[batch][draft] = true
		}
	}
}

func (model interactiveModel) draftLocations() []interactiveDraftLocation {
	locations := []interactiveDraftLocation{}
	for batch := range model.draftBatches {
		for draft := range model.draftBatches[batch].drafts {
			locations = append(locations, interactiveDraftLocation{batch: batch, draft: draft})
		}
	}
	return locations
}

func (model *interactiveModel) moveReviewCursor(delta int) {
	locations := model.draftLocations()
	if len(locations) == 0 {
		model.reviewCursor = 0
		return
	}
	model.reviewCursor = panes.Cycle(model.reviewCursor, delta, len(locations))
}

func (model *interactiveModel) toggleReviewDraft() {
	locations := model.draftLocations()
	if len(locations) == 0 || model.reviewCursor < 0 || model.reviewCursor >= len(locations) {
		return
	}
	location := locations[model.reviewCursor]
	included := append([]bool(nil), model.reviewIncluded[location.batch]...)
	included[location.draft] = !included[location.draft]
	model.reviewIncluded[location.batch] = included
}

func (model interactiveModel) editReviewDraft() (tea.Model, tea.Cmd) {
	locations := model.draftLocations()
	if len(locations) == 0 || model.reviewCursor < 0 || model.reviewCursor >= len(locations) {
		model.message = "no generated note is available to edit"
		return model, nil
	}
	model.editLocation = locations[model.reviewCursor]
	draft := model.draftBatches[model.editLocation.batch].drafts[model.editLocation.draft]
	value := draft.Summary
	if draft.Rationale != "" {
		value += "\n" + draft.Rationale
	}
	model.note.SetValue(value)
	model.note.Focus()
	model.mode, model.message = "review-edit", ""
	return model, textarea.Blink
}

func (model *interactiveModel) updateReviewDraft(summary, rationale string) {
	location := model.editLocation
	if location.batch < 0 || location.batch >= len(model.draftBatches) ||
		location.draft < 0 || location.draft >= len(model.draftBatches[location.batch].drafts) {
		return
	}
	batches := append([]noteGeneratedComparison(nil), model.draftBatches...)
	batch := batches[location.batch]
	batch.drafts = append([]provider.NoteDraft(nil), batch.drafts...)
	batch.drafts[location.draft].Summary = summary
	batch.drafts[location.draft].Rationale = rationale
	batches[location.batch] = batch
	model.draftBatches = batches
}

func (model interactiveModel) includedDraftBatches() ([]noteGeneratedComparison, int) {
	batches := make([]noteGeneratedComparison, len(model.draftBatches))
	total := 0
	for index, original := range model.draftBatches {
		batch := original
		batch.drafts = nil
		for draft, included := range model.reviewIncluded[index] {
			if included {
				batch.drafts = append(batch.drafts, original.drafts[draft])
				total++
			}
		}
		batches[index] = batch
	}
	return batches, total
}

func (model interactiveModel) confirmDraftReview() (tea.Model, tea.Cmd) {
	batches, total := model.includedDraftBatches()
	if total == 0 {
		model.message = "include at least one generated note before saving"
		return model, nil
	}
	model.mode, model.loading, model.message = "writing", true, ""
	return model, tea.Batch(model.spinner.Tick, model.prepareDraftWriteCommand(batches))
}

func (model interactiveModel) prepareDraftWriteCommand(batches []noteGeneratedComparison) tea.Cmd {
	configured := model.configured
	batches = append([]noteGeneratedComparison(nil), batches...)
	return func() tea.Msg {
		for _, batch := range batches {
			if err := interactiveVerifyNoteSnapshot(batch.spec, batch.snapshot, "writing notes"); err != nil {
				return interactiveWritePrepared{err: err}
			}
		}
		discovery, err := interactiveDiscoverProviders(configured.Providers.Directory)
		if err != nil {
			return interactiveWritePrepared{err: err}
		}
		writers, err := selectOneNoteProvider(
			discovery.Providers, provider.ActionNotesCreate, configured.Notes.Store, "store", "store",
		)
		if err != nil {
			return interactiveWritePrepared{err: err}
		}
		return interactiveWritePrepared{batches: batches, writer: writers[0]}
	}
}

func (model interactiveModel) writeNextComparisonCommand() tea.Cmd {
	if model.writeIndex < 0 || model.writeIndex >= len(model.writeBatches) {
		return nil
	}
	batch := model.writeBatches[model.writeIndex]
	writer := model.writer
	timeout := time.Duration(model.configured.Providers.Timeout)
	return func() tea.Msg {
		created, err := interactiveWriteNoteComparison(context.Background(), batch, writer, timeout)
		return interactiveComparisonWritten{result: interactiveWriteResult{
			commit: interactiveBatchCommit(batch), count: len(created), err: err,
		}}
	}
}

func interactiveBatchCommit(batch noteGeneratedComparison) string {
	if batch.snapshot.head != "" {
		return batch.snapshot.head
	}
	return batch.commit
}

func (model *interactiveModel) closeDraftReview() {
	model.mode, model.loading = "normal", false
	model.draftBatches = nil
	model.generationSpecs = nil
	model.generator = provider.LoadedManifest{}
	model.reviewIncluded = nil
	model.reviewCursor = 0
	model.writeBatches = nil
	model.writer = provider.LoadedManifest{}
	model.writeResults = nil
	model.writeIndex = 0
	model.note.Blur()
}

func (model *interactiveModel) closeDraftResults() tea.Cmd {
	refresh := false
	for _, result := range model.writeResults {
		refresh = refresh || result.err == nil && result.count > 0
	}
	model.closeDraftReview()
	if !refresh {
		return nil
	}
	model.options.refresh, model.loading = true, true
	return tea.Batch(model.spinner.Tick, model.refreshCommand())
}

func (model interactiveModel) draftReviewView() string {
	switch model.mode {
	case "generating":
		return model.generationProgressView()
	case "review-edit":
		return model.reviewEditView()
	case "writing":
		return model.writeProgressView()
	case "results":
		return model.writeResultsView()
	default:
		rows := model.visibleReviewRows()
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, row.text)
		}
		if len(lines) == 0 {
			return interactiveMuted.Render("No note drafts were generated")
		}
		return strings.Join(lines, "\n")
	}
}

func (model interactiveModel) generationProgressView() string {
	total := len(model.generationSpecs)
	if total == 0 {
		total = len(model.selectedCommitsForGeneration())
	}
	done := len(model.draftBatches)
	lines := []string{fmt.Sprintf("%s Generating notes for commit %d/%d", model.spinner.View(), min(done+1, total), total)}
	for index, selected := range model.generationSpecs {
		marker := "○"
		if index < done {
			marker = "✓"
		} else if index == done {
			marker = model.spinner.View()
		}
		lines = append(lines, fmt.Sprintf("%s %s", marker, shortInteractiveOID(selected.commit)))
	}
	return strings.Join(lines, "\n")
}

func (model interactiveModel) reviewRows() []interactiveReviewRow {
	rows := []interactiveReviewRow{}
	cursor := 0
	for batchIndex, batch := range model.draftBatches {
		commit := interactiveBatchCommit(batch)
		title := shortInteractiveOID(commit)
		if summary := model.historySummary(commit); summary != "" {
			title += "  " + summary
		}
		rows = append(rows, interactiveReviewRow{text: interactiveAccent.Render("commit " + title), cursor: -1})
		paths := make([]string, 0, len(batch.drafts))
		byPath := map[string][]int{}
		for draftIndex, draft := range batch.drafts {
			path := draft.Anchor.Path
			if _, found := byPath[path]; !found {
				paths = append(paths, path)
			}
			byPath[path] = append(byPath[path], draftIndex)
		}
		slices.Sort(paths)
		for _, path := range paths {
			rows = append(rows, interactiveReviewRow{text: "  " + interactiveActive.Render(path), cursor: -1})
			for _, draftIndex := range byPath[path] {
				draft := batch.drafts[draftIndex]
				included := model.reviewIncluded[batchIndex][draftIndex]
				marker := "[ ]"
				if included {
					marker = "[x]"
				}
				line := fmt.Sprintf("    %s %s", marker, draft.Summary)
				if cursor == model.reviewCursor {
					line = interactiveActive.Render(line)
				}
				rows = append(rows, interactiveReviewRow{text: line, cursor: cursor})
				if draft.Rationale != "" {
					rows = append(rows, interactiveReviewRow{text: interactiveMuted.Render("        " + cleanNoteOneLine(draft.Rationale)), cursor: -1})
				}
				cursor++
			}
		}
	}
	return rows
}

func (model interactiveModel) visibleReviewRows() []interactiveReviewRow {
	rows := model.reviewRows()
	limit := max(1, model.bodyHeight()-2)
	if len(rows) <= limit {
		return rows
	}
	selected := 0
	for index, row := range rows {
		if row.cursor == model.reviewCursor {
			selected = index
			break
		}
	}
	start := max(0, selected-limit/2)
	if start+limit > len(rows) {
		start = len(rows) - limit
	}
	return rows[start : start+limit]
}

func (model interactiveModel) reviewEditView() string {
	locations := model.draftLocations()
	if model.reviewCursor < 0 || model.reviewCursor >= len(locations) {
		return interactiveError.Render("The selected draft is no longer available")
	}
	location := locations[model.reviewCursor]
	draft := model.draftBatches[location.batch].drafts[location.draft]
	return interactiveAccent.Render(shortInteractiveOID(interactiveBatchCommit(model.draftBatches[location.batch]))) +
		"  " + interactiveActive.Render(draft.Anchor.Path) + "\n\n" + model.note.View()
}

func (model interactiveModel) writeProgressView() string {
	total := len(model.writeBatches)
	done := model.writeIndex
	lines := []string{fmt.Sprintf("%s Saving notes for commit %d/%d", model.spinner.View(), min(done+1, total), total)}
	for index, batch := range model.writeBatches {
		marker := "○"
		if index < len(model.writeResults) {
			if model.writeResults[index].err != nil {
				marker = "!"
			} else {
				marker = "✓"
			}
		} else if index == done {
			marker = model.spinner.View()
		}
		lines = append(lines, fmt.Sprintf("%s %s", marker, shortInteractiveOID(interactiveBatchCommit(batch))))
	}
	return strings.Join(lines, "\n")
}

func (model interactiveModel) writeResultsView() string {
	lines := []string{interactiveAccent.Render("Write results")}
	for _, result := range model.writeResults {
		commit := shortInteractiveOID(result.commit)
		if result.err != nil {
			lines = append(lines, interactiveError.Render(fmt.Sprintf("! %s  %s", commit, result.err)))
			continue
		}
		lines = append(lines, interactiveActive.Render(fmt.Sprintf("✓ %s  %d note(s) saved", commit, result.count)))
	}
	return strings.Join(lines, "\n")
}

func (model interactiveModel) historySummary(oid string) string {
	for _, commit := range model.snapshot.History {
		if commit.OID == oid {
			return commit.Summary
		}
	}
	return ""
}

func shortInteractiveOID(oid string) string {
	if len(oid) > 8 {
		return oid[:8]
	}
	return oid
}
