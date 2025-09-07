# Development Tools

This directory contains scripts and utilities for Loqa Hub development.

## Git Hooks

### install-git-hooks.sh

Installs git hooks that automatically remove Claude Code AI attributions from commit messages.

```bash
./tools/install-git-hooks.sh
```

**What it does:**
- Installs `prepare-commit-msg` hook to remove AI attributions before the commit editor opens
- Installs `commit-msg` hook as a fallback to clean up any remaining attributions
- Removes both the emoji line and Co-Authored-By line automatically

**Attributions removed:**
- `🤖 Generated with [Claude Code](https://claude.ai/code)`  
- `Co-Authored-By: Claude <noreply@anthropic.com>`

This ensures clean commit messages without manual editing, following the project's coding standards.

## Usage

Run the installation script in any Loqa repository:

```bash
# From the repository root
./tools/install-git-hooks.sh
```

The hooks will then automatically clean commit messages for all future commits in that repository.