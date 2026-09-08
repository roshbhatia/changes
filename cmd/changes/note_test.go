package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roshbhatia/changes/internal/provider"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/completion"
)

func TestNoteLineInPatchFindsBothSidesAndRejectsHiddenLines(t *testing.T) {
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,2 +2,2 @@\n-old\n+new\n same\n"
	for _, test := range []struct {
		side string
		line int
		want string
	}{
		{side: provider.NoteSideLeft, line: 2, want: "old"},
		{side: provider.NoteSideRight, line: 2, want: "new"},
		{side: provider.NoteSideLeft, line: 3, want: "same"},
		{side: provider.NoteSideRight, line: 3, want: "same"},
	} {
		got, err := noteLineInPatch(patch, test.side, test.line)
		if err != nil || got != test.want {
			t.Fatalf("%s:%d = %q, %v; want %q", test.side, test.line, got, err, test.want)
		}
	}
	if _, err := noteLineInPatch(patch, provider.NoteSideRight, 1); err == nil {
		t.Fatal("line outside the rendered diff was accepted")
	}
	if _, err := noteRangeInPatch(patch, provider.NoteSideRight, 1, 2); err == nil {
		t.Fatal("range with a hidden start line was accepted")
	}
}

func TestNoteRangeInFilePatchUsesTheSelectedFileAndSide(t *testing.T) {
	patch := "diff --git a/first.go b/first.go\n--- a/first.go\n+++ b/first.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -10 +10 @@\n-before\n+after\n"
	if _, err := noteRangeInFilePatch(patch, "main.go", provider.NoteSideRight, 0, 1); err == nil {
		t.Fatal("a line visible only in another file was accepted")
	}
	got, err := noteRangeInFilePatch(patch, "main.go", provider.NoteSideRight, 0, 10)
	if err != nil || got != "after" {
		t.Fatalf("main.go:10 = %q, %v", got, err)
	}
	deleted := "diff --git a/old.go b/old.go\n--- a/old.go\n+++ /dev/null\n@@ -1 +0,0 @@\n--- not-a-header\n"
	got, err = noteRangeInFilePatch(deleted, "old.go", provider.NoteSideLeft, 0, 1)
	if err != nil || got != "-- not-a-header" {
		t.Fatalf("deleted old.go:1 = %q, %v", got, err)
	}
	quoted := "diff --git quoted\n--- \"a/space\\nname.go\"\n+++ \"b/space\\nname.go\"\n@@ -1 +1 @@\n-old\n+new\n"
	got, err = noteRangeInFilePatch(quoted, "space\nname.go", provider.NoteSideRight, 0, 1)
	if err != nil || got != "new" {
		t.Fatalf("quoted path:1 = %q, %v", got, err)
	}
	for name, patch := range map[string]string{
		"binary":       "diff --git a/blob.bin b/blob.bin\nindex 8352675..b842680 100644\nBinary files a/blob.bin and b/blob.bin differ\n",
		"binary space": "diff --git a/blob data.bin b/blob data.bin\nindex 8352675..b842680 100644\nBinary files a/blob data.bin and b/blob data.bin differ\n",
		"mode":         "diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\n",
		"rename":       "diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n",
	} {
		path := "blob.bin"
		if name == "binary space" {
			path = "blob data.bin"
		} else if name == "mode" {
			path = "script.sh"
		} else if name == "rename" {
			path = "new.go"
		}
		if _, err := noteRangeInFilePatch(patch, path, provider.NoteSideRight, 0, 0); err != nil {
			t.Fatalf("%s file note: %v", name, err)
		}
	}
	combined := "diff --cc conflict.go\nindex 1111111,2222222..3333333\n--- a/conflict.go\n+++ b/conflict.go\n@@@ -1,1 -1,1 +1,1 @@@\n++merged\n"
	if _, err := noteRangeInFilePatch(combined, "conflict.go", provider.NoteSideRight, 0, 0); err != nil {
		t.Fatalf("combined file note: %v", err)
	}
	if _, err := noteRangeInFilePatch(combined, "conflict.go", provider.NoteSideRight, 0, 1); err == nil || !strings.Contains(err.Error(), "file notes only") {
		t.Fatalf("combined line note error = %v", err)
	}
}

