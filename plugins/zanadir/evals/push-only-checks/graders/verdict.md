---
type: llm
---

PASS if the response explains that the workflow runs only on pushes to main, so lint, tests and npm audit run after a change has merged and cannot block a pull request, and recommends running them on pull requests.
FAIL if the response says the existing checks gate changes before merge, or does not mention the trigger.
