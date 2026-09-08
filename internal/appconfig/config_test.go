package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultUsesBuiltinWithoutProvidersOrDifftool(t *testing.T) {
	t.Parallel()
	configured := Default()
	if configured.Diff.Engine != "builtin" || len(configured.Diff.Filter) != 0 || len(configured.Diff.Difftool) != 0 || configured.Providers.Directory != "" {
		t.Fatalf("default config = %+v", configured)
	}
}

func TestLoadYAMLAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `diff:
  engine: filter
  filter: [review-patch, --no-pager]
  difftool: [review-files, $LOCAL, $REMOTE]
  layout: unified
color: auto
providers:
  cacheMaxEntries: 32
  cacheTtl: 30m
  directory: /tmp/providers
  timeout: 15s
notes:
  editor: [nvim, --clean, $FILE]
  generator: yaml-generator
  refreshInterval: 12s
  store: yaml-store
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHANGES_DIFF_LAYOUT", "side-by-side")
	t.Setenv("CHANGES_PROVIDERS_DIRECTORY", "/tmp/environment-providers")
	t.Setenv("CHANGES_PROVIDERS_CACHE_TTL", "45m")
	t.Setenv("CHANGES_NOTES_GENERATOR", "environment-generator")
	t.Setenv("CHANGES_NOTES_STORE", "environment-store")
	configured, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Diff.Engine != "filter" || configured.Diff.Layout != "side-by-side" || configured.Providers.Directory != "/tmp/environment-providers" {
		t.Fatalf("loaded config = %+v", configured)
	}
	if configured.Diff.Filter[0] != "review-patch" || configured.Diff.Difftool[0] != "review-files" {
		t.Fatalf("loaded diff commands = %+v", configured.Diff)
	}
	if configured.Providers.CacheMaxEntries != 32 || configured.Providers.CacheTTL.Duration() != 45*time.Minute {
		t.Fatalf("loaded cache config = %+v", configured.Providers)
	}
	if len(configured.Notes.Editor) != 3 || configured.Notes.Editor[0] != "nvim" ||
		configured.Notes.Generator != "environment-generator" || configured.Notes.Store != "environment-store" ||
		configured.Notes.RefreshInterval.Duration() != 12*time.Second {
		t.Fatalf("loaded notes config = %+v", configured.Notes)
	}
}

func TestSchemaIncludesProviders(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"$schema"`, `"providers"`, `"cacheMaxEntries"`, `"cacheTtl"`, `"directory"`, `"filter"`, `"difftool"`, `"notes"`, `"editor"`, `"generator"`, `"refreshInterval"`, `"store"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("schema omits %s", want)
		}
	}
}

// go-utils v0.12.0 dropped Duration's JSONSchema method, so without the field
// tags every duration would reflect as an integer.
func TestSchemaKeepsDurationsAsStrings(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions map[string]struct {
			Properties map[string]struct {
				Type    string `json:"type"`
				Pattern string `json:"pattern"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	for _, field := range [][2]string{
		{"Interactive", "cacheTtl"}, {"Notes", "generatorTimeout"}, {"Notes", "refreshInterval"},
		{"Providers", "cacheTtl"}, {"Providers", "timeout"},
	} {
		got := schema.Definitions[field[0]].Properties[field[1]]
		if got.Type != "string" || !strings.Contains(got.Pattern, "(ns|us|µs|ms|s|m|h)") {
			t.Fatalf("%s.%s schema = %+v", field[0], field[1], got)
		}
	}
}