func TestNoteSpecUsesCanonicalRepositoryPath(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	spec, relative, err := noteSpec(repository, "main.go", noteComparisonFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if relative != "main.go" || len(spec.Paths) != 1 {
		t.Fatalf("spec = %+v, relative = %q", spec, relative)
	}
}

func TestStableComparisonRetriesMovingEndpoints(t *testing.T) {
	identities := [][2]string{{"base-a", "head-a"}, {"base-b", "head-b"}, {"base-b", "head-b"}, {"base-b", "head-b"}}
	reads := 0
	patches := 0
	patch, base, head, err := stableComparison(func() (resolvedNoteComparison, error) {
		identity := identities[reads]
		reads++
		return resolvedNoteComparison{
			spec: source.Spec{From: identity[0], To: identity[1]},
			base: identity[0], head: identity[1],
		}, nil
	}, func(source.Spec) (string, error) {
		patches++
		if patches > 2 {
			return "patch-3", nil
		}
		return fmt.Sprintf("patch-%d", patches), nil
	})
	if err != nil || patch != "patch-3" || base != "base-b" || head != "head-b" {
		t.Fatalf("stable comparison = %q %q %q, %v", patch, base, head, err)
	}

	reads = 0
	_, _, _, err = stableComparison(func() (resolvedNoteComparison, error) {
		reads++
		base := fmt.Sprintf("base-%d", reads)
		return resolvedNoteComparison{spec: source.Spec{From: base, To: "head"}, base: base, head: "head"}, nil
	}, func(source.Spec) (string, error) { return "patch", nil })
	if err == nil || !strings.Contains(err.Error(), "changed while capturing") {
		t.Fatalf("moving comparison error = %v", err)
	}
}

func TestStableExpectedComparisonRepeatsPatchAndDigestAsOneTuple(t *testing.T) {
	patches := []string{"patch-b", "patch-a", "patch-a", "patch-a"}
	patchRead := 0
	digestRead := 0
	patch, base, head, err := stableExpectedComparison(
		func() (resolvedNoteComparison, error) {
			return resolvedNoteComparison{
				spec: source.Spec{From: "base", To: "head"},
				base: "base", head: "head",
			}, nil
		},
		func(source.Spec) (string, error) {
			value := patches[patchRead]
			patchRead++
			return value, nil
		},
		func(source.Spec) (string, error) {
			digestRead++
			return "digest-a", nil
		},
		"digest-a",
	)
	if err != nil || patch != "patch-a" || base != "base" || head != "head" || patchRead != 4 || digestRead != 4 {
		t.Fatalf("stable tuple = %q %q %q, patch reads=%d digest reads=%d, %v", patch, base, head, patchRead, digestRead, err)
	}
}

func TestResolveNoteComparisonPinsRefsAndPreservesMutableSides(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "committed\n"})
	commit, err := noteGitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	committed, err := resolveNoteComparison(source.Spec{Dir: repository, From: "HEAD", To: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if committed.spec.From != commit || committed.spec.To != commit || committed.base != commit || committed.head != commit || !committed.spec.NoLazyFetch {
		t.Fatalf("resolved commit comparison = %+v", committed)
	}

	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("index\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "add", "main.go")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	staged, err := resolveNoteComparison(source.Spec{Dir: repository, Staged: true})
	if err != nil {
		t.Fatal(err)
	}
	if !staged.spec.Staged || staged.spec.From != commit || staged.spec.To != "" || staged.base != commit || staged.head != "" {
		t.Fatalf("resolved staged comparison = %+v", staged)
	}
	content, err := noteComparisonFile(staged.spec, "main.go", provider.NoteSideRight)
	if err != nil || string(content) != "index\n" {
		t.Fatalf("resolved staged right side = %q, %v", content, err)
	}

	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	working, err := resolveNoteComparison(source.Spec{Dir: repository})
	if err != nil {
		t.Fatal(err)
	}
	if working.spec.From != "" || working.spec.To != "" || working.spec.Staged || !strings.HasPrefix(working.base, "index:") || working.head != "" {
		t.Fatalf("resolved working comparison = %+v", working)
	}
	content, err = noteComparisonFile(working.spec, "main.go", provider.NoteSideRight)
	if err != nil || string(content) != "working\n" {
		t.Fatalf("resolved working right side = %q, %v", content, err)
	}
}

func TestNoteGitCommandIsolatesReplacementAndFSMonitorState(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "committed\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(t.TempDir(), "fsmonitor")
	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	script := "#!/bin/sh\n: > \"${CHANGES_FSMONITOR_MARKER:?}\"\nprintf '%s\\0' \"$2\"\n"
	if err := os.WriteFile(hook, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"config", "core.fsmonitor", hook}, {"config", "core.fsmonitorHookVersion", "2"}} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	t.Setenv("CHANGES_FSMONITOR_MARKER", marker)
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	t.Setenv("GIT_NO_REPLACE_OBJECTS", "0")
	t.Setenv("GIT_REPLACE_REF_BASE", "refs/replace/custom/")
	probe := exec.Command("git", "ls-files")
	probe.Dir = repository
	if output, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("fsmonitor probe: %v\n%s", err, output)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("configured fsmonitor did not run: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	command := noteGitCommand(repository, "ls-files")
	values := map[string]string{}
	for _, entry := range command.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	if values["GIT_NO_LAZY_FETCH"] != "1" || values["GIT_NO_REPLACE_OBJECTS"] != "1" {
		t.Fatalf("note Git environment = %#v", values)
	}
	if _, ok := values["GIT_REPLACE_REF_BASE"]; ok {
		t.Fatalf("note Git environment kept replacement ref base: %#v", values)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated ls-files: %v\n%s", err, output)
	}
	if _, err := noteGitOutput(repository, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := validateExpectedNoteFileDomain(source.Spec{Dir: repository}, "main.go", provider.NoteSideRight); err != nil {
		t.Fatal(err)
	}
	if _, err := noteComparisonFile(source.Spec{Dir: repository, From: "HEAD", To: "HEAD"}, "main.go", provider.NoteSideRight); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("note Git command ran configured fsmonitor: %v", err)
	}
}

func TestStableExpectedComparisonFreezesResolvedEndpointsAcrossABA(t *testing.T) {
	live := "A/B"
	resolveCalls := 0
	resolve := func() (resolvedNoteComparison, error) {
		resolveCalls++
		switch resolveCalls {
		case 1:
			if live != "A/B" {
				t.Fatalf("initial refs = %s", live)
			}
			live = "C/D"
		case 2:
			if live != "C/D" {
				t.Fatalf("moving refs = %s", live)
			}
			live = "A/B"
		default:
			t.Fatalf("unexpected resolution %d", resolveCalls)
		}
		return resolvedNoteComparison{
			spec: source.Spec{From: "oid-a", To: "oid-b"},
			base: "oid-a", head: "oid-b",
		}, nil
	}
	readPatch := func(spec source.Spec) (string, error) {
		if live != "C/D" || spec.From != "oid-a" || spec.To != "oid-b" {
			t.Fatalf("patch read used live refs: live=%s spec=%+v", live, spec)
		}
		return "patch-a-b", nil
	}
	readDigest := func(spec source.Spec) (string, error) {
		if live != "C/D" || spec.From != "oid-a" || spec.To != "oid-b" {
			t.Fatalf("digest read used live refs: live=%s spec=%+v", live, spec)
		}
		return "digest-b", nil
	}

	patch, base, head, err := stableExpectedComparison(resolve, readPatch, readDigest, "digest-b")
	if err != nil || patch != "patch-a-b" || base != "oid-a" || head != "oid-b" || live != "A/B" {
		t.Fatalf("stable ABA tuple = %q %q %q, live=%s, %v", patch, base, head, live, err)
	}
}

func TestNoteComparisonCaptureDoesNotLazyFetchPromisorObjects(t *testing.T) {
	sourceRepository := t.TempDir()
	prepareRepository(t, sourceRepository, map[string]string{"main.go": "old\n"})
	for _, contents := range []string{"new\n", "new\n"} {
		if err := os.WriteFile(filepath.Join(sourceRepository, "main.go"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		arguments := []string{"commit", "--quiet", "--allow-empty", "-am", "fixture"}
		command := exec.Command("git", arguments...)
		command.Dir = sourceRepository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	gitOutput := func(directory string, arguments ...string) string {
		t.Helper()
		command := exec.Command("git", arguments...)
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	base := gitOutput(sourceRepository, "rev-parse", "HEAD~2")
	changed := gitOutput(sourceRepository, "rev-parse", "HEAD~1")
	head := gitOutput(sourceRepository, "rev-parse", "HEAD")
	missingBlob := gitOutput(sourceRepository, "rev-parse", "HEAD:main.go")

	bareRepository := filepath.Join(t.TempDir(), "origin.git")
	gitOutput(t.TempDir(), "clone", "--quiet", "--bare", sourceRepository, bareRepository)
	gitOutput(bareRepository, "config", "uploadpack.allowFilter", "true")

	tests := []struct {
		name    string
		capture func(string) error
	}{
		{
			name: "note list",
			capture: func(repository string) error {
				_, err := stableNoteSnapshot(source.Spec{Dir: repository, From: base, To: changed})
				return err
			},
		},
		{
			name: "note add",
			capture: func(repository string) error {
				if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("new\n"), 0o600); err != nil {
					return err
				}
				expected := fmt.Sprintf("%x", sha256.Sum256([]byte("new\n")))
				_, _, _, _, err := stableNoteComparison(
					source.Spec{Dir: repository, From: changed, To: head},
					"main.go", provider.NoteSideRight, expected,
				)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := filepath.Join(t.TempDir(), "partial")
			gitOutput(t.TempDir(), "-c", "protocol.file.allow=always", "clone", "--quiet", "--filter=blob:none", "--no-checkout", "file://"+bareRepository, repository)
			if _, err := noteGitOutput(repository, "cat-file", "-e", missingBlob); err == nil {
				t.Fatal("filtered clone contains the promised blob")
			}

			probe := filepath.Join(t.TempDir(), "remote-accessed")
			helper := filepath.Join(t.TempDir(), "remote-helper")
			script := fmt.Sprintf("#!/bin/sh\n: > %q\nexit 1\n", probe)
			if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			gitOutput(repository, "config", "protocol.ext.allow", "always")
			gitOutput(repository, "remote", "set-url", "origin", "ext::"+helper)

			if err := test.capture(repository); err == nil {
				t.Fatal("comparison unexpectedly succeeded with a missing promised object")
			}
			if _, err := os.Stat(probe); !os.IsNotExist(err) {
				t.Fatalf("comparison accessed the promisor remote: %v", err)
			}
			if _, err := noteGitOutput(repository, "cat-file", "-e", missingBlob); err == nil {
				t.Fatal("comparison fetched the promised blob")
			}
		})
	}
}

func TestExpectedNoteFileMatchesSelectedComparisonSide(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "base\n"})
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("index\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "add", "main.go")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	if err := os.WriteFile(path, []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	digest := func(value string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
	}
	tests := []struct {
		name string
		spec source.Spec
		side string
		want string
	}{
		{name: "working right", spec: source.Spec{Dir: repository}, side: provider.NoteSideRight, want: "working\n"},
		{name: "working left", spec: source.Spec{Dir: repository}, side: provider.NoteSideLeft, want: "index\n"},
		{name: "staged right", spec: source.Spec{Dir: repository, Staged: true}, side: provider.NoteSideRight, want: "index\n"},
		{name: "staged left", spec: source.Spec{Dir: repository, Staged: true}, side: provider.NoteSideLeft, want: "base\n"},
		{name: "commit right", spec: source.Spec{Dir: repository, From: "HEAD", To: "HEAD"}, side: provider.NoteSideRight, want: "base\n"},
		{name: "commit left", spec: source.Spec{Dir: repository, From: "HEAD", To: "HEAD"}, side: provider.NoteSideLeft, want: "base\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := noteComparisonFile(test.spec, "main.go", test.side)
			if err != nil || string(content) != test.want {
				t.Fatalf("selected content = %q, %v; want %q", content, err, test.want)
			}
			if err := validateExpectedNoteFile(test.spec, "main.go", test.side, digest(test.want)); err != nil {
				t.Fatalf("matching digest: %v", err)
			}
			if err := validateExpectedNoteFile(test.spec, "main.go", test.side, digest("other\n")); err == nil {
				t.Fatal("mismatched digest was accepted")
			}
		})
	}
}

func TestExpectedNoteFileRejectsUnsupportedWorkingByteDomains(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "base\n"})
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("base\n")))
	if err := os.WriteFile(filepath.Join(repository, ".gitattributes"), []byte("main.go filter=review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := source.Spec{Dir: repository}
	if err := validateExpectedNoteFile(spec, "main.go", provider.NoteSideRight, expected); err == nil || !strings.Contains(err.Error(), "unsupported Git filter conversion") {
		t.Fatalf("clean-filter error = %v", err)
	}

	link := filepath.Join(repository, "link.go")
	if err := os.Symlink("main.go", link); err != nil {
		t.Fatal(err)
	}
	if err := validateExpectedNoteFile(spec, "link.go", provider.NoteSideRight, expected); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestNoteAddRejectsInvalidExpectedFileDigest(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"note", "add", "--file", "main.go", "--message", "context", "--expected-file-sha256", "bad")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "must be a 64-character SHA-256 digest") {
		t.Fatalf("error = %v, output = %s", err, output)
	}
}

