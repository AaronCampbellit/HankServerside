import { readFileSync } from "node:fs";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Optional API proxy for local dev against a running Hank deployment, e.g.:
//   HANK_DEV_API_PROXY=https://hankdemo.campbellservers.com npm run dev
// HANK_DEV_SESSION_TOKEN_FILE can point at a file holding a session token; the
// proxy then attaches it as the session cookie so the app is signed in.
const proxyTarget = process.env.HANK_DEV_API_PROXY;
const sessionTokenFile = process.env.HANK_DEV_SESSION_TOKEN_FILE;

function devSessionCookie(): string {
  if (!sessionTokenFile) return "";
  try {
    return `hank_remote_session=${readFileSync(sessionTokenFile, "utf8").trim()}`;
  } catch {
    return "";
  }
}

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../../internal/cloud/ui/react",
    emptyOutDir: true,
  },
  server: proxyTarget
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
                proxyReq.setHeader("cookie", existing ? `${existing}; ${session}` : session);
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
    : undefined,
  test: {
    environment: "jsdom",
    setupFiles: ["./src/setupTests.ts"],
  },
});
