package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// A Jenkinsfile records no trigger, so a control grades unknown unless the
// file itself weakens it.
func TestJenkinsfilesAreGradedEndToEnd(t *testing.T) {
	verdicts := gradeFile(t, parser.NewJenkinsParser(), "Jenkinsfile", `
pipeline {
    stages {
        stage('Scan') {
            steps {
                catchError(buildResult: 'SUCCESS') { sh 'trufflehog filesystem .' }
                snykSecurity(failOnIssues: false)
                withSonarQubeEnv('sonar') { sh 'mvn sonar:sonar' }
                junit 'target/surefire-reports/*.xml'
            }
        }
    }
}
`)
	assert.Equal(t, map[string]Verdict{
		"Secrets Detection": Advisory,
		"SCA":               Advisory,
		"SAST":              Unknown,
		"Unit Tests":        Unknown,
	}, verdicts)
}