func TestNoteAddVerifiesCommitAndStagedSidesBeforeProviderCreation(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "base\n"})
	path := filepath.Join(repository, "main.go")
	if err := os.WriteFile(path, []byte("committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "main.go"}, {"commit", "--quiet", "-m", "committed"}} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}

	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes.create:
    description: create notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := func(value string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
	}
	run := func(arguments ...string) (string, error) {
		command := noteCLICommand(t, repository, arguments...)
		output, err := command.CombinedOutput()
		return string(output), err
	}
	baseArgs := []string{"note", "add", "--config", config, "--provider", "fake", "--file", "main.go", "--line", "1", "--message", "context"}

	output, err := run(append(baseArgs, "--commit", "HEAD", "--expected-file-sha256", digest("other\n"))...)
	if err == nil || !strings.Contains(output, "does not match the selected diff side") {
		t.Fatalf("commit mismatch = %v\n%s", err, output)
	}
	if _, statErr := os.Stat(capture); !os.IsNotExist(statErr) {
		t.Fatalf("provider ran for commit mismatch: %v", statErr)
	}
	if output, err = run(append(baseArgs, "--commit", "HEAD", "--expected-file-sha256", digest("committed\n"))...); err != nil {
		t.Fatalf("matching commit side: %v\n%s", err, output)
	}
	if err := os.Remove(capture); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("index\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "add", "main.go")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	if err := os.WriteFile(path, []byte("working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err = run(append(baseArgs, "--staged", "--expected-file-sha256", digest("working\n"))...)
	if err == nil || !strings.Contains(output, "does not match the selected diff side") {
		t.Fatalf("staged mismatch = %v\n%s", err, output)
	}
	if _, statErr := os.Stat(capture); !os.IsNotExist(statErr) {
		t.Fatalf("provider ran for staged mismatch: %v", statErr)
	}
	if output, err = run(append(baseArgs, "--staged", "--expected-file-sha256", digest("index\n"))...); err != nil {
		t.Fatalf("matching staged side: %v\n%s", err, output)
	}
}

func TestNoteAddUsesExactIndexPathsWithStageLikePrefixes(t *testing.T) {
	repository := t.TempDir()
	files := map[string]string{"main.go": "distractor\n"}
	for _, prefix := range []string{"0", "1", "2", "3"} {
		files[prefix+":main.go"] = "old\n"
	}
	prepareRepository(t, repository, files)

	want := map[string]string{}
	for _, prefix := range []string{"0", "1", "2", "3"} {
		name := prefix + ":main.go"
		want[name] = "selected-" + prefix + "\n"
		if err := os.WriteFile(filepath.Join(repository, name), []byte(want[name]), 0o600); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("git", "add", "--", name)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git add %s: %v\n%s", name, err, output)
		}
	}

	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes.create:
    description: create notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := func(value string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
	}
	distractorDigest := digest(files["main.go"])

	for _, prefix := range []string{"0", "1", "2", "3"} {
		name := prefix + ":main.go"
		baseArgs := []string{
			"note", "add", "--config", config, "--provider", "fake", "--staged",
			"--file", name, "--line", "1", "--message", "context",
		}
		output, err := noteCLICommand(t, repository, append(baseArgs, "--expected-file-sha256", distractorDigest)...).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "does not match the selected diff side") {
			t.Fatalf("%s accepted another index path: %v\n%s", name, err, output)
		}
		if _, statErr := os.Stat(capture); !os.IsNotExist(statErr) {
			t.Fatalf("provider ran for mismatched %s: %v", name, statErr)
		}

		output, err = noteCLICommand(t, repository, append(baseArgs, "--expected-file-sha256", digest(want[name]))...).CombinedOutput()
		if err != nil {
			t.Fatalf("matching %s: %v\n%s", name, err, output)
		}
		payload, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		var request provider.Request
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatal(err)
		}
		if request.Note == nil || request.Note.Anchor.Path != name || request.Note.Anchor.Context != strings.TrimSuffix(want[name], "\n") {
			t.Fatalf("%s note anchor = %+v", name, request.Note)
		}
		if err := os.Remove(capture); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNoteProviderSelectionErrorsNameTheCommandFlag(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	providers := t.TempDir()
	manifests := map[string]string{
		"generator.yaml": "version: provider/v1\nname: generator\ndescription: test\ncommand: [\"true\"]\nactions:\n  changes.notes.generate:\n    description: test\n",
		"writer-a.yaml":  "version: provider/v1\nname: writer-a\ndescription: test\ncommand: [\"true\"]\nactions:\n  changes.notes.create:\n    description: test\n",
		"writer-b.yaml":  "version: provider/v1\nname: writer-b\ndescription: test\ncommand: [\"true\"]\nactions:\n  changes.notes.create:\n    description: test\n",
	}
	for name, manifest := range manifests {
		if err := os.WriteFile(filepath.Join(providers, name), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := noteCLICommand(t, repository, "note", "add", "--config", config,
		"--file", "main.go", "--line", "1", "--message", "context").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "select one with --provider") || strings.Contains(string(output), "select one with --store") {
		t.Fatalf("note add selection error = %v\n%s", err, output)
	}

	output, err = noteCLICommand(t, repository, "note", "generate", "--config", config,
		"--provider", "generator").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "select one with --store") {
		t.Fatalf("note generate selection error = %v\n%s", err, output)
	}
}

func TestSplitNoteMessageOnlyStripsEditorComments(t *testing.T) {
	message := "Summary\n\nDetail\n# literal"
	summary, rationale := splitNoteMessage(message, false)
	if summary != "Summary" || rationale != "Detail\n# literal" {
		t.Fatalf("literal message = %q / %q", summary, rationale)
	}
	summary, rationale = splitNoteMessage(message, true)
	if summary != "Summary" || rationale != "Detail" {
		t.Fatalf("editor message = %q / %q", summary, rationale)
	}
}

func TestEditNoteUsesConfiguredFilePlaceholder(t *testing.T) {
	script := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Edited summary\\n\\nEdited detail\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	message, err := editNote([]string{script, "$FILE"})
	if err != nil {
		t.Fatal(err)
	}
	summary, rationale := splitNoteMessage(message, true)
	if summary != "Edited summary" || rationale != "Edited detail" {
		t.Fatalf("edited message = %q / %q", summary, rationale)
	}
}

func TestEditNoteParsesQuotedEnvironmentEditor(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "editor with spaces")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "editor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Environment editor\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", fmt.Sprintf("%q", script))
	t.Setenv("EDITOR", "")
	message, err := editNote(nil)
	if err != nil {
		t.Fatal(err)
	}
	if message != "Environment editor\n" {
		t.Fatalf("message = %q", message)
	}
}

