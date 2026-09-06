// changes-provider-github-pr projects GitHub pull request review threads into
// the Changes note protocol. It is read-only.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/roshbhatia/changes/internal/provider"
	gitutil "github.com/roshbhatia/go-utils/git"
)

const (
	providerName = "github-pr"
	maxOutput    = 16 << 20
	maxPages     = 100
)

const reviewThreadsQuery = `query($owner: String!, $name: String!, $number: Int!, $endCursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $endCursor) {
        nodes {
          id
          isOutdated
          isResolved
          path
          diffSide
          startDiffSide
          startLine
          line
          originalStartLine
          originalLine
          subjectType
          comments(first: 100) {
            nodes {
              id
              body
              url
              createdAt
              updatedAt
              author { login }
              replyTo { id }
              commit { oid }
              originalCommit { oid }
            }
            pageInfo { hasNextPage endCursor }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type commandRunner func(directory string, arguments ...string) ([]byte, error)
type mergeBaseResolver func(directory, host, owner, name, base, head string) (string, error)

type pullView struct {
	Number     int    `json:"number"`
	BaseRefOID string `json:"baseRefOid"`
	HeadRefOID string `json:"headRefOid"`
	URL        string `json:"url"`
}

type graphPage struct {
	Data struct {
		Repository *struct {
			PullRequest *struct {
				ReviewThreads threadConnection `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type threadConnection struct {
	Nodes    []reviewThread `json:"nodes"`
	PageInfo pageInfo       `json:"pageInfo"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type reviewThread struct {
	ID                string            `json:"id"`
	IsOutdated        bool              `json:"isOutdated"`
	IsResolved        bool              `json:"isResolved"`
	Path              string            `json:"path"`
	DiffSide          string            `json:"diffSide"`
	StartDiffSide     *string           `json:"startDiffSide"`
	StartLine         *int              `json:"startLine"`
	Line              *int              `json:"line"`
	OriginalStartLine *int              `json:"originalStartLine"`
	OriginalLine      *int              `json:"originalLine"`
	SubjectType       string            `json:"subjectType"`
	Comments          commentConnection `json:"comments"`
}

type commentConnection struct {
	Nodes    []reviewComment `json:"nodes"`
	PageInfo pageInfo        `json:"pageInfo"`
}

type reviewComment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Author    *struct {
		Login string `json:"login"`
	} `json:"author"`
	ReplyTo *struct {
		ID string `json:"id"`
	} `json:"replyTo"`
	Commit *struct {
		OID string `json:"oid"`
	} `json:"commit"`
	OriginalCommit *struct {
		OID string `json:"oid"`
	} `json:"originalCommit"`
}

func main() {
	request, err := decodeRequest(os.Stdin)
	if err != nil {
		die("decode request: %v", err)
	}
	if request.Version != provider.ProtocolVersion || request.Action != provider.ActionNotes {
		die("expected %s %s", provider.ProtocolVersion, provider.ActionNotes)
	}
	if !request.Validation {
		matched, err := hasGitHubRemote(request.Directory, githubHostConfigured)
		if err != nil {
			die("discover GitHub host: %v", err)
		}
		if !matched {
			if err := json.NewEncoder(os.Stdout).Encode(provider.Response{
				Version: provider.ProtocolVersion, Notes: []provider.Note{},
			}); err != nil {
				die("encode response: %v", err)
			}
			return
		}
	}
	response, err := notes(request, runGH, githubMergeBase)
	if err != nil {
		die("%v", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		die("encode response: %v", err)
	}
}

func decodeRequest(reader io.Reader) (provider.Request, error) {
	return decodeRequestWithLimit(reader, maxOutput)
}

func decodeRequestWithLimit(reader io.Reader, limit int64) (provider.Request, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return provider.Request{}, err
	}
	if int64(len(data)) > limit {
		return provider.Request{}, fmt.Errorf("request exceeds %d bytes", limit)
	}
	request := provider.Request{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return provider.Request{}, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return provider.Request{}, errors.New("request must contain one JSON value")
	}
	return request, nil
}

func hasGitHubRemote(directory string, configured func(string) (bool, error)) (bool, error) {
	output, err := gitutil.Output(directory, "remote")
	if err != nil {
		return false, nil
	}
	for _, name := range strings.Fields(output) {
		remotes, err := gitutil.Output(directory, "remote", "get-url", "--all", name)
		if err != nil {
			continue
		}
		for _, remote := range strings.Split(remotes, "\n") {
			host := remoteHost(remote)
			if strings.EqualFold(host, "github.com") {
				return true, nil
			}
			if host != "" {
				known, err := configured(host)
				if err != nil {
					return false, err
				}
				if known {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func isGitHubRemote(value string) bool {
	return strings.EqualFold(remoteHost(value), "github.com")
}

func remoteHost(value string) string {
	value = strings.TrimSpace(value)
	if before, _, ok := strings.Cut(value, ":"); ok && !strings.Contains(value, "://") && !strings.Contains(before, "/") {
		host := before
		if _, suffix, found := strings.Cut(host, "@"); found {
			host = suffix
		}
		return strings.ToLower(host)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	switch parsed.Scheme {
	case "git", "http", "https", "ssh":
		return urlAuthority(parsed)
	default:
		return ""
	}
}

func notes(request provider.Request, run commandRunner, resolveBase mergeBaseResolver) (provider.Response, error) {
	if request.Validation {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{validationNote()}}, nil
	}
	if request.Head == "" {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
	}
	pullData, err := run(request.Directory, "pr", "view", "--json", "number,baseRefOid,headRefOid,url")
	if err != nil {
		if noPullRequest(err) {
			return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
		}
		return provider.Response{}, fmt.Errorf("find pull request: %w", err)
	}
	var pull pullView
	if err := decodeOne(pullData, &pull); err != nil {
		return provider.Response{}, fmt.Errorf("decode pull request: %w", err)
	}
	if pull.Number <= 0 || pull.BaseRefOID == "" || pull.HeadRefOID == "" {
		return provider.Response{}, errors.New("pull request number or comparison object is missing")
	}
	if request.Head != pull.HeadRefOID {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
	}
	host, owner, name, err := pullRepository(pull)
	if err != nil {
		return provider.Response{}, err
	}
	comparisonBase, err := resolveBase(request.Directory, host, owner, name, pull.BaseRefOID, pull.HeadRefOID)
	if err != nil {
		return provider.Response{}, fmt.Errorf("resolve pull request merge base: %w", err)
	}
	if request.Base != comparisonBase || request.Head != pull.HeadRefOID {
		return provider.Response{Version: provider.ProtocolVersion, Notes: []provider.Note{}}, nil
	}
	all := []provider.Note{}
	cursor := ""
	totalOutput := 0
	for page := 0; page < maxPages; page++ {
		arguments := []string{
			"api", "graphql", "--hostname", host, "-f", "query=" + reviewThreadsQuery,
			"-F", "owner=" + owner, "-F", "name=" + name,
			"-F", "number=" + strconv.Itoa(pull.Number),
		}
		if cursor != "" {
			arguments = append(arguments, "-F", "endCursor="+cursor)
		}
		data, err := run(request.Directory, arguments...)
		if err != nil {
			return provider.Response{}, fmt.Errorf("read review threads: %w", err)
		}
		totalOutput, err = addOutputSize(totalOutput, len(data), maxOutput)
		if err != nil {
			return provider.Response{}, err
		}
		var response graphPage
		if err := decodeOne(data, &response); err != nil {
			return provider.Response{}, fmt.Errorf("decode review threads: %w", err)
		}
		if len(response.Errors) > 0 {
			messages := make([]string, 0, len(response.Errors))
			for _, graphError := range response.Errors {
				messages = append(messages, cleanOneLine(graphError.Message))
			}
			return provider.Response{}, fmt.Errorf("GraphQL: %s", strings.Join(messages, "; "))
		}
		if response.Data.Repository == nil || response.Data.Repository.PullRequest == nil {
			return provider.Response{}, errors.New("GraphQL response omitted the pull request")
		}
		connection := response.Data.Repository.PullRequest.ReviewThreads
		for _, thread := range connection.Nodes {
			if thread.Comments.PageInfo.HasNextPage {
				return provider.Response{}, fmt.Errorf("review thread %s has more than 100 replies", thread.ID)
			}
			found, err := normalizeThread(request, pull, comparisonBase, thread)
			if err != nil {
				return provider.Response{}, err
			}
			all = append(all, found...)
		}
		if !connection.PageInfo.HasNextPage {
			return provider.Response{Version: provider.ProtocolVersion, Notes: all}, nil
		}
		if connection.PageInfo.EndCursor == "" || connection.PageInfo.EndCursor == cursor {
			return provider.Response{}, errors.New("review thread pagination did not advance")
		}
		cursor = connection.PageInfo.EndCursor
	}
	return provider.Response{}, fmt.Errorf("review threads exceed %d pages", maxPages)
}

func addOutputSize(total, next, limit int) (int, error) {
	if next < 0 || total > limit-next {
		return total, fmt.Errorf("review thread responses exceed %d bytes", limit)
	}
	return total + next, nil
}

func pullRepository(pull pullView) (string, string, string, error) {
	parsed, err := url.Parse(pull.URL)
	if err != nil || parsed.Hostname() == "" {
		return "", "", "", fmt.Errorf("pull request URL is invalid: %q", pull.URL)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] != "pull" {
		return "", "", "", fmt.Errorf("pull request URL is invalid: %q", pull.URL)
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number != pull.Number {
		return "", "", "", fmt.Errorf("pull request URL does not match pull request %d", pull.Number)
	}
	return urlAuthority(parsed), parts[0], parts[1], nil
}

func normalizeThread(request provider.Request, pull pullView, comparisonBase string, thread reviewThread) ([]provider.Note, error) {
	if thread.ID == "" || thread.Path == "" {
		return nil, errors.New("review thread identity or path is missing")
	}
	side := thread.DiffSide
	if side == "" && thread.StartDiffSide != nil {
		side = *thread.StartDiffSide
	}
	if side != provider.NoteSideLeft && side != provider.NoteSideRight {
		return nil, fmt.Errorf("review thread %s has invalid diff side %q", thread.ID, side)
	}
	line := number(thread.Line)
	start := number(thread.StartLine)
	startSide := ""
	if start > 0 {
		startSide = side
		if thread.StartDiffSide != nil {
			startSide = *thread.StartDiffSide
		}
	}
	state := provider.NoteStateOpen
	if thread.IsResolved {
		state = provider.NoteStateResolved
	}
	quality := provider.PlacementExact
	if thread.IsOutdated {
		quality = provider.PlacementOutdated
	}
	notes := make([]provider.Note, 0, len(thread.Comments.Nodes))
	for _, comment := range thread.Comments.Nodes {
		if comment.ID == "" {
			return nil, fmt.Errorf("review thread %s contains a comment without an id", thread.ID)
		}
		summary, rationale := splitBody(comment.Body)
		if summary == "" {
			summary = "Empty review comment"
		}
		head := pull.HeadRefOID
		if comment.OriginalCommit != nil && comment.OriginalCommit.OID != "" {
			head = comment.OriginalCommit.OID
		} else if comment.Commit != nil && comment.Commit.OID != "" {
			head = comment.Commit.OID
		}
		author := "unknown"
		if comment.Author != nil && comment.Author.Login != "" {
			author = comment.Author.Login
		}
		replyTo := ""
		if comment.ReplyTo != nil && comment.ReplyTo.ID != "" {
			replyTo = providerName + ":" + comment.ReplyTo.ID
		}
		anchorLine := number(thread.OriginalLine)
		if anchorLine == 0 {
			anchorLine = line
		}
		anchorStart := number(thread.OriginalStartLine)
		if anchorStart == 0 && anchorLine == line {
			anchorStart = start
		}
		anchorStartSide := ""
		if anchorStart > 0 {
			anchorStartSide = side
			if thread.StartDiffSide != nil {
				anchorStartSide = *thread.StartDiffSide
			}
		}
		notes = append(notes, provider.Note{
			ID: providerName + ":" + comment.ID, Source: providerName, SourceID: comment.ID,
			ThreadID: providerName + ":" + thread.ID, ReplyTo: replyTo,
			Summary: summary, Rationale: rationale, Author: author,
			Origin: provider.NoteOriginExternal, Authority: provider.NoteAuthorityExternal,
			State: state, CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt, URL: comment.URL,
			Anchor: provider.NoteAnchor{
				Path: thread.Path, Side: side, StartSide: anchorStartSide,
				StartLine: anchorStart, Line: anchorLine,
				Head: head, Target: provider.NoteTargetCommits,
			},
			Placement: provider.NotePlacement{
				Path: thread.Path, Side: side, StartSide: startSide,
				StartLine: start, Line: line, Base: comparisonBase, Head: pull.HeadRefOID,
				Fingerprint: request.Fingerprint, Target: provider.NoteTargetCommits, Quality: quality,
			},
		})
	}
	return notes, nil
}

func validationNote() provider.Note {
	return provider.Note{
		ID: providerName + ":validation", Source: providerName, SourceID: "validation",
		ThreadID: providerName + ":thread", Summary: "Review the ready change",
		Author: "provider-validation", Origin: provider.NoteOriginExternal,
		Authority: provider.NoteAuthorityExternal, State: provider.NoteStateOpen,
		Anchor: provider.NoteAnchor{
			Path: "main.ts", Side: provider.NoteSideRight, Line: 2,
			Target: provider.NoteTargetWorking,
		},
		Placement: provider.NotePlacement{
			Path: "main.ts", Side: provider.NoteSideRight, Line: 2,
			Fingerprint: "provider-validation", Target: provider.NoteTargetWorking,
			Quality: provider.PlacementExact,
		},
	}
}

func runGH(directory string, arguments ...string) ([]byte, error) {
	command := exec.Command("gh", arguments...)
	command.Dir = directory
	command.Env = githubEnvironment()
	stdout := cappedBuffer{limit: maxOutput}
	stderr := cappedBuffer{limit: maxOutput}
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("gh output exceeds %d bytes", maxOutput)
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = runErr.Error()
		}
		return nil, errors.New(message)
	}
	return stdout.Bytes(), nil
}

func githubEnvironment() []string {
	environment := gitutil.CleanEnv()
	cleaned := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "GH_REPO=") {
			cleaned = append(cleaned, entry)
		}
	}
	return cleaned
}

func urlAuthority(parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

func githubHostConfigured(host string) (bool, error) {
	data, err := runGH("", "auth", "status", "--json", "hosts")
	if err != nil {
		return false, err
	}
	status := struct {
		Hosts map[string]json.RawMessage `json:"hosts"`
	}{}
	if err := decodeOne(data, &status); err != nil {
		return false, err
	}
	for configured := range status.Hosts {
		if strings.EqualFold(configured, host) {
			return true, nil
		}
	}
	return false, nil
}

func githubMergeBase(directory, host, owner, name, base, head string) (string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/compare/%s...%s", owner, name, base, head)
	output, err := runGH(directory, "api", "--hostname", host, endpoint, "--jq", ".merge_base_commit.sha")
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", errors.New("GitHub compare returned no merge base")
	}
	return value, nil
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *cappedBuffer) Write(value []byte) (int, error) {
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

var noPullRequestPattern = regexp.MustCompile(`(?i)^(?:no pull requests found for branch|could not find a pull request for branch) (?:"[^"]+"|[^\s:]+)$`)

func noPullRequest(err error) bool {
	return noPullRequestPattern.MatchString(strings.TrimSpace(err.Error()))
}

func splitBody(body string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(cleanText(body), "\r\n", "\n"), "\n")
	for index, line := range lines {
		summary := strings.TrimSpace(line)
		if summary == "" {
			continue
		}
		return cleanOneLine(summary), strings.TrimSpace(strings.Join(lines[index+1:], "\n"))
	}
	return "", ""
}

func decodeOne(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON value has trailing data")
	}
	return nil
}

func number(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func cleanOneLine(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func cleanText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func die(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "changes-provider-github-pr: "+format+"\n", arguments...)
	os.Exit(1)
}
