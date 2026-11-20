{
  "score": {
    "covered": 4,
    "total": 11
  },
  "platforms": [
    "github"
  ],
  "uncovered": [
    {
      "id": "Secrets Detection",
      "name": "Data Leakage \u0026 Secrets Detection",
      "description": "Tools that help detect sensitive information such as secrets, API keys, and credentials in source code and configurations.",
      "suggestions": [
        {
          "name": "Gitleaks",
          "repository": "https://github.com/gitleaks/gitleaks",
          "description": "A fast and configurable tool for detecting secrets in Git repositories."
        },
        {
          "name": "TruffleHog",
          "repository": "https://github.com/trufflesecurity/trufflehog",
          "description": "Searches through Git repositories for secrets and credentials using entropy analysis and pattern matching."
        },
        {
          "name": "GitGuardian CLI",
          "repository": "https://github.com/GitGuardian/ggshield",
          "description": "A CLI tool to prevent sensitive data leaks by scanning repositories, commits, and code."
        },
        {
          "name": "Detect-Secrets",
          "repository": "https://github.com/Yelp/detect-secrets",
          "description": "An enterprise-friendly tool to detect secrets in source code using pre-commit hooks."
        }
      ]
    },
    {
      "id": "License Compliance",
      "name": "License Compliance Tools",
      "description": "Tools for checking open-source license compliance and ensuring proper usage of dependencies.",
      "suggestions": [
        {
          "name": "FOSSA",
          "repository": "https://github.com/fossas/fossa-cli",
          "description": "An open-source license compliance and security scanner."
        },
        {
          "name": "LicenseFinder",
          "repository": "https://github.com/pivotal/LicenseFinder",
          "description": "A tool to gather and report on dependencies and their licenses."
        },
        {
          "name": "Grant",
          "repository": "https://github.com/anchore/grant",
          "description": "Search an SBOM for licenses and the packages they belong to."
        }
      ]
    },
    {
      "id": "End Of Life",
      "name": "End-of-Life Package Checkers",
      "description": "Tools for identifying outdated or end-of-life software packages to ensure system security and compliance.",
      "suggestions": [
        {
          "name": "Trivy",
          "repository": "https://github.com/aquasecurity/trivy",
          "description": "Scans images and filesystems, and fails a build with --exit-on-eol once the operating system has reached end of life."
        },
        {
          "name": "Docker Scout",
          "repository": "https://github.com/docker/scout-action",
          "description": "Evaluates images against policies that include out-of-date and end-of-life base images, and recommends a supported replacement."
        }
      ]
    },
    {
      "id": "Performance Testing",
      "name": "Performance and Reliability Testing Tools",
      "description": "Tools for load, stress and reliability testing, to verify a system holds up under expected and peak traffic.",
      "suggestions": [
        {
          "name": "k6",
          "repository": "https://github.com/grafana/k6",
          "description": "Grafana k6 is an open-source, developer-friendly, and extensible load testing tool. k6 allows you to prevent performance issues and proactively improve reliability."
        },
        {
          "name": "JMeter",
          "repository": "https://github.com/apache/jmeter",
          "description": "An Apache project designed to load test functional behavior and measure performance, with support for various protocols and servers."
        },
        {
          "name": "Gatling",
          "repository": "https://github.com/gatling/gatling",
          "description": "A highly capable load testing tool built for continuous integration and testing web applications, with a focus on maintainability and developer productivity."
        },
        {
          "name": "Apache Bench",
          "repository": "https://httpd.apache.org/docs/2.4/programs/ab.html",
          "description": "A simple command line tool for benchmarking HTTP servers, part of the Apache HTTP server project, ideal for quick performance checks."
        },
        {
          "name": "Artillery",
          "repository": "https://github.com/artilleryio/artillery",
          "description": "An easy-to-use load testing and functional testing toolkit for testing microservices and APIs with support for multiple protocols."
        },
        {
          "name": "Taurus",
          "repository": "https://github.com/Blazemeter/taurus",
          "description": "Taurus is an open-source framework that runs JMeter, Gatling, Locust, and other load tests from YAML configuration. Configured pass/fail criteria can stop a run as failed; sample failures alone do not necessarily produce a non-zero exit code."
        }
      ]
    },
    {
      "id": "SAST",
      "name": "Static Application Security Testing",
      "description": "Tools that analyse source code for security flaws such as injection, unsafe deserialisation and hardcoded credentials, before the code ever runs.",
      "suggestions": [
        {
          "name": "Semgrep",
          "repository": "https://github.com/semgrep/semgrep",
          "description": "A fast multi-language static analysis engine with a large community rule registry."
        },
        {
          "name": "CodeQL",
          "repository": "https://github.com/github/codeql-action",
          "description": "GitHub's semantic code analysis engine, free for public repositories and wired into the Security tab."
        },
        {
          "name": "SonarQube",
          "repository": "https://github.com/SonarSource/sonarqube-scan-action",
          "description": "Static analysis for security and maintainability across more than twenty languages."
        },
        {
          "name": "gosec",
          "repository": "https://github.com/securego/gosec",
          "description": "A security scanner for Go source code, inspecting the AST for common insecure patterns.",
          "language": "Go"
        }
      ]
    },
    {
      "id": "Supply Chain",
      "name": "Supply Chain Security and SBOM",
      "description": "Tools that record what a build contains and prove where it came from, so a compromised dependency or artifact can be traced rather than guessed at.",
      "suggestions": [
        {
          "name": "Syft",
          "repository": "https://github.com/anchore/syft",
          "description": "Generates an SBOM from container images and filesystems, in SPDX or CycloneDX format."
        },
        {
          "name": "CycloneDX",
          "repository": "https://github.com/cdxgen/cdxgen",
          "description": "Generates CycloneDX bills of materials across many language ecosystems."
        },
        {
          "name": "Cosign",
          "repository": "https://github.com/sigstore/cosign",
          "description": "Signs and verifies container images and artifacts, keylessly via Sigstore."
        },
        {
          "name": "SLSA GitHub Generator",
          "repository": "https://github.com/slsa-framework/slsa-github-generator",
          "description": "Produces SLSA provenance for a build, attesting how an artifact was produced."
        },
        {
          "name": "GitHub Artifact Attestations",
          "repository": "https://github.com/actions/attest-build-provenance",
          "description": "Creates signed build provenance for artifacts directly from a workflow."
        }
      ]
    },
    {
      "id": "Workflow Security",
      "name": "Workflow Security and Hardening",
      "description": "Tools that check the pipeline definition itself - unpinned actions, over-broad token permissions, script injection through untrusted input - rather than the code it builds.",
      "suggestions": [
        {
          "name": "zizmor",
          "repository": "https://github.com/zizmorcore/zizmor",
          "description": "Finds security problems in GitHub Actions workflows: unpinned actions, dangerous triggers, template injection and excessive permissions."
        },
        {
          "name": "actionlint",
          "repository": "https://github.com/rhysd/actionlint",
          "description": "Static checker for GitHub Actions workflows, covering syntax, expressions, shell scripts in run steps and action input validity."
        },
        {
          "name": "OpenSSF Scorecard",
          "repository": "https://github.com/ossf/scorecard-action",
          "description": "Grades a repository against supply-chain practices including pinned dependencies, token permissions and branch protection."
        },
        {
          "name": "poutine",
          "repository": "https://github.com/boostsecurityio/poutine",
          "description": "Finds vulnerabilities and misconfigurations in GitHub Actions and GitLab CI pipelines, such as injection from untrusted input, untrusted checkouts and unpinnable components."
        }
      ]
    }
  ],
  "weakened": [
    {
      "category": "SCA",
      "verdict": "advisory",
      "location": ".github/workflows/ci.yml"
    }
  ],
  "notApplicable": [
    "IaC Security",
    "Agent Skill Security"
  ],
  "covered": [
    {
      "category": "SCA",
      "verdict": "advisory",
      "tools": [
        {
          "name": "Trivy",
          "verdict": "advisory",
          "locations": [
            ".github/workflows/ci.yml"
          ]
        }
      ]
    },
    {
      "category": "Coverage",
      "verdict": "enforcing",
      "tools": [
        {
          "name": "go test",
          "verdict": "enforcing",
          "locations": [
            ".github/workflows/ci.yml"
          ]
        }
      ]
    },
    {
      "category": "Linter",
      "verdict": "enforcing",
      "tools": [
        {
          "name": "go vet",
          "verdict": "enforcing",
          "locations": [
            ".github/workflows/ci.yml"
          ]
        }
      ]
    },
    {
      "category": "Unit Tests",
      "verdict": "enforcing",
      "tools": [
        {
          "name": "a job named test",
          "verdict": "enforcing",
          "locations": [
            ".github/workflows/ci.yml"
          ]
        },
        {
          "name": "go test",
          "verdict": "enforcing",
          "locations": [
            ".github/workflows/ci.yml"
          ]
        }
      ]
    }
  ]
}