func TestExplicitEmptyMessageFileDoesNotOpenEditor(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"note", "add", "--file", "main.go", "--message-file", empty)
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1", "VISUAL=false", "EDITOR=false")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "note message has no summary") {
		t.Fatalf("error = %v, output = %s", err, output)
	}
	command = exec.Command(os.Args[0], "-test.run=TestMainHelperProcess", "--",
		"note", "add", "--file", "main.go", "--message", "")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "GO_WANT_MAIN_HELPER=1", "VISUAL=false", "EDITOR=false")
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "note message has no summary") {
		t.Fatalf("empty --message error = %v, output = %s", err, output)
	}
}

func TestStableNoteSnapshotIgnoresDisplayPathFilter(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"first.go": "old\n", "second.go": "old\n"})
	for _, name := range []string{"first.go", "second.go"} {
		if err := os.WriteFile(filepath.Join(repository, name), []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	spec := source.Spec{Dir: repository, Paths: []string{filepath.Join(repository, "first.go")}}
	displayed, err := spec.Diff()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := stableNoteSnapshot(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(displayed, "second.go") || !strings.Contains(snapshot.patch, "second.go") {
		t.Fatalf("displayed = %q, comparison = %q", displayed, snapshot.patch)
	}
}

func TestValidateNoteVisibilityRejectsInvisibleExactLine(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "old\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:note",
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 999,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementExact,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err == nil {
		t.Fatal("invisible exact placement was accepted")
	}
	note.Placement.Line = 1
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatalf("visible exact placement: %v", err)
	}
}

func TestValidateNoteVisibilityChecksMixedSideRangeStart(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": ""})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:range",
		Placement: provider.NotePlacement{
			Path: "main.go", StartSide: provider.NoteSideLeft, StartLine: 1,
			Side: provider.NoteSideRight, Line: 2,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementExact,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err == nil {
		t.Fatal("mixed-side range with invisible start was accepted")
	}
}

func TestValidateNoteVisibilityDegradesInvisibleContextToFile(t *testing.T) {
	repository := t.TempDir()
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	prepareRepository(t, repository, map[string]string{"main.go": before})
	after := strings.Replace(before, "ten\n", "changed\n", 1)
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "fake:context",
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementContext,
		},
	}
	patch, err := (source.Spec{Dir: repository}).Diff()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatal(err)
	}
	if note.Placement.Quality != provider.PlacementFile || note.Placement.Line != 0 {
		t.Fatalf("degraded placement = %+v", note.Placement)
	}
}

