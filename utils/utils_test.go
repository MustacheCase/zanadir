package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	Name  string            `yaml:"name"`
	Age   int               `yaml:"age"`
	Tags  []string          `yaml:"tags"`
	Extra map[string]string `yaml:"extra"`
}

func TestReadYAML(t *testing.T) {
	// Create a temporary YAML file for testing
	yamlContent := `
name: Test Name
age: 30
tags:
  - tag1
  - tag2
extra:
  key1: value1
  key2: value2
`
	tmpFile, err := os.CreateTemp("", "test-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name()) // Clean up the file after the test

	_, err = tmpFile.WriteString(yamlContent)
	assert.NoError(t, err)
	tmpFile.Close()

	// Define the target structure
	var result testStruct

	// Call the ReadYAML function
	err = ReadYAML(tmpFile.Name(), &result)
	assert.NoError(t, err)

	// Assert the unmarshalled data
	assert.Equal(t, "Test Name", result.Name)
	assert.Equal(t, 30, result.Age)
	assert.Equal(t, []string{"tag1", "tag2"}, result.Tags)
	assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, result.Extra)
}

func TestReadYAML_FileNotFound(t *testing.T) {
	var result testStruct
	err := ReadYAML("nonexistent-file.yaml", &result)
	assert.Error(t, err)
}

func TestReadYAML_InvalidYAML(t *testing.T) {
	// Create a temporary invalid YAML file for testing
	invalidYAMLContent := `
name: Test Name
age: thirty  # Invalid value for an integer field
`
	tmpFile, err := os.CreateTemp("", "test-invalid-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name()) // Clean up the file after the test

	_, err = tmpFile.WriteString(invalidYAMLContent)
	assert.NoError(t, err)
	tmpFile.Close()

	// Define the target structure
	var result testStruct

	// Call the ReadYAML function
	err = ReadYAML(tmpFile.Name(), &result)
	assert.Error(t, err)
}
