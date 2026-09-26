// Bundles the page into internal/ui/dist, which the Go binary embeds.
import { build } from "esbuild";
import { copyFileSync, cpSync, mkdirSync, readdirSync, rmSync } from "node:fs";

const out = new URL("../internal/ui/dist/", import.meta.url);
mkdirSync(out, { recursive: true });
for (const f of readdirSync(out)) if (f !== ".gitkeep") rmSync(new URL(f, out), { recursive: true });
const common = { bundle: true, format: "esm", target: "chrome120", minify: true, sourcemap: false, legalComments: "linked" };
const file = (name) => new URL(name, out).pathname.replace(/^\/([A-Za-z]:)/, "$1");
await build({ ...common, entryPoints: ["src/main.ts"], outfile: file("app.js") });
await build({ ...common, entryPoints: ["src/library.ts"], outfile: file("library.js") });
for (const f of ["index.html", "tokens.css", "app.css", "library.html", "library.css", "icon-64.png", "icon-64-dark.png"]) copyFileSync(new URL(`src/${f}`, import.meta.url), new URL(f, out));
// The bundled font, WOFF2 (assets/make_fonts.py); the server sends it as it is.
cpSync(new URL("src/fonts/", import.meta.url), new URL("fonts/", out), { recursive: true });