func TestCommittedExactNoteHiddenByDiffContextRendersAtFileLevel(t *testing.T) {
	repository := t.TempDir()
	lines := make([]string, 20)
	for index := range lines {
		lines[index] = fmt.Sprintf("line %02d", index+1)
	}
	prepareRepository(t, repository, map[string]string{"main.go": strings.Join(lines, "\n") + "\n"})
	base, err := noteGitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	lines[19] = "changed line 20"
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "main.go"}, {"commit", "--quiet", "-m", "head"}} {
		if _, err := noteGitOutput(repository, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	head, err := noteGitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes:
    description: read notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		context string
		want    string
	}{
		{context: "1", want: "● file"},
		{context: "30", want: "● line 2@right"},
	} {
		t.Run("context-"+test.context, func(t *testing.T) {
			if _, err := noteGitOutput(repository, "config", "diff.context", test.context); err != nil {
				t.Fatal(err)
			}
			out := runNoteCLI(
				t, repository, "--config", config, "--color", "never", "--no-symbols", "--no-calls",
				base, head,
			)
			if !strings.Contains(out, test.want) || !strings.Contains(out, "Remember this context") {
				t.Fatalf("render with diff.context=%s =\n%s", test.context, out)
			}
		})
	}
}

func TestValidateNoteVisibilityDegradesMismatchedContextToFile(t *testing.T) {
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+visible-a\n"
	note := provider.Note{
		ID:     "fake:context",
		Anchor: provider.NoteAnchor{Context: "target"},
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 1,
			Target: provider.NoteTargetWorking, Quality: provider.PlacementContext,
		},
	}
	if err := validateNoteVisibility(patch, &note); err != nil {
		t.Fatal(err)
	}
	if note.Placement.Quality != provider.PlacementFile || note.Placement.Line != 0 {
		t.Fatalf("mismatched context placement = %+v", note.Placement)
	}
}

func TestNoteLocationIncludesSideAndRange(t *testing.T) {
	note := provider.Note{Placement: provider.NotePlacement{
		Path: "main.go", Side: provider.NoteSideLeft, Line: 8, Quality: provider.PlacementExact,
	}}
	if got := noteLocation(note); got != "main.go:8@left" {
		t.Fatalf("single-line location = %q", got)
	}
	note.Placement.StartSide = provider.NoteSideRight
	note.Placement.StartLine = 4
	if got := noteLocation(note); got != "main.go:4@right-8@left" {
		t.Fatalf("range location = %q", got)
	}
}

func TestNoteCLIAuthorsListsAndRendersThroughProvider(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\ntwo\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\nchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes:
    description: read notes
  changes.notes.create:
    description: create notes
  changes.notes.generate:
    description: generate notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runNoteCLI(t, repository, "note", "add", "--config", config, "--provider", "fake",
		"--file", "main.go", "--line", "2", "--message", "Harness context\nWhy it changed.",
		"--author", "harness", "--origin", "agent", "--session", "session-7")
	if !strings.Contains(out, "note: fake:created main.go:2") {
		t.Fatalf("note add output = %s", out)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var request provider.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if request.Action != provider.ActionNotesCreate || request.Note == nil ||
		request.Note.Session != "session-7" || request.Note.Anchor.Context != "changed" ||
		request.Note.Anchor.Target != provider.NoteTargetWorking || request.Base == "" || request.Head != "" {
		t.Fatalf("create request = %+v", request)
	}

	out = runNoteCLI(t, repository, "note", "list", "--config", config, "--provider", "fake")
	for _, want := range []string{"notes", "main.go:2", "Remember this context", "reviewer via fake"} {
		if !strings.Contains(out, want) {
			t.Fatalf("note list omitted %q:\n%s", want, out)
		}
	}
	out = runNoteCLI(t, repository, "--config", config, "--color", "never", "--no-symbols", "--no-calls")
	for _, want := range []string{"1 file", "main.go", "+ changed", "● line 2@right", "Remember this context"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render omitted %q:\n%s", want, out)
		}
	}
	out = runNoteCLI(t, repository, "note", "generate", "--config", config,
		"--provider", "fake", "--store", "fake", "--session", "review-9")
	for _, want := range []string{"note: fake:created-1 main.go:2", "note: fake:created-2 main.go:1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("note generate output omitted %q: %s", want, out)
		}
	}
	payload, err = os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if request.Action != provider.ActionNotesCreate || len(request.Notes) != 2 ||
		request.Notes[0].Key != "fake:generated" || request.Notes[0].Summary != "Generated review note" ||
		request.Notes[0].Session != "review-9" || request.Notes[0].Anchor.Context != "changed" ||
		request.Notes[1].Key != "fake:generated-2" || request.Notes[1].Anchor.Context != "one" {
		t.Fatalf("generated create request = %+v", request)
	}
}

