SCA Open Source Tools is not covered. Add to .github/workflows/security.yml:

  - name: Scan dependencies
    uses: anchore/scan-action@27805bf3b4e84b4a5c980df22ed233c00390a439 # v7.4.2
    with:
      path: .
      fail-build: true

  https://github.com/anchore/grype

Data Leakage & Secrets Detection is not covered. Add to .github/workflows/security.yml:

  # The CLI rather than gitleaks-action, which needs a licence key on
  # organisation accounts.
  - name: Detect secrets
    run: |
      curl -sSfLo gitleaks.tar.gz https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz
      echo "551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb  gitleaks.tar.gz" | sha256sum -c -
      tar -xzf gitleaks.tar.gz gitleaks
      ./gitleaks git --verbose .

  https://github.com/gitleaks/gitleaks

License Compliance Tools is not covered. Add to .github/workflows/security.yml:

  # Grant has no action of its own, and denies every licence without a
  # policy. This one allows permissive licences; edit the list to yours.
  # Install dependencies first where grant reads licences from them
  # (go mod download, pip install).
  - name: Check licences
    run: |
      curl -sSfLo grant.tar.gz https://github.com/anchore/grant/releases/download/v0.6.8/grant_0.6.8_linux_amd64.tar.gz
      echo "6500f8bbf0f20fb993de8084686e199f0ba1eb494769ff75454286d5ef63f919  grant.tar.gz" | sha256sum -c -
      tar -xzf grant.tar.gz grant
      cat > "$RUNNER_TEMP/grant.yaml" <<'EOF'
      require-license: false
      allow: [Apache.*, MIT.*, BSD.*, ISC, 0BSD, MPL.*, PSF.*, Python.*, Unlicense, CC0.*, Zlib, BSL-1.0]
      EOF
      ./grant check -c "$RUNNER_TEMP/grant.yaml" dir:.

  https://github.com/anchore/grant

Code Coverage Tools is not covered. Add to .github/workflows/security.yml:

  # Set the floor to today's coverage, which pytest prints, so the build
  # fails when it drops. Install your project's dependencies first.
  - name: Test with a coverage floor
    run: |
      pip install pytest pytest-cov
      pytest --cov --cov-fail-under=50

  https://github.com/pytest-dev/pytest-cov

Linting Tools is not covered. Add to .github/workflows/security.yml:

  - name: Lint
    run: pipx run ruff==0.17.3 check .

  https://github.com/astral-sh/ruff

Static Application Security Testing is not covered. Add to .github/workflows/security.yml:

  # semgrep-action is archived; the CLI is what Semgrep documents now.
  - name: Static analysis
    run: pipx run semgrep==1.183.0 scan --config auto --error

  https://github.com/semgrep/semgrep

Supply Chain Security and SBOM is not covered. Add to .github/workflows/security.yml:

  - name: Generate SBOM
    uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
    with:
      format: spdx-json
      artifact-name: sbom.spdx.json

  https://github.com/anchore/syft

Workflow Security and Hardening is not covered. Add to .github/workflows/security.yml:

  # advanced-security: false fails the job on findings instead of
  # uploading them, which would need security-events: write.
  - name: Check workflows for security problems
    uses: zizmorcore/zizmor-action@53290badfa369cb0bf7ad16342cb62c2fa47ac7b # v0.8.0
    with:
      advanced-security: false

  https://github.com/zizmorcore/zizmor

Not covered, with no snippet that fits:
  End-of-Life Package Checkers: Trivy checks a container image, and there is no Dockerfile at the repository root to build one. If it does not apply here, exclude it with -e "End Of Life".
  Performance and Reliability Testing Tools: k6 needs a load test script, load-test.js, for a service started in CI. If it does not apply here, exclude it with -e "Performance Testing".
