// changes-provider-calldiff adds call edges to the Changes provider protocol.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/go-utils/diffview"
	"github.com/roshbhatia/go-utils/git"
)

type callTrees struct {
	Trees []struct {
		ASCII string `json:"ascii"`
	} `json:"trees"`
}

var callSite = regexp.MustCompile(`:(\d+)(?:-\d+)?\s*$`)

func main() {
	request := provider.Request{}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fail(err)
	}
	if request.Version != provider.ProtocolVersion || request.Action != provider.ActionCalls {
		fail(fmt.Errorf("unsupported request %q %q", request.Version, request.Action))
	}
	edges, err := calls(request)
	if err != nil {
		fail(err)
	}
	response := provider.Response{Version: provider.ProtocolVersion, Edges: edges}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fail(err)
	}
}

func calls(request provider.Request) (map[string][]diffview.Edge, error) {
	out := map[string][]diffview.Edge{}
	from, to, err := comparisonRefs(request)
	if err != nil {
		return nil, err
	}
	seen := map[string]map[diffview.Edge]bool{}
	for _, file := range request.Files {
		trees, err := callDiff(request.Directory, from, to, file)
		if err != nil {
			return nil, err
		}
		collectEdges(out, seen, trees, request.Files)
	}
	return out, nil
}

func callDiff(directory, from, to, file string) (callTrees, error) {
	args := []string{"diff"}
	if from != "" {
		args = append(args, from)
	}
	if to != "" {
		args = append(args, to)
	}
	args = append(args, "--locs", "--format", "json", commandPath(file))
	command := exec.Command("calldiff", args...)
	command.Dir = directory
	command.Env = git.CleanEnv()
	blob, err := command.CombinedOutput()
	if err != nil {
		return callTrees{}, fmt.Errorf("calldiff %q: %s: %w", file, strings.TrimSpace(string(blob)), err)
	}
	trees := callTrees{}
	if err := json.Unmarshal(blob, &trees); err != nil {
		return callTrees{}, fmt.Errorf("decode calldiff output for %q: %w", file, err)
	}
	return trees, nil
}

func commandPath(path string) string {
	if strings.HasPrefix(path, "-") {
		return "." + string(filepath.Separator) + path
	}
	return path
}

func collectEdges(out map[string][]diffview.Edge, seen map[string]map[diffview.Edge]bool, trees callTrees, files []string) {
	for _, tree := range trees.Trees {
		for _, line := range strings.Split(tree.ASCII, "\n") {
			if line == "" || (line[0] != '+' && line[0] != '-') {
				continue
			}
			location := line[1:]
			match := callSite.FindStringSubmatchIndex(location)
			if match == nil {
				continue
			}
			lineNumber, err := strconv.Atoi(location[match[2]:match[3]])
			if err != nil {
				continue
			}
			path, ok := requestedPath(location[:match[0]], files)
			if !ok {
				continue
			}
			edge := diffview.Edge{Line: lineNumber, Added: line[0] == '+'}
			if seen[path] == nil {
				seen[path] = map[diffview.Edge]bool{}
			}
			if !seen[path][edge] {
				seen[path][edge] = true
				out[path] = append(out[path], edge)
			}
		}
	}
}

func requestedPath(location string, files []string) (string, bool) {
	matched := ""
	for _, path := range files {
		if len(path) > len(matched) && strings.HasSuffix(location, path) {
			matched = path
		}
	}
	return matched, matched != ""
}

func comparisonRefs(request provider.Request) (string, string, error) {
	if !request.Staged {
		return request.From, request.To, nil
	}
	if request.To != "" {
		return "", "", errors.New("staged comparisons accept at most one revision")
	}
	index, err := git.Output(request.Directory, "write-tree")
	if err != nil {
		return "", "", fmt.Errorf("write index tree: %w", err)
	}
	base := request.From
	if base == "" {
		if _, headErr := git.Output(request.Directory, "rev-parse", "--verify", "HEAD"); headErr == nil {
			base = "HEAD"
		} else {
			empty, emptyErr := git.Output(request.Directory, "hash-object", "-t", "tree", "/dev/null")
			if emptyErr != nil {
				return "", "", fmt.Errorf("create empty base tree: %w", emptyErr)
			}
			base, err = snapshotCommit(request.Directory, strings.TrimSpace(empty), "")
			if err != nil {
				return "", "", fmt.Errorf("create empty base commit: %w", err)
			}
		}
	}
	base, err = commitObject(request.Directory, base)
	if err != nil {
		return "", "", fmt.Errorf("resolve staged base %q: %w", request.From, err)
	}
	staged, err := snapshotCommit(request.Directory, strings.TrimSpace(index), base)
	if err != nil {
		return "", "", fmt.Errorf("create staged commit: %w", err)
	}
	return base, staged, nil
}

func commitObject(directory, revision string) (string, error) {
	if git.Succeeds(directory, "rev-parse", "--verify", "--quiet", revision+"^{commit}") {
		return revision, nil
	}
	tree, err := git.Output(directory, "rev-parse", "--verify", revision+"^{tree}")
	if err != nil {
		return "", err
	}
	return snapshotCommit(directory, strings.TrimSpace(tree), "")
}

func snapshotCommit(directory, tree, parent string) (string, error) {
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-m", "Changes staged snapshot")
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = snapshotEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func snapshotEnvironment() []string {
	blocked := map[string]bool{
		"GIT_AUTHOR_NAME":     true,
		"GIT_AUTHOR_EMAIL":    true,
		"GIT_AUTHOR_DATE":     true,
		"GIT_COMMITTER_NAME":  true,
		"GIT_COMMITTER_EMAIL": true,
		"GIT_COMMITTER_DATE":  true,
	}
	environment := make([]string, 0, len(os.Environ())+6)
	for _, entry := range git.CleanEnv() {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"GIT_AUTHOR_NAME=Changes",
		"GIT_AUTHOR_EMAIL=changes@example.invalid",
		"GIT_AUTHOR_DATE=1970-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=Changes",
		"GIT_COMMITTER_EMAIL=changes@example.invalid",
		"GIT_COMMITTER_DATE=1970-01-01T00:00:00Z",
	)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