func TestNoteGenerateDraftsRepeatedFirstParentCommitsWithoutWriter(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\ntwo\n"})
	commitFile := func(contents, message string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, arguments := range [][]string{{"add", "main.go"}, {"commit", "--quiet", "-m", message}} {
			command := exec.Command("git", arguments...)
			command.Dir = repository
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", arguments, err, output)
			}
		}
		oid, err := noteGitOutput(repository, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		return oid
	}
	first := commitFile("one\nfirst\n", "first")
	second := commitFile("one\nsecond\n", "second")
	firstParent, err := noteGitOutput(repository, "rev-parse", first+"^1")
	if err != nil {
		t.Fatal(err)
	}

	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: test generator
command: [%q, %q, %q, %q]
actions:
  changes.notes.generate:
    description: generate notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "generator.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}

	output := runNoteCLI(
		t, repository, "note", "generate", "--config", config, "--provider", "fake",
		"--commit", first, "--commit", "HEAD", "--draft", "--json", "--session", "review-12",
	)
	var document noteDraftDocument
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode draft output: %v\n%s", err, output)
	}
	if document.Version != "changes.note-drafts/v1" || len(document.Comparisons) != 2 {
		t.Fatalf("draft document = %+v", document)
	}
	for index, expected := range []struct {
		commit string
		base   string
	}{{first, firstParent}, {second, first}} {
		comparison := document.Comparisons[index]
		if comparison.Commit != expected.commit || comparison.Head != expected.commit || comparison.Base != expected.base {
			t.Fatalf("comparison %d = %+v", index, comparison)
		}
		if len(comparison.Notes) != 2 || comparison.Notes[0].Session != "review-12" ||
			comparison.Notes[0].Anchor.Head != expected.commit || comparison.Notes[0].Anchor.Base != expected.base ||
			comparison.Notes[0].Anchor.Context == "" {
			t.Fatalf("comparison %d drafts = %+v", index, comparison.Notes)
		}
	}
	requestData, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var lastRequest provider.Request
	if err := json.Unmarshal(requestData, &lastRequest); err != nil {
		t.Fatal(err)
	}
	if lastRequest.Action != provider.ActionNotesGenerate || lastRequest.Head != second || lastRequest.From != first {
		t.Fatalf("last provider request = %+v", lastRequest)
	}
}

