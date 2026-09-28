// Comment overlay: a 1920×1080 canvas fitted to the picture rectangle and
// drawn from the video's own presented-frame clock.

import { commentFonts, ensureCommentFont } from "./fonts.ts";
import NiconiComments from "./nico/niconicomments.js";
import type { CommentSettings } from "./settings.ts";
import type { V1Thread } from "./threads.ts";

/**
 * Presentation clock from requestVideoFrameCallback. In "display" mode the
 * time between frames is interpolated at the playback rate so scrolling is
 * drawn at the display's refresh rate; it never runs more than a quarter
 * second past the last presented frame.
 */
export class Clock {
  private media = 0;
  private at = 0;
  private have = false;
  frames = 0;
  /** Called for every presented frame (also a paused seek's new frame). */
  onFrame: (() => void) | null = null;

  constructor(private readonly video: HTMLVideoElement, signal?: AbortSignal) {
    const cb = (_now: number, md: VideoFrameCallbackMetadata) => {
      if (signal?.aborted) return;
      this.media = md.mediaTime;
      this.at = md.expectedDisplayTime;
      this.have = true;
      this.frames++;
      video.requestVideoFrameCallback(cb);
      this.onFrame?.();
    };
    video.requestVideoFrameCallback(cb);
    video.addEventListener("seeking", () => (this.have = false), { signal });
  }

  /** Seconds of original media time on screen now. */
  now(interpolate: boolean): number {
    const v = this.video;
    if (!this.have) return v.currentTime;
    if (!interpolate || v.paused || v.seeking || v.readyState < 3) return this.media;
    const dt = ((performance.now() - this.at) / 1000) * v.playbackRate;
    return this.media + Math.min(Math.max(dt, 0), 0.25);
  }

  /** Frame counter, to tell whether a new frame arrived. */
  get stamp(): number {
    return this.frames;
  }
}

export interface DrawStats {
  frames: number;
  meanMs: number;
  maxMs: number;
  /** 99th percentile, and how many draws took over 8 ms (one frame at 120 Hz). */
  p99Ms: number;
  over8: number;
}

export class Overlay {
  private nc: NiconiComments | null = null;
  private lastStamp = -1;
  private lastVpos = NaN;
  private raf = 0;
  private running = false;
  private drawTimes: number[] = [];
  buildMs = 0;
  error: string | null = null;
  drawErrors = 0;
  lastDrawError: string | null = null;

  constructor(
    private readonly video: HTMLVideoElement,
    private cv: HTMLCanvasElement,
    private readonly clock: Clock,
    private settings: CommentSettings,
    private offsetMs: number,
    signal?: AbortSignal,
  ) {
    const ro = new ResizeObserver(() => this.fit());
    ro.observe(video);
    video.addEventListener("loadedmetadata", () => this.fit(), { signal });
    this.fit();
    clock.onFrame = () => this.kick();
    for (const ev of ["play", "seeking", "seeked", "ratechange", "loadeddata"]) video.addEventListener(ev, () => this.kick(), { signal });
    signal?.addEventListener("abort", () => {
      ro.disconnect();
      this.destroy();
    });
  }

  /**
   * The draw loop runs only while something can change: playing with
   * comments on, or a redraw is owed. Paused, even an empty rAF loop keeps
   * the page producing frames at the display rate (measured 7 % of a core
   * at 120 Hz); stopped, a paused picture costs nothing.
   */
  private kick(): void {
    if (this.running) return;
    this.running = true;
    this.raf = requestAnimationFrame(this.loop);
  }

  private readonly loop = (ts: number): void => {
    // Schedule first: an exception while drawing must not end the loop.
    this.raf = requestAnimationFrame(this.loop);
    try {
      this.frame(ts);
    } catch (e) {
      this.drawErrors++;
      this.lastDrawError = String((e as Error)?.stack ?? e);
      return;
    }
    const v = this.video;
    const stopped = (v.paused || v.ended) && !v.seeking && this.lastVpos === this.nicoMs() / 10;
    if (!this.nc || !this.settings.enabled || stopped) {
      cancelAnimationFrame(this.raf);
      this.running = false;
    }
  };

