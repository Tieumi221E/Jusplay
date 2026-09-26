// Playback-time filtering. It never changes the archive: it builds a new
// threads array for the renderer and counts what each rule removed, so the
// UI can show exactly what is hidden.

import type { FilterSettings, CommentSettings, Fork } from "./settings.ts";

export interface V1Comment {
  id: string;
  no: number;
  vposMs: number;
  body: string;
  commands: string[];
  userId: string;
  isPremium: boolean;
  score: number;
  postedAt: string;
  nicoruCount: number;
  nicoruId: string | null;
  source: string;
  isMyPost: boolean;
  [extra: string]: unknown;
}

export interface V1Thread {
  id: unknown;
  fork: string;
  commentCount: number;
  comments: V1Comment[];
  [extra: string]: unknown;
}

export type Rule =
  | "fork" | "script" | "time" | "ngShare" | "user" | "word" | "command"
  | "naka" | "ue" | "shita" | "colored" | "big" | "small" | "ca" | "anonymous" | "length" | "cap";

export interface FilterStats {
  total: number;
  shown: number;
  byRule: Partial<Record<Rule, number>>;
  byFork: Record<string, { total: number; shown: number }>;
  /** NG word entries that are not valid regular expressions. */
  badPatterns: string[];
  /** The total budget in effect (0 = none) and how many were eligible for it. */
  cap: { limit: number; eligible: number };
}

/**
 * Comment art, recognised as the renderer does (niconicomments
 * changeCALayer/getUsersScore): an account scores 5 for each comment with
 * ca, patissier, ender or full, and half the line count for each comment of
 * more than two lines; at 10 or more all its comments are art. A comment
 * with one of those commands, or of more than two lines, is art by itself.
 * Artists seldom use a "ca" command: a picture is usually many single-line
 * comments at one moment, from one account.
 */
const CA_COMMANDS = new Set(["ca", "patissier", "ender", "full"]);
const lineBreaks = (body: string) => (body.match(/\r\n|\n|\r/g) ?? []).length;

export function artRecogniser(threads: V1Thread[]): (fork: string, cm: V1Comment) => boolean {
  const score = new Map<string, number>();
  for (const t of threads) {
    for (const cm of t.comments) {
      let add = 0;
      if (cm.commands.some((x) => CA_COMMANDS.has(x))) add += 5;
      const lines = lineBreaks(cm.body);
      if (lines > 2) add += lines / 2;
      if (add) score.set(cm.userId, (score.get(cm.userId) ?? 0) + add);
    }
  }
  return (fork, cm) => fork !== "owner" && ((score.get(cm.userId) ?? 0) >= 10 || cm.commands.some((x) => CA_COMMANDS.has(x.toLowerCase())) || lineBreaks(cm.body) > 2);
}

/**
 * Comments kept under the cap. Owner comments and comment art (above) are
 * exempt: dropping lines of an art would break it. Each other comment is
 * kept when a hash of its identity falls under limit/eligible, so the
 * choice is the same on every rebuild and density keeps its shape.
 */
export function capLimit(durationSec: number, perSecond: number): number {
  return perSecond > 0 ? Math.max(100, Math.floor(durationSec * perSecond)) : 0;
}

/** Uniform in [0, 1): FNV-1a, then murmur3's fmix32 so that ids differing
 * only in their last characters do not land next to each other. */
function fnv1a(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  h ^= h >>> 16;
  h = Math.imul(h, 0x85ebca6b) >>> 0;
  h ^= h >>> 13;
  h = Math.imul(h, 0xc2b2ae35) >>> 0;
  h = (h ^ (h >>> 16)) >>> 0; // ^ yields a signed int32; keep it unsigned
  return h / 0x100000000;
}

// Niconico's shared-NG levels hide comments whose score is at or below the
// threshold. Values from public documentation; not yet checked against the
// live site.
export const NG_SHARE_THRESHOLD = { weak: -10000, medium: -4800, strong: -1000 } as const;

const COLORS = new Set([
  "white", "red", "pink", "orange", "yellow", "green", "cyan", "blue", "purple", "black",
  "white2", "niconicowhite", "red2", "truered", "pink2", "orange2", "passionorange", "yellow2", "madyellow",
  "green2", "elementalgreen", "cyan2", "blue2", "marinblue", "purple2", "nobleviolet", "black2",
]);

function isColored(cmd: string): boolean {
  return (COLORS.has(cmd) && cmd !== "white") || /^#[0-9a-f]{3,8}$/i.test(cmd);
}

type Matcher = (body: string) => boolean;

