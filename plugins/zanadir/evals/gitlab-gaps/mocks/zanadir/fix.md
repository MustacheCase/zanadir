SCA Open Source Tools is not covered. Add to .gitlab-ci.yml:

  dependency_scanning:
    stage: .pre
    image:
      name: aquasec/trivy:0.76.1
      entrypoint: [""]
    script:
      - trivy fs --exit-code 1 .

  https://github.com/aquasecurity/trivy

Data Leakage & Secrets Detection is not covered. Add to .gitlab-ci.yml:

  secret_detection:
    stage: .pre
    image:
      name: zricethezav/gitleaks:v8.30.1
      entrypoint: [""]
    script:
      - gitleaks git --verbose .

  https://github.com/gitleaks/gitleaks

End-of-Life Package Checkers is not covered. Add to .gitlab-ci.yml:

  dependency_scanning:
    stage: .pre
    image:
      name: aquasec/trivy:0.76.1
      entrypoint: [""]
    script:
      - trivy fs --exit-code 1 .

  https://github.com/aquasecurity/trivy

Workflow Security and Hardening is not covered. Add to .gitlab-ci.yml:

  # Fails the pipeline on error findings. The entrypoint override lets
  # the runner's shell start; the image's own entrypoint is glsec.
  glsec:
    stage: .pre
    image:
      name: ghcr.io/glsec/glsec:1.23.0
      entrypoint: [""]
    script:
      - glsec .gitlab-ci.yml

  https://github.com/glsec/glsec

Not covered, with no snippet that fits:
  License Compliance Tools: no ready-made snippet for this repository; suggested tools: FOSSA, LicenseFinder, Grant.
  Code Coverage Tools: no ready-made snippet for this repository; suggested tools: pytest-cov, Codecov, Coveralls.
  Performance and Reliability Testing Tools: no ready-made snippet for this repository; suggested tools: k6, JMeter, Gatling, Apache Bench, Artillery, Taurus.
  Static Application Security Testing: no ready-made snippet for this repository; suggested tools: Semgrep, CodeQL, SonarQube, Bandit.
  Supply Chain Security and SBOM: no ready-made snippet for this repository; suggested tools: Syft, CycloneDX, Cosign, SLSA GitHub Generator, GitHub Artifact Attestations.
