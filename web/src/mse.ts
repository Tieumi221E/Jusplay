// Media Source engine: one SourceBuffer per track, each fed by its own
// fMP4 stream from api/stream/<kind>?t=. The server says how that stream's
// timeline maps to the original file (X-Jusplay-Offset) and where it
// starts (X-Jusplay-Start); video drops anything presented before the
// start (open-GOP leading pictures that cannot be decoded after a seek).

export interface TrackInfo {
  mime: string;
  codec: string;
  profile?: string;
  language?: string;
}

export interface MediaInfo {
  duration: number;
  video: TrackInfo;
  audio: TrackInfo | null;
  keyframes: number[];
  /** Why there is no audio when the file has a track the server could not package. */
  audioNote?: string;
}

type Kind = "video" | "audio";

const AHEAD = 60; // seconds buffered ahead before reading pauses
const BEHIND = 30; // seconds kept behind the playhead

class Feeder {
  private gen = 0;
  private abort: AbortController | null = null;
  private queue: Promise<void> = Promise.resolve();
  ended = false;
  lastError: string | null = null;
  /** Wall-clock ms spent until the first byte of the newest stream was appended. */
  firstAppendMs = 0;

  constructor(
    readonly kind: Kind,
    readonly sb: SourceBuffer,
    private readonly video: HTMLVideoElement,
    private readonly onEnded: () => void,
    private readonly id: string,
  ) {}

  /** Run op once the SourceBuffer is idle, and wait for it to finish. */
  private op(fn: () => void): Promise<void> {
    const run = () =>
      new Promise<void>((resolve, reject) => {
        const done = () => {
          this.sb.removeEventListener("updateend", done);
          this.sb.removeEventListener("error", fail);
          resolve();
        };
        const fail = () => {
          this.sb.removeEventListener("updateend", done);
          this.sb.removeEventListener("error", fail);
          reject(new Error(`${this.kind} SourceBuffer error`));
        };
        this.sb.addEventListener("updateend", done);
        this.sb.addEventListener("error", fail);
        try {
          fn();
        } catch (e) {
          this.sb.removeEventListener("updateend", done);
          this.sb.removeEventListener("error", fail);
          reject(e);
          return;
        }
        if (!this.sb.updating) done();
      });
    const p = this.queue.then(run, run);
    this.queue = p.catch(() => undefined);
    return p;
  }

  bufferedAt(t: number): boolean {
    const b = this.sb.buffered;
    for (let i = 0; i < b.length; i++) if (b.start(i) <= t + 0.05 && t < b.end(i) - 0.2) return true;
    return false;
  }

  private ahead(t: number): number {
    const b = this.sb.buffered;
    for (let i = 0; i < b.length; i++) if (b.start(i) <= t + 0.05 && t < b.end(i)) return b.end(i) - t;
    return 0;
  }

  private async evict(t: number): Promise<void> {
    const b = this.sb.buffered;
    const ranges: [number, number][] = [];
    for (let i = 0; i < b.length; i++) ranges.push([b.start(i), b.end(i)]);
    for (const [s, e] of ranges) {
      if (e < t - BEHIND) await this.op(() => this.sb.remove(s, e));
      else if (s < t - BEHIND) await this.op(() => this.sb.remove(s, t - BEHIND));
      if (s > t + AHEAD + 30) await this.op(() => this.sb.remove(s, e));
    }
  }

  /** (Re)start streaming from the keyframe at or before t. */
  async start(t: number): Promise<void> {
    const gen = ++this.gen;
    this.abort?.abort();
    const ac = (this.abort = new AbortController());
    this.ended = false;
    this.lastError = null;
    const t0 = performance.now();
    let res: Response;
    try {
      res = await fetch(`api/stream/${this.kind}?id=${encodeURIComponent(this.id)}&t=${t.toFixed(3)}`, { signal: ac.signal });
    } catch (e) {
      if (gen === this.gen) this.lastError = String(e);
      return;
    }
    if (gen !== this.gen) return;
    if (!res.ok || !res.body) {
      this.lastError = `${this.kind} stream: HTTP ${res.status} ${await res.text()}`;
      return;
    }
    const offset = Number(res.headers.get("X-Jusplay-Offset"));
    const start = Number(res.headers.get("X-Jusplay-Start"));
    // A new stream begins with a new init segment: reset the parser, then
    // map its timeline and drop what presents before its start.
    await this.op(() => undefined);
    try {
      this.sb.abort(); // throws once endOfStream() ended the source; the parser is idle then
    } catch {
      /* ended */
    }
    this.sb.timestampOffset = offset;
    this.sb.appendWindowStart = 0;
    this.sb.appendWindowEnd = Infinity;
    if (this.kind === "video") this.sb.appendWindowStart = Math.max(0, start);
    const reader = res.body.getReader();
    let first = true;
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (gen !== this.gen) return;
        if (done) break;
        while (this.ahead(this.video.currentTime) > AHEAD && gen === this.gen) {
          await this.evict(this.video.currentTime);
          await new Promise((r) => setTimeout(r, 250));
        }
        if (gen !== this.gen) return;
        await this.append(value);
        if (first) {
          this.firstAppendMs = performance.now() - t0;
          first = false;
        }
      }
      this.ended = true;
      this.onEnded();
    } catch (e) {
      if (gen === this.gen && !ac.signal.aborted) this.lastError = `${this.kind}: ${e}`;
    } finally {
      reader.releaseLock();
    }
  }

  private async append(chunk: Uint8Array): Promise<void> {
    for (let attempt = 0; ; attempt++) {
      try {
        await this.op(() => this.sb.appendBuffer(chunk as BufferSource));
        return;
      } catch (e) {
        if ((e as DOMException).name !== "QuotaExceededError" || attempt > 3) throw e;
        await this.evict(this.video.currentTime);
        await new Promise((r) => setTimeout(r, 500));
      }
    }
  }

  stop(): void {
    this.gen++;
    this.abort?.abort();
  }
}

