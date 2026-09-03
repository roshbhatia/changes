package engine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/roshbhatia/changes/internal/source"
	"github.com/roshbhatia/go-utils/diffview"
	gitutil "github.com/roshbhatia/go-utils/git"
)

var (
	PatchNames = []string{"builtin", "filter"}
	FileNames  = []string{"builtin", "difftool"}
)

type Options struct {
	Color    string
	Difftool []string
	Filter   []string
	Label    string
	Layout   string
	Width    int
}

func validateName(name string, names []string, kind string) error {
	found := false
	for _, candidate := range names {
		found = found || name == candidate
	}
	if !found {
		return fmt.Errorf("unknown %s engine %q", kind, name)
	}
	return nil
}

func ValidatePatch(name string, options Options) error {
	if err := validateName(name, PatchNames, "patch"); err != nil {
		return err
	}
	if options.Layout != "side-by-side" && options.Layout != "unified" {
		return fmt.Errorf("layout must be side-by-side or unified")
	}
	if name == "filter" && len(options.Filter) == 0 {
		return errors.New("filter engine requires diff.filter in config or -filter")
	}
	return nil
}

func ValidateFiles(name string, options Options) error {
	if err := validateName(name, FileNames, "file"); err != nil {
		return err
	}
	if options.Layout != "side-by-side" && options.Layout != "unified" {
		return fmt.Errorf("layout must be side-by-side or unified")
	}
	if name == "difftool" && len(options.Difftool) == 0 {
		return errors.New("difftool engine requires diff.difftool in config or -difftool")
	}
	return nil
}

func Patch(name string, patch string, options Options) (string, error) {
	patch = labelPatch(patch, "", "", options.Label)
	switch name {
	case "builtin":
		if options.Layout == "unified" {
			return normalizeColor(patch, options.Color), nil
		}
		return diffview.Render(diffview.Options{
			Files:   diffview.Parse(patch),
			Unified: false,
			Width:   options.Width,
		}), nil
	case "filter":
		if hasFilePlaceholder(options.Filter) {
			return "", errors.New("patch filter must not contain $LOCAL, $REMOTE, or $MERGED; configure diff.difftool for file comparison")
		}
		out, err := pipe(options.Filter, patch)
		return normalizeColor(out, options.Color), err
	default:
		return "", fmt.Errorf("%s needs repository context", name)
	}
}

// Files compares two paths with the built-in fallback or configured difftool.
func Files(name, local, remote string, options Options) (string, error) {
	if name == "difftool" {
		return Difftool(local, remote, options)
	}
	color := "never"
	if options.Layout == "unified" {
		color = options.Color
	}
	patch, err := source.FileDiff(local, remote, color)
	if err != nil {
		return "", err
	}
	patch = labelPatch(patch, local, remote, options.Label)
	return Patch("builtin", patch, options)
}

func Difftool(local, remote string, options Options) (string, error) {
	command := expand(options.Difftool, local, remote, options.Label)
	if !hasFilePlaceholder(options.Difftool) {
		command = append(command, local, remote)
	}
	out, err := output(command, true)
	return normalizeColor(out, options.Color), err
}

func labelPatch(patch, local, remote, label string) string {
	if label == "" {
		return patch
	}
	label = filepath.ToSlash(label)
	paths := []string{filepath.ToSlash(local), filepath.ToSlash(remote)}
	for index, path := range paths {
		paths[index] = strings.TrimPrefix(path, "/")
	}
	lines := strings.Split(patch, "\n")
	for index, line := range lines {
		plain := ansi.Strip(line)
		if !strings.HasPrefix(plain, "diff --git ") && !strings.HasPrefix(plain, "--- ") && !strings.HasPrefix(plain, "+++ ") {
			continue
		}
		for _, path := range paths {
			if path != "" {
				line = strings.ReplaceAll(line, path, label)
			}
		}
		lines[index] = line
	}
	return strings.Join(lines, "\n")
}

func pipe(command []string, input string) (string, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = strings.NewReader(input)
	return run(cmd, false)
}

func output(command []string, differencesOK bool) (string, error) {
	cmd := exec.Command(command[0], command[1:]...)
	return run(cmd, differencesOK)
}

func run(cmd *exec.Cmd, differencesOK bool) (string, error) {
	cmd.Env = os.Environ()
	if filepath.Base(cmd.Path) == "git" {
		cmd.Env = gitutil.CleanEnv()
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if !differencesOK || !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("%s: %s: %w", cmd.Path, strings.TrimSpace(stderr.String()), err)
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

func normalizeColor(output, color string) string {
	if color == "never" {
		return ansi.Strip(output)
	}
	return output
}

func expand(command []string, local, remote, merged string) []string {
	out := make([]string, len(command))
	for index, argument := range command {
		replacer := strings.NewReplacer(
			"$LOCAL", local,
			"$REMOTE", remote,
			"$MERGED", merged,
		)
		out[index] = replacer.Replace(argument)
	}
	return out
}

func hasFilePlaceholder(command []string) bool {
	for _, argument := range command {
		if strings.Contains(argument, "$LOCAL") || strings.Contains(argument, "$REMOTE") || strings.Contains(argument, "$MERGED") {
			return true
		}
	}
	return false
}
