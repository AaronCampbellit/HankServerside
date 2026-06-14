# Hermes Hank App

Hermes routes explicit `/Hermes` assistant prompts to a configured Hermes-compatible `/v1/responses` API.
The `/Hermes` entry is declared in `app.json` under `assistant.slash_commands`;
after the app is installed and enabled, Hank chat reads it from `/v1/home/apps`.

Build and install:

```bash
scripts/package-hermes-app.sh
```

Import `dist/hermes.hankapp` from Settings > Apps, then use the installed app's
Configure action. The form is rendered from `app.json` `config.settings_schema`;
do not configure Hermes from Settings > Connections.

Settings > Apps also controls the app-level access mode. `admins_only` keeps all
Hermes commands limited to home admins; `home_members` makes every command in
this installed app available to regular home members.

Required public config:

- `api_base_url`
- `model`
- `timeout_seconds`

Required secret config:

- `api_key`
