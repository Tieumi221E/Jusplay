// The comment font: Jus Sans, the one font of the app (fonts/LICENSE.txt, a
// subset of Source Han Sans 2.005), in every comment style, with Jus Emoji
// (Noto Color Emoji) for the emoji it has none of. The site draws with the
// Windows fonts; they are not open, so they are not used, and comment art
// can differ from it by the fonts' widths (docs/architecture.md, comment
// fonts). The comment canvas is Japanese (lang), so Han characters take
// their Japanese forms whatever the interface language.

import NiconiComments from "./nico/niconicomments.js";
import type { V1Thread } from "./filter.ts";

const FONT = `"Jus Sans", "Jus Emoji"`;

interface FontItem { font: string; offset: number; weight: number }
interface Fonts { html5: Record<"defont" | "gothic" | "mincho", FontItem>; flash: Record<"gulim" | "simsun", string> }

// The win8_1 profile's per-style offsets and weights, with the font replaced.
const table = (NiconiComments as unknown as { internal: { definition: { fonts: { fonts: Record<string, Fonts["html5"]> } } } })
  .internal.definition.fonts.fonts.win8_1;

/** niconicomments `config.fonts`. */
export function commentFonts(): Fonts {
  const item = (x: FontItem): FontItem => ({ ...x, font: FONT });
  return {
    html5: { defont: item(table.defont), gothic: item(table.gothic), mincho: item(table.mincho) },
    flash: { gulim: `normal 600 [size]px ${FONT}`, simsun: `normal 400 [size]px ${FONT}` },
  };
}

// Characters drawn from Jus Emoji: pictographs, the emoji presentation
// selector, keycaps and joiners; the regional indicators are its second file.
const EMOJI = /[\u{1F000}-\u{1FAFF}\u{E0020}-\u{E007F}️⃣‍]/u;
const FLAG = /[\u{1F1E6}-\u{1F1FF}]/u;

/**
 * Canvas text does not wait for a web font: a comment measured before its
 * font loads is laid out in another. So the faces a layout will use are
 * loaded first: both weights of Jus Sans (defont is 600, the others 400),
 * and each emoji file only when a comment has one of its characters.
 */
let sans: Promise<unknown> | null = null, emoji: Promise<unknown> | null = null, flags: Promise<unknown> | null = null;
export function ensureCommentFont(threads: V1Thread[]): Promise<void> {
  sans ??= Promise.all([document.fonts.load(`400 32px "Jus Sans"`, "あ"), document.fonts.load(`600 32px "Jus Sans"`, "あ")]);
  const wait: Promise<unknown>[] = [sans];
  let e = false, f = false;
  for (const t of threads) {
    for (const c of t.comments) {
      const b = (c as { body?: unknown }).body;
      if (typeof b !== "string") continue;
      if (!e && EMOJI.test(b)) e = true;
      if (!f && FLAG.test(b)) f = true;
      if (e && f) break;
    }
  }
  if (e) wait.push((emoji ??= document.fonts.load(`32px "Jus Emoji"`, "\u{1F600}")));
  if (f) wait.push((flags ??= document.fonts.load(`32px "Jus Emoji"`, "\u{1F1EF}\u{1F1F5}")));
  return Promise.race([Promise.all(wait).then(() => undefined), new Promise<void>((r) => setTimeout(r, 3000))]);
}
