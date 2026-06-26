# Hank App Product Notes

Status: product-facing reference extracted from historical docs and Hank profile notes.

This document captures the Hank iOS app behavior that is still useful when coordinating the server contract. It intentionally avoids server implementation details, database tasks, and historical backend checklists.

## Product Identity

Hank is the user-facing iOS app for a single self-hosted Hank Remote deployment. The app should feel like one home, one connected remote surface, and one assistant layer rather than a multi-tenant server console.

Core app areas:

- Home dashboard
- Home Assistant controls and status
- Files / SMB-backed file browser through Hank Remote APIs
- Profile notes and shared Home notes
- Hank Assistant chat
- Device calendar bridge
- Settings for Home, people, permissions, connections, apps, recovery, backups, notifications, and assistant behavior

## Single Home Model

The app should treat Hank Remote as one deployment-scoped Home.

- No Remote home picker.
- No Remote home creation flow after first setup.
- First admin creates the initial Home.
- Additional people join by invitation.
- Users have `admin` or `member` roles.
- Admins manage Home settings, people, permissions, service profiles, app packages, storage/recovery, and agent tokens.
- Members use the features they are allowed to use.

## App Experience Rules

- The iPhone app should not use direct remote SMB access. Files move through authenticated Hank Cloud and home-agent flows.
- Home Assistant credentials stay behind the home-agent/server architecture.
- Profile notes are personal; shared Home notes are collaborative and permission-controlled.
- The app should use note-specific APIs instead of reconstructing notes from generic file operations.
- Storage, backup, restore, app package management, member management, and permission editing are admin-oriented experiences.
- Notification text must stay redacted and must not include passwords, database URLs, command output, token values, backup encryption values, note body text, or raw Home Assistant payloads.

## Assistant Behavior

The Hank Assistant should prefer typed Hank tools over guessing for notes, files, calendar, Home Assistant, project docs, and prior Hank context where enabled.

It should:

- Search and answer over permitted Hank data.
- Perform explicit actions such as appending to notes or creating calendar events.
- Return structured targets so the app can deep-link to notes, files, folders, events, entities, or documents.
- Ask concise clarification questions when a target is ambiguous.
- Require confirmation for risky, destructive, ambiguous, or high-impact actions.

Default confirmation policy:

- Auto-run when the target is uniquely resolved and the action is not destructive.
- Always confirm delete, remove, cancel, primary restore, data-discarding commands, and low-confidence or ambiguous destinations.

## Useful Regression Phrases

- `Add drinks to the grocery list`
- `add email patrick to work notes`
- `find the 2025 tax documents folder`
- `add family dinner to the 12th of july`
- `whats on my calandar for today`
- `find the hank feature note`
- `add link to hank feature note`
