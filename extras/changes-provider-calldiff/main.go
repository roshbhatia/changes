// changes-provider-calldiff adds call edges to the Changes provider protocol.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/go-utils/diffview"
)

type callTrees struct {
	Trees []struct {
		ASCII string `json:"ascii"`
	} `json:"trees"`
}

var callSite = regexp.MustCompile(`([^\s]+):(\d+)(?:-\d+)?\s*$`)

func main() {
	request := provider.Request{}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fail(err)
	}
	response := provider.Response{Edges: calls(request)}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fail(err)
	}
}

func calls(request provider.Request) map[string][]diffview.Edge {
	out := map[string][]diffview.Edge{}
	args := []string{"diff"}
	if request.From != "" {
		args = append(args, request.From)
	}
	if request.To != "" {
		args = append(args, request.To)
	}
	args = append(args, "--locs", "--format", "json")
	args = append(args, request.Files...)
	command := exec.Command("calldiff", args...)
	command.Dir = request.Directory
	blob, err := command.Output()
	if err != nil {
		return out
	}
	trees := callTrees{}
	if json.Unmarshal(blob, &trees) != nil {
		return out
	}
	seen := map[string]map[diffview.Edge]bool{}
	for _, tree := range trees.Trees {
		for _, line := range strings.Split(tree.ASCII, "\n") {
			if line == "" || (line[0] != '+' && line[0] != '-') {
				continue
			}
			match := callSite.FindStringSubmatch(line[1:])
			if match == nil {
				continue
			}
			lineNumber, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			path := match[1]
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
	return out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
