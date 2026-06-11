# Hermes Hank App

Hermes routes explicit `/Hermes` assistant prompts to a configured Hermes-compatible `/v1/responses` API.

Build and install:

```bash
scripts/package-hermes-app.sh
```

Import `dist/hermes.hankapp` from Settings > Apps, then use the installed app's
Configure action. The form is rendered from `app.json` `config.settings_schema`;
do not configure Hermes from Settings > Connections.

Required public config:

- `api_base_url`
- `model`
- `timeout_seconds`

Required secret config:

- `api_key`
