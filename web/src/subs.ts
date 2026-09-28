// Subtitles: the tracks a video has and the two shown (a main one and a
// second one under it, e.g. a translation over the original).
//
// Tracks come from files beside the video ("ep.chs.ass"), a file picked by
// hand, the video's own text tracks (Matroska), and lines made from its
// sound by AI (aisubs.ts: what was said, and its translation). All are
// drawn the same way, in Jusplay's own style; lines an ASS script puts at
// the top (signs, notes) are drawn at the top. The choice is kept per video
// in its folder records; with none made, a track in the translation
// language is taken, else the first one, else — when the user asked for it
// — the AI translation.

import type { SubtitleSettings } from "./settings.ts";
import { L, prefs } from "./i18n.ts";
import { cap } from "./cap.ts";

/** A subtitle line as the window reads it (internal/subs, subs.show). */
export interface Line {
  start: number;
  end: number;
  text: string;
  top?: boolean;
}
import { AiMaker } from "./aisubs.ts";

export interface TrackOption {
  key: string;
  label: string;
  disabled?: boolean;
}

interface SubFile {
  name: string;
  format: string;
  lang: string;
  tag: string;
}

interface EmbeddedTrack {
  number: number;
  codec: string;
  language?: string;
  name?: string;
  default?: boolean;
  text: boolean;
  lang: string;
}

interface Tracks {
  files: SubFile[] | null;
  manual?: SubFile;
  embedded: EmbeddedTrack[] | null;
  pick: string;
  pick2: string;
  ai: { available: boolean; translate: boolean; reason: string };
}

/** A line stays up this long after it is said (until the next one), so it can be read to the end. */
const LINGER = 0.8;

const langName = (l: string): string =>
  ({ "zh-Hans": "简体中文", "zh-Hant": "繁體中文", ja: "日本語", en: "English", ko: "한국어" } as Record<string, string>)[l] ?? "";

/** The page's translation targets (settings) against the tracks' language tags. */
const targetLang = (t: string): string => (t === "zh" ? "zh-Hans" : t);

export class Subtitles {
  readonly ai: AiMaker;
  tracks: TrackOption[] = [{ key: "off", label: "" }];
  pick = "off";
  pick2 = "off";
  /** Called when the track list or the AI state changes (the settings rows follow). */
  onTracks: () => void = () => undefined;

  private info: Tracks | null = null;
  private loaded = new Map<string, Line[]>();
  private loading = new Map<string, Promise<void>>();
  private errors = new Map<string, string>();
  private shown = "";
  /** A track was chosen before the list arrived (the selftest): kept. */
  private chosen = false;

  constructor(
    private video: HTMLVideoElement,
    private id: string,
    duration: number,
    private s: SubtitleSettings,
    private box: HTMLElement,
    private topBox: HTMLElement,
    private signal: AbortSignal,
  ) {
    this.ai = new AiMaker(video, id, duration, s, () => ({
      on: this.s.show && (this.pick.startsWith("ai:") || this.pick2.startsWith("ai:")),
      translate: this.pick === "ai:tr" || this.pick2 === "ai:tr",
    }), signal);
    this.ai.onChange = () => {
      this.shown = "";
      this.render();
    };
    video.addEventListener("timeupdate", () => this.render(), { signal });
    video.addEventListener("seeked", () => this.render(), { signal });
  }

  /** The translation language in effect. */
  get target(): string {
    return this.s.target || (prefs.lang === "ja" ? "ja" : "zh");
  }

  async start(): Promise<void> {
    await this.refresh(true);
    void this.ai.start();
  }

  /** Read the track list again (a file was picked, AI settings changed). */
  async refresh(first = false): Promise<void> {
    try {
      const r = await fetch(`api/subs/tracks?id=${encodeURIComponent(this.id)}`, { signal: this.signal });
      this.info = (await r.json()) as Tracks;
    } catch {
      if (this.signal.aborted) return;
    }
    this.buildOptions();
    if (first && this.info && !this.chosen) {
      const known = (k: string) => this.tracks.some((t) => t.key === k && !t.disabled);
      this.pick = this.info.pick && (this.info.pick === "off" || known(this.info.pick)) ? this.info.pick : this.defaultPick();
      this.pick2 = this.info.pick2 && known(this.info.pick2) ? this.info.pick2 : "off";
      await Promise.all([this.load(this.pick), this.load(this.pick2)]);
    }
    this.update();
    this.onTracks();
  }

  private buildOptions(): void {
    const t: TrackOption[] = [{ key: "off", label: L("关", "オフ") }];
    const i = this.info;
    for (const f of i?.files ?? []) t.push({ key: `file:${f.name}`, label: `${L("外挂", "外部")} · ${f.tag || f.name}${f.lang ? `（${langName(f.lang)}）` : ""}` });
    if (i?.manual) t.push({ key: "manual", label: `${L("文件", "ファイル")} · ${i.manual.name}` });
    for (const e of i?.embedded ?? []) {
      const what = e.name || langName(e.lang) || e.language || e.codec;
      t.push(e.text
        ? { key: `mkv:${e.number}`, label: `${L("内嵌", "内蔵")} · ${what}` }
        : { key: `mkv:${e.number}`, label: `${L("内嵌", "内蔵")} · ${what}${L("（图形字幕，暂不支持）", "（画像字幕、未対応）")}`, disabled: true });
    }
    const ai = i?.ai;
    const off = !ai?.available;
    t.push({ key: "ai:tr", label: `AI · ${L("译文", "翻訳")}`, disabled: off || !ai?.translate });
    t.push({ key: "ai:src", label: `AI · ${L("原文", "原文")}`, disabled: off });
    this.tracks = t;
  }

