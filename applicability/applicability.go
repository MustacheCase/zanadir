// Package applicability decides which categories a repository has no use for,
// so it is not told to adopt tooling that would have nothing to scan.
package applicability

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/utils"
)

var iacFiles = map[string]bool{
	".terraform.lock.hcl": true,
	"terragrunt.hcl":      true,
	"chart.yaml":          true,
	"kustomization.yaml":  true,
	"kustomization.yml":   true,
	"containerfile":       true,
	"docker-compose.yml":  true,
	"docker-compose.yaml": true,
	"compose.yml":         true,
	"compose.yaml":        true,
	"pulumi.yaml":         true,
	"serverless.yml":      true,
	"serverless.yaml":     true,
	"cdk.json":            true,
}

var iacSuffixes = []string{".tf", ".tfvars", ".bicep", ".dockerfile"}

// A manifest nested in one of these can be named anything.
var iacDirs = map[string]bool{
	"terraform":      true,
	"kubernetes":     true,
	"k8s":            true,
	"manifests":      true,
	"charts":         true,
	"helm":           true,
	"infra":          true,
	"infrastructure": true,
}

func isIaC(name string, isDir bool) bool {
	lower := strings.ToLower(name)
	if isDir {
		return iacDirs[lower]
	}
	if iacFiles[lower] || strings.HasPrefix(lower, "dockerfile") {
		return true
	}
	for _, suffix := range iacSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// hasIaC walks until it finds one marker, so only a repository with none is
// walked in full.
func hasIaC(dir string) bool {
	found := false

	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && path != dir {
			if utils.IsDependencyDir(name) || strings.HasPrefix(name, ".") && !isIaC(name, true) {
				return fs.SkipDir
			}
		}
		if isIaC(name, entry.IsDir()) {
			found = true
			return fs.SkipAll
		}
		return nil
	})

	return found
}

// NotApplicable returns the categories a repository has nothing for. The
// others cannot be ruled out without guessing.
func NotApplicable(dir string) []string {
	if _, err := os.Stat(dir); err != nil {
		return nil
	}

	var ruledOut []string
	if !hasIaC(dir) {
		ruledOut = append(ruledOut, string(models.IaC))
	}
	// Every tool in Workflow Security reads GitHub Actions workflows.
	if !parser.Uses(dir, parser.GitHub) {
		ruledOut = append(ruledOut, string(models.WorkflowSecurity))
	}
	return ruledOut
}
