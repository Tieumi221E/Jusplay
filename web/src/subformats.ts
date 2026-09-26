// Subtitle files and tracks as plain timed lines. Styling is not kept:
// Jusplay draws every subtitle in its own style (subs.ts); only what a
// line says, when, and whether it belongs at the top (ASS \an7–9, used
// for signs and notes) survive. ASS vector drawings are dropped.

export interface Line {
  start: number;
  end: number;
  text: string;
  top?: boolean;
}

// ---- text encoding ----

const kana = /[぀-ヿ]/g;
const cjk = /[぀-ヿ一-鿿]/g;

/**
 * Subtitle files come in whatever encoding their maker used: UTF-8 (with
 * or without a BOM), UTF-16, or a legacy one — GBK/GB18030 and Big5 for
 * Chinese, Shift-JIS for Japanese. Tried strictly in turn; a Shift-JIS
 * reading that is almost all kanji is really Chinese (Japanese lines
 * always carry kana), and GB18030 accepts nearly any bytes, so it is last
 * before Big5.
 */
export function decodeText(b: Uint8Array): string {
  if (b[0] === 0xef && b[1] === 0xbb && b[2] === 0xbf) return new TextDecoder("utf-8").decode(b.subarray(3));
  if (b[0] === 0xff && b[1] === 0xfe) return new TextDecoder("utf-16le").decode(b.subarray(2));
  if (b[0] === 0xfe && b[1] === 0xff) return new TextDecoder("utf-16be").decode(b.subarray(2));
  const strict = (enc: string): string | null => {
    try {
      return new TextDecoder(enc, { fatal: true }).decode(b);
    } catch {
      return null;
    }
  };
  const utf8 = strict("utf-8");
  if (utf8 !== null) return utf8;
  const sjis = strict("shift_jis");
  if (sjis !== null) {
    const k = sjis.match(kana)?.length ?? 0, c = sjis.match(cjk)?.length ?? 0;
    if (c === 0 || k / c > 0.1) return sjis;
  }
  return strict("gb18030") ?? strict("big5") ?? new TextDecoder("gb18030").decode(b);
}

// ---- times ----

/** "01:02:03,450", "1:02:03.45", "02:03.450" → seconds (NaN if not a time). */
function time(s: string): number {
  const m = /^(?:(\d+):)?(\d{1,2}):(\d{1,2})(?:[.,](\d{1,3}))?$/.exec(s.trim());
  if (!m) return NaN;
  const frac = m[4] ? Number(m[4]) / 10 ** m[4].length : 0;
  return Number(m[1] ?? 0) * 3600 + Number(m[2]) * 60 + Number(m[3]) + frac;
}

// ---- SRT and WebVTT ----

/** Markup in SRT/VTT text: <i>, <font …>, <c.x>, <v Name>, <00:01.000>, {\an8}. */
function plain(text: string): { text: string; top: boolean } {
  const top = /\{\\an[789]\}/.test(text);
  return { text: text.replace(/\{\\[^}]*\}/g, "").replace(/<[^>]*>/g, "").replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&nbsp;/g, " ").trim(), top };
}

/** SRT, and WebVTT (the same blocks with a header, optional ids and NOTE/STYLE blocks). */
export function parseSrt(src: string): Line[] {
  const out: Line[] = [];
  for (const block of src.replace(/\r\n?/g, "\n").split(/\n{2,}/)) {
    const rows = block.split("\n");
    const i = rows.findIndex((r) => r.includes("-->"));
    if (i < 0) continue;
    const [a, b] = rows[i].split("-->");
    const start = time(a), end = time(b.trim().split(/\s+/)[0]);
    if (!(end > start)) continue;
    const { text, top } = plain(rows.slice(i + 1).join("\n"));
    if (text) out.push(top ? { start, end, text, top } : { start, end, text });
  }
  return out.sort((x, y) => x.start - y.start);
}

// ---- ASS / SSA ----

/** An ASS event's text: override tags dropped, line breaks kept, drawings skipped. */
export function assText(raw: string): { text: string; top: boolean } | null {
  if (/\{[^}]*\\p[1-9]/.test(raw)) return null; // vector drawing
  const top = /\{[^}]*\\an[789]/.test(raw) || /\{[^}]*\\a(?:5|6|7|9|10|11)(?![0-9])/.test(raw);
  const text = raw.replace(/\{[^}]*\}/g, "").replace(/\\N/g, "\n").replace(/\\n/g, "\n").replace(/\\h/g, " ").trim();
  return text ? { text, top } : null;
}

/** A whole ASS/SSA script: the [Events] Dialogue lines. */
export function parseAss(src: string): Line[] {
  const out: Line[] = [];
  let fmt: string[] | null = null, inEvents = false;
  for (const row of src.replace(/\r\n?/g, "\n").split("\n")) {
    const r = row.trim();
    if (r.startsWith("[")) {
      inEvents = r.toLowerCase() === "[events]";
      continue;
    }
    if (!inEvents) continue;
    if (r.toLowerCase().startsWith("format:")) {
      fmt = r.slice(7).split(",").map((x) => x.trim().toLowerCase());
      continue;
    }
    if (!r.toLowerCase().startsWith("dialogue:") || !fmt) continue;
    const parts = r.slice(9).trim().split(",");
    const n = fmt.length;
    const fields = [...parts.slice(0, n - 1), parts.slice(n - 1).join(",")];
    const get = (k: string) => fields[fmt!.indexOf(k)] ?? "";
    const start = time(get("start")), end = time(get("end"));
    const t = assText(get("text"));
    if (!(end > start) || !t) continue;
    out.push(t.top ? { start, end, text: t.text, top: true } : { start, end, text: t.text });
  }
  return out.sort((x, y) => x.start - y.start);
}

/** A subtitle file's lines, by its format ("srt", "vtt", "ass"). */
export function parseFile(bytes: Uint8Array, format: string): Line[] {
  const src = decodeText(bytes);
  return format === "ass" ? parseAss(src) : parseSrt(src);
}

// ---- Matroska text tracks ----

export interface EmbeddedEvent {
  start: number;
  end: number;
  data: string;
}

/**
 * A Matroska text track's events (api/subs/embedded). In ASS/SSA tracks a
 * block holds the event without its times: "ReadOrder,Layer,Style,Name,
 * MarginL,MarginR,MarginV,Effect,Text" (SSA: "…,Marked,Style,…"); the text
 * is what follows the eighth comma. An event without a length lasts until
 * the next one (at most 10 s).
 */
export function fromEmbedded(codec: string, events: EmbeddedEvent[]): Line[] {
  const ass = codec === "S_TEXT/ASS" || codec === "S_TEXT/SSA";
  const out: Line[] = [];
  events.forEach((e, i) => {
    let t: { text: string; top: boolean } | null;
    if (ass) {
      const parts = e.data.split(",");
      t = assText(parts.slice(8).join(","));
    } else t = plain(e.data);
    if (!t || !t.text) return;
    const end = e.end > e.start ? e.end : Math.min(events[i + 1]?.start ?? e.start + 10, e.start + 10);
    out.push(t.top ? { start: e.start, end, text: t.text, top: true } : { start: e.start, end, text: t.text });
  });
  return out.sort((x, y) => x.start - y.start);
}
