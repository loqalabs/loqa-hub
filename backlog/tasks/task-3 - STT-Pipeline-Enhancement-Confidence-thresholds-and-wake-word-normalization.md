---
id: task-3
title: STT Pipeline Enhancement - Confidence thresholds and wake word normalization
status: To Do
assignee:
  - development
created_date: '2025-09-10 21:28'
labels:
  - stt
  - voice-pipeline
  - quality
  - wake-word
  - confidence
dependencies: []
priority: medium
---

## Description

Strip wake word ('Hey Loqa') before passing to intent parser, normalize common misspellings of 'Loqa' (e.g., 'Luca'), define and enforce default confidence threshold for rejecting low-quality transcriptions. Implement 'did you mean?' patterns for low-confidence commands.

## Implementation Notes

GitHub Issue: https://github.com/loqalabs/loqa-hub/issues/20. Moved from main loqa repository task-4 for proper service ownership.
