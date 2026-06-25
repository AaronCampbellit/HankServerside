# Hank App Auth Migration

This document describes the app-side WebSocket auth contract for Hank.

## Goal

Move the iPhone app away from putting long-lived `session_token` values into `/ws/app` URLs.

The new model is:

- HTTP API calls: keep using `Authorization: Bearer <session_token>`
- App WebSocket: first request a short-lived app WebSocket ticket, then connect to `/ws/app?app_ticket=...`
- Dashboard: cookie-only browser auth, already migrated on the server side

This keeps the app stable while tightening the public browser surface and reducing token exposure in logs, history, and copied URLs.

## New Server Endpoint

The app should call:

- `POST /v1/ws/app-ticket`

Auth:

- `Authorization: Bearer <session_token>`

Response shape:

```json
{
  "ticket": "raw-short-lived-ticket",
  "expires_at": "2026-04-14T18:30:00Z",
  "websocket_path": "/ws/app?app_ticket=raw-short-lived-ticket"
}
```

Behavior:

- the ticket is short-lived
- the ticket is single-use
- the server validates the underlying app session before accepting it

## Hank App Changes

1. Keep the existing login flow.
   The app can keep using `POST /v1/auth/login` and storing the returned `session_token`.

2. Keep Bearer auth for HTTP.
   Continue sending `Authorization: Bearer <session_token>` for authenticated HTTP endpoints.

3. Change WebSocket connection setup.
   Before opening `/ws/app`, request a WebSocket ticket:

```http
POST /v1/ws/app-ticket
Authorization: Bearer <session_token>
```

4. Open the socket with the returned ticket.

```text
wss://<cloud-host>/ws/app?app_ticket=<ticket>
```

5. Treat ticket issuance as disposable.
   If the socket fails before opening, fetch a new ticket and retry.

6. Do not reuse tickets.
   A ticket is intentionally single-use.

## Current Compatibility

The server no longer accepts long-lived `session_token` values in the `/ws/app` query string. App WebSocket clients must use a short-lived app ticket, or authenticate with the normal Bearer header in non-browser test harnesses.

## Why This Is Better

- avoids putting long-lived session tokens into WebSocket URLs
- reduces credential leakage risk through copied URLs and logs
- keeps browser and app auth models separated cleanly
- allows the dashboard to stay cookie-only without forcing an app rewrite

## ChatGPT/Codex Account Linking (Experimental)

To enable experimental subscription-backed ChatGPT usage inside Hank Assistant, the app should support the server-side ChatGPT/Codex link flow. This does not replace the supported OpenAI API-key provider; API billing/auth remain separate.

### Endpoints

- `GET /v1/oauth/openai/start`
- `GET /v1/oauth/openai/status`
- `GET /v1/home/assistant/settings`
- `PUT /v1/home/assistant/settings`

### App Flow

1. Ensure the user is authenticated with normal Hank session auth (`Authorization: Bearer <session_token>`).
2. Call `GET /v1/oauth/openai/start`.
3. Show `verification_url`, `user_code`, `expires_at`, and `poll_after_seconds` when the response includes `auth_mode: "device_code"`.
4. The server polls the auth service and stores the final tokens server-side.
5. App should call `GET /v1/oauth/openai/status` until linked, failed, or expired, then show success/failure status in Assistant settings.

The older browser-redirect OpenAI OAuth callback has been removed. Keep OpenAI API-key provider support as the production API-key path, and use this device-code flow only for ChatGPT/Codex subscription-backed chat.

### UX Requirements

- Add an Assistant settings card:
  - linked/not linked status
  - "Link ChatGPT/Codex" action when the server reports `auth_provider: "chatgpt_codex"` or `auth_mode: "device_code"`
  - pending device code, plan type, token expiry, and "Relink" if link fails/expired
- Show actionable error copy if device-code linking fails or expires.
- If not linked, Assistant tab should still work for local flows where available, but clearly indicate ChatGPT subscription-backed features require linking.
- The current dashboard can show the device code. Hank iOS needs a follow-up UI change if it should show the device code natively instead of only opening a browser link.

### HankAI Harness Settings

The server now exposes per-user, per-Home HankAI harness settings through `GET`/`PUT /v1/home/assistant/settings`.

The settings include:
- source access toggles for personal notes, shared notes, files, calendar, and Home Assistant
- project docs access for Hank Remote README, contract docs, setup docs, phase docs, and runbooks
- `system_prompt`, which is the system message sent to the active chat provider
- a server-owned maximum context window

These settings apply to the next assistant message without restarting Hank Remote. If Hank iOS adds native controls later, it should treat these as privacy and behavior controls for what Hank context may be sent to the active model provider.
