// Package mcpserver serves zanadir's scan and fix as Model Context Protocol
// tools, so a coding agent can check a repository's CI before changing it.
package mcpserver

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Target names the repository a tool reads.
type Target struct {
	Dir                string   `json:"dir,omitempty" jsonschema:"path to a local checkout; defaults to the current directory"`
	Repo               string   `json:"repo,omitempty" jsonschema:"a GitHub repository to read without cloning, as github.com/owner/name or github.com/owner/name@ref; instead of dir"`
	ExcludedCategories []string `json:"excludedCategories,omitempty" jsonschema:"categories that do not apply to this repository, such as Performance Testing"`
}

const scanDescription = `Reads the repository's CI configuration (GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, Azure Pipelines, Jenkins) and reports, as JSON:
- score: categories covered out of those that apply;
- uncovered: categories with no tool at all (SCA, secrets detection, SAST, linting, tests, coverage, licences, end-of-life, IaC, supply chain, workflow security, performance testing), each with tools that fit the repository's languages;
- weakened: controls that are present but cannot fail a build, graded partial (not on every change), advisory (continue-on-error, || true, --exit-code 0, schedule-only) or unknown (behind a condition);
- notApplicable and platforms.
Run it before changing CI, and again after, to confirm the change.`

const fixDescription = `Prints ready-to-paste CI configuration for each uncovered category, for the CI platform the repository uses. It changes no files: adapt the snippets into the existing workflows.`

// fetch, setup and stdio are replaced in tests.
var (
	fetch               = remote.FetchContext
	setup               = handler.Setup
	stdio mcp.Transport = &mcp.StdioTransport{}
)

// New returns a server with the scan and fix tools, reporting version.
func New(version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "zanadir", Title: "zanadir", Version: version}, nil)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(true)}
	mcp.AddTool(server, &mcp.Tool{Name: "scan", Title: "Grade the CI", Description: scanDescription, Annotations: readOnly}, scan)
	mcp.AddTool(server, &mcp.Tool{Name: "fix", Title: "CI configuration for the gaps", Description: fixDescription, Annotations: readOnly}, fix)
	return server
}

// Run serves over stdin and stdout until ctx ends or the client leaves.
func Run(ctx context.Context, version string) error {
	return New(version).Run(ctx, stdio)
}

func scan(ctx context.Context, _ *mcp.CallToolRequest, target Target) (*mcp.CallToolResult, any, error) {
	return run(ctx, target, func(h *handler.Handler, cfg *config.Config, out *bytes.Buffer) error {
		cfg.Output, cfg.Out = config.OutputJSON, out
		return h.Execute(cfg)
	})
}

func fix(ctx context.Context, _ *mcp.CallToolRequest, target Target) (*mcp.CallToolResult, any, error) {
	return run(ctx, target, func(h *handler.Handler, cfg *config.Config, out *bytes.Buffer) error {
		return h.Fix(cfg, out)
	})
}

func run(ctx context.Context, target Target, do func(*handler.Handler, *config.Config, *bytes.Buffer) error) (*mcp.CallToolResult, any, error) {
	cfg, cleanup, err := configure(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	h, err := setup()
	if err != nil {
		return nil, nil, err
	}
	var out bytes.Buffer
	if err := do(h, cfg, &out); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.String()}}}, nil, nil
}

func configure(ctx context.Context, target Target) (*config.Config, func(), error) {
	cfg := &config.Config{}
	for _, name := range target.ExcludedCategories {
		category, ok := models.ResolveCategory(name)
		if !ok {
			return nil, nil, fmt.Errorf("unknown category %q; categories are %v", name, models.CategoryNames())
		}
		cfg.ExcludedCategories = append(cfg.ExcludedCategories, string(category))
	}
	if target.Repo == "" {
		dir, err := filepath.Abs(cmp.Or(target.Dir, "."))
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
