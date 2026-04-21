# Hank Remote Roadmap

This roadmap breaks the `Hank Remote` system into implementation phases so work can be handed to Codex in clean chunks.

## Phase List

1. [Phase 1: Identity And Routing](./phase-1-identity-and-routing.md)
2. [Phase 2: Home Assistant Remote Access](./phase-2-home-assistant.md)
3. [Phase 3: File API And NAS Access](./phase-3-files.md)
4. [Phase 4: Notes Sync And Background Flows](./phase-4-notes.md)
5. [Phase 5: Operations, Security, And Production Readiness](./phase-5-operations.md)

## Delivery Order

Build the system in this order:

1. Make cloud-to-agent routing and auth real.
2. Ship Home Assistant first because it is the fastest end-to-end win.
3. Replace remote SMB from the app with a higher-level file API.
4. Move notes onto the same relay model.
5. Add persistence, hardening, observability, and deployment polish.

## Rules Across Every Phase

- Keep the app-facing API high-level.
- Keep the home agent outbound-only.
- Do not expose SMB directly to the internet.
- Prefer simple JSON over HTTPS/WebSocket unless streaming or performance forces a change.
- Add tests when protocol or routing behavior changes.
- Keep cloud concerns and agent concerns separated in code.
