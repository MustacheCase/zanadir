// Package mcpserver serves zanadir's scan and fix as Model Context Protocol
// tools over stdio, so a coding agent can check a repository's CI before
// changing it.
package mcpserver

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
)

// Target names the repository a tool reads.
type Target struct {
	Dir                string   `json:"dir"`
	Repo               string   `json:"repo"`
	ExcludedCategories []string `json:"excludedCategories"`
}

const scanDescription = `Reads the repository's CI configuration (GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, Azure Pipelines, Jenkins) and reports, as JSON:
- score: categories covered out of those that apply;
- uncovered: categories with no tool at all (SCA, secrets detection, SAST, linting, tests, coverage, licences, end-of-life, IaC, supply chain, workflow security, performance testing), each with tools that fit the repository's languages;
- weakened: controls that are present but cannot fail a build, graded partial (not on every change), advisory (continue-on-error, || true, --exit-code 0, schedule-only) or unknown (behind a condition);
- notApplicable and platforms.
Run it before changing CI, and again after, to confirm the change.`

const fixDescription = `Prints ready-to-paste CI configuration for each uncovered category, for the CI platform the repository uses. It changes no files: adapt the snippets into the existing workflows.`

var targetSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "dir": {"type": "string", "description": "path to a local checkout; defaults to the current directory"},
    "repo": {"type": "string", "description": "a GitHub repository to read without cloning, as github.com/owner/name or github.com/owner/name@ref; instead of dir"},
    "excludedCategories": {"type": "array", "items": {"type": "string"}, "description": "categories that do not apply to this repository, such as Performance Testing"}
  },
  "additionalProperties": false
}`)

type tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations map[string]any  `json:"annotations"`
	do          func(*handler.Handler, *config.Config, *bytes.Buffer) error
}

var readOnly = map[string]any{"readOnlyHint": true, "idempotentHint": true, "openWorldHint": true}

var tools = []tool{
	{Name: "scan", Title: "Grade the CI", Description: scanDescription, InputSchema: targetSchema, Annotations: readOnly,
		do: func(h *handler.Handler, cfg *config.Config, out *bytes.Buffer) error {
			cfg.Output, cfg.Out = config.OutputJSON, out
			return h.Execute(cfg)
		}},
	{Name: "fix", Title: "CI configuration for the gaps", Description: fixDescription, InputSchema: targetSchema, Annotations: readOnly,
		do: func(h *handler.Handler, cfg *config.Config, out *bytes.Buffer) error { return h.Fix(cfg, out) }},
}

// protocolVersions are the versions this server speaks, newest first.
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// fetch, setup, stdin and readerExited are replaced in tests.
var (
	fetch                  = remote.FetchContext
	setup                  = handler.Setup
	stdin        io.Reader = os.Stdin
	readerExited           = func() {}
)

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Options say what the server reports and where its tools read by default.
type Options struct {
	Version string
	// Dir is the repository a tool reads when the call names none; empty
	// means the working directory.
	Dir string
}

// Run serves on stdin and stdout until the client closes stdin or ctx ends.
func Run(ctx context.Context, opts Options) error {
	return Serve(ctx, stdin, os.Stdout, opts)
}

// Serve answers the newline-delimited JSON-RPC messages read from in on out.
func Serve(ctx context.Context, in io.Reader, out io.Writer, opts Options) error {
	lines := make(chan []byte)
	ended := make(chan error, 1)
	go func() {
		defer readerExited()
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 64<<10), 16<<20)
		for scanner.Scan() {
			select {
			case lines <- slices.Clone(scanner.Bytes()):
			case <-ctx.Done():
				return
			}
		}
		ended <- scanner.Err()
	}()
	encoder := json.NewEncoder(out)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-ended:
			return err
		case line := <-lines:
			if reply := handle(ctx, line, opts); reply != nil {
				if err := encoder.Encode(reply); err != nil {
					return err
				}
			}
		}
	}
}

func handle(ctx context.Context, line []byte, opts Options) *response {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error: " + err.Error()}}
	}
	if req.ID == nil {
		return nil
	}
	reply := &response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		agreed := protocolVersions[0]
		if slices.Contains(protocolVersions, params.ProtocolVersion) {
			agreed = params.ProtocolVersion
		}
		reply.Result = map[string]any{
			"protocolVersion": agreed,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "zanadir", "title": "zanadir", "version": opts.Version},
		}
	case "ping":
		reply.Result = map[string]any{}
	case "tools/list":
		reply.Result = map[string]any{"tools": tools}
	case "tools/call":
		reply.Result, reply.Error = callTool(ctx, req.Params, opts.Dir)
	default:
		reply.Error = &rpcError{-32601, "method not found: " + req.Method}
	}
	return reply
}

func callTool(ctx context.Context, raw json.RawMessage, dir string) (any, *rpcError) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &rpcError{-32602, "invalid params: " + err.Error()}
	}
	i := slices.IndexFunc(tools, func(t tool) bool { return t.Name == params.Name })
	if i < 0 {
		return nil, &rpcError{-32602, fmt.Sprintf("unknown tool %q", params.Name)}
	}
	text, err := run(ctx, tools[i], params.Arguments, dir)
	if err != nil {
		text = err.Error()
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": err != nil}, nil
}

func run(ctx context.Context, t tool, arguments json.RawMessage, dir string) (string, error) {
	var target Target
	if len(arguments) > 0 && string(arguments) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(arguments))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&target); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	cfg, cleanup, err := configure(ctx, target, dir)
	if err != nil {
		return "", err
	}
	defer cleanup()
	h, err := setup()
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.do(h, cfg, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

func configure(ctx context.Context, target Target, defaultDir string) (*config.Config, func(), error) {
	cfg := &config.Config{}
	for _, name := range target.ExcludedCategories {
		category, ok := models.ResolveCategory(name)
		if !ok {
			return nil, nil, fmt.Errorf("unknown category %q; categories are %v", name, models.CategoryNames())
		}
		cfg.ExcludedCategories = append(cfg.ExcludedCategories, string(category))
	}
	if target.Repo == "" {
		dir, err := filepath.Abs(cmp.Or(target.Dir, defaultDir, "."))
		cfg.Dir = dir
		return cfg, func() {}, err
	}
	if target.Dir != "" {
		return nil, nil, errors.New("give dir or repo, not both")
	}
	repo, err := remote.Parse(target.Repo)
	if err != nil {
		return nil, nil, err
	}
	dir, cleanup, err := fetch(ctx, repo, remote.Token())
	if err != nil {
		return nil, nil, err
	}
	cfg.Dir = dir
	return cfg, cleanup, nil
}
