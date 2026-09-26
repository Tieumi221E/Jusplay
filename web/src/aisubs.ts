// Subtitles made from the video's own sound (internal/ai, internal/subs).
// The server has no audio decoder, so the page decodes: it fetches a
// stretch of the audio as fragmented MP4 (api/subs/audio), lets the engine
// decode it resampled to 16 kHz (OfflineAudioContext), mixes it to mono and
// posts the samples (api/subs/chunk). The server finds the lines in it,
// writes them down and translates them, and keeps them in the video's
// folder records.
//
// While an AI track is shown, the stretch just ahead of the playhead is
// made, so a line is ready before it is said; "make the whole episode" goes
// on to the end. Stretches already made are never sent again (the track's
// covered ranges), so a second viewing, or a seek back, reads them.

import type { SubtitleSettings } from "./settings.ts";
import { prefs } from "./i18n.ts";

export interface SubCue {
  start: number;
  end: number;
  lang: string;
  text: string;
  tr?: Record<string, string>;
}

interface Status {
  available: boolean;
  reason: string;
  error?: string;
  models?: { asr: string; mt: string };
  track: { covered: [number, number][] | null; cues: SubCue[] | null };
}

/** Seconds made ahead of the playhead while watching. */
const AHEAD = 60;
/** Seconds of audio per request: long enough for whole lines, short enough to show the first soon. */
const CHUNK = 15;
/** The first request where nothing is made yet: shorter, so the first line comes sooner. */
const FIRST_CHUNK = 6;

/** The recogniser's language name matches a target code (internal/ai SameLanguage). */
export function sameLanguage(l: string, target: string): boolean {
  return ({ zh: ["Chinese", "Cantonese"], "zh-Hant": ["Chinese", "Cantonese"], ja: ["Japanese"], en: ["English"], ko: ["Korean"] } as Record<string, string[]>)[target]?.includes(l) ?? false;
}

export type SubsState =
  | { kind: "unavailable"; reason: string }
  | { kind: "idle" | "working" | "done"; covered: number; duration: number }
  | { kind: "error"; message: string };

export class AiMaker {
  cues: SubCue[] = [];
  covered: [number, number][] = [];
  available = false;
  reason = "";
  models: { asr: string; mt: string } | null = null;
  state: SubsState = { kind: "unavailable", reason: "" };
  /** Per request timings, for the selftest (-subs). */
  timings: { start: number; seconds: number; fetchMs: number; decodeMs: number; serverMs: number; lines: number }[] = [];
  /** Called when new lines are made. */
  onChange: () => void = () => undefined;
  /** Called when the state (progress, errors) changes. */
  onState: () => void = () => undefined;

  private whole = false;
  private warmed = false;
  private req: AbortController | null = null;
  private wakeUp: (() => void) | null = null;
  private failures = 0;

  constructor(
    private video: HTMLVideoElement,
    private id: string,
    private duration: number,
    private s: SubtitleSettings,
    /** Whether an AI track is shown, and whether its translation is. */
    private want: () => { on: boolean; translate: boolean },
    private signal: AbortSignal,
  ) {
    const kick = () => {
      this.req?.abort(); // the stretch for the old place is no longer wanted
      this.wake();
    };
    video.addEventListener("seeking", kick, { signal });
    signal.addEventListener("abort", () => {
      this.req?.abort();
      this.wake();
    });
  }

  /** The translation language in effect. */
  get target(): string {
    return this.s.target || (prefs.lang === "ja" ? "ja" : "zh");
  }

  async start(): Promise<void> {
    await this.fetchStatus();
    this.setState();
    this.update();
    void this.loop();
  }

  private async fetchStatus(): Promise<void> {
    try {
      const r = await fetch(`api/subs?id=${encodeURIComponent(this.id)}`, { signal: this.signal });
      const st = (await r.json()) as Status;
      this.available = st.available;
      this.reason = st.reason;
      this.models = st.models ?? null;
      this.cues = st.track.cues ?? [];
      this.covered = st.track.covered ?? [];
      if (st.error) this.fail(st.error);
    } catch (e) {
      if (this.signal.aborted) return;
      this.available = false;
      this.reason = String(e);
    }
  }

