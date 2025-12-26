# Git Hooks

This directory contains git hooks for the mu project.

## Installation

To install the hooks, run:

```bash
./scripts/install-hooks.sh
```

This will copy the hook scripts to your `.git/hooks/` directory.

## Available Hooks

### pre-commit

Runs before every commit to:
- Execute all tests (`go test ./... -short`)
- Prevent commits if tests fail

This helps catch regressions before they're committed to the repository.

**Skip the hook (not recommended):**
```bash
git commit --no-verify
```

## Maintenance

The hook scripts are version controlled in this `scripts/` directory. After updating a hook:

1. Edit the script in `scripts/`
2. Re-run `./scripts/install-hooks.sh` to update your local `.git/hooks/`
3. Commit the updated script so other developers get the changes

## Why Git Hooks?

Git hooks in `.git/hooks/` are not tracked by git. By keeping our hooks in `scripts/` and providing an install script, we can:
- Version control our hooks
- Share them with all developers
- Make updates easy to deploy
