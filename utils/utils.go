package utils

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ReadYAML reads a YAML file from the given path and unmarshals it into the provided target structure.
func ReadYAML(filePath string, target interface{}) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return err
	}

	return nil
}
