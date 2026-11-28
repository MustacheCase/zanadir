---
description: Trivy runs on every pull request but only prints what it finds, so the dependency scan cannot fail the build.
tags: [weakened]
max_turns: 20
allowed_tools: [Read, Glob, Grep, Skill]
---

Our CI runs a dependency scan on every pull request. Is it actually protecting us, and what else is the pipeline missing? Don't change anything yet, just tell me what you'd fix first.
