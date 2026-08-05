import { readdir, stat } from "node:fs/promises";
import path from "node:path";

const MAX_CHUNK_BYTES = 500_000;
const assetsDirectory = path.resolve(process.argv[2] || "../../internal/cloud/ui/react/assets");
const chunkNames = (await readdir(assetsDirectory)).filter((name) => name.endsWith(".js"));
const chunks = await Promise.all(chunkNames.map(async (name) => ({
  name,
  size: (await stat(path.join(assetsDirectory, name))).size,
})));
const oversized = chunks.filter((chunk) => chunk.size > MAX_CHUNK_BYTES);

if (oversized.length) {
  for (const chunk of oversized) {
    console.error(`${chunk.name}: ${chunk.size} bytes exceeds the ${MAX_CHUNK_BYTES}-byte bundle budget.`);
  }
  process.exitCode = 1;
} else {
  const largest = chunks.sort((left, right) => right.size - left.size)[0];
  console.log(`Bundle budget passed: ${largest?.name || "no JavaScript chunks"}${largest ? ` is ${largest.size} bytes` : ""}.`);
}
