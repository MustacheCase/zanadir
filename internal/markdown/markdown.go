// Package markdown makes untrusted values safe to put in a Markdown report.
package markdown

import "strings"

// Cell makes a value safe to sit in a table cell: a literal pipe ends the
// cell, and a newline ends the row.
func Cell(text string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(text), " "), "|", "\\|")
}

// Text makes a value safe as Markdown prose: a file name or a reason can hold
// <, which would otherwise reach a PR comment as raw HTML.
func Text(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(strings.Join(strings.Fields(text), " "))
}

// Code wraps a value in a code span its own backticks cannot close.
func Code(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	fence := "`"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return fence + text + fence
}
