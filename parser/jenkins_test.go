package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseJenkins(t *testing.T, jenkinsfile string) []*models.Job {
	t.Helper()
	artifacts, err := NewJenkinsParser().Parse(testrepo.Write(t, map[string]string{"Jenkinsfile": jenkinsfile}))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "Jenkins", artifacts[0].Name)
	assert.Empty(t, artifacts[0].Triggers, "which builds run is set on the Jenkins job")
	return artifacts[0].Jobs
}

// byStep indexes jobs by the command they run, or the step they call.
func byStep(jobs []*models.Job) map[string]*models.Job {
	steps := map[string]*models.Job{}
	for _, job := range jobs {
		key := job.Run
		if key == "" {
			key = job.Package
		}
		steps[key] = job
	}
	return steps
}

func TestJenkinsDeclarativePipeline(t *testing.T) {
	steps := byStep(parseJenkins(t, `
// a comment with sh 'not a step'
/* and a block comment
   sh 'nor this' */
pipeline {
    agent any
    stages {
        stage('Scan')
        {
            steps {
                sh 'semgrep ci'
                sh "trivy fs ."
                sh """
                    gitleaks detect
                """
                sh '''grype dir:. \'quoted\''''
                bat 'npm test'
                powershell(script: 'Invoke-Pester')
                sh script: 'go vet ./...', label: 'vet'
                dependencyCheck additionalArguments: '--scan .', odcInstallation: 'DC'
                snykSecurity(failOnIssues: false, severity: 'high', additional: [a: 1])
            }
        }
    }
    post {
        always {
            junit 'reports/*.xml'
        }
    }
}
`))
	for _, run := range []string{"semgrep ci", "trivy fs .", "npm test", "Invoke-Pester", "go vet ./...", "grype dir:. 'quoted'"} {
		require.Contains(t, steps, run)
		assert.Equal(t, "Scan", steps[run].Name)
	}
	assert.Contains(t, steps, "\n                    gitleaks detect\n                ")
	assert.NotContains(t, steps, "not a step")

	assert.Equal(t, map[string]string{"additionalArguments": "--scan .", "odcInstallation": "DC"}, steps["dependencyCheck"].With)
	assert.Equal(t, map[string]string{"failOnIssues": "false", "severity": "high"}, steps["snykSecurity"].With,
		"a value that is not a scalar is left out")
	assert.Equal(t, "", steps["junit"].Name, "post runs outside any stage")
}

// Without parentheses an argument list ends at its last value. It used to run
// on to the next comma and swallow the statements after it.
func TestJenkinsBareArgumentsDoNotSwallowTheNextStatement(t *testing.T) {
	steps := byStep(parseJenkins(t, `
node {
    milestone label: 'Start'
    git branch: 'main', url: env.REPO, credentialsId: credentials('id')
    sh 'make test'
    withCredentials([string(credentialsId: 'x', variable: 'Y')]) {
        sh 'make release'
    }
}
`))
	assert.Equal(t, map[string]string{"label": "Start"}, steps["milestone"].With)
	assert.Equal(t, map[string]string{"branch": "main"}, steps["git"].With)
	assert.Contains(t, steps, "make test")
	assert.Contains(t, steps, "make release")
}

func TestJenkinsCatchError(t *testing.T) {
	steps := byStep(parseJenkins(t, `
node {
    catchError(buildResult: 'SUCCESS', stageResult: 'FAILURE') { sh 'trufflehog .' }
    catchError(buildResult: null) { sh 'trivy fs .' }
    catchError(buildResult: 'FAILURE') { sh 'semgrep ci' }
    catchError() { sh 'gitleaks detect' }
    catchError(buildResult: 'UNSTABLE') { sh 'grype .' }
    warnError('scan failed') { sh 'osv-scanner .' }
}
`))
	assert.True(t, steps["trufflehog ."].ContinueOnError, "the build stays green whatever it finds")
	assert.True(t, steps["trivy fs ."].ContinueOnError)
	assert.False(t, steps["semgrep ci"].ContinueOnError)
	assert.False(t, steps["gitleaks detect"].ContinueOnError, "the default buildResult is FAILURE")
	assert.Equal(t, "catchError(buildResult: UNSTABLE)", steps["grype ."].If)
	assert.Equal(t, "warnError", steps["osv-scanner ."].If)
	assert.NotContains(t, steps, "catchError")
}

