package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/changes/internal/workspaceview"
)

type attributionLoaded struct {
	content string
	epoch   uint64
}

func contributorBadge(contributors []source.Contributor) string {
	for _, person := range contributors {
		if person.AI {
			return " [AI declared]"
		}
	}
	return ""
}

func contributorSummary(contributors []source.Contributor) string {
	parts := []string{}
	for _, person := range contributors {
		if person.Role == "author" && !person.AI {
			continue
		}
		parts = append(parts, person.Role+": "+cleanNoteOneLine(person.Name))
	}
	if len(parts) == 0 {
		return "no assistance declared"
	}
	return strings.Join(parts, "; ")
}

func (model interactiveModel) attributionCommand(path string) tea.Cmd {
	epoch := model.currentRefreshEpoch()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return attributionLoaded{epoch: epoch, content: lineAttribution(ctx, model.root, model.options, model.snapshot, path)}
	}
}

func lineAttribution(ctx context.Context, root string, options workspaceOptions, snapshot workspaceview.Snapshot, path string) string {
	var file workspaceview.File
	for _, candidate := range snapshot.Files {
		if candidate.Path == path {
			file = candidate
			break
		}
	}
	if file.Path == "" {
		return "The selected file is no longer in this comparison."
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		return string(out), err
	}
	spec, err := workspaceSource(root, options)
	if err != nil {
		return err.Error()
	}
	base, head := spec.ComparisonIDs()
	oldPath := path
	if patch, err := spec.RawDiff(); err == nil {
		for _, section := range splitPatchFiles(patch) {
			if displayDiffPath(section.newPath) == path && section.oldPath != "/dev/null" {
				oldPath = displayDiffPath(section.oldPath)
				break
			}
		}
	}
	if spec.Staged && head == "" {
		candidate, candidateErr := run("rev-parse", "--verify", "refs/pr/head^{commit}")
		_, comparisonErr := run("diff", "--cached", "--quiet", "refs/pr/head", "--")
		if candidateErr == nil && comparisonErr == nil {
			head = strings.TrimSpace(candidate)
		}

	}
	ranges := map[string][][2]int{"added": {}, "deleted": {}}
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			number := line.NewLine
			if line.Kind == "deleted" {
				number = line.OldLine
			}
			if line.Kind != "added" && line.Kind != "deleted" || number <= 0 {
				continue
			}
			spans := ranges[line.Kind]
			if len(spans) > 0 && spans[len(spans)-1][1]+1 == number {
				spans[len(spans)-1][1] = number
			} else {
				spans = append(spans, [2]int{number, number})
			}
			ranges[line.Kind] = spans
		}
	}
	oldBlame, newBlame := map[int]source.BlameLine{}, map[int]source.BlameLine{}
	failures := []string{}
	for _, side := range []string{"deleted", "added"} {
		revision, target := base, oldPath
		if side == "added" {
			revision, target = head, path
		}
		var contents *string
		var contentErr error
		if strings.HasPrefix(revision, "index:") || revision == "" {
			var text string
			if side == "deleted" || spec.Staged {
				text, contentErr = run("show", ":"+target)
			} else {
				at := filepath.Join(root, target)
				info, statErr := os.Lstat(at)
				if statErr != nil {
					contentErr = statErr
				} else if !info.Mode().IsRegular() || info.Size() > 16*1024*1024 {
					contentErr = fmt.Errorf("unsupported working file")
				} else {
					var data []byte
					data, contentErr = os.ReadFile(at)
					text = string(data)
				}
			}
			contents = &text
		}
		if contentErr != nil {
			failures = append(failures, side+": content unavailable")
			continue
		}
		result, blameErr := source.Blame(ctx, root, revision, target, contents, ranges[side])
		if blameErr != nil {
			failures = append(failures, side+": Git attribution unavailable")
			continue
		}
		if side == "added" {
			newBlame = result
		} else {
			oldBlame = result
		}
	}
	commits := []string{}
	for _, blame := range []map[int]source.BlameLine{oldBlame, newBlame} {
		for _, line := range blame {
			if line.Commit != strings.Repeat("0", 40) && !slices.Contains(commits, line.Commit) {
				commits = append(commits, line.Commit)
			}
		}
	}
	contributors, metadataErr := source.CommitContributors(ctx, root, commits)
	if metadataErr != nil {
		failures = append(failures, "commit attribution trailers unavailable")
	}
	rows := []string{interactiveAccent.Render("Recorded authorship: " + cleanNoteOneLine(path)), "Deleted lines show their previous author. Added lines show their last recorded author.", "Trailers declare assistance; they do not prove which agent wrote each line.", "Uncommitted lines have no recorded author. Esc returns to the full diff.", ""}
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind != "added" && line.Kind != "deleted" {
				continue
			}
			number, blame, sign, style := line.NewLine, newBlame, "+", contextAdd
			if line.Kind == "deleted" {
				number, blame, sign, style = line.OldLine, oldBlame, "-", contextDel
			}
			who := "uncommitted or unavailable; authorship unknown"
			if origin, ok := blame[number]; ok && origin.Commit != strings.Repeat("0", 40) {
				who = cleanNoteOneLine(origin.Author) + " <" + cleanNoteOneLine(origin.Email) + "> " + origin.Commit[:8] + "; " + contributorSummary(contributors[origin.Commit])
				if origin.Boundary {
					who += "; history boundary"
				}
			}
			rows = append(rows, style.Render(fmt.Sprintf("%s %d %s", sign, number, who)), "  "+cleanNoteOneLine(line.Text))
		}
	}
	if len(failures) > 0 {
		rows = append(rows, "", interactiveMuted.Render(strings.Join(failures, "; ")))
	}
	return strings.Join(rows, "\n")
}
