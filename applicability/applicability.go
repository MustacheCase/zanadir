// Package applicability decides which categories a repository has no use for,
// so it is not told to adopt tooling that would have nothing to scan.
package applicability

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

var manifestExtensions = map[string]bool{".yaml": true, ".yml": true, ".json": true, ".tpl": true, ".hcl": true, ".jsonnet": true}

// A Dockerfile.json is data about a Dockerfile, not one.
var dataExtensions = map[string]bool{".json": true, ".yaml": true, ".yml": true, ".txt": true, ".md": true, ".snap": true, ".golden": true, ".out": true}

var (
	cloudFormation = regexp.MustCompile(`AWSTemplateFormatVersion|["']?Type["']?\s*:\s*["']?AWS::\w+::`)
	armTemplate    = regexp.MustCompile(`deploymentTemplate\.json`)
	k8sAPIVersion  = regexp.MustCompile(`(?m)^apiVersion:\s*\S`)
	k8sKind        = regexp.MustCompile(`(?m)^kind:\s*["']?(?:Deployment|StatefulSet|DaemonSet|ReplicaSet|Pod|Job|CronJob|Service|Ingress|ConfigMap|Secret|Namespace|ServiceAccount|(?:Cluster)?Role(?:Binding)?|PersistentVolume(?:Claim)?|NetworkPolicy|HorizontalPodAutoscaler|CustomResourceDefinition)["']?\s*$`)
)

func isIaCFile(name string, inIaCDir bool) bool {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)
	switch {
	case iacFiles[lower], inIaCDir && manifestExtensions[ext]:
		return true
	case strings.HasPrefix(lower, "dockerfile"):
		return !dataExtensions[ext]
	}
	for _, suffix := range iacSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// isManifest reads the start of a YAML or JSON file for CloudFormation, an
// ARM template or a Kubernetes object, which can be named anything.
func isManifest(path, name string) bool {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)
	if ext == ".json" && strings.HasPrefix(lower, "azuredeploy") {
		return true
	}
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 32<<10)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if cloudFormation.Match(head) || armTemplate.Match(head) {
		return true
	}
	return ext != ".json" && k8sAPIVersion.Match(head) && k8sKind.Match(head)
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
		if entry.IsDir() {
			if path != dir && (utils.IsDependencyDir(name) || utils.IsTestDir(name) || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		inIaCDir := slices.ContainsFunc(strings.Split(filepath.Dir(rel), string(filepath.Separator)), func(part string) bool {
			return iacDirs[strings.ToLower(part)]
		})
		if isIaCFile(name, inIaCDir) || isManifest(path, name) {
			found = true
			return fs.SkipAll
		}
		return nil
	})

	return found
}

// NotApplicable returns the categories a repository has nothing for. The
// others cannot be ruled out without guessing. Files ignore says are not the
// repository's own, such as the workflow fix --write generates, do not count.
func NotApplicable(dir string, ignore func(string) bool) []string {
	if _, err := os.Stat(dir); err != nil {
		return nil
	}

	var ruledOut []string
	if !hasIaC(dir) {
		ruledOut = append(ruledOut, string(models.IaC))
	}
	// Every tool in Workflow Security reads GitHub Actions workflows.
	if !slices.ContainsFunc(parser.GithubWorkflows(dir), func(path string) bool { return ignore == nil || !ignore(path) }) {
		ruledOut = append(ruledOut, string(models.WorkflowSecurity))
	}
	return ruledOut
}