func TestJenkinsConditions(t *testing.T) {
	jobs := parseJenkins(t, `
pipeline {
    stages {
        stage('Release') {
            when { branch 'main' }
            steps { sh 'cosign sign img' }
        }
        stage('Always') {
            steps {
                sh(script: 'make check', returnStatus: true)
                script {
                    if (env.CHANGE_ID) {
                        sh 'semgrep ci'
                    } else {
                        sh 'semgrep scan'
                    }
                    try {
                        sh 'trivy fs .'
                    } catch (err) {
                        echo err.getMessage()
                    }
                }
            }
        }
    }
}
`)
	steps := byStep(jobs)
	assert.Equal(t, "when { branch main }", steps["cosign sign img"].JobIf)
	assert.Equal(t, "returnStatus: true", steps["make check"].If)
	assert.Equal(t, "if (env . CHANGE_ID)", steps["semgrep ci"].JobIf)
	assert.Equal(t, "else", steps["semgrep scan"].JobIf)
	assert.Empty(t, steps["trivy fs ."].JobIf, "a try block always runs")
	assert.NotContains(t, steps, "catch")
	assert.NotContains(t, steps, "getMessage", "a method called on a value is not a step")
}

func TestJenkinsToleratesWhatItCannotRead(t *testing.T) {
	jobs := parseJenkins(t, `
mavenBuild {
    dependencyCheck = 'enabled'
}
sh ''
sh(returnStdout: true)
sh 'unterminated
/* unterminated comment`)
	steps := byStep(jobs)
	assert.Contains(t, steps, "unterminated")
	assert.Len(t, steps, 1, "an assignment is not a call, and a step with no command is not recorded")
}

func TestJenkinsFiles(t *testing.T) {
	root := testrepo.Write(t, map[string]string{
		"Jenkinsfile":         "sh 'make'\n",
		"Jenkinsfile.release": "sh 'make release'\n",
		"Jenkinsfile.md":      "# docs\n",
		"JenkinsfileOld":      "sh 'old'\n",
	})
	require.NoError(t, os.Mkdir(filepath.Join(root, "Jenkinsfile.d"), 0o755))

	p := NewJenkinsParser()
	assert.True(t, p.Exists(root))
	assert.False(t, p.Exists(t.TempDir()))

	artifacts, err := p.Parse(root)
	require.NoError(t, err)
	var names []string
	for _, a := range artifacts {
		names = append(names, filepath.Base(a.Location))
	}
	assert.Equal(t, []string{"Jenkinsfile", "Jenkinsfile.release"}, names)
}

func TestJenkinsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	root := testrepo.Write(t, map[string]string{"Jenkinsfile": "sh 'make'\n"})
	require.NoError(t, os.Chmod(filepath.Join(root, "Jenkinsfile"), 0o000))
	artifacts, err := NewJenkinsParser().Parse(root)
	require.NoError(t, err)
	assert.Contains(t, artifacts[0].Skipped, "permission denied")
}

func TestJenkinsParallelNestedConditionsAndOperators(t *testing.T) {
	steps := byStep(parseJenkins(t, `
pipeline {
    stages {
        stage('Checks') {
            when { anyOf { branch 'main'; changeRequest() } }
            steps {
                parallel 'unit': { sh 'go test ./...' }, 'lint': { sh 'golangci-lint run' }
                script {
                    if (env.BRANCH_NAME == 'main') { sh 'cosign sign' }
                }
            }
        }
    }
}
sh '''never closed`+"\n// trailing comment with no newline"))

	cond := "when { anyOf { branch main changeRequest ( ) } }"
	assert.Equal(t, cond, steps["go test ./..."].JobIf)
	assert.Equal(t, cond, steps["golangci-lint run"].JobIf)
	assert.Equal(t, cond+" && if (env . BRANCH_NAME == main)", steps["cosign sign"].JobIf)
	assert.Contains(t, steps, "never closed\n// trailing comment with no newline")
}

func TestGroovyTokensEndOnALineComment(t *testing.T) {
	assert.Equal(t, []groovyToken{{groovyIdent, "sh"}, {groovyString, "x"}}, groovyTokens("sh 'x' // done"))
}