  /** The frame on screen is stale (settings, size, offset, renderer). */
  private invalidate(): void {
    this.lastVpos = NaN;
    this.kick();
  }

  get canvas(): HTMLCanvasElement {
    return this.cv;
  }

  /**
   * Rebuild the renderer; niconicomments has no runtime option changes.
   * Each build gets a fresh canvas: destroy() loses the canvas's WebGL
   * context for good (WEBGL_lose_context), and a renderer built on the
   * same canvas afterwards draws nothing, silently. The new renderer is
   * built before the old one goes, so there is no blank gap.
   *
   * The layout is built in slices that hand the main thread back (nico/
   * README.md), so input and frames go on meanwhile; the old renderer keeps
   * drawing until the new one is ready. A load superseded by a newer one
   * is dropped.
   */
  private loadGen = 0;
  async load(threads: V1Thread[] | null, s: CommentSettings): Promise<void> {
    const gen = ++this.loadGen;
    this.settings = s;
    const next = document.createElement("canvas");
    next.id = this.cv.id;
    next.width = this.cv.width;
    next.height = this.cv.height;
    next.style.cssText = this.cv.style.cssText;
    let nc: NiconiComments | null = null;
    let error: string | null = null;
    const t0 = performance.now();
    if (threads) {
      try {
        await ensureCommentFont(threads);
        if (gen !== this.loadGen) return;
        nc = createRenderer(next, threads, s, true);
        if (!(await nc.build(3))) return;
      } catch (e) {
        error = String((e as Error)?.message ?? e);
        nc?.destroy();
        nc = null;
      }
    }
    if (gen !== this.loadGen) {
      nc?.destroy();
      return;
    }
    this.error = error;
    this.buildMs = performance.now() - t0;
    const old = this.nc;
    this.cv.replaceWith(next);
    this.cv = next;
    this.nc = nc;
    old?.destroy();
    this.applyStyle();
    this.invalidate();
  }

  /**
   * Draw the current time and read the pixels back in the same task (a
   * WebGL canvas is cleared once composited, so later reads see nothing).
   * Returns a hash and the count of non-zero bytes, for tests.
   */
  probe(offsetMs = 0): { hash: number; ink: number } {
    if (!this.nc) return { hash: 0, ink: 0 };
    this.nc.drawCanvas(this.nicoMs() / 10 + offsetMs / 10, true);
    const t = document.createElement("canvas");
    t.width = 192;
    t.height = 108;
    const g = t.getContext("2d")!;
    g.drawImage(this.cv, 0, 0, 192, 108);
    const d = g.getImageData(0, 0, 192, 108).data;
    let hash = 0, ink = 0;
    for (let i = 0; i < d.length; i++) {
      hash = (hash * 31 + d[i]) % 1000000007;
      if (d[i]) ink++;
    }
    this.invalidate();
    return { hash, ink };
  }

  setOffset(ms: number): void {
    this.offsetMs = ms;
    this.invalidate();
  }

  get offset(): number {
    return this.offsetMs;
  }

  /** Current Niconico time in ms for the frame on screen. */
  nicoMs(): number {
    return this.clock.now(this.settings.frameRate !== "video") * 1000 + this.offsetMs;
  }

  /** Apply style-only settings without rebuilding the renderer. */
  restyle(s: CommentSettings): void {
    this.settings = s;
    this.applyStyle();
    this.invalidate();
    if (!s.enabled) this.nc?.clear();
  }

  private applyStyle(): void {
    const s = this.settings;
    this.canvas.style.opacity = String(s.enabled ? s.opacity : 0);
    this.canvas.style.clipPath = s.area < 100 ? `inset(0 0 ${100 - s.area}% 0)` : "";
  }

  private fit(): void {
    const v = this.video, box = v.getBoundingClientRect();
    const vw = v.videoWidth || 16, vh = v.videoHeight || 9;
    const scale = Math.min(box.width / vw, box.height / vh);
    const w = vw * scale, h = vh * scale;
    // Comments use the 16:9 stage; a 4:3 picture gets the width of its box.
    const cs = this.canvas.style;
    cs.width = `${w}px`;
    cs.height = `${h}px`;
    cs.left = `${box.left + (box.width - w) / 2}px`;
    cs.top = `${box.top + (box.height - h) / 2}px`;
    this.invalidate();
  }

