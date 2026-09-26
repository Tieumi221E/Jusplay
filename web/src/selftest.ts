// ?selftest=1: drive the real player and report measurements to
// api/selftest, where the Go side prints them and closes the window.

import type { Engine } from "./mse.ts";
import type { Clock, Overlay } from "./overlay.ts";
import type { Settings } from "./settings.ts";
import type { FilterStats } from "./filter.ts";
import { L, prefs, setPref } from "./i18n.ts";

interface Ctx {
  video: HTMLVideoElement;
  engine: Engine;
  overlay: Overlay;
  clock: Clock;
  session: { media: { duration: number; keyframes: number[]; frameRate: string } };
  settings: Settings;
  rebuild: () => Promise<void>;
  stats: () => FilterStats | null;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export async function selftest(c: Ctx): Promise<void> {
  const { video: v } = c;
  const R: Record<string, unknown> = { ua: navigator.userAgent, dpr: devicePixelRatio, errors: [] as string[] };
  const errors = R.errors as string[];
  v.addEventListener("error", () => errors.push(`video: ${v.error?.message}`));
  addEventListener("error", (e) => errors.push(`uncaught: ${e.message}`));
  addEventListener("unhandledrejection", (e) => errors.push(`unhandled rejection: ${e.reason}`));
  v.muted = true;
  const [n, d] = c.session.media.frameRate.split("/").map(Number);
  const frameDur = d ? d / n : 1 / 24;

  const playFor = async (seconds: number) => {
    const q0 = v.getVideoPlaybackQuality(), f0 = c.clock.frames, w0 = performance.now(), m0 = v.currentTime;
    let waiting = 0;
    const onWait = () => waiting++;
    v.addEventListener("waiting", onWait);
    c.overlay.takeDrawStats();
    await sleep(seconds * 1000);
    v.removeEventListener("waiting", onWait);
    const q1 = v.getVideoPlaybackQuality(), wall = (performance.now() - w0) / 1000, media = v.currentTime - m0;
    return {
      wallSeconds: +wall.toFixed(2), mediaAdvanced: +media.toFixed(3),
      presentedFrames: c.clock.frames - f0, expectedFrames: +(media / frameDur).toFixed(1),
      dropped: q1.droppedVideoFrames - q0.droppedVideoFrames, waitingEvents: waiting,
      draw: c.overlay.takeDrawStats(),
    };
  };

  // The first presented frame near t. A callback still pending from before
  // the seek would report the old frame, so frames far from t are skipped.
  const frameNear = (t: number, timeoutMs: number) =>
    Promise.race([
      new Promise<{ now: number; md: VideoFrameCallbackMetadata }>((resolve) => {
        const cb = (now: number, md: VideoFrameCallbackMetadata) =>
          Math.abs(md.mediaTime - t) < 1 ? resolve({ now, md }) : v.requestVideoFrameCallback(cb);
        v.requestVideoFrameCallback(cb);
      }),
      sleep(timeoutMs).then(() => null),
    ]);

  const seekTo = async (t: number) => {
    const t0 = performance.now();
    const f = frameNear(t, 8000);
    v.currentTime = t;
    const fr = await f;
    return fr
      ? { target: t, frameMs: +(fr.now - t0).toFixed(0), mediaTime: +fr.md.mediaTime.toFixed(4),
          errorFrames: +((fr.md.mediaTime - t) / frameDur).toFixed(2) }
      : { target: t, frameMs: "no frame within 8 s" };
  };

  // 1. start from the beginning
  await new Promise((r) => (v.readyState >= 3 ? r(null) : v.addEventListener("canplay", r, { once: true })));
  await v.play();
  R.fromStart = await playFor(4);
  R.firstAppendMs = c.engine.firstAppendMs();
  // A language switch rewrites every text on the page (budget: 50 ms).
  {
    const lang = prefs.lang, t0 = performance.now();
    setPref("lang", lang === "ja" ? "zh" : "ja");
    const there = performance.now() - t0, t1 = performance.now();
    setPref("lang", lang);
    R.langSwitchMs = [Math.round(there * 10) / 10, Math.round((performance.now() - t1) * 10) / 10];
  }
  R.startup = { ...(window as unknown as { kpMarks: Record<string, number> }).kpMarks, buildMs: Math.round(c.overlay.buildMs), timeOrigin: Math.round(performance.timeOrigin) };

  // 2. jump far ahead (outside the buffer) into the densest comments and keep playing
  const restartsBefore = c.engine.restarts;
  const jump = await seekTo(788);
  R.jumpOutsideBuffer = { ...jump, streamRestarts: c.engine.restarts - restartsBefore };
  R.denseSection = await playFor(12);
  v.pause();
  await sleep(300);

  // 3. paused seeks: inside the buffer, then far away
  const seeks = [];
  for (const t of [793.5, 795.25, 60, 400, 900.5, 1300, 5.25, 1419]) seeks.push(await seekTo(t));
  R.pausedSeeks = seeks;

  // 4. play across a stream position reached by continuous reading (no seek)
  await seekTo(300);
  await v.play();
  R.longRun = await playFor(20);
  v.pause();

  // 4b. drag the real 字号 slider while playing; the canvas must still
  // show comments, and different ones as time passes (checked on pixels
  // read right after drawing, not on draw calls).
  await seekTo(790);
  await v.play();
  await sleep(1000);
  document.getElementById("open-settings")!.click();
  const row = [...document.querySelectorAll<HTMLElement>("#panel .row")].find((r) => r.querySelector(".lbl")?.textContent === L("字号", "文字サイズ"))!;
  const range = row.querySelector<HTMLInputElement>("input[type=range]")!;
  const drag = async (values: number[]) => {
    for (const x of values) {
      range.value = String(x);
      range.dispatchEvent(new Event("input", { bubbles: true }));
      await sleep(40);
    }
    range.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const moving = async () => {
    c.overlay.takeDrawStats();
    const f0 = c.clock.frames, a = c.overlay.probe();
    await sleep(800);
    const b = c.overlay.probe();
    return { ink: b.ink, changed: a.hash !== b.hash, draws: c.overlay.takeDrawStats().frames, videoFrames: c.clock.frames - f0, t: +v.currentTime.toFixed(2) };
  };
  const before4b = await moving();
  await drag([1.05, 1.1, 1.15, 1.2, 1.25, 1.3]);
  await sleep(1200);
  const afterUp = await moving();
  await drag([1.2, 1.1, 1.0, 0.9, 0.8]);
  await sleep(1200);
  const afterDown = await moving();
  await drag([1.0]);
  await sleep(1200);
  document.getElementById("close-settings")!.click();
  v.pause();
  R.scaleDrag = { before: before4b, afterUp, afterDown, drawErrors: c.overlay.drawErrors, lastDrawError: c.overlay.lastDrawError, overlayError: c.overlay.error };

  // 5. settings: filter rebuild cost and effect
  const before = c.stats();
  const t0 = performance.now();
  c.settings.filters.hide.naka = true;
  await c.rebuild();
  const afterNaka = c.stats();
  const rebuildMs = performance.now() - t0;
  const inkWithoutScrolling = c.overlay.probe().ink;
  c.settings.filters.hide.naka = false;
  const t1 = performance.now();
  await c.rebuild();
  const fullRebuildMs = performance.now() - t1;
  R.inkAfterRebuilds = { withoutScrolling: inkWithoutScrolling, restored: c.overlay.probe().ink };
  R.filter = { total: before?.total, shown: before?.shown, shownWithoutScrolling: afterNaka?.shown, byRule: afterNaka?.byRule,
    smallRebuildMs: +rebuildMs.toFixed(1), fullRebuildMs: +fullRebuildMs.toFixed(1), rendererBuildMs: +c.overlay.buildMs.toFixed(1) };

  errors.push(...c.engine.errors());
  if (c.overlay.error) errors.push(`overlay: ${c.overlay.error}`);
  const mem = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory;
  R.jsHeapMB = mem ? Math.round(mem.usedJSHeapSize / 2 ** 20) : null;
  await fetch("api/selftest", { method: "POST", body: JSON.stringify(R) });
}
