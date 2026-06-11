# Hermes Chat Workflow

This note records the HankAI `/Hermes` workflow for talking to a Hermes Agent
running on another VM inside the home/server network.

## Hermes Integration Surface

Hermes Agent exposes an API server that is compatible with OpenAI-style HTTP
clients. The relevant Hermes docs are:

- <https://hermes-agent.nousresearch.com/docs/user-guide/features/api-server>
- <https://hermes-agent.nousresearch.com/docs/developer-guide/programmatic-integration>
- <https://hermes-agent.nousresearch.com/docs/user-guide/messaging>

For Hank Remote, use the Hermes API server instead of adding another public chat
platform:

- enable Hermes with `API_SERVER_ENABLED=true`
- set `API_SERVER_KEY` on the Hermes VM
- run the Hermes gateway so it listens on the private VM address or a private
  network listener reachable by the Hank home agent
- call `POST /v1/responses` with `Authorization: Bearer <API_SERVER_KEY>`

The Hermes docs also describe a TUI gateway JSON-RPC protocol and messaging
platform adapters. Those are richer surfaces, but the API server is the narrowest
fit for the first Hank chat feature because it is plain HTTP, supports stateful
conversations, and does not require Hank to become a Hermes messaging-platform
adapter.

## Hank Remote Shape

The Hank app still talks only to Hank Cloud. The cloud keeps the authenticated
HankAI assistant session endpoint and detects only explicit prompts that start
with `/Hermes`.

Flow:

1. Hank app posts a normal assistant message to
   `POST /v1/home/assistant/sessions/{sessionID}/messages`.
2. Hank Cloud classifies `/Hermes <prompt>` as the installed Hermes app chat
   command when the home agent advertises `apps.hermes.chat`.
3. Hank Cloud sends `apps.invoke` over the existing outbound home-agent
   WebSocket with `app_id: "hermes"` and `command_id: "chat"`.
4. The Hank home agent runs the installed Hermes package, and the package posts
   to the Hermes API server on the private network.
5. Hermes returns text through the package response, and Hank persists that text
   as the assistant reply in
   the same HankAI session.

The Hermes bearer key stays in the agent-side installed app config. It is not
sent to the Hank app. The cloud stores only non-secret installed-app metadata
such as the API base URL, model, timeout, status, and whether a key is set.

## Agent Configuration

Build the Hermes app package and import it from Settings > Apps:

```bash
scripts/package-hermes-app.sh
```

Import `dist/hermes.hankapp`, then use the installed app's Configure action to
set the Hermes API base URL, model, timeout, and API key. The Settings > Apps
form is rendered from `packages/hermes/app.json` `config.settings_schema`; there
is no separate Hermes form in Settings > Connections.

The configured API base URL may include `/v1`; both
`http://hermes-vm:8642` and `http://hermes-vm:8642/v1` are accepted.

## Current Guardrails

- Hermes chat is explicit only: the command must start with `/Hermes`, which is
  exposed from the installed enabled Hermes app manifest.
- Empty `/Hermes` prompts return a usage hint instead of calling Hermes.
- Hank Cloud routes this workflow through the home agent; it does not expose the
  Hermes API server publicly.
- Home members must be admins to use Hermes chat for now because Hermes can have
  terminal and tool access beyond Hank's normal per-feature permissions.
- The first implementation returns final text only. It does not expose Hermes
  streaming, approvals, run cancellation, file uploads, or tool-progress UI.
