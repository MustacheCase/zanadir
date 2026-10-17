// Package shell reads the commands of a CI step's script without running it.
package shell

import (
	"regexp"
	"strings"
)

var separator = regexp.MustCompile(`\|\||&&|[;|\n]`)

// Commands splits a script into commands, dropping comments: they name tools
// without running them.
func Commands(script string) []string {
	var commands []string
	for _, command := range separator.Split(strings.ReplaceAll(script, "\\\n", " "), -1) {
		if command = strings.TrimSpace(command); command != "" && !strings.HasPrefix(command, "#") {
			commands = append(commands, command)
		}
	}
	return commands
}

// Words returns a command's program and arguments, past sudo and VAR=value.
func Words(command string) []string {
	words := strings.Fields(command)
	for len(words) > 0 && (words[0] == "sudo" || strings.Contains(words[0], "=")) {
		words = words[1:]
	}
	return words
}
