package output

import (
	"bytes"
	"encoding/json"
	"errors"
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

// A clean repository used to render an empty table: three border lines and a
// header row with no body, which reads as a bug rather than a pass.
func TestResponse_TableStatesWhenNothingToSuggest(t *testing.T) {
	service := NewOutputService()

	out := captureStdout(func() {
		if err := service.Response(nil, config.OutputTable, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "All categories are covered") {
		t.Errorf("expected an explicit all-clear, got:\n%q", out)
	}
	if strings.Contains(out, "CATEGORY") || strings.Contains(out, "|---") {
		t.Errorf("expected no table when there is nothing to show, got:\n%s", out)
	}
}

func TestResponse_TableLeadsWithACount(t *testing.T) {
	service := NewOutputService()

	out := captureStdout(func() {
		if err := service.Response(getSampleSuggestions(), config.OutputTable, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.HasPrefix(out, "2 categories need attention:") {
		t.Errorf("expected the count to lead the output, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "suggested tools") {
		t.Errorf("table should still render below the headline:\n%s", out)
	}
}

func TestHeadlineIsGrammatical(t *testing.T) {
	for count, want := range map[int]string{
		0: "All categories are covered - no suggestions.",
		1: "1 category needs attention:",
		2: "2 categories need attention:",
	} {
		if got := headline(count); got != want {
			t.Errorf("headline(%d) = %q, want %q", count, got, want)
		}
	}
}

// Escape codes in a redirected report would corrupt it for any consumer.
func TestResponse_NoColourWhenNotATerminal(t *testing.T) {
	service := NewOutputService()
	path := filepath.Join(t.TempDir(), "report.txt")

	if err := service.Response(getSampleSuggestions(), config.OutputTable, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	if bytes.Contains(data, []byte("\x1b[")) {
		t.Errorf("file report contains ANSI escapes:\n%q", data)
	}

	out := captureStdout(func() {
		_ = service.Response(getSampleSuggestions(), config.OutputTable, "")
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("piped stdout contains ANSI escapes:\n%q", out)
	}
}

// The machine formats must not gain a human headline.
func TestResponse_MachineFormatsHaveNoHeadline(t *testing.T) {
	service := NewOutputService()

	for _, format := range []string{"json", config.OutputSARIF} {
		out := captureStdout(func() {
			if err := service.Response(getSampleSuggestions(), format, ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if !strings.HasPrefix(strings.TrimSpace(out), "[") && !strings.HasPrefix(strings.TrimSpace(out), "{") {
			t.Errorf("%s output should start with its own payload, got:\n%.80s", format, out)
		}
		if strings.Contains(out, "need attention") {
			t.Errorf("%s output must not contain the human headline", format)
		}
	}
}

// failingWriter reports an error on every write, standing in for a full disk or
// a closed pipe.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestUseColour(t *testing.T) {
	t.Run("not a file", func(t *testing.T) {
		if useColour(&bytes.Buffer{}) {
			t.Error("a plain writer is not a terminal")
		}
	})

	t.Run("regular file", func(t *testing.T) {
		f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()

		if useColour(f) {
			t.Error("a regular file is not a terminal")
		}
	})

	t.Run("NO_COLOR wins over everything", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		if useColour(os.Stdout) {
			t.Error("NO_COLOR must suppress colour even on a terminal")
		}
	})
}

func TestPaint(t *testing.T) {
	if got := paint(false, ansiBold, "plain"); got != "plain" {
		t.Errorf("disabled paint should not wrap: %q", got)
	}
	if got := paint(true, ansiBold, "loud"); got != ansiBold+"loud"+ansiReset {
		t.Errorf("enabled paint should wrap in the style: %q", got)
	}
}

// A report that cannot be written must surface the failure rather than
// reporting success on output nobody received.
func TestRenderPropagatesWriteErrors(t *testing.T) {
	boom := errors.New("disk full")
	w := failingWriter{err: boom}

	for _, format := range []string{config.OutputTable, config.OutputSARIF, "json"} {
		t.Run(format, func(t *testing.T) {
			err := render(w, getSampleSuggestions(), format)
			if !errors.Is(err, boom) {
				t.Errorf("expected the write error to propagate, got %v", err)
			}
		})
	}
}

func TestRenderPropagatesWriteErrorOnAllClear(t *testing.T) {
	boom := errors.New("disk full")

	if err := render(failingWriter{err: boom}, nil, config.OutputTable); !errors.Is(err, boom) {
		t.Errorf("expected the write error to propagate, got %v", err)
	}
}

// A report is written to be consumed by another process, often another user:
// the Docker action runs as root while the following step runs as the runner
// user. An owner-only report is unreadable there, which broke SARIF upload.
func TestResponse_ReportIsReadableByOthers(t *testing.T) {
	service := NewOutputService()
	path := filepath.Join(t.TempDir(), "zanadir.sarif")

	if err := service.Response(getSampleSuggestions(), config.OutputSARIF, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o044 == 0 {
		t.Errorf("report mode %#o is owner-only; another user cannot read it "+
			"(if this fails locally, check for a restrictive umask)", perm)
	}
}
