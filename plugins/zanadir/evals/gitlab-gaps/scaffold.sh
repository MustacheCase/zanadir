#!/bin/bash
# A Python service on GitLab CI with tests and a linter and nothing else.
set -euo pipefail
mkdir -p app tests
printf 'flask==3.1.2\n' > requirements.txt
printf 'def health():\n    return "ok"\n' > app/__init__.py
printf 'from app import health\n\n\ndef test_health():\n    assert health() == "ok"\n' > tests/test_health.py
cat > .gitlab-ci.yml <<'YML'
stages: [test]

lint:
  stage: test
  image: python:3.13
  script:
    - pip install flake8
    - flake8 app tests

test:
  stage: test
  image: python:3.13
  script:
    - pip install -r requirements.txt pytest
    - pytest
YML
git init -q && git add -A && git -c user.name=eval -c user.email=eval@example.com commit -qm init
