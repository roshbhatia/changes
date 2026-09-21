package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/roshbhatia/go-utils/diffview"
)

// Zero-width markers preserve exact file locations through grouping and note insertion.
func markFileLocations(options diffview.Options, insertions []noteInsertion) (diffview.Options, map[string]string) {
	options.Files = slices.Clone(options.Files)
	options.Symbols = maps.Clone(options.Symbols)
	options.Edges = maps.Clone(options.Edges)
	if options.Symbols == nil {
		options.Symbols = map[string][]diffview.Symbol{}
	}
	if options.Edges == nil {
		options.Edges = map[string][]diffview.Edge{}
	}
	anchors := map[string]string{}
	for index := range options.Files {
		original := options.Files[index].Path
		marker := fmt.Sprintf("\x1b]changes-file=%d\x07", index)
		marked := original + marker
		anchors[marker] = cleanMarkedPath(original, insertions)
		options.Files[index].Path = marked
		options.Symbols[marked] = options.Symbols[original]
		options.Edges[marked] = options.Edges[original]
	}
	return options, anchors
}

func collectFileLocations(body string, anchors map[string]string, rows map[string]int) string {
	lines := strings.Split(body, "\n")
	for row, line := range lines {
		for marker, path := range anchors {
			if strings.Contains(line, marker) {
				if previous, ok := rows[path]; !ok || row < previous {
					rows[path] = row
				}
				line = strings.ReplaceAll(line, marker, "")
			}
		}
		lines[row] = line
	}
	return strings.Join(lines, "\n")
}

func cleanMarkedPath(path string, insertions []noteInsertion) string {
	for _, insertion := range insertions {
		path = strings.ReplaceAll(path, insertion.marker, "")
	}
	return path
}
