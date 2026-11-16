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
	"github.com/MustacheCase/zanadir/skills"
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

// codeManifests mark a source package. One named like an infrastructure
// directory, such as packages/adapters/k8s, is code about it, not manifests.
var codeManifests = []string{"package.json", "go.mod", "Cargo.toml", "pyproject.toml", "setup.py", "pom.xml",
	"build.gradle", "build.gradle.kts", "Gemfile", "composer.json"}

var manifestExtensions = map[string]bool{".yaml": true, ".yml": true, ".json": true, ".tpl": true, ".hcl": true, ".jsonnet": true}

// A Dockerfile.json is data about a Dockerfile, not one.
var dataExtensions = map[string]bool{".json": true, ".yaml": true, ".yml": true, ".txt": true, ".md": true, ".snap": true, ".golden": true, ".out": true}

var (
	cloudFormation = regexp.MustCompile(`AWSTemplateFormatVersion|["']?Type["']?\s*:\s*["']?AWS::\w+::`)
	armTemplate    = regexp.MustCompile(`deploymentTemplate\.json`)
	k8sAPIVersion  = regexp.MustCompile(`(?m)^apiVersion:\s*\S`)
	k8sKind        = regexp.MustCompile(`(?m)^kind:\s*["']?(?:Deployment|StatefulSet|DaemonSet|ReplicaSet|Pod|Job|CronJob|Service|Ingress|ConfigMap|Secret|Namespace|ServiceAccount|(?:Cluster)?Role(?:Binding)?|PersistentVolume(?:Claim)?|NetworkPolicy|HorizontalPodAutoscaler|CustomResourceDefinition)["']?\s*$`)
	// A Nuclei template looks for exposed or misconfigured infrastructure.
	nucleiID       = regexp.MustCompile(`(?m)^id:[ \t]*\S`)
	nucleiInfo     = regexp.MustCompile(`(?m)^info:[ \t]*$`)
	nucleiSeverity = regexp.MustCompile(`(?m)^[ \t]+severity:[ \t]*\S`)
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
	head := readHead(path)
	if cloudFormation.Match(head) || armTemplate.Match(head) {
		return true
	}
	return ext != ".json" && k8sAPIVersion.Match(head) && k8sKind.Match(head)
}

func readHead(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 32<<10)
	n, _ := io.ReadFull(f, head)
	return head[:n]
}

func isDetectionTemplate(path, name string) bool {
	if ext := strings.ToLower(filepath.Ext(name)); ext != ".yaml" && ext != ".yml" {
		return false
	}
	head := readHead(path)
	return nucleiID.Match(head) && nucleiInfo.Match(head) && nucleiSeverity.Match(head)
}

func codePackage(dir string, seen map[string]bool) bool {
	if is, ok := seen[dir]; ok {
		return is
	}
	seen[dir] = slices.ContainsFunc(codeManifests, func(name string) bool {
		info, err := os.Lstat(filepath.Join(dir, name))
		return err == nil && info.Mode().IsRegular()
	})
	return seen[dir]
}

// hasIaC walks until it finds one marker, so only a repository with none is
// walked in full.
func hasIaC(dir string) bool {
	found := false
	packages := map[string]bool{}

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
		inIaCDir := false
		parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
		for i, part := range parts {
			if iacDirs[strings.ToLower(part)] && !codePackage(filepath.Join(dir, filepath.Join(parts[:i+1]...)), packages) {
				inIaCDir = true
				break
			}
		}
		if (isIaCFile(name, inIaCDir) || isManifest(path, name)) && !isDetectionTemplate(path, name) {
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
	// The tools in Workflow Security read GitHub Actions workflows or GitLab CI.
	ownWorkflow := slices.ContainsFunc(parser.GithubWorkflows(dir), func(path string) bool { return ignore == nil || !ignore(path) })
	if !ownWorkflow && !parser.Uses(dir, parser.GitLab) {
		ruledOut = append(ruledOut, string(models.WorkflowSecurity))
	}
	if !skills.Present(dir) {
		ruledOut = append(ruledOut, string(models.AgentSkillSecurity))
	}
	return ruledOut
}
