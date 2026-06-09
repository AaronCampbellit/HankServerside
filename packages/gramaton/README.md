# Gramaton Hank App

Gramaton powers the optional `/gramaton` HankAI media search and download workflow as an installable agent app.

The app inherits the home agent file environment, including `HANK_REMOTE_AGENT_FILES_ROOT` and `HANK_REMOTE_SMB_SHARES_JSON`, so destination validation and file writes continue through Hank's source-aware file service.

Required app config:

- `enabled`
- `base_url`
- `username`
- `source_id`
- destination paths
- `require_confirmation`

Required secret config:

- `password`
