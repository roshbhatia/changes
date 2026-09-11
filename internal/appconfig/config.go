// Package appconfig defines the public Changes configuration surface.
package appconfig

import (
	"time"

	sharedconfig "github.com/roshbhatia/go-utils/config"
	providerlib "github.com/roshbhatia/go-utils/provider"
)

// Config is loaded from YAML, then overridden by CHANGES_* environment values.
type Config struct {
	Color       string      `json:"color,omitempty" yaml:"color" jsonschema:"enum=auto,enum=always,enum=never"`
	Diff        Diff        `json:"diff,omitempty" yaml:"diff"`
	Interactive Interactive `json:"interactive,omitempty" yaml:"interactive"`
	Notes       Notes       `json:"notes,omitempty" yaml:"notes"`
	Providers   Providers   `json:"providers,omitempty" yaml:"providers"`
}

type Interactive struct {
	Reader          []string             `json:"reader,omitempty" yaml:"reader,omitempty"`
	CacheMaxEntries int                  `json:"cacheMaxEntries,omitempty" yaml:"cacheMaxEntries,omitempty" jsonschema:"minimum=0"`
	CacheTTL        providerlib.Duration `json:"cacheTtl,omitempty" yaml:"cacheTtl,omitempty"`
	Dock            string               `json:"dock,omitempty" yaml:"dock" jsonschema:"enum=left,enum=bottom"`
	HistoryLimit    int                  `json:"historyLimit,omitempty" yaml:"historyLimit,omitempty" jsonschema:"minimum=1"`
	Navigator       string               `json:"navigator,omitempty" yaml:"navigator" jsonschema:"enum=tree,enum=list"`
	NoteInput       string               `json:"noteInput,omitempty" yaml:"noteInput" jsonschema:"enum=popup,enum=editor"`
	Progress        bool                 `json:"progress" yaml:"progress"`
}

// Notes configures interactive note authoring. The command receives the draft
// file through $FILE, or as its final argument when the placeholder is absent.
type Notes struct {
	Editor           []string             `json:"editor,omitempty" yaml:"editor"`
	Generator        string               `json:"generator,omitempty" yaml:"generator,omitempty"`
	GeneratorTimeout providerlib.Duration `json:"generatorTimeout,omitempty" yaml:"generatorTimeout,omitempty"`
	RefreshInterval  providerlib.Duration `json:"refreshInterval,omitempty" yaml:"refreshInterval,omitempty"`
	Store            string               `json:"store,omitempty" yaml:"store,omitempty"`
}

// Providers controls external provider discovery and execution.
type Providers struct {
	CacheMaxEntries int                  `json:"cacheMaxEntries,omitempty" yaml:"cacheMaxEntries,omitempty" jsonschema:"minimum=0"`
	CacheTTL        providerlib.Duration `json:"cacheTtl,omitempty" yaml:"cacheTtl,omitempty"`
	Directory       string               `json:"directory,omitempty" yaml:"directory,omitempty"`
	Group           string               `json:"group,omitempty" yaml:"group,omitempty"`
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
		Interactive: Interactive{
			CacheMaxEntries: 64,
			CacheTTL:        providerlib.Duration(24 * time.Hour),
			Dock:            "left",
			HistoryLimit:    50,
			Navigator:       "tree",
			NoteInput:       "popup",
			Progress:        true,
		},
		Notes: Notes{
			GeneratorTimeout: providerlib.Duration(5 * time.Minute),
			RefreshInterval:  providerlib.Duration(30 * time.Second),
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
