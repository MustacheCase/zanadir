package utils

import (
	"fmt"
	"strings"
	"unicode"
)

// Printable escapes control and other non-printing characters in s, so a path
// taken from a scanned repository cannot drive the terminal it is printed to.
func Printable(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			fmt.Fprintf(&b, "\\u%04x", r)
		}
	}
	return b.String()
}