func TestNoteGenerateReportsEveryCommitWhenAWriteFails(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\nroot\n"})
	commitFile := func(contents, message string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, arguments := range [][]string{{"add", "main.go"}, {"commit", "--quiet", "-m", message}} {
			command := exec.Command("git", arguments...)
			command.Dir = repository
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", arguments, err, output)
			}
		}
		oid, err := noteGitOutput(repository, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		return oid
	}
	first := commitFile("one\nfirst\n", "first")
	second := commitFile("one\nsecond\n", "second")
	third := commitFile("one\nthird\n", "third")

	providers := t.TempDir()
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	failureMarker := filepath.Join(t.TempDir(), "failed-once")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: generator and writer with one injected failure
command: [%q, %q, %q, %q, %q, %q]
actions:
  changes.notes.generate:
    description: generate notes
  changes.notes.create:
    description: create notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", requestLog, second, failureMarker)
	if err := os.WriteFile(filepath.Join(providers, "sequenced.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	arguments := []string{
		"note", "generate", "--config", config, "--provider", "fake", "--store", "fake",
		"--commit", first, "--commit", second, "--commit", third, "--json",
	}

	stdout, stderr, err := runNoteCLIWithStreams(t, repository, arguments...)
	if err == nil {
		t.Fatalf("write failure exited successfully:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	var failed noteWriteDocument
	if decodeErr := json.Unmarshal([]byte(stdout), &failed); decodeErr != nil {
		t.Fatalf("decode write results: %v\nstdout: %s\nstderr: %s", decodeErr, stdout, stderr)
	}
	assertNoteWriteStatuses(t, failed, []string{first, second, third}, []string{
		noteWriteSuccess, noteWriteFailure, noteWriteUnattempted,
	})
	if len(failed.Comparisons[0].Notes) != 2 || failed.Comparisons[1].Error == "" ||
		!strings.Contains(failed.Comparisons[2].Error, "not attempted") {
		t.Fatalf("partial write results = %+v", failed.Comparisons)
	}
	if !strings.Contains(stderr, second) || !strings.Contains(stderr, "injected write failure") {
		t.Fatalf("write failure diagnostic did not identify commit %s:\n%s", second, stderr)
	}
	requests := readNoteProviderRequests(t, requestLog)
	assertNoteCreateAttempts(t, requests, map[string]int{first: 1, second: 1, third: 0})
	firstKeys := noteRequestKeys(t, requests, first)

	stdout, stderr, err = runNoteCLIWithStreams(t, repository, arguments...)
	if err != nil {
		t.Fatalf("idempotent retry: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var retried noteWriteDocument
	if decodeErr := json.Unmarshal([]byte(stdout), &retried); decodeErr != nil {
		t.Fatalf("decode retry results: %v\n%s", decodeErr, stdout)
	}
	assertNoteWriteStatuses(t, retried, []string{first, second, third}, []string{
		noteWriteSuccess, noteWriteSuccess, noteWriteSuccess,
	})
	requests = readNoteProviderRequests(t, requestLog)
	assertNoteCreateAttempts(t, requests, map[string]int{first: 2, second: 2, third: 1})
	if retryKeys := noteRequestKeys(t, requests, first); retryKeys != firstKeys {
		t.Fatalf("retry changed the first commit note keys: %s then %s", firstKeys, retryKeys)
	}
}

func TestNoteWriteTextIdentifiesEachCommitOutcome(t *testing.T) {
	commits := []string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)}
	outcomes := []noteWriteOutcome{
		{
			batch: noteGeneratedComparison{snapshot: noteSnapshot{head: commits[0]}}, status: noteWriteSuccess,
			notes: []provider.Note{{ID: "stored", Anchor: provider.NoteAnchor{Path: "main.go", Line: 2}}},
		},
		{
			batch: noteGeneratedComparison{snapshot: noteSnapshot{head: commits[1]}}, status: noteWriteFailure,
			err: fmt.Errorf("store refused the batch"),
		},
		{
			batch: noteGeneratedComparison{snapshot: noteSnapshot{head: commits[2]}}, status: noteWriteUnattempted,
			err: fmt.Errorf("not attempted after an earlier commit write failed"),
		},
	}
	var output bytes.Buffer
	if err := printNoteWriteOutcomes(&output, outcomes, false); err != nil {
		t.Fatal(err)
	}
	for index, status := range []string{noteWriteSuccess, noteWriteFailure, noteWriteUnattempted} {
		if !strings.Contains(output.String(), commits[index]) || !strings.Contains(output.String(), status) {
			t.Fatalf("text results omitted %s %s:\n%s", commits[index], status, output.String())
		}
	}
}

func assertNoteWriteStatuses(t *testing.T, document noteWriteDocument, commits, statuses []string) {
	t.Helper()
	if document.Version != "changes.note-write-results/v1" || len(document.Comparisons) != len(commits) {
		t.Fatalf("write document = %+v", document)
	}
	for index := range commits {
		comparison := document.Comparisons[index]
		if comparison.Commit != commits[index] || comparison.Head != commits[index] || comparison.Status != statuses[index] {
			t.Fatalf("comparison %d = %+v", index, comparison)
		}
	}
}

func runNoteCLIWithStreams(t *testing.T, directory string, arguments ...string) (string, string, error) {
	t.Helper()
	command := noteCLICommand(t, directory, arguments...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func readNoteProviderRequests(t *testing.T, path string) []provider.Request {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(payload), []byte("\n"))
	requests := make([]provider.Request, 0, len(lines))
	for _, line := range lines {
		var request provider.Request
		if err := json.Unmarshal(line, &request); err != nil {
			t.Fatalf("decode provider request: %v\n%s", err, line)
		}
		requests = append(requests, request)
	}
	return requests
}

func assertNoteCreateAttempts(t *testing.T, requests []provider.Request, expected map[string]int) {
	t.Helper()
	actual := map[string]int{}
	for _, request := range requests {
		if request.Action == provider.ActionNotesCreate {
			actual[request.Head]++
		}
	}
	for commit, count := range expected {
		if actual[commit] != count {
			t.Fatalf("create attempts for %s = %d, want %d; all attempts = %#v", commit, actual[commit], count, actual)
		}
	}
}

func noteRequestKeys(t *testing.T, requests []provider.Request, head string) string {
	t.Helper()
	var first string
	for _, request := range requests {
		if request.Action != provider.ActionNotesCreate || request.Head != head {
			continue
		}
		payload, err := json.Marshal(request.Notes)
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = string(payload)
			continue
		}
		if string(payload) != first {
			t.Fatalf("note keys changed for %s: %s then %s", head, first, payload)
		}
	}
	return first
}

func TestNoteGenerateDraftFlagValidationAndComparisonExclusivity(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\ntwo\n"})
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("one\nchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := noteCLICommand(t, repository, "note", "generate", "--draft").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--draft requires --json") {
		t.Fatalf("draft without JSON = %v\n%s", err, output)
	}
	output, err = noteCLICommand(
		t, repository, "note", "generate", "--commit", "HEAD", "--commit", "HEAD~1", "--from", "HEAD~2",
	).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--commit cannot be combined") {
		t.Fatalf("mixed comparisons = %v\n%s", err, output)
	}
}

func TestNoteGenerateAcceptsRootCommitAsEmptyTreeComparison(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\ntwo\n"})
	specs, err := noteGenerationSpecs(repository, noteComparisonFlags{}, noteCommitFlags{"HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].spec.From == "" || specs[0].spec.To == "" {
		t.Fatalf("root comparison = %+v", specs)
	}
	patch, err := specs[0].spec.NoteDiff()
	if err != nil || !strings.Contains(patch, "main.go") {
		t.Fatalf("root comparison patch = %q, %v", patch, err)
	}
}

func TestFilteredRenderUsesFullNoteIdentityAndFilteredPlacement(t *testing.T) {
	repository := t.TempDir()
	prepareRepository(t, repository, map[string]string{"main.go": "one\nold main\n", "other.go": "one\nold other\n"})
	for name, content := range map[string]string{"main.go": "one\nnew main\n", "other.go": "one\nnew other\n"} {
		if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	filtered := source.Spec{Dir: repository, Paths: []string{filepath.Join(repository, "main.go")}}
	snapshot, err := stableNoteSnapshot(filtered)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.files) != 2 || !strings.Contains(snapshot.patch, "other.go") {
		t.Fatalf("full note comparison = files %#v, patch %q", snapshot.files, snapshot.patch)
	}
	if !strings.Contains(snapshot.placementPatch, "main.go") || strings.Contains(snapshot.placementPatch, "other.go") {
		t.Fatalf("filtered placement patch = %q", snapshot.placementPatch)
	}

	providers := t.TempDir()
	capture := filepath.Join(t.TempDir(), "request.json")
	manifest := fmt.Sprintf(`version: provider/v1
name: fake
description: fake note provider
command: [%q, %q, %q, %q]
actions:
  changes.notes:
    description: read notes
`, os.Args[0], "-test.run=TestFakeNoteProviderProcess", "--", capture)
	if err := os.WriteFile(filepath.Join(providers, "fake.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runNoteCLI(
		t, repository,
		"--config", config, "--color", "never", "--no-symbols", "--no-calls", "--", "main.go",
	)
	if !strings.Contains(out, "Remember this context") || !strings.Contains(out, "main.go") || strings.Contains(out, "other.go") {
		t.Fatalf("filtered render = %s", out)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var request provider.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	wantFingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.patch)))
	if len(request.Files) != 1 || request.Files[0] != "main.go" || request.Fingerprint != wantFingerprint {
		t.Fatalf("filtered note request = %+v", request)
	}
}

func TestNoteProviderCompletionUsesConfigAndAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config folder")
	providers := filepath.Join(root, "providers")
	if err := os.MkdirAll(providers, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, action := range map[string]string{
		"generator": provider.ActionNotesGenerate,
		"reader":    provider.ActionNotes,
		"writer":    provider.ActionNotesCreate,
	} {
		manifest := fmt.Sprintf("version: provider/v1\nname: %s\ndescription: test\ncommand: [true]\nactions:\n  %s:\n    description: test\n", name, action)
		if err := os.WriteFile(filepath.Join(providers, name+".yaml"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(root, "changes.yaml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("providers:\n  directory: %q\n", providers)), 0o600); err != nil {
		t.Fatal(err)
	}
	context := fmt.Sprintf(`changes note add --config %q --provider `, config)
	output := runNoteCLI(t, t.TempDir(), "__values", "note-writers", context)
	if output != "writer\n" {
		t.Fatalf("writer completion = %q", output)
	}
	context = fmt.Sprintf(`changes note generate --config %q --provider `, config)
	output = runNoteCLI(t, t.TempDir(), "__values", "note-generators", context)
	if output != "generator\n" {
		t.Fatalf("generator completion = %q", output)
	}
	context = fmt.Sprintf(`changes provider validate --config %q `, config)
	output = runNoteCLI(t, t.TempDir(), "__values", "providers", context)
	if output != "generator\nreader\nwriter\n" {
		t.Fatalf("provider completion = %q", output)
	}
}

func TestNoteGenerateMetadataPublishesDraftAndRepeatableCommitFlags(t *testing.T) {
	metadata := subcommandMetadata("note", "generate")
	flags := map[string]completion.Flag{}
	for _, configured := range metadata.Flags {
		flags[configured.Name] = configured
	}
	if _, ok := flags["draft"]; !ok {
		t.Fatal("note generate metadata omits --draft")
	}
	commit, ok := flags["commit"]
	if !ok || !strings.Contains(commit.Description, "repeat") {
		t.Fatalf("note generate --commit metadata = %+v", commit)
	}
	for _, shell := range []string{"bash", "zsh", "fish", "nu"} {
		generated, err := completion.Generate(shell, completionGeneratorMetadata())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(generated, "draft") {
			t.Fatalf("%s completion omits --draft", shell)
		}
	}
}

func TestFakeNoteProviderProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	var request provider.Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	capture := os.Args[separator+1]
	if separator+3 < len(os.Args) {
		file, openErr := os.OpenFile(capture, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			t.Fatal(openErr)
		}
		if _, writeErr := file.Write(append(payload, '\n')); writeErr != nil {
			_ = file.Close()
			t.Fatal(writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		failHead := os.Args[separator+2]
		failureMarker := os.Args[separator+3]
		if request.Action == provider.ActionNotesCreate && request.Head == failHead {
			if _, statErr := os.Stat(failureMarker); os.IsNotExist(statErr) {
				if markerErr := os.WriteFile(failureMarker, []byte("failed\n"), 0o600); markerErr != nil {
					t.Fatal(markerErr)
				}
				fmt.Fprintln(os.Stderr, "injected write failure")
				os.Exit(7)
			}
		}
	} else if err := os.WriteFile(capture, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	note := provider.Note{
		ID: "read", Source: "fake", SourceID: "read", Summary: "Remember this context",
		Rationale: "It explains the changed line.", Author: "reviewer",
		Origin: provider.NoteOriginExternal, Authority: provider.NoteAuthorityExternal,
		State: provider.NoteStateOpen,
		Anchor: provider.NoteAnchor{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2,
			Base: request.Base, Head: request.Head, Target: requestNoteTarget(request),
		},
		Placement: provider.NotePlacement{
			Path: "main.go", Side: provider.NoteSideRight, Line: 2,
			Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
			Target: requestNoteTarget(request), Quality: provider.PlacementExact,
		},
	}
	responseNotes := []provider.Note{note}
	if request.Action == provider.ActionNotesGenerate {
		note = provider.Note{
			ID: "generated", Source: "fake", SourceID: "generated", Summary: "Generated review note",
			Rationale: "This line changes behavior.", Author: "review-agent",
			Origin: provider.NoteOriginAgent, Authority: provider.NoteAuthorityAdvisory,
			State: provider.NoteStateOpen,
			Anchor: provider.NoteAnchor{
				Path: "main.go", Side: provider.NoteSideRight, Line: 2,
				Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
				Target: requestNoteTarget(request),
			},
			Placement: provider.NotePlacement{
				Path: "main.go", Side: provider.NoteSideRight, Line: 2,
				Base: request.Base, Head: request.Head, Fingerprint: request.Fingerprint,
				Target: requestNoteTarget(request), Quality: provider.PlacementExact,
			},
		}
		second := note
		second.ID = "generated-2"
		second.SourceID = "generated-2"
		second.Summary = "Second generated review note"
		second.Anchor.Line = 1
		second.Placement.Line = 1
		responseNotes = []provider.Note{note, second}
	}
	if request.Action == provider.ActionNotesCreate {
		drafts := request.Notes
		if request.Note != nil {
			drafts = []provider.NoteDraft{*request.Note}
		}
		notes := make([]provider.Note, 0, len(drafts))
		for index, draft := range drafts {
			sourceID := "created"
			if len(drafts) > 1 {
				sourceID = fmt.Sprintf("created-%d", index+1)
			}
			notes = append(notes, provider.Note{
				ID: sourceID, Source: "fake", SourceID: sourceID, Summary: draft.Summary,
				Author: draft.Author, Origin: draft.Origin, Authority: provider.NoteAuthorityAdvisory,
				State: provider.NoteStateOpen, Anchor: draft.Anchor,
				Placement: provider.NotePlacement{
					Path: draft.Anchor.Path, Side: draft.Anchor.Side,
					StartSide: draft.Anchor.StartSide, StartLine: draft.Anchor.StartLine,
					Line: draft.Anchor.Line, Base: request.Base, Head: request.Head,
					Fingerprint: request.Fingerprint, Target: requestNoteTarget(request),
					Quality: provider.PlacementExact,
				},
			})
		}
		if err := json.NewEncoder(os.Stdout).Encode(provider.Response{Version: provider.ProtocolVersion, Notes: notes}); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	if err := json.NewEncoder(os.Stdout).Encode(provider.Response{
		Version: provider.ProtocolVersion, Notes: responseNotes,
	}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func requestNoteTarget(request provider.Request) string {
	if request.To != "" || request.Head != "" {
		return provider.NoteTargetCommits
	}
	if request.Staged {
		return provider.NoteTargetIndex
	}
	return provider.NoteTargetWorking
}

func runNoteCLI(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := noteCLICommand(t, directory, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("changes %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func noteCLICommand(t *testing.T, directory string, arguments ...string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestMainHelperProcess", "--"}, arguments...)...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_DATA_DIRS="+t.TempDir(),
	)
	return command
}
