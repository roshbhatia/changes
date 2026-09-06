package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roshbhatia/changes/internal/provider"
	gitutil "github.com/roshbhatia/go-utils/git"
)

func TestHasGitHubRemoteIgnoresInheritedGitTargetAndOtherHosts(t *testing.T) {
	repository := t.TempDir()
	other := t.TempDir()
	for _, directory := range []string{repository, other} {
		command := exec.Command("git", "init", "--quiet")
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, output)
		}
	}
	denied := func(string) (bool, error) { return false, nil }
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	if matched, err := hasGitHubRemote(repository, denied); err != nil || matched {
		t.Fatal("repository without a remote reported one")
	}
	command := exec.Command("git", "remote", "add", "origin", "https://gitlab.com/owner/repo.git")
	command.Dir = repository
	command.Env = gitutil.CleanEnv()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, output)
	}
	if matched, err := hasGitHubRemote(repository, denied); err != nil || matched {
		t.Fatal("non-GitHub remote was accepted")
	}
	command = exec.Command("git", "remote", "add", "upstream", "git@github.com:owner/repo.git")
	command.Dir = repository
	command.Env = gitutil.CleanEnv()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, output)
	}
	if matched, err := hasGitHubRemote(repository, denied); err != nil || !matched {
		t.Fatal("GitHub remote was not found")
	}
	command = exec.Command("git", "remote", "remove", "upstream")
	command.Dir = repository
	command.Env = gitutil.CleanEnv()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git remote remove: %v\n%s", err, output)
	}
	command = exec.Command("git", "remote", "add", "enterprise", "ssh://git@git.example.test/owner/repo.git")
	command.Dir = repository
	command.Env = gitutil.CleanEnv()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, output)
	}
	if matched, err := hasGitHubRemote(repository, func(host string) (bool, error) {
		return host == "git.example.test", nil
	}); err != nil || !matched {
		t.Fatal("authenticated GitHub Enterprise remote was not found")
	}
	for _, remote := range []string{
		"https://github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
	} {
		if !isGitHubRemote(remote) {
			t.Fatalf("GitHub remote %q was rejected", remote)
		}
	}
	if got := remoteHost("https://ghe.example:8443/owner/repo.git"); got != "ghe.example:8443" {
		t.Fatalf("enterprise authority = %q", got)
	}
}

