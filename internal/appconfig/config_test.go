package appconfig

import (
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
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHANGES_DIFF_LAYOUT", "side-by-side")
	t.Setenv("CHANGES_PROVIDERS_DIRECTORY", "/tmp/environment-providers")
	t.Setenv("CHANGES_PROVIDERS_CACHE_TTL", "45m")
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
}

func TestSchemaIncludesProviders(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"$schema"`, `"providers"`, `"cacheMaxEntries"`, `"cacheTtl"`, `"directory"`, `"filter"`, `"difftool"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("schema omits %s", want)
		}
	}
}
