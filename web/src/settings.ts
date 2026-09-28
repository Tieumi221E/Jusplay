// Settings document stored by the Go side (api/settings). Missing or
// malformed fields fall back to defaults field by field, so older files
// keep working when settings are added.

export type Fork = "owner" | "main" | "easy";
export type NgShareLevel = "none" | "weak" | "medium" | "strong";

export interface CommentSettings {
  enabled: boolean;
  forks: Record<Fork, boolean>;
  /** Owner NicoScript (@デフォルト, @逆, …) is executed. */
  runScripts: boolean;
  /** Canvas opacity, 0–1. */
  opacity: number;
  /** niconicomments `scale`: per-comment text size factor. */
  scale: number;
  mode: "default" | "html5" | "flash";
  keepCA: boolean;
  /** niconicomments `commentLimit`; 0 means unlimited. */
  limit: number;
  /** Which comments `limit` drops: "asc" hides older first. */
  limitOrder: "asc" | "desc";
  strokeColor: string;
  strokeOpacity: number;
  /** Multiplier on niconicomments' default outline width. */
  strokeWidth: number;
  /** Percentage of the picture height (from the top) comments may cover. */
  area: number;
  /** "display": draw every display refresh with interpolated time; "video": once per video frame. */
  frameRate: "display" | "60" | "video";
  /** Translate comments into this language ("off", "zh", "ja"). */
  translate: string;
  /** While translating, hide the comments not translated yet. */
  hideUntranslated: boolean;
}

export interface FilterSettings {
  /** Substrings, or /regex/flags. */
  ngWords: string[];
  ngUsers: string[];
  /** Hide comments carrying any of these commands, e.g. "big", "red", "#ff0000". */
  ngCommands: string[];
  hide: {
    naka: boolean;
    ue: boolean;
    shita: boolean;
    colored: boolean;
    big: boolean;
    small: boolean;
    ca: boolean;
    anonymous: boolean;
  };
  ngShare: NgShareLevel;
  /** Hide non-CA comments longer than this many characters; 0 = off. */
  maxLength: number;
  /** Only comments posted at or before this ISO date(time); "" = off. */
  postedBefore: string;
  /**
   * Total budget in comments per second of video (at least 100 overall),
   * as danmk.py does; 0 = no cap. Applied after the other filters.
   */
  capPerSecond: number;
}

/** Subtitles (subs.ts). Which tracks a video shows is kept with the video. */
export interface SubtitleSettings {
  /** Subtitles shown at all (T). */
  show: boolean;
  /** With no subtitles of its own, a video gets the AI translation. */
  ai: boolean;
  /** Translation language: "zh", "zh-Hant", "ja", "en", "ko"; "" follows the interface language. */
  target: string;
  /** Text size factor. */
  scale: number;
  /** Language spoken in the video (the recogniser's name, e.g. "Japanese"); "" decides from the first lines. */
  source: string;
}

export const SUB_TARGETS = ["zh", "zh-Hant", "ja", "en", "ko"];
export const SUB_SOURCES = ["Japanese", "Chinese", "English", "Korean", "Cantonese", "French", "German", "Spanish", "Russian"];

export interface Settings {
  /** 2: keepCA on by default (see merge). */
  v: 2;
  playback: { volume: number; muted: boolean; rate: number };
  comments: CommentSettings;
  subtitles: SubtitleSettings;
  /** Comment analysis (danmaku): hotspot marks on the seek bar, and the report. */
  analysis: { enabled: boolean };
  filters: FilterSettings;
  /** Per-video values keyed by Session.key. */
  videos: Record<string, { offsetMs: number }>;
  /** 记一笔 (note.add): the Jusnote notebook folder and the note in it. */
  notes: { notebook: string; file: string };
}

