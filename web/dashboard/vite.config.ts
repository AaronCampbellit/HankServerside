import { readFileSync } from "node:fs";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Optional API proxy for local dev against a running Hank deployment, e.g.:
//   HANK_DEV_API_PROXY=https://hankdemo.campbellservers.com npm run dev
// HANK_DEV_SESSION_TOKEN_FILE can point at a file holding a session token; the
// proxy then attaches it as the session cookie so the app is signed in.
const proxyTarget = process.env.HANK_DEV_API_PROXY;
const sessionTokenFile = process.env.HANK_DEV_SESSION_TOKEN_FILE;
const desktopAcceptanceIdentityFile = process.env.HANK_DEV_DESKTOP_IDENTITY_FILE;

function devSessionCookie(): string {
  if (!sessionTokenFile) return "";
  try {
    return `hank_remote_session=${readFileSync(sessionTokenFile, "utf8").trim()}`;
  } catch {
    return "";
  }
}

export default defineConfig({
  plugins: [react(), {
    name: "hank-desktop-acceptance-identity",
    configureServer(server) {
      if (!desktopAcceptanceIdentityFile) return;
      server.middlewares.use("/__hank/desktop-acceptance-identity", (_request, response) => {
        try {
          const state = JSON.parse(readFileSync(desktopAcceptanceIdentityFile, "utf8")) as Record<string, unknown>;
          response.setHeader("Cache-Control", "no-store");
          response.setHeader("Content-Type", "application/json");
          response.end(JSON.stringify({
            device_id: state.operator_device_id,
            private_key_pkcs8: state.operator_private_key_pkcs8,
            public_key_spki: state.operator_public_key_spki,
          }));
        } catch {
          response.statusCode = 404;
          response.end("acceptance identity unavailable");
        }
      });
    },
  }],
  build: {
    outDir: "../../internal/cloud/ui/react",
    emptyOutDir: true,
  },
  server: {
    fs: { allow: [new URL("../..", import.meta.url).pathname] },
    ...(proxyTarget
      ? {
        proxy: {
          "/v1": {
            target: proxyTarget,
            changeOrigin: true,
            ws: true,
            rewriteWsOrigin: true,
            cookieDomainRewrite: "",
            configure(proxy) {
              const appendSession = (proxyReq: import("node:http").ClientRequest) => {
                const session = devSessionCookie();
                if (!session) return;
                const existing = proxyReq.getHeader("cookie");
                const csrf = "hank-dev-csrf";
                const credentials = `${session}; hank_remote_csrf=${csrf}`;
                proxyReq.setHeader("cookie", existing ? `${existing}; ${credentials}` : credentials);
                if (["POST", "PUT", "PATCH", "DELETE"].includes(proxyReq.method || "")) {
                  proxyReq.setHeader("X-Hank-CSRF-Token", csrf);
                }
              };
              proxy.on("proxyReq", appendSession);
              proxy.on("proxyReqWs", appendSession);
            },
          },
          "/ws": {
            target: proxyTarget,
            changeOrigin: true,
            ws: true,
            rewriteWsOrigin: true,
          },
        },
      }
      : {}),
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/setupTests.ts"],
  },
});
