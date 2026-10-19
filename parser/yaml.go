package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
)

// The configuration zanadir reads was written by someone else, so these
// helpers never fail: a value they cannot read is skipped, and one malformed
// entry must not abort the scan of a whole repository.

// resolve follows YAML aliases to the node they name.
func resolve(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// scalarIs reports whether node is a scalar equal to value, ignoring case.
func scalarIs(node *yaml.Node, value string) bool {
	node = resolve(node)
	return node.Kind == yaml.ScalarNode && strings.EqualFold(node.Value, value)
}

// scalars keeps the scalar values of a mapping, such as a task's inputs or a
// pipe's variables, which rules compare as text.
func scalars(m map[string]yaml.Node) map[string]string {
	if len(m) == 0 {
		return nil
	}
	values := make(map[string]string, len(m))
	for key, value := range m {
		if v := resolve(&value); v.Kind == yaml.ScalarNode {
			values[key] = v.Value
		}
	}
	return values
}

// stringList accepts either a scalar or a sequence, which "needs" and the
// branch filters both allow.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	*s = decodeStrings(node)
	return nil
}

// decodeStrings reads a scalar or a sequence of scalars, skipping whatever it
// cannot read rather than discarding the entries beside it.
func decodeStrings(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if node.Decode(&one) != nil || one == "" {
			return nil
		}
		return []string{one}

	case yaml.SequenceNode:
		values := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var one string
			if item.Decode(&one) == nil && one != "" {
				values = append(values, one)
			}
		}
		if len(values) == 0 {
			return nil
		}
		return values
	}
	return nil
}

// imageName reads a container image given as a name or as a mapping with one.
func imageName(node *yaml.Node) string {
	node = resolve(node)
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value
	case yaml.MappingNode:
		var image struct {
			Name string `yaml:"name"`
		}
		if node.Decode(&image) == nil {
			return image.Name
		}
	}
	return ""
}

// scriptLines flattens a script given as one line or as lists nested to any
// depth. A !reference points into another job and cannot be followed, so it
// contributes nothing rather than its own arguments.
func scriptLines(node *yaml.Node) []string {
	node = resolve(node)
	if node.Tag == "!reference" {
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value != "" {
			return []string{node.Value}
		}
	case yaml.SequenceNode:
		var lines []string
		for _, item := range node.Content {
			lines = append(lines, scriptLines(item)...)
		}
		return lines
	}
	return nil
}

// invalidYAML is a configuration file that was read but does not parse. The
// platform refuses to run such a file too, so it protects nothing.
type invalidYAML struct{ err error }

func (e invalidYAML) Error() string { return e.err.Error() }

// readYAML decodes the first document of a configuration file into target.
func readYAML(root, path string, target any) error {
	data, err := repofs.ReadFile(root, path)
	if err != nil {
		return err
	}
	if err := decodeNext(yaml.NewDecoder(bytes.NewReader(data)), target); !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// decodeNext decodes the decoder's next document into target, returning
// io.EOF once there are none left.
func decodeNext(decoder *yaml.Decoder, target any) error {
	var doc yaml.Node
	err := decoder.Decode(&doc)
	if err == nil {
		foldMergeKeys(&doc)
		err = doc.Decode(target)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return invalidYAML{err}
	}
	return err
}

// foldMergeKeys turns repeated "<<" keys in a mapping into one merge of a
// list. CircleCI accepts the repeats, which are common there, but yaml.v3
// rejects them as duplicate keys.
func foldMergeKeys(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		var first *yaml.Node
		kept := node.Content[:0]
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!merge" {
				kept = append(kept, key, value)
				continue
			}
			if first == nil {
				first = &yaml.Node{Kind: yaml.SequenceNode}
				kept = append(kept, key, first)
			}
			if value.Kind == yaml.SequenceNode {
				first.Content = append(first.Content, value.Content...)
			} else {
				first.Content = append(first.Content, value)
			}
		}
		node.Content = kept
	}
	for _, child := range node.Content {
		foldMergeKeys(child)
	}
}

// unreadable answers a configuration file that could not be read or parsed:
// an artifact with no jobs that says why it was skipped, so one broken file -
// invalid YAML, a dangling symlink, no permission - does not fail the scan of
// a whole repository.
func unreadable(path string, err error) *models.Artifact {
	reason := fmt.Sprintf("it cannot be read (%s)", oneLine(err))
	var invalid invalidYAML
	if errors.As(err, &invalid) {
		reason = fmt.Sprintf("it is not valid YAML (%s), so the CI platform cannot run it either", oneLine(invalid))
	}
	return &models.Artifact{Location: path, Skipped: reason}
}

func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// skippedFile is unreadable for a platform with a single configuration file.
func skippedFile(path string, err error) ([]*models.Artifact, error) {
	return []*models.Artifact{unreadable(path, err)}, nil
}
