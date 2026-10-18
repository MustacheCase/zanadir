package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/MustacheCase/zanadir/rules"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// File is the YAML read by --config. Each key is named after the flag it sets.
type File struct {
	ExcludedCategories []string `yaml:"excluded-categories"`
	Enforce            *bool    `yaml:"enforce"`
	FailOn             []string `yaml:"fail-on"`
	FailOnWeakened     *bool    `yaml:"fail-on-weakened"`
	RequiredChecks     *bool    `yaml:"required-checks"`
	Baseline           string   `yaml:"baseline"`
	Debug              *bool    `yaml:"debug"`
	Output             string   `yaml:"output"`
	OutputFile         string   `yaml:"output-file"`
	Badge              string   `yaml:"badge"`
	// Rules has no flag: it is only ever set from a file.
	Rules []rules.FileRule `yaml:"rules"`
}

func loadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &f, nil
}

func (f *File) values(base string) map[string][]string {
	v := map[string][]string{}
	if f.ExcludedCategories != nil {
		v["excluded-categories"] = f.ExcludedCategories
	}
	if f.FailOn != nil {
		v["fail-on"] = f.FailOn
	}
	for name, b := range map[string]*bool{"enforce": f.Enforce, "fail-on-weakened": f.FailOnWeakened, "required-checks": f.RequiredChecks, "debug": f.Debug} {
		if b != nil {
			v[name] = []string{strconv.FormatBool(*b)}
		}
	}
	if f.Output != "" {
		v["output"] = []string{f.Output}
	}
	for name, p := range map[string]string{"baseline": f.Baseline, "output-file": f.OutputFile, "badge": f.Badge} {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		v[name] = []string{p}
	}
	return v
}

// applyFile fills every flag the command defines but was not given from the
// file named by --config. Keys for flags the command lacks are skipped, so scan
// and fix can share one file.
func applyFile(cmd *cobra.Command) (*File, error) {
	path, _ := cmd.Flags().GetString("config")
	if path == "" {
		return &File{}, nil
	}
	f, err := loadFile(path)
	if err != nil {
		return nil, err
	}

	for name, vals := range f.values(filepath.Dir(path)) {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || flag.Changed {
			continue
		}
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			err = slice.Replace(vals)
		} else {
			err = flag.Value.Set(vals[0])
		}
		if err != nil {
			return nil, fmt.Errorf("config %s: %s: %w", path, name, err)
		}
	}
	return f, nil
}
