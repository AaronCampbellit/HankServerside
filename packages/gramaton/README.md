# Gramaton Hank App

Gramaton powers the optional `/gramaton` HankAI media search and download workflow as an installable agent app.
The `/gramaton` entry is declared in `app.json` under `assistant.slash_commands`;
after the app is installed and enabled, Hank chat reads it from `/v1/home/apps`.

The app inherits the home agent file environment, including `HANK_REMOTE_AGENT_FILES_ROOT` and `HANK_REMOTE_SMB_SHARES_JSON`, so destination validation and file writes continue through Hank's source-aware file service.

Build and install:

```bash
scripts/package-gramaton-app.sh
```

Import `dist/gramaton.hankapp` from Settings > Apps, then use the installed
app's Configure action. The form is rendered from `app.json`
`config.settings_schema`; do not configure Gramaton from AI Settings.

Settings > Apps also controls the app-level access mode. `admins_only` keeps all
Gramaton commands limited to home admins; `home_members` makes every command in
this installed app available to regular home members.

Required app config:

- `enabled`
- `base_url`
- `username`
- `source_id`
- destination paths
- `require_confirmation`

Required secret config:

- `password`