export class Engine {
  readonly ms = new MediaSource();
  private feeders: Feeder[] = [];
  private restartTimer = 0;
  private url = "";
  private readonly seeking = () => this.onSeeking();
  /** Stream restarts caused by seeks outside the buffer. */
  restarts = 0;

  constructor(private readonly video: HTMLVideoElement, private readonly info: MediaInfo, private readonly id: string) {}

  /** Why the video cannot play here, or null. */
  static unsupported(info: MediaInfo): string | null {
    if (!("MediaSource" in window)) return "MediaSource is not available";
    if (!MediaSource.isTypeSupported(info.video.mime)) return `this engine cannot decode ${info.video.codec} (${info.video.mime})`;
    return null;
  }

  /**
   * Why the audio will not play (the picture still does), or null: a track
   * the engine cannot decode is left out rather than stopping playback.
   */
  static audioProblem(info: MediaInfo): string | null {
    if (info.audio && !MediaSource.isTypeSupported(info.audio.mime)) {
      const why = `${info.audio.codec} (${info.audio.mime})`;
      info.audio = null;
      return why;
    }
    return info.audioNote ?? null;
  }

  /** Start streaming at t (resume position) instead of the beginning. */
  async init(startAt = 0): Promise<void> {
    const opened = new Promise((r) => this.ms.addEventListener("sourceopen", r, { once: true }));
    this.url = URL.createObjectURL(this.ms);
    this.video.src = this.url;
    await opened;
    this.ms.duration = this.info.duration;
    const onEnded = () => {
      if (this.feeders.every((f) => f.ended) && this.ms.readyState === "open") {
        try {
          this.ms.endOfStream();
        } catch {
          /* a feeder restarted meanwhile */
        }
      }
    };
    this.feeders.push(new Feeder("video", this.ms.addSourceBuffer(this.info.video.mime), this.video, onEnded, this.id));
    if (this.info.audio) this.feeders.push(new Feeder("audio", this.ms.addSourceBuffer(this.info.audio.mime), this.video, onEnded, this.id));
    this.video.addEventListener("seeking", this.seeking);
    this.startAll(startAt);
    if (startAt > 0) this.video.currentTime = startAt;
  }

  private startAll(t: number): void {
    for (const f of this.feeders) void f.start(t);
  }

  private onSeeking(): void {
    const t = this.video.currentTime;
    if (this.feeders.every((f) => f.bufferedAt(t))) return;
    // Coalesce scrubbing: restart once the position settles briefly.
    clearTimeout(this.restartTimer);
    this.restartTimer = window.setTimeout(() => {
      this.restarts++;
      this.startAll(this.video.currentTime);
    }, 60);
  }

  /** Seconds buffered per track, and the number of separate ranges. */
  buffered(): { kind: string; seconds: number; ranges: number }[] {
    return this.feeders.map((f) => {
      const b = f.sb.buffered;
      let seconds = 0;
      for (let i = 0; i < b.length; i++) seconds += b.end(i) - b.start(i);
      return { kind: f.kind, seconds: +seconds.toFixed(1), ranges: b.length };
    });
  }

  errors(): string[] {
    return this.feeders.map((f) => f.lastError).filter((x): x is string => !!x);
  }

  firstAppendMs(): number[] {
    return this.feeders.map((f) => f.firstAppendMs);
  }

  /** Stop streaming and let go of the video (another episode takes it next). */
  destroy(): void {
    for (const f of this.feeders) f.stop();
    this.video.removeEventListener("seeking", this.seeking);
    clearTimeout(this.restartTimer);
    if (this.url) URL.revokeObjectURL(this.url);
  }
}
