# Notes Undo/Redo History Implementation Plan

**Goal:** Make Notes undo and redo deterministic across typing, deletion, paste, and formatting, with 50 recoverable actions in each direction.

**Architecture:** Keep editor history local to the active note. Store pre-action body snapshots with an action kind, coalesce only consecutive typing or deletion bursts of the same kind within 750 ms, and keep formatting/paste operations discrete. Reset history when the active note changes; autosave remains independent and saves the resulting editor state in the background.

**Tech Stack:** React, TypeScript, Vitest, Testing Library.

### Task 1: Define action-aware history behavior

1. Add tests proving typing bursts coalesce, action-kind changes create boundaries, undo/redo preserve exact order, and the history limit is 50.
2. Run the focused tests and confirm they fail against the current time-only history.
3. Implement typed history entries and a 50-entry cap for both stacks.
4. Run the focused tests, full dashboard tests, production build, and `git diff --check`.
5. Commit and push the verified change.
