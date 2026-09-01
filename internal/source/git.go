// Package source turns the three tools that describe a change into the layers
// diffview renders: git for the lines, ast-grep for the symbols, calldiff for
// the call edges. Each layer degrades on its own, so a language ast-grep cannot
// parse still prints its hunks.
package source

import (
	"bytes"
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
	return s.runDiff("never", nil)
}

// DisplayDiff asks Git to render its own patch. It is the zero-configuration
// display engine and stays independent from the analysis patch above it.
func (s Spec) DisplayDiff(color string) (string, error) {
	return s.runDiff(color, nil)
}

// Difftastic uses Git's external-diff protocol. Git still selects revisions,
// staged content, and pathspecs, while Difftastic compares each file pair.
func (s Spec) Difftastic(layout, color string) (string, error) {
	if layout == "unified" {
		layout = "inline"
	}
	env := map[string]string{
		"DFT_COLOR":         color,
		"DFT_DISPLAY":       layout,
		"GIT_EXTERNAL_DIFF": "difft",
	}
	return s.runDiff("never", env)
}

func (s Spec) args(color string, external bool) []string {
	args := []string{"diff", "--color=" + color, "--find-renames"}
	if !external {
		args = append(args, "--no-ext-diff")
	}
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

func (s Spec) runDiff(color string, extraEnv map[string]string) (string, error) {
	args := s.args(color, len(extraEnv) > 0)
	if len(extraEnv) == 0 {
		out, err := git.Output(s.Dir, args...)
		if err != nil {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return out, nil
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Dir
	cmd.Env = git.CleanEnv()
	for key, value := range extraEnv {
		kept := cmd.Env[:0]
		for _, entry := range cmd.Env {
			if !strings.HasPrefix(entry, key+"=") {
				kept = append(kept, entry)
			}
		}
		cmd.Env = kept
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
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