  /** With no choice made: a track in the translation language, else the first one, else AI if wanted. */
  private defaultPick(): string {
    const want = targetLang(this.target);
    const i = this.info!;
    const inLang = [...(i.files ?? []).filter((f) => f.lang === want).map((f) => `file:${f.name}`),
      ...(i.embedded ?? []).filter((e) => e.text && e.lang === want).map((e) => `mkv:${e.number}`)];
    if (inLang.length) return inLang[0];
    if (i.files?.length) return `file:${i.files[0].name}`;
    const emb = (i.embedded ?? []).filter((e) => e.text);
    if (emb.length) return `mkv:${(emb.find((e) => e.default) ?? emb[0]).number}`;
    if (this.s.ai && i.ai.available) return i.ai.translate ? "ai:tr" : "ai:src";
    return "off";
  }

  /** Choose the track for a slot (0: main, 1: second) and remember it for this video. */
  async choose(slot: 0 | 1, key: string, file = ""): Promise<void> {
    this.chosen = true;
    if (slot === 0) this.pick = key;
    else this.pick2 = key;
    if (key !== "off") this.s.show = true;
    cap("subs.pick", { entry: this.id, pick: this.pick, pick2: this.pick2, ...(file ? { file } : {}) }).catch(() => undefined);
    await this.load(key);
    this.update();
  }

  /** A subtitle file picked by hand becomes the main track. */
  async useFile(path: string): Promise<string | null> {
    try {
      await cap("subs.pick", { entry: this.id, pick: "manual", pick2: this.pick2, file: path });
    } catch (e) {
      return (e as Error).message;
    }
    this.loaded.delete("manual");
    this.errors.delete("manual");
    await this.refresh();
    await this.choose(0, "manual", path);
    return null;
  }

  /** The error loading a track, if any. */
  errorOf(key: string): string | undefined {
    return this.errors.get(key);
  }

  private load(key: string): Promise<void> {
    if (key === "off" || key.startsWith("ai:") || this.loaded.has(key)) return Promise.resolve();
    let p = this.loading.get(key);
    if (!p) {
      p = this.fetchTrack(key).then(
        (lines) => void this.loaded.set(key, lines),
        (e) => {
          this.errors.set(key, e instanceof Error ? e.message : String(e));
          this.loaded.set(key, []);
        },
      ).finally(() => this.loading.delete(key));
      this.loading.set(key, p);
    }
    return p;
  }

  private async fetchTrack(key: string): Promise<Line[]> {
    // Read by the window, the same way the command line's subs show reads it.
    const r = await cap<{ lines: Line[] }>("subs.show", { entry: this.id, track: key }, this.signal);
    return r.lines;
  }

  /** Settings or the choice changed. */
  update(): void {
    this.box.style.setProperty("--sub-scale", String(this.s.scale));
    this.topBox.style.setProperty("--sub-scale", String(this.s.scale));
    this.ai.update();
    this.shown = "";
    this.render();
  }

  /** The lines of a track at time t. */
  private linesAt(key: string, t: number): Line[] {
    if (key === "off") return [];
    if (key.startsWith("ai:")) {
      const c = cueAt(this.ai.cues, t);
      if (!c) return [];
      if (key === "ai:src") return [{ start: c.start, end: c.end, text: c.text }];
      // Not translated yet (the translator is loading): what was said, until it is.
      const tr = c.tr?.[this.target] ?? c.text;
      return tr ? [{ start: c.start, end: c.end, text: tr }] : [];
    }
    return activeAt(this.loaded.get(key) ?? [], t);
  }

  render(): void {
    const t = this.video.currentTime;
    const on = this.s.show;
    const main = on ? this.linesAt(this.pick, t) : [];
    const second = on && this.pick2 !== this.pick ? this.linesAt(this.pick2, t) : [];
    const text = (ls: Line[]) => ls.map((l) => l.text).join("\n");
    const top = text([...main, ...second].filter((l) => l.top));
    const a = text(main.filter((l) => !l.top)), b = text(second.filter((l) => !l.top));
    const key = `${a}\u0000${b}\u0000${top}`;
    if (key === this.shown) return;
    this.shown = key;
    const [ea, eb] = this.box.children as unknown as HTMLElement[];
    ea.textContent = a;
    eb.textContent = b && b !== a ? b : "";
    this.box.classList.toggle("empty", !a && !b);
    this.topBox.textContent = top;
    this.topBox.classList.toggle("empty", !top);
  }
}

/** The AI line at t (lines do not overlap), shown a little past its end. */
function cueAt<T extends { start: number; end: number }>(cues: T[], t: number): T | null {
  let lo = 0, hi = cues.length - 1, found: T | null = null;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (cues[mid].start <= t) {
      found = cues[mid];
      lo = mid + 1;
    } else hi = mid - 1;
  }
  if (!found) return null;
  const next = cues[lo]?.start ?? Infinity;
  return t < Math.max(found.end, Math.min(found.end + LINGER, next)) ? found : null;
}

/** Every line of a file's track showing at t (dialogue and a sign may overlap). */
export function activeAt(lines: Line[], t: number): Line[] {
  let lo = 0, hi = lines.length - 1, last = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (lines[mid].start <= t) {
      last = mid;
      lo = mid + 1;
    } else hi = mid - 1;
  }
  const out: Line[] = [];
  // Lines longer than a minute are rare; look back that far.
  for (let i = last; i >= 0 && t - lines[i].start < 60; i--) if (t < lines[i].end) out.push(lines[i]);
  return out.reverse();
}