  /** What is shown, or the settings, changed. */
  update(): void {
    if ((this.want().on || this.whole) && this.available && !this.warmed) {
      this.warmed = true;
      fetch(`api/subs/warm?id=${encodeURIComponent(this.id)}`, { method: "POST" }).catch(() => undefined);
    }
    this.wake();
  }

  /** The backends changed (AI settings): read the status again. */
  async reload(): Promise<void> {
    this.warmed = false;
    this.failures = 0;
    if (this.state.kind === "error") this.state = { kind: "unavailable", reason: "" };
    await this.fetchStatus();
    this.setState();
    this.update();
  }

  /** Make the whole episode, not only what is just ahead. */
  makeAll(): void {
    this.whole = true;
    this.failures = 0;
    this.wake();
  }

  get makingAll(): boolean {
    return this.whole;
  }

  // ---- making ----

  private wake(): void {
    const w = this.wakeUp;
    this.wakeUp = null;
    w?.();
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((r) => {
      const t = setTimeout(() => {
        this.wakeUp = null;
        r();
      }, ms);
      this.wakeUp = () => {
        clearTimeout(t);
        r();
      };
    });
  }

  private coveredSeconds(): number {
    return this.covered.reduce((n, [a, b]) => n + Math.min(b, this.duration) - Math.max(0, a), 0);
  }

  private setState(kind?: "idle" | "working"): void {
    if (!this.available) this.state = { kind: "unavailable", reason: this.reason };
    else if (this.state.kind === "error" && !kind) return;
    else {
      const covered = this.coveredSeconds();
      this.state = { kind: covered >= this.duration - 1 ? "done" : kind ?? "idle", covered, duration: this.duration };
    }
    this.onState();
  }

  private fail(message: string): void {
    this.state = { kind: "error", message };
    this.onState();
  }

  /** The first moment from t not yet made, or null when all is made up to the horizon. */
  private gap(t: number, horizon: number): number | null {
    let p = Math.max(0, t);
    for (const [a, b] of this.covered) if (a <= p + 0.05 && b > p) p = b;
    return p < horizon - 0.05 ? p : null;
  }

  private cover(a: number, b: number): void {
    const all = [...this.covered, [a, b] as [number, number]].sort((x, y) => x[0] - y[0]);
    const out: [number, number][] = [];
    for (const r of all) {
      const last = out[out.length - 1];
      if (last && r[0] <= last[1] + 0.05) last[1] = Math.max(last[1], r[1]);
      else out.push([r[0], r[1]]);
    }
    this.covered = out;
  }

  private merge(cues: SubCue[]): void {
    for (const c of cues) {
      const i = this.cues.findIndex((x) => Math.abs(x.start - c.start) <= 0.3);
      if (i >= 0) this.cues[i] = c;
      else this.cues.push(c);
    }
    this.cues.sort((a, b) => a.start - b.start);
    this.onChange();
  }

