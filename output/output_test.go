package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/suggester"
)

func getSampleSuggestions() []*suggester.CategorySuggestion {
	return []*suggester.CategorySuggestion{
		{
			Name:        "Secrets",
			Description: "Detect hardcoded secrets in source code repositories using specialized tools.",
			Suggestions: []*suggester.Suggestion{
				{Name: "Gitleaks"},
				{Name: "TruffleHog"},
			},
		},
		{
			Name:        "Licenses",
			Description: "Analyze open source license usage and compliance.",
			Suggestions: []*suggester.Suggestion{
				{Name: "FOSSA"},
			},
		},
	}
}

func captureStdout(f func()) string {
	var buf bytes.Buffer
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = stdout
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestResponse_JSONOutput(t *testing.T) {
	service := NewOutputService()
	suggestions := getSampleSuggestions()

	out := captureStdout(func() {
		err := service.Response(suggestions, "json", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Validate that it's valid JSON
	var result []*suggester.CategorySuggestion
	err := json.Unmarshal([]byte(out), &result)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if len(result) != len(suggestions) {
		t.Errorf("expected %d suggestions, got %d", len(suggestions), len(result))
	}
}

func TestResponse_TableOutput(t *testing.T) {
	service := NewOutputService()
	suggestions := getSampleSuggestions()

	out := captureStdout(func() {
		err := service.Response(suggestions, config.OutputTable, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	lowerOut := strings.ToLower(out)
	if !strings.Contains(lowerOut, "category") || !strings.Contains(lowerOut, "description") || !strings.Contains(lowerOut, "suggested tools") {
		t.Errorf("table header is missing or incorrect in output:\n%v", out)
	}
}

func TestResponse_SARIFOutput(t *testing.T) {
	service := NewOutputService()
	suggestions := getSampleSuggestions()

	out := captureStdout(func() {
		err := service.Response(suggestions, config.OutputSARIF, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var report sarifLog
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}

	if report.Version != sarifVersion {
		t.Errorf("expected SARIF version %q, got %q", sarifVersion, report.Version)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}
	if got := len(report.Runs[0].Results); got != len(suggestions) {
		t.Errorf("expected %d results, got %d", len(suggestions), got)
	}
}

func TestWrapTextEmptyInput(t *testing.T) {
	if got := wrapText("   ", 10); got != "" {
		t.Errorf("expected empty string for blank input, got %q", got)
	}
}

// Writing to a file is what lets CI hand a SARIF report to a consumer such as
// GitHub code scanning, so the report must land in the file and not on stdout.
func TestResponse_WritesSARIFToFile(t *testing.T) {
	service := NewOutputService()
	suggestions := getSampleSuggestions()
	path := filepath.Join(t.TempDir(), "zanadir.sarif")

	out := captureStdout(func() {
		if err := service.Response(suggestions, config.OutputSARIF, path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.TrimSpace(out) != "" {
		t.Errorf("expected nothing on stdout when writing to a file, got:\n%s", out)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report file not written: %v", err)
	}
	var report sarifLog
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("file is not valid SARIF: %v", err)
	}
	if len(report.Runs) != 1 || len(report.Runs[0].Results) != len(suggestions) {
		t.Errorf("expected 1 run with %d results, got %d runs", len(suggestions), len(report.Runs))
	}
}

func TestResponse_WritesTableToFile(t *testing.T) {
	service := NewOutputService()
	path := filepath.Join(t.TempDir(), "report.txt")

	if err := service.Response(getSampleSuggestions(), config.OutputTable, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report file not written: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "suggested tools") {
		t.Errorf("table header missing from file:\n%s", data)
	}
}

// A report the operator asked for but never got is worse than no report.
func TestResponse_ReportsUnwritableDestination(t *testing.T) {
	service := NewOutputService()
	path := filepath.Join(t.TempDir(), "no-such-dir", "zanadir.sarif")

	err := service.Response(getSampleSuggestions(), config.OutputSARIF, path)
	if err == nil {
		t.Fatal("expected an error for an unwritable destination")
	}
	if !strings.Contains(err.Error(), "failed to open output file") {
		t.Errorf("unexpected error: %v", err)
	}
}
