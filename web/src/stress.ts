// ?selftest=1&stress=<seconds>: long random playback with seeks and
// scrubbing, sampling resources every 10 s, to look for growth or stalls.

import type { Engine } from "./mse.ts";
import type { Clock, Overlay } from "./overlay.ts";

interface Ctx {
  video: HTMLVideoElement;
  engine: Engine;
  overlay: Overlay;
  clock: Clock;
  duration: number;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export async function stress(c: Ctx, seconds: number): Promise<Record<string, unknown>> {
  const { video: v, engine } = c;
  v.muted = true;
  const errors: string[] = [];
  addEventListener("error", (e) => errors.push(`uncaught: ${e.message}`));
  addEventListener("unhandledrejection", (e) => errors.push(`unhandled: ${e.reason}`));
  v.addEventListener("error", () => errors.push(`video: ${v.error?.message}`));

  // Deterministic pseudo-random sequence, so a failing run can be replayed.
  let seed = 20260925;
  const rnd = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);

  const samples: Record<string, unknown>[] = [];
  const stalls: { at: number; t: number; readyState: number; buffered: unknown }[] = [];
  const actions: Record<string, number> = {};
  const t0 = performance.now();
  let lastT = v.currentTime, lastMove = performance.now();
  const sample = async () => {
    const q = v.getVideoPlaybackQuality();
    const mem = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory;
    const go = await fetch("api/stats").then((r) => r.json()).catch(() => null);
    samples.push({
      s: Math.round((performance.now() - t0) / 1000), t: +v.currentTime.toFixed(1), jsHeapMB: mem ? Math.round(mem.usedJSHeapSize / 2 ** 20) : null,
      buffered: engine.buffered(), restarts: engine.restarts, frames: q.totalVideoFrames, dropped: q.droppedVideoFrames,
      ink: c.overlay.probe().ink, drawErrors: c.overlay.drawErrors, go,
    });
  };
  const watchdog = window.setInterval(() => {
    // Playing, not seeking, yet the picture has not advanced for 5 s.
    if (!v.paused && !v.seeking && Math.abs(v.currentTime - lastT) < 0.01) {
      if (performance.now() - lastMove > 5000 && !stalls.some((s) => performance.now() - s.at < 5000)) {
        stalls.push({ at: performance.now(), t: v.currentTime, readyState: v.readyState, buffered: engine.buffered() });
      }
    } else {
      lastT = v.currentTime;
      lastMove = performance.now();
    }
  }, 500);

  await v.play().catch(() => undefined);
  let nextSample = 0;
  while (performance.now() - t0 < seconds * 1000) {
    if (performance.now() - t0 >= nextSample) {
      await sample();
      nextSample += 10000;
    }
    const r = rnd(), d = c.duration;
    const act = r < 0.45 ? "jump" : r < 0.7 ? "scrub" : r < 0.85 ? "nudge" : r < 0.95 ? "pause" : "rate";
    actions[act] = (actions[act] ?? 0) + 1;
    switch (act) {
      case "jump":
        v.currentTime = rnd() * (d - 5);
        await sleep(1000 + rnd() * 3000);
        break;
      case "scrub": {
        let t = rnd() * (d - 60);
        for (let i = 0, n = 8 + Math.floor(rnd() * 12); i < n; i++) {
          t = Math.min(d - 5, Math.max(0, t + (rnd() - 0.3) * 20));
          v.currentTime = t;
          await sleep(30 + rnd() * 50);
        }
        await sleep(2000);
        break;
      }
      case "nudge":
        v.currentTime = Math.min(d - 5, Math.max(0, v.currentTime + (rnd() < 0.5 ? -5 : 5)));
        await sleep(800);
        break;
      case "pause":
        v.pause();
        await sleep(1000);
        await v.play().catch(() => undefined);
        await sleep(1000);
        break;
      case "rate":
        v.playbackRate = [1, 1.5, 2, 0.5][Math.floor(rnd() * 4)];
        await sleep(1500);
        v.playbackRate = 1;
        break;
    }
    if (v.paused) await v.play().catch(() => undefined);
  }
  clearInterval(watchdog);
  await sample();
  v.pause();
  errors.push(...engine.errors());
  return { stressSeconds: seconds, actions, samples, stalls: stalls.map(({ at, ...s }) => ({ atS: Math.round((at - t0) / 1000), ...s })), errors };
}
