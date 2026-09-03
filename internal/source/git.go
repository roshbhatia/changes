// Package source reads repository comparisons through Git.
package source

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/roshbhatia/go-utils/git"
)

// Spec names one comparison, in git's own terms: no refs is HEAD against the
// working tree, one ref is that ref against the working tree, two refs compare
// the trees. Staged swaps the working tree for the index.
type Spec struct {
	Dir    string
	From   string
	To     string
	Staged bool
	Paths  []string
}

// Diff returns the unified patch. Zero context would drop the side by side
// view's carried lines, and git's default of three is what the renderer was
// tuned against.
func (s Spec) Diff() (string, error) {
	return s.runDiff("never")
}

// DisplayDiff asks Git to render its own patch. It is the zero-configuration
// display engine and stays independent from the analysis patch above it.
func (s Spec) DisplayDiff(color string) (string, error) {
	return s.runDiff(color)
}

// FileDiff returns a unified patch for two paths without repository context.
// The exit status for a detected difference is successful for this operation.
func FileDiff(local, remote, color string) (string, error) {
	arguments := []string{"diff", "--no-index", "--color=" + color, "--", local, remote}
	command := exec.Command("git", arguments...)
	command.Env = git.CleanEnv()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("compare files: %s: %w", strings.TrimSpace(stderr.String()), err)
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s Spec) args(color string) []string {
	args := []string{"diff", "--color=" + color, "--find-renames"}
	args = append(args, "--no-ext-diff")
	if s.Staged {
		args = append(args, "--cached")
	}
	if s.From != "" {
		args = append(args, s.From)
	}
	if s.To != "" {
		args = append(args, s.To)
	}
	if len(s.Paths) > 0 {
		args = append(args, "--")
		args = append(args, s.Paths...)
	}
	return args
}

func (s Spec) runDiff(color string) (string, error) {
	args := s.args(color)
	out, err := git.Output(s.Dir, args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// Root resolves the repository the paths are relative to. Every layer keys on
// a repo relative path, so the three tools have to agree on one root.
func Root(dir string) (string, error) {
	if !git.IsRepo(dir) {
		return "", fmt.Errorf("%s is not a git repository", dir)
	}
	return git.Root(dir)
}

// IsRev asks git whether a positional argument names a tree. flag.Parse eats
// the -- separator, so a bare path would otherwise reach git as a revision and
// fail as an unknown one. git disambiguates the same way.
func IsRev(dir, s string) bool {
	if s == "" {
		return false
	}
	return git.Succeeds(dir, "rev-parse", "--verify", "--quiet", s+"^{object}")
}

// Revision resolves the left side of a comparison from either a revision or a
// time. git already reads both, so this asks git rather than parsing: a name it
// knows as a tree wins, and anything else goes to `rev-list -1 --before`, which
// takes "2 hours ago", "yesterday" and "2026-08-01" alike.
//
// The two are one flag because a reader asking "what changed since lunch" and
// one asking "what changed since that commit" want the same answer shape, and
// making them pick the right flag first is friction with no payoff.
func Revision(dir, since string) string {
	if IsRev(dir, since) {
		return since
	}
	// --before with no committish walks nothing, so HEAD names the branch to
	// walk back along.
	out, err := git.Output(dir, "rev-list", "-1", "--before="+since, "HEAD")
	if err != nil {
		return ""
	}
	at := strings.TrimSpace(out)

	// git reads an unparseable date as "now" rather than failing, so
	// `--since "not a thing"` resolved to HEAD and quietly became the default
	// comparison. A window that lands on HEAD carries no window, whether the
	// value was garbage or a second ago, so both are refused here.
	head, err := git.Output(dir, "rev-parse", "HEAD")
	if err == nil && at == strings.TrimSpace(head) {
		return ""
	}
	return at
}