function compileWords(words: string[], bad: string[]): Matcher[] {
  const out: Matcher[] = [];
  for (const w of words) {
    if (!w) continue;
    const m = /^\/(.+)\/([a-z]*)$/.exec(w);
    if (m) {
      try {
        const re = new RegExp(m[1], m[2].replace(/[gy]/g, ""));
        out.push((b) => re.test(b));
      } catch {
        bad.push(w);
      }
    } else out.push((b) => b.includes(w));
  }
  return out;
}

export function isScript(c: V1Comment): boolean {
  return c.body.startsWith("@") || c.body.startsWith("＠");
}

export function applyFilters(threads: V1Thread[], f: FilterSettings, c: Pick<CommentSettings, "forks" | "runScripts">, durationSec = 0): { threads: V1Thread[]; stats: FilterStats } {
  const stats: FilterStats = { total: 0, shown: 0, byRule: {}, byFork: {}, badPatterns: [], cap: { limit: 0, eligible: 0 } };
  const words = compileWords(f.ngWords, stats.badPatterns);
  const users = new Set(f.ngUsers.filter(Boolean));
  const ngCmds = new Set(f.ngCommands.map((x) => x.trim().toLowerCase()).filter(Boolean));
  const cutoff = f.postedBefore ? Date.parse(f.postedBefore) : NaN;
  const shareMax = f.ngShare === "none" ? -Infinity : NG_SHARE_THRESHOLD[f.ngShare];
  const isArt = artRecogniser(threads);

  const rule = (fork: string, cm: V1Comment): Rule | null => {
    if (fork in c.forks && !c.forks[fork as Fork]) return "fork";
    const owner = fork === "owner";
    if (owner) return isScript(cm) && !c.runScripts ? "script" : null;
    // Viewer filters never touch owner comments, as on the site.
    if (Number.isFinite(cutoff) && Date.parse(cm.postedAt) > cutoff) return "time";
    if (cm.score <= shareMax) return "ngShare";
    if (users.has(cm.userId)) return "user";
    if (words.some((m) => m(cm.body))) return "word";
    const cmds = cm.commands.map((x) => x.toLowerCase());
    if (cmds.some((x) => ngCmds.has(x))) return "command";
    const h = f.hide;
    const ue = cmds.includes("ue"), shita = cmds.includes("shita");
    if (h.ue && ue) return "ue";
    if (h.shita && shita) return "shita";
    if (h.naka && !ue && !shita) return "naka";
    if (h.colored && cmds.some(isColored)) return "colored";
    if (h.big && cmds.includes("big")) return "big";
    if (h.small && cmds.includes("small")) return "small";
    const ca = isArt(fork, cm);
    if (h.ca && ca) return "ca";
    if (h.anonymous && cmds.includes("184")) return "anonymous";
    if (f.maxLength > 0 && !ca && [...cm.body].length > f.maxLength) return "length";
    return null;
  };

  // Pass 1: the rules above.
  const passed: { t: V1Thread; kept: V1Comment[] }[] = [];
  const exempt = (fork: string, cm: V1Comment) => fork === "owner" || isArt(fork, cm);
  let eligible = 0;
  for (const t of threads) {
    const fs = (stats.byFork[t.fork] ??= { total: 0, shown: 0 });
    const kept: V1Comment[] = [];
    for (const cm of t.comments) {
      stats.total++;
      fs.total++;
      const r = rule(t.fork, cm);
      if (r) stats.byRule[r] = (stats.byRule[r] ?? 0) + 1;
      else {
        kept.push(cm);
        if (!exempt(t.fork, cm)) eligible++;
      }
    }
    passed.push({ t, kept });
  }
  // Pass 2: the total budget, spent only on comments that survived.
  const limit = capLimit(durationSec, f.capPerSecond);
  const exemptCount = passed.reduce((n, p) => n + p.kept.filter((cm) => exempt(p.t.fork, cm)).length, 0);
  const room = Math.max(0, limit - exemptCount);
  const p = limit > 0 && eligible > room ? room / eligible : 1;
  stats.cap = { limit, eligible };
  const out: V1Thread[] = [];
  for (const { t, kept } of passed) {
    const fs = stats.byFork[t.fork];
    const final = p >= 1 ? kept : kept.filter((cm) => {
      if (exempt(t.fork, cm) || fnv1a(`${String(t.id)}/${t.fork}/${cm.id || cm.no}`) < p) return true;
      stats.byRule.cap = (stats.byRule.cap ?? 0) + 1;
      return false;
    });
    stats.shown += final.length;
    fs.shown += final.length;
    if (final.length) out.push({ ...t, comments: final });
  }
  return { threads: out, stats };
}
