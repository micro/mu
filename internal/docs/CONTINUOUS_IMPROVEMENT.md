# Continuous Improvement

Mu uses an autonomous improvement loop driven by Codex. On a schedule,
a GitHub Action opens a tracking issue, dispatches Codex, and Codex
implements one improvement increment — then opens a PR with auto-merge
enabled. When CI passes, the PR lands automatically.

## How It Works

1. GitHub Action runs on schedule (hourly)
2. Opens a fresh issue titled "Continuous improvement increment #N"
3. Posts an @codex comment with instructions
4. Codex picks one item, implements it, runs tests, opens PR
5. GitHub auto-merge lands the PR when CI is green

## What Codex Should Work On

Priority order — pick the single highest-value item:

### 1. Bug Fixes
- Fix any failing tests
- Fix known issues in the issue tracker
- Fix error handling gaps

### 2. Test Coverage
- Add tests for untested packages
- Add edge case tests for critical paths (agent routing, trade execution, auth)

### 3. Code Quality
- Remove dead code and unused imports
- Fix inconsistencies (error handling patterns, logging)
- Simplify complex functions

### 4. Feature Improvements
- Improve agent routing accuracy
- Better error messages for users
- Performance improvements (caching, reducing API calls)
- Improve documentation

### 5. Infrastructure
- Improve CI pipeline
- Add linting rules
- Improve build times

## Rules

- **One concern per PR** — don't bundle unrelated changes
- **Don't touch brand/positioning** — no changes to taglines, descriptions, marketing copy
- **Don't break public API** — MCP tool names, A2A protocol, webhook endpoints stay stable
- **Don't change env var names** — existing deployments depend on them
- **Run all checks** — `go build ./...`, `go test ./...`, `go vet ./...`
- **Keep PRs small** — under 200 lines of change is ideal
- **Write clear commit messages** — explain what and why
