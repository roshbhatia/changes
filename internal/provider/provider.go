// Package provider runs optional change-analysis commands through one JSON contract.
package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roshbhatia/go-utils/diffview"
)

const (
	CapabilityCalls   = "calls"
	CapabilitySymbols = "symbols"
)

// Manifest declares one external provider and the capabilities it advertises.
type Manifest struct {
	Name         string   `json:"name" yaml:"name" jsonschema:"required"`
	Command      []string `json:"command" yaml:"command" jsonschema:"required,minItems=1"`
	Capabilities []string `json:"capabilities" yaml:"capabilities" jsonschema:"required,minItems=1"`
}

// Request describes one repository comparison without naming an implementation.
type Request struct {
	Directory   string   `json:"directory"`
	Files       []string `json:"files"`
	From        string   `json:"from,omitempty"`
	Fingerprint string   `json:"fingerprint"`
	Staged      bool     `json:"staged,omitempty"`
	To          string   `json:"to,omitempty"`
}

// Response carries any analysis layers produced by a provider.
type Response struct {
	Edges   map[string][]diffview.Edge   `json:"edges,omitempty"`
	Symbols map[string][]diffview.Symbol `json:"symbols,omitempty"`
}

// Supports reports whether a provider advertises one capability.
func (config Manifest) Supports(capability string) bool {
	for _, candidate := range config.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

// Run sends one request on stdin and decodes one response from stdout.
func Run(ctx context.Context, config Manifest, request Request) (Response, error) {
	if config.Name == "" {
		return Response{}, errors.New("provider name is required")
	}
	if len(config.Command) == 0 {
		return Response{}, fmt.Errorf("provider %s has no command", config.Name)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	cachePath, err := resultPath(config, payload)
	if err == nil {
		if cached, readErr := os.ReadFile(cachePath); readErr == nil {
			response := Response{}
			if json.Unmarshal(cached, &response) == nil {
				return response, nil
			}
		}
	}
	command := exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	command.Env = os.Environ()
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return Response{}, fmt.Errorf("provider %s: %s: %w", config.Name, message, err)
		}
		return Response{}, fmt.Errorf("provider %s: %w", config.Name, err)
	}
	response := Response{}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return Response{}, fmt.Errorf("provider %s returned invalid JSON: %w", config.Name, err)
	}
	if cachePath != "" {
		_ = writeResult(cachePath, stdout.Bytes())
	}
	return response, nil
}

func resultPath(config Manifest, payload []byte) (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	identity := struct {
		Command []string `json:"command"`
		Input   []byte   `json:"input"`
		Name    string   `json:"name"`
	}{config.Command, payload, config.Name}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return filepath.Join(root, "changes", "providers", digest+".json"), nil
}

func writeResult(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".provider-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
