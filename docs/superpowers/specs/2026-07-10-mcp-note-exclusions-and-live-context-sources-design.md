# MCP Note Exclusions and Live Context Sources Design

## Goal

Give Hank users a lightweight way to keep selected notes and notebooks out of the remote MCP connector, and let the same connector read approved project code live from configured File Server shares through the home agent.

The note lock is an MCP visibility marker. It is not encryption, authentication, or protection from another person who can use Hank.

## Scope

This feature adds:

- A lock/unlock button in the Hank Notes toolbar for notes and notebooks.
- A persisted `mcp_excluded` flag on profile notes.
- Inherited MCP exclusion for every note inside an excluded notebook.
- Server-side exclusion across all remote MCP note tools.
- A working MCP Context Sources panel in the dashboard GUI.
- Live, read-only project listing, search, and file reads routed from MCP through Hank Cloud to the home agent and an approved File Server share path.

This feature does not add note encryption, password prompts, hidden notes in Hank's normal UI, MCP file writes, cached project snapshots, arbitrary host path access, or nested notebooks.

## Note Visibility Model

Each profile note has a non-null `mcp_excluded` boolean with a default of `false`. The field is returned by the normal profile Notes API and accepted by create and update operations. It has no effect on normal Hank Notes list, search, fetch, edit, collaboration, attachment, or delete behavior.

A profile note is effectively excluded from MCP when either condition is true:

1. Its own `mcp_excluded` flag is true.
2. Its `parent_id` identifies a notebook whose `mcp_excluded` flag is true.

Because notebooks cannot be nested, effective exclusion requires at most one parent lookup. Moving an unlocked note out of an excluded notebook makes it visible to MCP again. Moving it into an excluded notebook makes it unavailable to MCP immediately. A note's own exclusion flag remains unchanged during moves.

The dashboard renders an unlocked or locked icon in the note/notebook editor toolbar. Activating it saves the flag through the existing profile Notes save path and updates the list state. The control has accessible labels such as `Exclude from MCP` and `Include in MCP`. Notebook rows and note rows display the locked state so users can understand why a child is unavailable to MCP; inherited exclusion is visually distinguished in helper text but does not modify the child's own flag.

## MCP Note Enforcement

Exclusion is enforced in the cloud MCP boundary, not only in the React UI and not in Hank's general Notes service. HankAI and normal authenticated Notes APIs continue to see these records.

The MCP note tools behave as follows:

- `list_notes` omits directly and inherited excluded notes and notebooks.
- `search_notes` searches only MCP-visible notes.
- `list_note_tags` computes counts only from MCP-visible notes.
- `get_note`, `update_note`, `append_note`, and `delete_note` return the existing friendly `not found` result for an effectively excluded note ID.
- `create_note` creates a visible root note because the current MCP contract does not accept a notebook parent or exclusion flag.

Filtering happens before content, previews, tags, or titles are serialized into MCP responses. This prevents metadata leakage as well as body access. The MCP tool schemas do not expose an override that can include excluded records.

## MCP Context Source Model

An MCP context source is a per-user, read-only grant to one project directory reachable through the user's home agent. Each source stores:

- Stable source ID.
- Owning user ID and home ID.
- Display name, such as `MiniHank`.
- Existing File Server source/share ID.
- Relative root path inside that share, such as `/Projects/MiniHank`.
- Enabled state.
- Created and updated timestamps.
- Last successful test timestamp and last test error summary.

The database does not store SMB credentials. It references an existing File Server source already configured for the home agent. Deleting or disabling a context source removes it from MCP immediately without modifying the underlying files or File Server configuration.

The feature uses a forward migration and the repository's migration/status/drift-check path. The table has ownership and enabled-state indexes. Context source names must be non-empty and unique per user. The selected File Server source and relative root are required.

## Dashboard GUI

The existing Settings > AI MCP Connector panel gains an `MCP Context Sources` section. It is available whenever MCP is enabled and remains visible with explanatory disabled state when no home agent or File Server share is available.

The section provides:

- A list of configured sources showing name, share, root path, enabled state, and latest connection test result.
- `Add source`, `Edit`, `Test`, enable/disable, and `Remove` controls.
- A source form with project name, File Server share selector, and relative project folder path.
- Plain-language copy that the source is read-only, accessed live, and unavailable while the agent/share is offline.
- Validation errors for duplicate names, missing shares, invalid roots, unavailable agents, roots that do not exist, and roots that are not directories.

`Test` performs a live agent probe and succeeds only when the configured share is available and the root resolves to a contained directory. Saving a source validates the request shape server-side; it does not require the agent to be online, so an existing source can remain configured through an outage. The UI clearly separates `Saved` from `Live test passed`.

All context-source management routes require the existing authenticated dashboard session, enforce per-user ownership, use CSRF protection for writes, and audit create, update, test, enable/disable, and remove actions.

