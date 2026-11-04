package markdown

import "testing"

// A pipe ends a cell and a newline ends a row, so either one in a description
// silently breaks the rest of the table.
func TestMarkdownCellCannotBreakTheTable(t *testing.T) {
	for input, want := range map[string]string{
		"a | b":            `a \| b`,
		"line one\nline 2": "line one line 2",
		"  spaced   out  ": "spaced out",
		"plain":            "plain",
	} {
		if got := Cell(input); got != want {
			t.Errorf("Cell(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMarkdownTextAndCode(t *testing.T) {
	for got, want := range map[string]string{
		Text("a <img src=x>\n & b|c"): "a &lt;img src=x&gt; &amp; b|c",
		Code("ci.yml"):                "`ci.yml`",
		Code("a`<b>`.yml"):            "``a`<b>`.yml``",
		Code("`x`"):                   "`` `x` ``",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
