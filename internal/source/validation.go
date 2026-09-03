package source

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/roshbhatia/go-utils/git"
)

// ValidationFixture creates an isolated repository with one changed call.
func ValidationFixture() (string, error) {
	directory, err := os.MkdirTemp("", "changes-provider-validation-")
	if err != nil {
		return "", err
	}
	cleanup := func(err error) (string, error) {
		_ = os.RemoveAll(directory)
		return "", err
	}
	initial := `function ready(): boolean {
  return false
}

function main(): void {}
`
	changed := `function ready(): boolean {
  return true
}

function main(): void {
  ready()
}
`
	path := filepath.Join(directory, "main.ts")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return cleanup(err)
	}
	commands := [][]string{
		{"init", "--quiet"},
		{"config", "user.name", "Changes validation"},
		{"config", "user.email", "changes@example.invalid"},
		{"add", "main.ts"},
		{"commit", "--quiet", "-m", "validation fixture"},
	}
	for _, arguments := range commands {
		if err := git.Run(directory, arguments...); err != nil {
			return cleanup(fmt.Errorf("prepare validation fixture: %w", err))
		}
	}
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		return cleanup(err)
	}
	return directory, nil
}
