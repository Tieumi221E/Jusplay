// Bottom bar: comment density and buffered ranges drawn behind the seek
// slider, a hover tooltip, and a line of media and comment facts.

import { L } from "./i18n.ts";
import type { V1Thread } from "./threads.ts";
import { fmtTime } from "./ui.ts";

const BINS = 240;

export class SeekBar {
  private counts = new Float32Array(BINS);
  private max = 1;
  private vpos: Float64Array = new Float64Array(0); // sorted seconds
  /** Hotspots from the comment analysis (Niconico time), when it is on. */
  private hot: { peak: number; phrase: string }[] = [];

  constructor(
    private readonly canvas: HTMLCanvasElement,
    private readonly tip: HTMLElement,
    private readonly range: HTMLInputElement,
    private readonly video: HTMLVideoElement,
    private readonly duration: number,
    private readonly offsetSeconds: () => number,
    signal?: AbortSignal,
  ) {
    const ro = new ResizeObserver(() => this.draw());
    ro.observe(canvas);
    signal?.addEventListener("abort", () => ro.disconnect());
    for (const ev of ["progress", "seeked", "timeupdate"]) video.addEventListener(ev, () => this.draw(), { signal });
    range.addEventListener("mousemove", (e) => this.hover(e), { signal });
    range.addEventListener("mouseleave", () => (this.tip.hidden = true), { signal });
  }

  /** Comments as shown (after filters); times are Niconico time. */
  setComments(threads: V1Thread[] | null): void {
    const all: number[] = [];
    for (const t of threads ?? []) for (const c of t.comments) all.push(c.vposMs / 1000);
    all.sort((a, b) => a - b);
    this.vpos = Float64Array.from(all);
    this.counts.fill(0);
    for (const s of all) {
      const i = Math.floor((s / this.duration) * BINS);
      if (i >= 0 && i < BINS) this.counts[i]++;
    }
    this.max = Math.max(1, ...this.counts);
    this.draw();
  }

  /** Hotspots to mark above the density (report.ts HotspotR); [] clears them. */
  setHotspots(hs: { Peak: number; phrases: { name: string }[] | null }[]): void {
    this.hot = hs.map((h) => ({ peak: h.Peak, phrase: h.phrases?.[0]?.name ?? "" }));
    this.draw();
  }

  /** Comments whose Niconico time falls in [from, to) seconds. */
  countBetween(from: number, to: number): number {
    return lowerBound(this.vpos, to) - lowerBound(this.vpos, from);
  }

  private hover(e: MouseEvent): void {
    const r = this.range.getBoundingClientRect();
    const x = Math.min(Math.max(e.clientX - r.left, 0), r.width);
    const t = (x / r.width) * this.duration;
    const bin = (this.duration / BINS);
    const nico = t + this.offsetSeconds();
    const n = this.countBetween(nico - bin / 2, nico + bin / 2);
    // Near a hotspot: what was said there.
    const near = this.hot.find((h) => Math.abs(h.peak - nico) <= bin * 1.5);
    this.tip.textContent = `${fmtTime(t)} · ${n} ${L("条", "件")}${near?.phrase ? ` · ${L("“", "「")}${near.phrase}${L("”", "」")}` : ""}`;
    this.tip.hidden = false;
    this.tip.style.left = `${r.left + x}px`;
    this.tip.style.top = `${r.top - 30}px`;
  }

  draw(): void {
    const c = this.canvas, dpr = devicePixelRatio || 1;
    const w = Math.round(c.clientWidth * dpr), h = Math.round(c.clientHeight * dpr);
    if (!w || !h) return;
    if (c.width !== w || c.height !== h) {
      c.width = w;
      c.height = h;
    }
    const g = c.getContext("2d")!;
    g.clearRect(0, 0, w, h);
    // Colours come from the theme (app.css); read per draw, a few times a second.
    const cs = getComputedStyle(c), col = (name: string) => cs.getPropertyValue(name).trim();
    const x = (sec: number) => (sec / this.duration) * w;
    // Buffered ranges as a thin band along the bottom.
    const b = this.video.buffered;
    g.fillStyle = col("--buffered");
    for (let i = 0; i < b.length; i++) g.fillRect(x(b.start(i)), h - 3 * dpr, Math.max(1, x(b.end(i)) - x(b.start(i))), 3 * dpr);
    // Comment density; bins are in Niconico time, drawn at local time.
    const off = this.offsetSeconds(), bw = w / BINS, played = x(this.video.currentTime);
    const done = col("--density-played"), ahead = col("--density");
    for (let i = 0; i < BINS; i++) {
      if (!this.counts[i]) continue;
      const bh = Math.max(1, (this.counts[i] / this.max) * (h - 5 * dpr));
      const left = i * bw - x(off);
      g.fillStyle = left < played ? done : ahead;
      g.fillRect(left, h - 4 * dpr - bh, Math.max(1, bw - 0.5), bh);
    }
    // Hotspots: a small dot at the top, in the accent colour, ringed in the bar's surface.
    if (this.hot.length) {
      const r = 2.5 * dpr;
      for (const hs of this.hot) {
        const cx = x(hs.peak - off);
        g.beginPath();
        g.arc(cx, r + 1 * dpr, r + 1 * dpr, 0, Math.PI * 2);
        g.fillStyle = col("--hot-ring") || "rgba(0,0,0,.4)";
        g.fill();
        g.beginPath();
        g.arc(cx, r + 1 * dpr, r, 0, Math.PI * 2);
        g.fillStyle = col("--accent");
        g.fill();
      }
    }
  }
}

function lowerBound(a: Float64Array, v: number): number {
  let lo = 0, hi = a.length;
  while (lo < hi) {
    const m = (lo + hi) >> 1;
    if (a[m] < v) lo = m + 1;
    else hi = m;
  }
  return lo;
}

/** Seconds buffered ahead of t across all tracks (video.buffered is their intersection). */
export function bufferedAhead(v: HTMLVideoElement): number {
  const b = v.buffered, t = v.currentTime;
  for (let i = 0; i < b.length; i++) if (b.start(i) <= t + 0.05 && t < b.end(i)) return b.end(i) - t;
  return 0;
}
