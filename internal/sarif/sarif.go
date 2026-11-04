// Package sarif is the part of a SARIF 2.1.0 document zanadir writes.
package sarif

import (
	"encoding/json"
	"io"
	"net/url"
)

// Version and Schema identify the SARIF format written.
const (
	Version = "2.1.0"
	Schema  = "https://json.schemastore.org/sarif-2.1.0.json"
)

type Log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

type Run struct {
	Tool Tool `json:"tool"`
	// AutomationDetails sets the code scanning category, so two reports from
	// one tool do not replace each other's results.
	AutomationDetails *AutomationDetails `json:"automationDetails,omitempty"`
	Results           []Result           `json:"results"`
}

type AutomationDetails struct {
	ID string `json:"id"`
}

type Tool struct {
	Driver Driver `json:"driver"`
}

type Driver struct {
	Name           string `json:"name"`
	InformationURI string `json:"informationUri"`
	Rules          []Rule `json:"rules"`
}

type Rule struct {
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	ShortDescription     Message      `json:"shortDescription"`
	FullDescription      Message      `json:"fullDescription"`
	Help                 Message      `json:"help"`
	DefaultConfiguration RuleConfig   `json:"defaultConfiguration"`
	Properties           RuleProperty `json:"properties"`
}

type RuleConfig struct {
	Level string `json:"level"`
}

type RuleProperty struct {
	Tags []string `json:"tags"`
}

type Message struct {
	Text string `json:"text"`
}

type Result struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             Message           `json:"message"`
	Locations           []Location        `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type Location struct {
	PhysicalLocation PhysicalLocation `json:"physicalLocation"`
}

type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           Region           `json:"region"`
}

type ArtifactLocation struct {
	URI string `json:"uri"`
}

type Region struct {
	StartLine int `json:"startLine"`
}

// At builds the single-location list SARIF wants, or none when there is
// nowhere to point. Code scanning rejects a result with no location.
func At(uri string, line int) []Location {
	if uri == "" {
		return nil
	}
	return []Location{{
		PhysicalLocation: PhysicalLocation{
			ArtifactLocation: ArtifactLocation{URI: (&url.URL{Path: uri}).EscapedPath()},
			Region:           Region{StartLine: max(line, 1)},
		},
	}}
}

// Write writes log as indented JSON.
func Write(w io.Writer, log Log) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}
