# Loqa Git Hooks

This directory contains Git hooks that help maintain code quality and consistency across the Loqa project.

## Available Hooks

### commit-msg
Prevents AI tool attribution from being included in commit messages. Blocks patterns like:
- `Generated with Claude/ChatGPT/AI`
- `Co-Authored-By: Claude`
- `🤖 Generated with...`
- And other AI tool references

## Installation

To activate the hooks in your local repository, use Git's built-in hooks directory feature:

```bash
git config core.hooksPath .githooks
```

This tells Git to use the `.githooks` directory instead of `.git/hooks/` for all hooks. Since the `.githooks` directory is committed to the repository, everyone gets the same hooks automatically.

### Alternative (manual installation)
If you prefer the traditional approach:

```bash
cp .githooks/commit-msg .git/hooks/
chmod +x .git/hooks/commit-msg
```

## Why These Hooks?

These hooks help ensure that:
- Commit messages focus on the "what" and "why" rather than the tool used
- Git history remains clean and professional
- Attribution stays focused on human contributors

## Bypassing Hooks

If you absolutely need to bypass a hook temporarily:

```bash
git commit --no-verify
```

**Note:** Use this sparingly and only when necessary.