## Live Agent Data Flow

The remote flow is:

```text
MCP client
  -> POST /v1/mcp with the user's OAuth token
  -> Hank Cloud resolves an enabled source owned by that user
  -> Cloud sends a bounded read-only context command to that user's connected home agent
  -> Agent resolves the configured File Server source and project root
  -> Agent validates and reads the live share
  -> Text-only result returns through Cloud to MCP
```

Dedicated protocol commands separate MCP context reads from the general File Server mutation surface:

- `mcp.context.list`
- `mcp.context.search`
- `mcp.context.read`
- `mcp.context.test`

Requests include the File Server source ID, configured root, and a relative path or query. The agent joins paths beneath the configured root, cleans them, rejects absolute/escaping paths, resolves symlinks where supported, and confirms the resolved target remains inside the root. The agent never accepts a raw local host pathname from MCP.

The first release performs direct live traversal/search and does not build a persistent index. Search is bounded by result count, files visited, bytes inspected, and command timeout so a large share cannot monopolize the agent. Results state when traversal was truncated.

## Exposed MCP Project Tools

The MCP server advertises these read-only tools under the existing `docs:read` scope:

- `list_context_sources`: list the user's enabled project sources and availability metadata without credentials or host paths.
- `list_context_files`: list an approved project's immediate directory contents using source ID and relative path.
- `search_context`: search filenames and allowed text content inside one approved source.
- `read_context_file`: read one allowed text file from one approved source.

Tool output uses source-relative paths. It never returns SMB usernames, passwords, server credentials, or the agent's local mount paths. Unknown, disabled, or other-user source IDs return `not found`.

## File Safety and Content Policy

Context access is deny-by-default and read-only.

Allowed content includes common source and project-document formats such as Go, Swift, TypeScript, JavaScript, JSX/TSX, Python, Rust, Java, C-family source, shell, SQL, HTML/CSS, Markdown, JSON, YAML, TOML, XML, protobuf, module, and lock files.

The agent rejects:

- `.env` and `.env.*` files.
- `.git`, dependency/vendor, build-output, cache, and hidden directories.
- Private keys, certificates, database files, archives, media, binaries, and files without an approved text type.
- Files larger than the configured MCP read ceiling.
- Symlinks or paths that resolve outside the configured source root.

Initial fixed limits are 400 KB for an individual file read, 50 returned search results, 10,000 visited files, 20 MB of inspected text per search, and a 20-second cloud-to-agent command timeout. A bounded failure returns a useful MCP error or truncation marker rather than partial unlabelled output.

## Error Handling

The connector distinguishes:

- `not found`: unknown source, excluded note, disallowed file, or inaccessible relative path.
- `temporarily unavailable`: home agent disconnected, share unavailable, or command timeout.
- `invalid request`: missing query/path, malformed source ID, absolute path, or traversal attempt.
- Successful but truncated search: valid bounded result with a truncation notice.

Detailed path and transport errors are logged server-side without file contents or credentials. MCP receives concise messages. Context reads and searches are audited with user, source ID, operation, and request correlation ID, but not query results or file contents.

## Compatibility

Existing notes default to `mcp_excluded=false`. Existing MCP OAuth grants keep working because project tools reuse `docs:read`; no new consent is required. Existing clients that ignore the new Notes API field continue working.

The protocol version remains backward compatible because the new agent commands and payload fields are additive. An older agent will report the context capability as unavailable, and the dashboard will explain that the home agent must be updated before live context sources can be tested.

The Hank iOS app is outside this repository. The server contract will safely preserve `mcp_excluded=false` when an older client saves a note without the field. iOS lock-icon parity is a separate client update unless explicitly requested in the Hank iOS repository.

## Testing and Verification

Backend tests cover:

- Directly excluded notes disappearing from every MCP read/write tool.
- Children of excluded notebooks disappearing while unlocked children moved out become visible.
- Normal profile Notes APIs continuing to list, search, fetch, and edit excluded records.
- Migration, persistence, API round trips, ownership, CSRF, and audit events.
- Context-source ownership and disabled-source behavior.
- Cloud routing to the correct home agent and clear offline/timeout handling.
- Agent root containment, traversal rejection, symlink escape rejection, extension policy, hidden/secret exclusions, read limits, and bounded search truncation.

Frontend tests cover:

- Lock icon save behavior for notes and notebooks.
- Inherited notebook exclusion messaging.
- Context source list, add/edit/test/toggle/remove flows.
- Empty, no-share, offline-agent, validation-error, and success states.

Verification includes Go formatting, targeted Go tests, the full Go test suite and build, dashboard unit tests and production build, migration status, schema drift check, and `git diff --check`. A live test should configure one real File Server project root, connect through MCP, list/search/read a harmless source file, and confirm an excluded note and excluded notebook child are absent.
