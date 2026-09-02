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
	Description  string   `json:"description,omitempty" yaml:"description"`
	Requires     []string `json:"requires,omitempty" yaml:"requires"`
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

// ValidationCheck describes one provider contract check.
type ValidationCheck struct {
	Message string `json:"message"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

// Validation is the complete result for one configured provider.
type Validation struct {
	Capabilities []string          `json:"capabilities"`
	Checks       []ValidationCheck `json:"checks"`
	Command      []string          `json:"command,omitempty"`
	Description  string            `json:"description,omitempty"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
}

func validationCheck(name, message string, ok bool) ValidationCheck {
	status := "failed"
	if ok {
		status = "ok"
	}
	return ValidationCheck{Name: name, Message: message, Status: status}
}

// Validate runs one provider against a synthetic working tree and verifies its
// JSON response without reading or changing the caller's repository.
func Validate(ctx context.Context, config Manifest) Validation {
	result := Validation{
		Capabilities: append([]string{}, config.Capabilities...),
		Description:  config.Description,
		Name:         config.Name,
	}
	manifestOK := config.Name != "" && len(config.Command) > 0 && len(config.Capabilities) > 0
	for _, capability := range config.Capabilities {
		if capability != CapabilityCalls && capability != CapabilitySymbols {
			manifestOK = false
		}
	}
	result.Checks = append(result.Checks, validationCheck(
		"manifest", "name, command, and supported capabilities are valid", manifestOK,
	))
	if !manifestOK {
		result.Status = "failed"
		return result
	}
	executable, err := exec.LookPath(config.Command[0])
	if err != nil {
		result.Checks = append(result.Checks, validationCheck("executable", err.Error(), false))
		result.Status = "failed"
		return result
	}
	resolved := config
	resolved.Command = append([]string{executable}, config.Command[1:]...)
	result.Command = append([]string{}, resolved.Command...)
	result.Checks = append(result.Checks, validationCheck("executable", "resolved "+executable, true))
	for _, dependency := range config.Requires {
		resolvedDependency, err := exec.LookPath(dependency)
		if err != nil {
			result.Checks = append(result.Checks, validationCheck("dependency", dependency+" is unavailable", false))
			result.Status = "failed"
			return result
		}
		result.Checks = append(result.Checks, validationCheck("dependency", "resolved "+resolvedDependency, true))
	}
	directory, err := os.MkdirTemp("", "changes-provider-validation-")
	if err != nil {
		result.Checks = append(result.Checks, validationCheck("fixture", err.Error(), false))
		result.Status = "failed"
		return result
	}
	defer func() { _ = os.RemoveAll(directory) }()
	if err := os.WriteFile(filepath.Join(directory, "validation.go"), []byte("package validation\n\nfunc Ready() bool { return true }\n"), 0o600); err != nil {
		result.Checks = append(result.Checks, validationCheck("fixture", err.Error(), false))
		result.Status = "failed"
		return result
	}
	payload, _ := json.Marshal(Request{
		Directory: directory, Files: []string{"validation.go"}, Fingerprint: "provider-validation",
	})
	if _, err := execute(ctx, resolved, payload); err != nil {
		result.Checks = append(result.Checks, validationCheck("protocol", err.Error(), false))
		result.Status = "failed"
		return result
	}
	result.Checks = append(result.Checks, validationCheck("protocol", "accepted a synthetic request and returned valid JSON", true))
	result.Status = "ok"
	return result
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
	response, err := execute(ctx, config, payload)
	if err != nil {
		return Response{}, err
	}
	if cachePath != "" {
		encoded, _ := json.Marshal(response)
		_ = writeResult(cachePath, encoded)
	}
	return response, nil
}

func execute(ctx context.Context, config Manifest, payload []byte) (Response, error) {
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
