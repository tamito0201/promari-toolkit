// Builds the page into ../internal/interfaces/dashboard/assets, which the Go
// binary embeds. The output is committed: the release builds the binary with Go
// alone, without Node. `pnpm build` then `git diff --exit-code` checks it is current.
import { build } from "esbuild";
import { createHash } from "node:crypto";
import { copyFile, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";

const out = new URL("../internal/interfaces/dashboard/assets/", import.meta.url);
await rm(out, { recursive: true, force: true });
await mkdir(out, { recursive: true });
await build({
  entryPoints: { app: "src/main.ts" },
  bundle: true,
  minify: true,
  format: "esm",
  target: "es2022",
  outdir: out.pathname,
  legalComments: "none",
  logLevel: "warning",
});
await copyFile(new URL("src/index.html", import.meta.url), new URL("index.html", out));

// sourceHash must match sourceHash in internal/interfaces/dashboard/assets_test.go:
// SHA-256 over each input, in sorted order, as "<name>\0<content>\0".
const root = new URL("./", import.meta.url);
const src = (await readdir(new URL("src/", root))).filter((f) => !f.endsWith(".test.ts")).map((f) => `src/${f}`);
const inputs = [...src, "build.mjs", "package.json", "pnpm-lock.yaml"].sort();
const hash = createHash("sha256");
for (const name of inputs) {
  hash.update(`${name}\0`);
  hash.update(await readFile(new URL(name, root)));
  hash.update("\0");
}
await writeFile(new URL("source.sha256", out), `${hash.digest("hex")}\n`);
