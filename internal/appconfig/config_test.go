package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadYAMLAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "diff:\n  engine: command\n  command: [delta, --paging=never]\n  layout: unified\ncolor: auto\nproviders:\n  directory: /tmp/providers\n  timeout: 15s\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHANGES_DIFF_LAYOUT", "side-by-side")
	t.Setenv("CHANGES_PROVIDERS_DIRECTORY", "/tmp/environment-providers")
	configured, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Diff.Engine != "command" || configured.Diff.Layout != "side-by-side" || configured.Providers.Directory != "/tmp/environment-providers" {
		t.Fatalf("loaded config = %+v", configured)
	}
}

func TestSchemaIncludesProviders(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"$schema"`, `"providers"`, `"directory"`, `"command"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("schema omits %s", want)
		}
	}
}