export const defaults = (): Settings => ({
  v: 2,
  playback: { volume: 1, muted: false, rate: 1 },
  comments: {
    enabled: true,
    forks: { owner: true, main: true, easy: true },
    runScripts: true,
    opacity: 1,
    scale: 1,
    mode: "default",
    // On: comment art keeps its own collision layer and ignores the font
    // size setting, so a picture is not pushed apart or rescaled.
    keepCA: true,
    limit: 0,
    limitOrder: "asc",
    strokeColor: "#000000",
    strokeOpacity: 0.4,
    strokeWidth: 1,
    area: 100,
    frameRate: "display",
    translate: "off",
    hideUntranslated: true,
  },
  subtitles: { show: true, ai: false, target: "", scale: 1, source: "" },
  analysis: { enabled: false },
  filters: {
    ngWords: [],
    ngUsers: [],
    ngCommands: [],
    hide: { naka: false, ue: false, shita: false, colored: false, big: false, small: false, ca: false, anonymous: false },
    ngShare: "none",
    maxLength: 0,
    postedBefore: "",
    capPerSecond: 2,
  },
  videos: {},
  notes: { notebook: "", file: "Jusplay.md" },
});

type Json = unknown;

function isObj(v: Json): v is Record<string, Json> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Overlay stored values onto defaults, keeping only values of the right type and range. */
export function merge(stored: Json): Settings {
  const out = defaults();
  const walk = (def: Record<string, Json>, src: Json, path: string) => {
    if (!isObj(src)) return;
    for (const k of Object.keys(def)) {
      const d = def[k], s = src[k];
      if (s === undefined || path + k === "videos") continue; // validated entry by entry below
      if (isObj(d)) walk(d, s, path + k + ".");
      else if (Array.isArray(d)) {
        if (Array.isArray(s)) def[k] = s.filter((x) => typeof x === "string");
      } else if (typeof d === typeof s && (typeof s !== "number" || Number.isFinite(s))) def[k] = s;
    }
  };
  walk(out as unknown as Record<string, Json>, stored, "");
  // Version 1 stored keepCA: false only because that was the default then;
  // the default is now on, so a version 1 document takes it once.
  const storedV = isObj(stored) && typeof stored.v === "number" ? stored.v : 1;
  if (storedV < 2) out.comments.keepCA = true;
  out.v = 2;
  if (isObj(stored) && isObj(stored.videos)) {
    for (const [key, v] of Object.entries(stored.videos)) {
      if (isObj(v) && typeof v.offsetMs === "number" && Number.isFinite(v.offsetMs)) out.videos[key] = { offsetMs: v.offsetMs };
    }
  }
  const c = out.comments;
  c.opacity = clamp(c.opacity, 0, 1);
  c.scale = clamp(c.scale, 0.3, 3);
  c.limit = Math.round(clamp(c.limit, 0, 1000));
  c.strokeOpacity = clamp(c.strokeOpacity, 0, 1);
  c.strokeWidth = clamp(c.strokeWidth, 0, 4);
  c.area = clamp(c.area, 10, 100);
  if (!["default", "html5", "flash"].includes(c.mode)) c.mode = "default";
  if (!["asc", "desc"].includes(c.limitOrder)) c.limitOrder = "asc";
  if (!["display", "60", "video"].includes(c.frameRate)) c.frameRate = "display";
  if (!["off", "zh", "ja"].includes(c.translate)) c.translate = "off";
  if (!/^#[0-9a-fA-F]{6}$/.test(c.strokeColor)) c.strokeColor = "#000000";
  const f = out.filters;
  if (!["none", "weak", "medium", "strong"].includes(f.ngShare)) f.ngShare = "none";
  f.maxLength = Math.round(clamp(f.maxLength, 0, 10000));
  f.capPerSecond = clamp(f.capPerSecond, 0, 50);
  const sub = out.subtitles;
  if (sub.target !== "" && !SUB_TARGETS.includes(sub.target)) sub.target = "";
  sub.scale = clamp(sub.scale, 0.5, 2);
  if (sub.source !== "" && !SUB_SOURCES.includes(sub.source)) sub.source = "";
  const p = out.playback;
  p.volume = clamp(p.volume, 0, 1);
  p.rate = clamp(p.rate, 0.25, 4);
  return out;
}

export function clamp(x: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, x));
}
