// Package appconfig defines the public Changes configuration surface.
package appconfig

import (
	"github.com/roshbhatia/changes/internal/provider"
	sharedconfig "github.com/roshbhatia/go-utils/config"
)

// Config is loaded from YAML, then overridden by CHANGES_* environment values.
type Config struct {
	Color     string              `json:"color,omitempty" yaml:"color" jsonschema:"enum=auto,enum=always,enum=never"`
	Diff      Diff                `json:"diff,omitempty" yaml:"diff"`
	Providers []provider.Manifest `json:"providers,omitempty" yaml:"providers"`
}

// Diff selects the built-in renderer or a generic patch command.
type Diff struct {
	Command []string `json:"command,omitempty" yaml:"command"`
	Engine  string   `json:"engine,omitempty" yaml:"engine" jsonschema:"enum=git,enum=internal,enum=command"`
	Layout  string   `json:"layout,omitempty" yaml:"layout" jsonschema:"enum=unified,enum=side-by-side"`
}

// Default returns the dependency-free Git configuration.
func Default() Config {
	return Config{
		Color: "auto",
		Diff: Diff{
			Engine: "git",
			Layout: "unified",
		},
	}
}

// Load applies ~/.config/changes/config.yaml and CHANGES_* overrides.
func Load(path string) (Config, error) {
	return sharedconfig.Load(Default(), sharedconfig.Options{
		Name: "changes", EnvPrefix: "CHANGES", Path: path,
	})
}

// Schema emits the configuration schema from the same Go types.
func Schema() ([]byte, error) {
	return sharedconfig.Schema[Config]("Changes configuration")
}
