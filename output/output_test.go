package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
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
		err := service.Response(suggestions, "json")
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
		err := service.Response(suggestions, config.OutputTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	lowerOut := strings.ToLower(out)
	if !strings.Contains(lowerOut, "category") || !strings.Contains(lowerOut, "description") || !strings.Contains(lowerOut, "suggested tools") {
		t.Errorf("table header is missing or incorrect in output:\n%v", out)
	}
}
