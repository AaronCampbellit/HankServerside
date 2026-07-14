# Installed App Slash Commands in HankAI Dashboard

Date: 2026-07-13
Status: Approved for implementation

## Problem

The React HankAI dashboard uses a hardcoded list of built-in slash commands. That list does not include slash commands declared by enabled installable Hank apps, even though the Hank app contract exposes those commands through `GET /v1/home/apps`. As a result, an installed app such as Gramaton can be configured, enabled, and callable by the backend while its `/gramaton` command is absent from the dashboard command palette.

Live diagnostics confirmed that the installed Gramaton binary, saved configuration, provider authentication, and direct `search` command currently work. The regression is in the React command-discovery surface introduced during the dashboard migration.

## Decision

Restore the existing Hank app contract in the React dashboard. HankAI will load app metadata through the existing apps API, select slash commands from enabled apps, and merge those commands with the built-in dashboard commands.

Do not hardcode Gramaton, Hermes, YDownload, or any other installable app command in HankAI. Do not change the `.hankapp` manifest, agent invocation protocol, assistant routing, or database schema.

## Data Flow

1. When the HankAI page loads, request assistant status, assistant sessions, and the current home app list in parallel.
2. Keep the existing built-in command definitions as the stable core command set.
3. For each enabled app, read its `slash_commands` metadata returned by `GET /v1/home/apps`.
4. Convert valid package commands into the dashboard palette shape using the package command as the token and its description as the hint.
5. Merge built-in and installed-app commands without duplicate tokens, using case-insensitive token identity.
6. Filter the merged list as the user types and insert the selected token into the message draft using the existing interaction.
7. Submission continues through the existing assistant messages endpoint. Server-side installed-app resolution and authorization remain authoritative.

## Components

### Apps API model

Extend the frontend `AppSummary` type with the existing `slash_commands` response field. Each command contains `command`, `command_id`, and an optional `description`.

### HankAI page state

Store the discovered command list in the ready page state. Fetch app metadata in the same parallel bootstrap operation as assistant status and sessions so the change does not introduce a request waterfall.

### Command normalization

Build the palette from built-ins plus enabled app metadata. Ignore disabled apps and malformed or blank slash-command tokens. Use the manifest description as the visible hint, with a concise app-based fallback when no description is supplied. Preserve built-ins if an app declares a conflicting token.

## Error Handling

App command discovery is optional dashboard enrichment. If the apps request fails, HankAI should still load with built-in commands and existing conversations rather than replacing the page with an error. Actual command availability, membership access, and invocation errors continue to be enforced and reported by the server.

## Testing

Add focused React tests that first reproduce the regression and then prove:

- an enabled app-provided `/gramaton` command appears in the palette;
- selecting it inserts `/gramaton ` into the draft;
- a disabled app command is excluded;
- a failed app-list request leaves built-in commands usable;
- HankAI bootstrap requests remain parallel.

Run the targeted HankAI page tests, the dashboard test suite, the dashboard production build, and rendered interaction validation. For live proof, verify the deployed HankAI palette exposes `/gramaton` from app metadata and that submitting a real Gramaton search returns media results.

## Security and Persistence

This change adds no route, secret handling, authorization behavior, database write, or schema migration. The dashboard consumes already-filtered home app metadata, while the server remains responsible for membership filtering and app-command authorization.
