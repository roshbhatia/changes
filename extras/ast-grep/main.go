// changes-provider-ast-grep adds symbol ranges to the Changes provider protocol.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/diffview"
)

type outlineFile struct {
	Path  string `json:"path"`
	Items []struct {
		SymbolType string `json:"symbolType"`
		Name       string `json:"name"`
		Signature  string `json:"signature"`
		IsImport   bool   `json:"isImport"`
		Range      struct {
			Start struct {
				Line int `json:"line"`
			} `json:"start"`
			End struct {
				Line int `json:"line"`
			} `json:"end"`
		} `json:"range"`
	} `json:"items"`
}

func main() {
	request := provider.Request{}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fail(err)
	}
	if request.Version != provider.ProtocolVersion || request.Action != provider.ActionSymbols {
		fail(fmt.Errorf("unsupported request %q %q", request.Version, request.Action))
	}
	spec := source.Spec{
		Dir: request.Directory, From: request.From, Staged: request.Staged, To: request.To,
	}
	root, files, done := spec.Tree(request.Files)
	defer done()
	symbols, err := outline(root, files)
	if err != nil {
		fail(err)
	}
	response := provider.Response{Version: provider.ProtocolVersion, Symbols: symbols}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fail(err)
	}
}

func outline(root string, paths []string) (map[string][]diffview.Symbol, error) {
	out := map[string][]diffview.Symbol{}
	if len(paths) == 0 {
		return out, nil
	}
	command := exec.Command("ast-grep", append([]string{"outline", "--json=compact"}, paths...)...)
	command.Dir = root
	blob, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ast-grep outline: %s: %w", strings.TrimSpace(string(blob)), err)
	}
	files := []outlineFile{}
	if err := json.Unmarshal(blob, &files); err != nil {
		return nil, fmt.Errorf("decode ast-grep outline: %w", err)
	}
	for _, file := range files {
		key := file.Path
		if filepath.IsAbs(key) {
			if relative, err := filepath.Rel(root, key); err == nil {
				key = relative
			}
		}
		for _, item := range file.Items {
			if item.IsImport {
				continue
			}
			name := strings.TrimSpace(strings.TrimSuffix(strings.Join(strings.Fields(item.Signature), " "), "{"))
			if name == "" {
				name = item.Name
			}
			out[key] = append(out[key], diffview.Symbol{
				Kind: item.SymbolType, Name: name,
				From: item.Range.Start.Line + 1, To: item.Range.End.Line + 1,
			})
		}
	}
	return out, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