  private lastDrawAt = 0;

  private frame(now = performance.now()): void {
    if (!this.nc || !this.settings.enabled) return;
    // "60": skip refreshes closer than ~1/60 s to the last drawn one (a
    // 120 Hz display then draws every other refresh).
    if (this.settings.frameRate === "60" && now - this.lastDrawAt < 1000 / 60 - 2) return;
    const byVideoFrame = this.settings.frameRate === "video";
    if (byVideoFrame && this.clock.stamp === this.lastStamp && !this.video.paused) return;
    this.lastStamp = this.clock.stamp;
    const vpos = this.nicoMs() / 10;
    if (vpos === this.lastVpos) return;
    const force = Number.isNaN(this.lastVpos);
    this.lastVpos = vpos;
    this.lastDrawAt = now;
    const t0 = performance.now();
    // Forced only when the frame on screen is stale; otherwise niconicomments
    // skips frames with no scrolling comment and an unchanged set on screen.
    const drawn = this.nc.drawCanvas(vpos, force);
    const dt = performance.now() - t0;
    if (drawn && this.drawTimes.length < 100000) this.drawTimes.push(dt);
  }

  /**
   * The picture and the comments at this moment, as shown, on a 1920x1080
   * stage (for ?shots). The comments are drawn and read back in one task: a
   * WebGL canvas is cleared once composited.
   */
  snapshot(): HTMLCanvasElement {
    const out = document.createElement("canvas");
    out.width = 1920;
    out.height = 1080;
    const g = out.getContext("2d")!;
    g.fillStyle = "#000";
    g.fillRect(0, 0, 1920, 1080);
    const v = this.video, sc = Math.min(1920 / (v.videoWidth || 1920), 1080 / (v.videoHeight || 1080));
    const w = v.videoWidth * sc, h = v.videoHeight * sc;
    g.drawImage(v, (1920 - w) / 2, (1080 - h) / 2, w, h);
    if (this.nc && this.settings.enabled) {
      this.nc.drawCanvas(this.nicoMs() / 10, true);
      g.globalAlpha = this.settings.opacity;
      g.drawImage(this.cv, 0, 0, 1920, 1080);
    }
    this.lastVpos = NaN;
    return out;
  }

  takeDrawStats(): DrawStats {
    const a = this.drawTimes;
    this.drawTimes = [];
    if (!a.length) return { frames: 0, meanMs: 0, maxMs: 0, p99Ms: 0, over8: 0 };
    const sorted = a.slice().sort((x, y) => x - y);
    return { frames: a.length, meanMs: a.reduce((x, y) => x + y, 0) / a.length, maxMs: sorted[sorted.length - 1],
      p99Ms: sorted[Math.floor(sorted.length * 0.99)], over8: a.filter((x) => x > 8).length };
  }

  destroy(): void {
    cancelAnimationFrame(this.raf);
    this.running = false;
    this.loadGen++; // a build still running is dropped
    this.nc?.destroy();
    this.nc = null;
  }
}

/**
 * The comment renderer for these settings (also used by the layout check).
 * deferBuild: lay out later with build(), in slices.
 */
export function createRenderer(canvas: HTMLCanvasElement, threads: V1Thread[], s: CommentSettings, deferBuild = false): NiconiComments {
  return new NiconiComments(canvas, threads as never, {
    format: "v1",
    mode: s.mode,
    keepCA: s.keepCA,
    scale: s.scale,
    config: {
      commentLimit: s.limit > 0 ? s.limit : undefined,
      hideCommentOrder: s.limitOrder,
      contextStrokeColor: s.strokeColor,
      contextStrokeOpacity: s.strokeOpacity,
      contextLineWidth: { html5: 2.8 * s.strokeWidth, flash: 4 * s.strokeWidth },
      fonts: commentFonts() as never,
    },
  }, deferBuild);
}