  private async loop(): Promise<void> {
    while (!this.signal.aborted) {
      const want = this.want();
      if (!this.available || (!want.on && !this.whole) || this.failures >= 3) {
        await this.sleep(60_000);
        continue;
      }
      const t = Math.max(0, this.video.currentTime - 1);
      const horizon = this.whole ? this.duration : Math.min(this.duration, t + AHEAD);
      const needTr = want.translate;
      const target = this.target;
      const req = (this.req = new AbortController());
      const stop = () => req.abort();
      this.signal.addEventListener("abort", stop);
      try {
        // Lines made before the target changed get translated first.
        const untranslated = needTr && this.cues.some((c) => c.start >= t && c.start < t + AHEAD && !c.tr?.[target] && !sameLanguage(c.lang, target));
        const p = this.gap(t, horizon);
        const q = p === null && this.whole ? this.gap(0, this.duration) : p;
        if (untranslated) {
          this.setState("working");
          await this.translate(t, t + AHEAD, target, req.signal);
        } else if (q !== null) {
          this.setState("working");
          const fresh = !this.covered.some(([a, b]) => a <= q + 0.05 && b >= q - 0.05);
          await this.chunk(q, Math.min(this.duration, q + (fresh ? FIRST_CHUNK : CHUNK)), needTr ? target : "", req.signal);
        } else {
          if (this.whole && this.coveredSeconds() >= this.duration - 1) this.whole = false;
          this.setState();
          await this.sleep(1000);
        }
        this.failures = 0;
      } catch (e) {
        if (!req.signal.aborted && !this.signal.aborted) {
          this.failures++;
          this.fail(e instanceof Error ? e.message : String(e));
          await this.sleep(2000 * this.failures);
        }
      } finally {
        this.signal.removeEventListener("abort", stop);
      }
    }
  }

  private async chunk(t0: number, t1: number, target: string, signal: AbortSignal): Promise<void> {
    const id = encodeURIComponent(this.id);
    const w0 = performance.now();
    const r = await fetch(`api/subs/audio?id=${id}&t0=${t0}&t1=${t1}`, { signal });
    if (!r.ok) throw new Error(`audio: HTTP ${r.status} ${await r.text()}`);
    const start = Number(r.headers.get("X-Jusplay-Start") ?? t0);
    const mp4 = await r.arrayBuffer();
    const w1 = performance.now();
    const pcm = await decode(mp4);
    const w2 = performance.now();
    if (signal.aborted) return;
    const last = t1 >= this.duration - 0.05;
    const res = await fetch(`api/subs/chunk?id=${id}&start=${start}&target=${target}&lang=${this.s.source}&last=${last ? 1 : 0}`, { method: "POST", body: pcm.buffer as ArrayBuffer, signal });
    if (!res.ok) throw new Error(`${res.status === 503 ? "" : `HTTP ${res.status} `}${await res.text()}`);
    const out = (await res.json()) as { next: number; cues: SubCue[] | null };
    // Always forward: a chunk that is one unbroken line still moves on.
    this.cover(start, Math.max(out.next, Math.min(t1, t0 + 1)));
    this.merge(out.cues ?? []);
    this.timings.push({ start: +start.toFixed(2), seconds: +(pcm.length / 16000).toFixed(2), fetchMs: Math.round(w1 - w0), decodeMs: Math.round(w2 - w1), serverMs: Math.round(performance.now() - w2), lines: out.cues?.length ?? 0 });
  }

  private async translate(from: number, to: number, target: string, signal: AbortSignal): Promise<void> {
    const r = await fetch(`api/subs/translate?id=${encodeURIComponent(this.id)}&from=${from}&to=${to}&target=${target}`, { method: "POST", signal });
    if (!r.ok) throw new Error(`HTTP ${r.status} ${await r.text()}`);
    const out = (await r.json()) as { cues: SubCue[] | null };
    // Mark what could not be translated as done too, so the loop moves on.
    for (const c of this.cues) if (c.start >= from && c.start < to && !c.tr?.[target] && !sameLanguage(c.lang, target)) (c.tr ??= {})[target] = "";
    this.merge(out.cues ?? []);
  }
}

/** Decode a stretch of audio (fragmented MP4) to 16 kHz mono, 16-bit little-endian. */
async function decode(mp4: ArrayBuffer): Promise<Int16Array> {
  const ctx = new OfflineAudioContext(1, 1, 16000);
  const buf = await ctx.decodeAudioData(mp4);
  const ch = Array.from({ length: buf.numberOfChannels }, (_, i) => buf.getChannelData(i));
  const out = new Int16Array(buf.length);
  const k = 32767 / ch.length;
  for (let i = 0; i < out.length; i++) {
    let v = 0;
    for (const c of ch) v += c[i];
    out[i] = Math.max(-32768, Math.min(32767, Math.round(v * k)));
  }
  return out;
}
