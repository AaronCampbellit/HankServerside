/// <reference types="vite/client" />

declare module "node:fs" {
  export function readFileSync(path: string, encoding: string): string;
}
