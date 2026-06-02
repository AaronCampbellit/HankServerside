# Phase 3: File API And NAS Access

## Goal

Replace remote app-side SMB behavior with a server-mediated file API that the home agent fulfills locally.

## Why This Phase Matters

This is the architectural payoff. The iPhone app should stop trying to speak remote SMB directly once this exists.

## Scope

Build:

- agent-side NAS adapter
- high-level file API
- directory listing
- file download
- file upload
- directory creation
- rename and delete

Avoid:

- tunneling raw SMB packet semantics to the app
- exposing NAS endpoints directly to the cloud or internet

## Design Direction

The agent may still use SMB locally to reach the NAS, but the app should see a clean file API:

- list directory contents
- fetch file metadata
- stream file download
- upload file content
- create folder
- move or rename
- delete item

## Suggested Command Set

- `files.list`
- `files.stat`
- `files.download`
- `files.upload`
- `files.create_directory`
- `files.rename`
- `files.delete`

For large files, prefer streamed HTTP responses or chunked relay commands instead of giant single-message payloads.

## Deliverables

- local NAS adapter in the agent
- cloud relay for file commands
- app-facing file API contract
- file transfer strategy for upload and download

## Exit Criteria

Phase 3 is complete when:

- the app can browse directories remotely through Hank Cloud
- the app can download and upload files remotely
- remote file operations no longer depend on direct SMB from the phone
- errors are mapped into stable app-facing categories

## Testing Expectations

Add tests for:

- directory listing
- upload and download success
- rename and delete behavior
- missing path handling
- NAS timeout behavior
- large file transfer behavior

## Recommended First Tasks

1. Define app-facing file schemas.
2. Add an agent-side file service interface.
3. Implement a local SMB-backed adapter behind that interface.
4. Add cloud file routing.
5. Add transfer tests with realistic file sizes.
