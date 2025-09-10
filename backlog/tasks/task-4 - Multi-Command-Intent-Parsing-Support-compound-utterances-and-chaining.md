---
id: task-4
title: Multi-Command Intent Parsing - Support compound utterances and chaining
status: To Do
assignee:
  - development
created_date: '2025-09-10 21:29'
labels:
  - intent-parsing
  - llm
  - multi-command
  - chaining
  - pipeline
dependencies: []
priority: medium
---

## Description

Support multi-command chaining in parsed intent pipeline. Parse compound utterances (e.g., 'Turn off the lights and play music'), route sub-intents to appropriate skills in sequence, ensure skill responses can be executed in order without overlap or conflict, combine or sequence TTS responses for multiple results. If chaining fails, gracefully fallback to the first valid command.

## Implementation Notes

GitHub Issue: https://github.com/loqalabs/loqa-hub/issues/27. Moved from main loqa repository task-5 for proper service ownership.