func TestDecodeRequestRejectsUnknownFields(t *testing.T) {
	_, err := decodeRequest(bytes.NewBufferString(`{"version":"changes.provider/v1","validaton":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeRequestRejectsOversizedTrailingData(t *testing.T) {
	_, err := decodeRequestWithLimit(strings.NewReader("{}  {}"), 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestReviewThreadOutputBudgetIsAggregate(t *testing.T) {
	total, err := addOutputSize(4, 5, 10)
	if err != nil || total != 9 {
		t.Fatalf("output total = %d, %v", total, err)
	}
	if _, err := addOutputSize(total, 2, 10); err == nil {
		t.Fatal("aggregate output above the limit was accepted")
	}
}

func TestRunGHCleansInheritedGitTarget(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "gh")
	program := `#!/bin/sh
if test -n "${GIT_DIR-}" || test -n "${GIT_WORK_TREE-}" || test -n "${GH_REPO-}"; then
  printf '%s\n' 'inherited git target' >&2
  exit 1
fi
printf '%s\n' '{}'
`
	if err := os.WriteFile(script, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), ".git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GH_REPO", "other/repository")
	if _, err := runGH(directory, "version"); err != nil {
		t.Fatal(err)
	}
}

func TestRunGHRequiresAbsoluteOverride(t *testing.T) {
	t.Setenv("CHANGES_GH_COMMAND", "gh-test-double")
	_, err := runGH(t.TempDir(), "version")
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitHubMergeBaseUsesCompareAPI(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "gh")
	program := `#!/bin/sh
test "$1" = api
test "$2" = --hostname
test "$3" = git.example.test
test "$4" = repos/owner/repo/compare/base...head
test "$5" = --jq
test "$6" = .merge_base_commit.sha
printf '%s\n' merge-base
`
	if err := os.WriteFile(script, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+"/usr/bin:/bin")
	base, err := githubMergeBase(directory, "git.example.test", "owner", "repo", "base", "head")
	if err != nil {
		t.Fatal(err)
	}
	if base != "merge-base" {
		t.Fatalf("merge base = %q", base)
	}
}

func TestGitHubHostConfiguredDoesNotProbeRequestedHost(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "gh")
	program := `#!/bin/sh
test "$1" = auth
test "$2" = status
test "$3" = --json
test "$4" = hosts
test "$#" = 4
printf '%s\n' '{"hosts":{"ghe.example:8443":[]}}'
`
	if err := os.WriteFile(script, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("GH_ENTERPRISE_TOKEN", "must-not-be-sent-to-requested-host")
	configured, err := githubHostConfigured("ghe.example:8443")
	if err != nil || !configured {
		t.Fatalf("configured host was not selected: %v", err)
	}
	configured, err = githubHostConfigured("attacker.example")
	if err != nil || configured {
		t.Fatal("configured host selection did not use the known host list")
	}
}

func TestGitHubHostConfigurationFailureIsVisible(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "gh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' broken >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+"/usr/bin:/bin")
	if _, err := githubHostConfigured("ghe.example"); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("configuration error = %v", err)
	}
}

func TestNotesPaginatesAndPreservesReviewThreadState(t *testing.T) {
	graphCalls := 0
	run := func(directory string, arguments ...string) ([]byte, error) {
		switch {
		case slices.Equal(arguments, []string{"pr", "view", "--json", "number,baseRefOid,headRefOid,url"}):
			return []byte(`{"number":42,"baseRefOid":"base","headRefOid":"head","url":"https://github.com/upstream/repo/pull/42"}`), nil
		case len(arguments) >= 2 && arguments[0] == "api" && arguments[1] == "graphql":
			if !slices.Contains(arguments, "owner=upstream") || !slices.Contains(arguments, "name=repo") ||
				!slices.Contains(arguments, "github.com") {
				t.Fatalf("GraphQL repository arguments = %#v", arguments)
			}
			graphCalls++
			if graphCalls == 1 {
				if slices.Contains(arguments, "endCursor=next") {
					t.Fatal("first GraphQL request included the next cursor")
				}
				return []byte(firstGraphPage), nil
			}
			if !slices.Contains(arguments, "endCursor=next") {
				t.Fatalf("second GraphQL request lacks cursor: %#v", arguments)
			}
			return []byte(secondGraphPage), nil
		default:
			return nil, fmt.Errorf("unexpected gh arguments: %#v", arguments)
		}
	}
	response, err := notes(provider.Request{Directory: t.TempDir(), Base: "base", Head: "head"}, run, fixedMergeBase("base"))
	if err != nil {
		t.Fatal(err)
	}
	if graphCalls != 2 || len(response.Notes) != 3 {
		t.Fatalf("graph calls = %d, notes = %+v", graphCalls, response.Notes)
	}
	first, reply, outdated := response.Notes[0], response.Notes[1], response.Notes[2]
	if first.Anchor.Line != 8 || first.Placement.Line != 11 || first.Anchor.Head != "original" ||
		first.Anchor.StartSide != provider.NoteSideRight || first.Placement.StartSide != provider.NoteSideRight ||
		first.Anchor.Target != provider.NoteTargetCommits || first.State != provider.NoteStateOpen {
		t.Fatalf("first note = %+v", first)
	}
	if reply.ReplyTo != first.ID || reply.ThreadID != first.ThreadID {
		t.Fatalf("reply = %+v", reply)
	}
	if outdated.State != provider.NoteStateResolved || outdated.Placement.Quality != provider.PlacementOutdated ||
		outdated.Placement.Line != 0 ||
		outdated.Anchor.Side != provider.NoteSideLeft || outdated.Author != "unknown" {
		t.Fatalf("outdated note = %+v", outdated)
	}
}

func TestNotesReturnsEmptyWithoutPullRequest(t *testing.T) {
	response, err := notes(provider.Request{Directory: t.TempDir(), Head: "head"}, func(string, ...string) ([]byte, error) {
		return nil, errors.New("no pull requests found for branch main")
	}, fixedMergeBase("base"))
	if err != nil {
		t.Fatal(err)
	}
	if response.Notes == nil || len(response.Notes) != 0 {
		t.Fatalf("notes = %#v", response.Notes)
	}
}

func TestNotesDoesNotHideAuthenticationFailure(t *testing.T) {
	for _, message := range []string{
		"authentication required",
		"authentication required: no pull requests found for branch main",
		"no pull requests found for branch main\nauthentication required",
		"no pull requests found for branch main: authentication required",
	} {
		_, err := notes(provider.Request{Directory: t.TempDir(), Head: "head"}, func(string, ...string) ([]byte, error) {
			return nil, errors.New(message)
		}, fixedMergeBase("base"))
		if err == nil || !strings.Contains(err.Error(), "authentication required") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestNotesMatchesGitHubMergeBase(t *testing.T) {
	resolverCalled := false
	resolve := func(_ string, host, owner, name, base, head string) (string, error) {
		resolverCalled = true
		if host != "github.com" || owner != "owner" || name != "repo" || base != "base-tip" || head != "head" {
			t.Fatalf("merge-base inputs = %q %q %q %q %q", host, owner, name, base, head)
		}
		return "merge-base", nil
	}
	response, err := notes(provider.Request{Directory: t.TempDir(), Base: "merge-base", Head: "head"}, sequenceRunner(t,
		`{"number":42,"baseRefOid":"base-tip","headRefOid":"head","url":"https://github.com/owner/repo/pull/42"}`,
		strings.Replace(firstGraphPage, `"hasNextPage":true,"endCursor":"next"`, `"hasNextPage":false,"endCursor":""`, 1),
	), resolve)
	if err != nil {
		t.Fatal(err)
	}
	if !resolverCalled || len(response.Notes) != 2 || response.Notes[0].Anchor.Base != "" ||
		response.Notes[0].Placement.Base != "merge-base" {
		t.Fatalf("merge-base response = %+v", response.Notes)
	}
}

func TestNotesReturnsEmptyForAnotherComparison(t *testing.T) {
	notes, err := notes(provider.Request{Directory: t.TempDir(), Base: "other", Head: "head"}, sequenceRunner(t,
		`{"number":42,"baseRefOid":"base","headRefOid":"head","url":"https://github.com/owner/repo/pull/42"}`,
	), fixedMergeBase("base"))
	if err != nil {
		t.Fatal(err)
	}
	if len(notes.Notes) != 0 {
		t.Fatalf("notes = %+v", notes.Notes)
	}
}

func TestNotesFailsClosedWhenRepliesAreTruncated(t *testing.T) {
	run := sequenceRunner(t,
		`{"number":42,"baseRefOid":"base","headRefOid":"head","url":"https://github.com/owner/repo/pull/42"}`,
		strings.Replace(firstGraphPage, `"hasNextPage":false,"endCursor":""`, `"hasNextPage":true,"endCursor":"reply"`, 1),
	)
	_, err := notes(provider.Request{Directory: t.TempDir(), Base: "base", Head: "head"}, run, fixedMergeBase("base"))
	if err == nil || !strings.Contains(err.Error(), "more than 100 replies") {
		t.Fatalf("error = %v", err)
	}
}

func TestNotesRejectsMalformedGraphQLAndStalledPagination(t *testing.T) {
	for _, test := range []struct {
		name string
		page string
		want string
	}{
		{name: "graphql error", page: `{"errors":[{"message":"denied"}],"data":{}}`, want: "GraphQL: denied"},
		{name: "stalled cursor", page: strings.Replace(firstGraphPage, `"endCursor":"next"`, `"endCursor":""`, 1), want: "pagination did not advance"},
		{name: "unknown field", page: `{"unexpected":true}`, want: "unknown field"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := notes(provider.Request{Directory: t.TempDir(), Base: "base", Head: "head"}, sequenceRunner(t,
				`{"number":42,"baseRefOid":"base","headRefOid":"head","url":"https://github.com/owner/repo/pull/42"}`,
				test.page,
			), fixedMergeBase("base"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func fixedMergeBase(value string) mergeBaseResolver {
	return func(string, string, string, string, string, string) (string, error) { return value, nil }
}

func sequenceRunner(t *testing.T, responses ...string) commandRunner {
	t.Helper()
	index := 0
	return func(string, ...string) ([]byte, error) {
		if index >= len(responses) {
			t.Fatalf("unexpected command %d", index+1)
		}
		response := responses[index]
		index++
		return []byte(response), nil
	}
}

const firstGraphPage = `{
  "data": {"repository": {"pullRequest": {"reviewThreads": {
    "nodes": [{
      "id": "thread-1", "isOutdated": false, "isResolved": false,
      "path": "main.go", "diffSide": "RIGHT", "startDiffSide": null,
      "startLine": 10, "line": 11, "originalStartLine": 7, "originalLine": 8,
      "subjectType": "LINE",
      "comments": {
        "nodes": [
          {"id":"comment-1","body":"Check this invariant\nIt guards the caller.","url":"https://example.test/1","createdAt":"2026-09-05T01:00:00Z","updatedAt":"2026-09-05T01:01:00Z","author":{"login":"reviewer"},"replyTo":null,"commit":{"oid":"current"},"originalCommit":{"oid":"original"}},
          {"id":"comment-2","body":"Done","url":"https://example.test/2","createdAt":"2026-09-05T02:00:00Z","updatedAt":"2026-09-05T02:01:00Z","author":{"login":"author"},"replyTo":{"id":"comment-1"},"commit":{"oid":"current"},"originalCommit":{"oid":"original"}}
        ],
        "pageInfo": {"hasNextPage":false,"endCursor":""}
      }
    }],
    "pageInfo": {"hasNextPage":true,"endCursor":"next"}
  }}}}
}`

const secondGraphPage = `{
  "data": {"repository": {"pullRequest": {"reviewThreads": {
    "nodes": [{
      "id": "thread-2", "isOutdated": true, "isResolved": true,
      "path": "old.go", "diffSide": "LEFT", "startDiffSide": null,
      "startLine": null, "line": null, "originalStartLine": null, "originalLine": 4,
      "subjectType": "LINE",
      "comments": {
        "nodes": [{"id":"comment-3","body":"Removed path","url":"https://example.test/3","createdAt":"2026-09-05T03:00:00Z","updatedAt":"2026-09-05T03:01:00Z","author":null,"replyTo":null,"commit":{"oid":"current"},"originalCommit":null}],
        "pageInfo": {"hasNextPage":false,"endCursor":""}
      }
    }],
    "pageInfo": {"hasNextPage":false,"endCursor":""}
  }}}}
}`
