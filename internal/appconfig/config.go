// Package appconfig defines the public Changes configuration surface.
package appconfig

import (
	"time"

	sharedconfig "github.com/roshbhatia/go-utils/config"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

// Config is loaded from YAML, then overridden by CHANGES_* environment values.
type Config struct {
	Color     string    `json:"color,omitempty" yaml:"color" jsonschema:"enum=auto,enum=always,enum=never"`
	Diff      Diff      `json:"diff,omitempty" yaml:"diff"`
	Providers Providers `json:"providers,omitempty" yaml:"providers"`
}

// Providers controls external provider discovery and execution.
type Providers struct {
	CacheMaxEntries int                  `json:"cacheMaxEntries,omitempty" yaml:"cacheMaxEntries,omitempty" jsonschema:"minimum=0"`
	CacheTTL        providerlib.Duration `json:"cacheTtl,omitempty" yaml:"cacheTtl,omitempty"`
	Directory       string               `json:"directory,omitempty" yaml:"directory,omitempty"`
	Timeout         providerlib.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// Diff configures patch display and Git-compatible file comparison separately.
type Diff struct {
	Difftool []string `json:"difftool,omitempty" yaml:"difftool"`
	Engine   string   `json:"engine,omitempty" yaml:"engine" jsonschema:"enum=builtin,enum=filter"`
	Filter   []string `json:"filter,omitempty" yaml:"filter"`
	Layout   string   `json:"layout,omitempty" yaml:"layout" jsonschema:"enum=unified,enum=side-by-side"`
}

// Default returns the dependency-free Git configuration.
func Default() Config {
	return Config{
		Color: "auto",
		Diff: Diff{
			Engine: "builtin",
			Layout: "unified",
		},
		Providers: Providers{
			CacheMaxEntries: 256,
			CacheTTL:        providerlib.Duration(time.Hour),
			Timeout:         providerlib.Duration(20 * time.Second),
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
