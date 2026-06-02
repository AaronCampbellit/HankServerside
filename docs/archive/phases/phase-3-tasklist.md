# Phase 3 Tasklist

This tasklist turns the file API phase into concrete implementation work.

## Phase 3 Objective

Replace remote app-side SMB access with a higher-level file API fulfilled by the home agent.

## Definition Of Done

Phase 3 is done when all of these are true:

1. The app can browse remote directories through Hank Cloud.
2. The app can download and upload files through Hank Cloud.
3. The agent handles local NAS access.
4. The app no longer needs direct remote SMB semantics for remote use.

## Recommended Implementation Order

1. File domain schema
2. Agent-side file service abstraction
3. Local NAS adapter
4. Cloud relay and transfer behavior
5. Integration tests with realistic file operations

## Task Group 1: File Domain Schema

### Add Protocol Types

Define typed models for:

- file item
- file metadata
- list request and response
- upload request metadata
- download metadata
- create directory
- rename
- delete

### Suggested Commands

- `files.list`
- `files.stat`
- `files.download`
- `files.upload`
- `files.create_directory`
- `files.rename`
- `files.delete`

### Design Rules

- app should reason about folders and files, not raw SMB packets
- keep path handling normalized
- errors should be file-domain errors, not SMB-specific internals

## Task Group 2: Agent File Service Interface

### Create Package

Add:

- `internal/agent/files`

### Suggested Interface

```go
type Service interface {
    List(ctx context.Context, path string) ([]Item, error)
    Stat(ctx context.Context, path string) (Item, error)
    CreateDirectory(ctx context.Context, path string) error
    Rename(ctx context.Context, from string, to string) error
    Delete(ctx context.Context, path string, isDirectory bool) error
}
```

For transfers:

- add separate download and upload helpers
- do not force giant in-memory payloads for large files

## Task Group 3: Local NAS Adapter

### Build A Local Adapter

Implement a local file backend behind the agent service interface.

Options:

- SMB-backed adapter
- direct local filesystem adapter if the NAS is mounted

The first pass can use SMB locally if that is the easiest path inside the home network.

### Rules

- the cloud should never store NAS credentials
- the agent owns NAS access and secrets
- normalize errors before they leave the agent

## Task Group 4: Transfer Strategy

### Download

Pick one:

- streamed HTTP endpoint
- chunked WebSocket relay

For large files, prefer streaming over loading the whole file into memory.

### Upload

Pick one:

- multipart HTTP upload to cloud, then relay to agent
- chunked streamed upload over WebSocket

Prefer the simplest shape that supports retries later.

### Early Recommendation

Use:

- WebSocket control plane
- HTTP download/upload endpoints on cloud for actual file transfer

That keeps large transfers out of the command channel.

## Task Group 5: Cloud Relay

### Add File Routing

Cloud should:

- authorize app access to a home
- route metadata operations to the correct agent
- coordinate upload/download operations
- return stable errors if the agent is offline or transfer setup fails

### Suggested Files

- `internal/cloud/http_files.go`
- `internal/cloud/relay_files.go`

## Task Group 6: Agent Command Handling

### Extend Agent Dispatcher

Handle:

- list
- stat
- create directory
- rename
- delete

If using streamed transfer endpoints, add upload/download coordination messages as well.

## Task Group 7: Testing

### Unit Tests

Add tests for:

- path normalization
- item mapping
- error normalization
- protocol encode/decode

### Integration Tests

Required scenarios:

1. list a directory
2. create a directory
3. rename a file or folder
4. delete a file or folder
5. upload a file
6. download a file

Failure scenarios:

1. missing path
2. permission denied
3. agent offline
4. interrupted transfer
5. large file transfer behavior

## Task Group 8: Logging And Metrics

Add logs for:

- file operation type
- target home
- normalized path
- upload and download start/finish
- transfer error

Add basic metrics later if the metrics package already exists.

## Suggested File Additions

- `internal/protocol/files.go`
- `internal/agent/files/service.go`
- `internal/agent/files/smb_adapter.go`
- `internal/agent/commands_files.go`
- `internal/cloud/http_files.go`
- `internal/cloud/relay_files.go`
- `internal/agent/files/*_test.go`

## Suggested Codex Prompt For This Phase

> Implement Phase 3 from `docs/phase-3-files.md` and `docs/phase-3-tasklist.md`. Build a file-domain API with agent-side local NAS access, cloud relay support for directory and item operations, and a practical upload/download strategy that avoids raw remote SMB in the app. Add tests for list, create, rename, delete, upload, and download. Run formatting and `go build ./...` before finishing.
