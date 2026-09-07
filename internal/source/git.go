// Package source reads repository comparisons through Git.
package source

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/roshbhatia/go-utils/git"
)

// MaxPatchBytes bounds one patch before Changes copies or analyzes it.
const MaxPatchBytes = 64 << 20

// Spec names one comparison, in git's own terms: no refs is the index against
// the working tree, one ref is that ref against the working tree, and two refs
// compare the trees. Staged compares HEAD or one ref against the index.
type Spec struct {
	Dir         string
	From        string
	To          string
	Staged      bool
	Paths       []string
	NoLazyFetch bool
}

// ComparisonIDs resolves stable endpoint identities for a comparison. A
// working comparison uses a digest of the index entries as its base. The
// working tree and index have no right-side object, so Head is empty and the
// patch fingerprint identifies their current contents.
func (s Spec) ComparisonIDs() (base, head string) {
	if s.From == "" && s.To == "" && !s.Staged {
		return indexIdentity(s), ""
	}
	left := s.From
	if left == "" {
		left = "HEAD"
	}
	base, _ = s.objectID(left)
	if s.To != "" {
		head, _ = s.objectID(s.To)
	}
	return base, head
}

func indexIdentity(spec Spec) string {
	command := spec.command("ls-files", "--stage", "-z")
	digest := sha256.New()
	command.Stdout = digest
	if err := command.Run(); err != nil {
		return ""
	}
	return fmt.Sprintf("index:%x", digest.Sum(nil))
}

func (s Spec) objectID(revision string) (string, error) {
	var last error
	for _, kind := range []string{"commit", "tree"} {
		value, err := s.output("rev-parse", "--verify", revision+"^{"+kind+"}")
		if err == nil {
			return strings.TrimSpace(value), nil
		}
		last = err
	}
	return "", last
}

// Diff returns the unified patch. Zero context would drop the side by side
// view's carried lines, and git's default of three is what the renderer was
// tuned against.
func (s Spec) Diff() (string, error) {
	return s.runDiff("never")
}

// NoteDiff returns the raw tree delta used to identify a committed comparison.
func (s Spec) NoteDiff() (string, error) {
	s.NoLazyFetch = true
	args, err := s.noteIdentityArgs()
	if err != nil {
		return "", err
	}
	return s.runDiffArgs(args)
}

// Files returns repository-relative paths touched by the comparison. Reading
// names separately preserves a deleted file's old path, which a unified +++
// header reports as /dev/null.
func (s Spec) Files() ([]string, error) {
	args, err := s.args("never")
	if err != nil {
		return nil, err
	}
	args = append([]string{args[0], "--name-only", "-z"}, args[1:]...)
	command := s.command(args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	parts := bytes.Split(stdout.Bytes(), []byte{0})
	files := make([]string, 0, len(parts))
	for _, path := range parts {
		if len(path) != 0 {
			files = append(files, string(path))
		}
	}
	return files, nil
}

// NoteFiles returns every path that can anchor a note. Renames include both
// names because left-side notes use the source path and right-side notes use
// the destination path.
func (s Spec) NoteFiles() ([]string, error) {
	s.NoLazyFetch = true
	args, err := s.noteArgs("never")
	if err != nil {
		return nil, err
	}
	args = append([]string{args[0], "--name-status", "-z"}, args[1:]...)
	command := s.command(args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	records := bytes.Split(stdout.Bytes(), []byte{0})
	files := []string{}
	seen := map[string]bool{}
	for index := 0; index < len(records) && len(records[index]) != 0; {
		status := string(records[index])
		index++
		paths := 1
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			paths = 2
		}
		if index+paths > len(records) {
			return nil, errors.New("git returned a malformed name-status record")
		}
		for range paths {
			path := string(records[index])
			index++
			if path != "" && !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
		}
	}
	return files, nil
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

func (s Spec) args(color string) ([]string, error) {
	if s.Staged && s.To != "" {
		return nil, errors.New("staged comparisons accept at most one revision")
	}
	args := []string{"diff", "--color=" + color, "--find-renames", "--src-prefix=a/", "--dst-prefix=b/"}
	args = append(args, "--no-ext-diff", "--no-textconv")
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
	return args, nil
}

func (s Spec) noteArgs(color string) ([]string, error) {
	args, err := s.args(color)
	if err != nil {
		return nil, err
	}
	filtered := args[:1]
	for _, argument := range args[1:] {
		if argument != "--find-renames" {
			filtered = append(filtered, argument)
		}
	}
	args = filtered
	canonical := []string{
		"--unified=3",
		"--inter-hunk-context=0",
		"--diff-algorithm=myers",
		"--no-indent-heuristic",
		"--find-renames=50%",
		"-l0",
		"--no-relative",
		"-O/dev/null",
		"--full-index",
		"--submodule=short",
		"--ignore-submodules=none",
	}
	return append(append([]string{args[0]}, canonical...), args[1:]...), nil
}

func (s Spec) noteIdentityArgs() ([]string, error) {
	if s.Staged && s.To != "" {
		return nil, errors.New("staged comparisons accept at most one revision")
	}
	args := []string{
		"diff",
		"--raw",
		"-z",
		"--abbrev=64",
		"--diff-algorithm=myers",
		"--find-renames=50%",
		"-l0",
		"--no-relative",
		"-O/dev/null",
		"--submodule=short",
		"--ignore-submodules=none",
		"--color=never",
		"--no-ext-diff",
		"--no-textconv",
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
	return args, nil
}

func (s Spec) runDiff(color string) (string, error) {
	args, err := s.args(color)
	if err != nil {
		return "", err
	}
	return s.runDiffArgs(args)
}

func (s Spec) runDiffArgs(args []string) (string, error) {
	command := s.command(args...)
	stdout := limitedBuffer{limit: MaxPatchBytes}
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	if stdout.exceeded {
		return "", fmt.Errorf("git patch exceeds %d bytes", MaxPatchBytes)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s Spec) command(arguments ...string) *exec.Cmd {
	if s.NoLazyFetch {
		arguments = append([]string{
			"-c", "core.fsmonitor=false",
		}, arguments...)
	}
	command := exec.Command("git", arguments...)
	command.Dir = s.Dir
	command.Env = git.CleanEnv()
	if s.NoLazyFetch {
		environment := command.Env[:0]
		for _, entry := range command.Env {
			if strings.HasPrefix(entry, "GIT_NO_LAZY_FETCH=") ||
				strings.HasPrefix(entry, "GIT_NO_REPLACE_OBJECTS=") ||
				strings.HasPrefix(entry, "GIT_REPLACE_REF_BASE=") {
				continue
			}
			environment = append(environment, entry)
		}
		command.Env = append(environment, "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1")
	}
	return command
}

func (s Spec) output(arguments ...string) (string, error) {
	command := s.command(arguments...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		verb := ""
		if len(arguments) > 0 {
			verb = arguments[0]
		}
		return "", fmt.Errorf("git %s failed in %s: %s: %w", verb, s.Dir, strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - buffer.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = buffer.Buffer.Write(value[:remaining])
	}
	if remaining < len(value) {
		buffer.exceeded = true
	}
	return written, nil
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
