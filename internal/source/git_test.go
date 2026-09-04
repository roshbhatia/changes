package source

import (
	"slices"
	"strings"
	"testing"
)

func TestStagedDiffRejectsTwoRevisionsBeforeGit(t *testing.T) {
	_, err := (Spec{Staged: true, From: "HEAD~1", To: "HEAD"}).Diff()
	if err == nil || !strings.Contains(err.Error(), "at most one revision") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiffArgumentsSeparateRefLikePaths(t *testing.T) {
	t.Parallel()
	arguments, err := (Spec{From: "HEAD~1", Paths: []string{"HEAD", "main.go"}}).args("never")
	if err != nil {
		t.Fatal(err)
	}
	wantTail := []string{"HEAD~1", "--", "HEAD", "main.go"}
	if len(arguments) < len(wantTail) || !slices.Equal(arguments[len(arguments)-len(wantTail):], wantTail) {
		t.Fatalf("arguments = %#v, want tail %#v", arguments, wantTail)
	}
}
