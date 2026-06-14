# PWA Current Scope

Hank Remote does not currently serve a standalone Progressive Web App surface.

The active browser UI is the operator dashboard served from `/` and `/dashboard`. That dashboard covers setup, agent visibility, tokens, Settings panes, HankAI, file browsing, profile notes, backups, restore operations, and troubleshooting.

The previous `/pwa` route family is intentionally removed:

- `/pwa`
- `/pwa/`
- `/pwa/sw.js`
- `/pwa/manifest.webmanifest`
- `/assets/site.webmanifest`

Do not reintroduce install-app behavior, web manifests, or service-worker registration unless a future plan explicitly defines a separate mobile-web product surface. If that happens, the PWA should be built as a first-class app-facing experience instead of silently hanging off the operator dashboard.
