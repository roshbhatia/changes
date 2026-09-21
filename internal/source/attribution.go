package source

import (
	"context"
	"fmt"
	"net/mail"
	"os/exec"
	"strconv"
	"strings"
)

type Contributor struct {
	Role     string `json:"role" jsonschema:"required"`
	Name     string `json:"name" jsonschema:"required"`
	Email    string `json:"email,omitempty"`
	Evidence string `json:"evidence" jsonschema:"required"`
	AI       bool   `json:"ai,omitempty"`
}

func Contributors(author, email, trailers string) []Contributor {
	result := []Contributor{{Role: "author", Name: author, Email: email, Evidence: "git author", AI: nativeAgent(author, email)}}
	for _, line := range strings.Split(trailers, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var role string
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "co-authored-by":
			role = "co-author"
		case "assisted-by":
			role = "assistant"
		case "ai-authored-by":
			role = "ai-author"
		case "ai-assisted-by":
			role = "ai-assistant"
		default:
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		name, address := value, ""
		if parsed, err := mail.ParseAddress(value); err == nil {
			name, address = parsed.Name, parsed.Address
			if name == "" {
				name = address
			}
		}
		knownAgent := false
		switch strings.ToLower(name) {
		case "codex", "claude", "claude code", "copilot", "github copilot", "cursor", "devin", "gemini", "opencode":
			knownAgent = true
		}
		result = append(result, Contributor{Role: role, Name: name, Email: address, Evidence: strings.TrimSpace(key), AI: strings.HasPrefix(role, "ai-") || role == "assistant" && knownAgent || nativeAgent(name, address)})
	}
	return result
}

type BlameLine struct {
	Commit   string
	Author   string
	Email    string
	Boundary bool
}

func Blame(ctx context.Context, directory, revision, path string, contents *string, ranges [][2]int) (map[int]BlameLine, error) {
	if len(ranges) == 0 {
		return map[int]BlameLine{}, nil
	}
	args := []string{"--no-optional-locks", "blame", "--line-porcelain"}
	for _, span := range ranges {
		args = append(args, "-L", fmt.Sprintf("%d,%d", span[0], span[1]))
	}
	if contents != nil {
		args = append(args, "--contents", "-")
	} else {
		args = append(args, revision)
	}
	args = append(args, "--", path)
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	if contents != nil {
		command.Stdin = strings.NewReader(*contents)
	}
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("read line attribution: %w", err)
	}
	result := map[int]BlameLine{}
	var current BlameLine
	var number int
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && len(fields[0]) == 40 {
			if parsed, err := strconv.Atoi(fields[2]); err == nil {
				current = BlameLine{Commit: fields[0]}
				number = parsed
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "author "):
			current.Author = strings.TrimPrefix(line, "author ")
		case strings.HasPrefix(line, "author-mail "):
			current.Email = strings.Trim(strings.TrimPrefix(line, "author-mail "), "<>")
		case line == "boundary":
			current.Boundary = true
		case strings.HasPrefix(line, "\t"):
			result[number] = current
		}
	}
	return result, nil
}

func CommitContributors(ctx context.Context, directory string, commits []string) (map[string][]Contributor, error) {
	result := map[string][]Contributor{}
	if len(commits) == 0 {
		return result, nil
	}
	args := []string{"show", "--no-patch", "--no-show-signature", "--format=%H%x00%an%x00%ae%x00%(trailers:only,unfold)%x00"}
	args = append(args, commits...)
	args = append(args, "--")
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(output), "\x00")
	for at := 0; at+3 < len(fields); at += 4 {
		result[strings.TrimSpace(fields[at])] = Contributors(fields[at+1], fields[at+2], fields[at+3])
	}
	return result, nil
}

func nativeAgent(name, email string) bool {
	name, email = strings.ToLower(name), strings.ToLower(email)
	return strings.HasPrefix(name, "claude") && email == "noreply@anthropic.com" || strings.HasPrefix(name, "codex") && (email == "codex@openai.com" || email == "noreply@openai.com") || name == "copilot" && strings.HasSuffix(email, "+copilot@users.noreply.github.com")
}
