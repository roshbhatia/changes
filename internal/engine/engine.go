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
	"github.com/roshbhatia/go-utils/diffview"
	gitutil "github.com/roshbhatia/go-utils/git"
)

var Names = []string{"git", "internal", "command"}

type Options struct {
	Color   string
	Command []string
	Label   string
	Layout  string
	Width   int
}

func Validate(name string, options Options) error {
	found := false
	for _, candidate := range Names {
		found = found || name == candidate
	}
	if !found {
		return fmt.Errorf("unknown diff engine %q", name)
	}
	if options.Layout != "side-by-side" && options.Layout != "unified" {
		return fmt.Errorf("layout must be side-by-side or unified")
	}
	if name == "command" && len(options.Command) == 0 {
		return errors.New("command engine requires diff.command in config or -engine-command")
	}
	return nil
}

func Patch(name string, patch string, options Options) (string, error) {
	patch = labelPatch(patch, "", "", options.Label)
	switch name {
	case "git":
		return patch, nil
	case "internal":
		return diffview.Render(diffview.Options{
			Files:   diffview.Parse(patch),
			Unified: options.Layout == "unified",
			Width:   options.Width,
		}), nil
	case "command":
		out, err := pipe(options.Command, patch)
		return normalizeColor(out, options.Color), err
	default:
		return "", fmt.Errorf("%s needs repository context", name)
	}
}

func Files(name, local, remote string, options Options) (string, error) {
	if name == "command" {
		command := expand(options.Command, local, remote, options.Label)
		if !hasFilePlaceholder(options.Command) {
			command = append(command, local, remote)
		}
		out, err := output(command, true)
		return normalizeColor(out, options.Color), err
	}
	patch, err := output([]string{"git", "diff", "--no-index", "--color=never", "--", local, remote}, true)
	if err != nil {
		return "", err
	}
	if name == "git" {
		out, err := output([]string{"git", "diff", "--no-index", "--color=" + options.Color, "--", local, remote}, true)
		return labelPatch(out, local, remote, options.Label), err
	}
	return Patch(name, labelPatch(patch, local, remote, options.Label), options)
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
		if !strings.HasPrefix(line, "diff --git ") && !strings.HasPrefix(line, "--- ") && !strings.HasPrefix(line, "+++ ") {
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
		if strings.Contains(argument, "$LOCAL") || strings.Contains(argument, "$REMOTE") {
			return true
		}
	}
	return false
}
