// ?selftest&layout: build the comment layout of this episode several times
// and report a fingerprint of the result and the build times. The
// fingerprint covers, for every comment, its computed properties (size,
// font, line widths, …), its position (posY) and its box, and for every
// time slot of the timeline the order of the comments in it. It is the
// reference for any change to the renderer: same input, same fingerprint.

import type { V1Thread } from "./filter.ts";

interface Instance {
  comments: { index: number; posY: number; vpos: number; loc: string; width: number; height: number; long: number; invisible: boolean; comment: unknown }[];
  timeline: Record<number, { index: number }[]>;
  destroy(): void;
}

function fnv(h: number, s: string): number {
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619) >>> 0;
  return h;
}

/** Fingerprint of an instance's layout, as hashes (all, comments, timeline). */
export function fingerprint(nc: Instance): { all: string; comments: string; timeline: string; n: number; slots: number } {
  const skip = (k: string, v: unknown) => (k === "ctx" || k === "renderer" || k === "image" ? undefined : v);
  let hc = 2166136261;
  for (const c of nc.comments) {
    hc = fnv(hc, JSON.stringify([c.index, c.vpos, c.loc, c.posY, c.width, c.height, c.long, c.invisible]));
    hc = fnv(hc, JSON.stringify(c.comment, skip));
  }
  let ht = 2166136261;
  let slots = 0;
  // Slots from IntervalTimeline (nico/README.md, change 5), or the original map.
  const it = (nc as unknown as { timelineSlots?(): Iterable<[number, { index: number }[]]> }).timelineSlots?.()
    ?? Object.keys(nc.timeline).map(Number).sort((a, b) => a - b).map((k) => [k, nc.timeline[k]] as [number, { index: number }[]]);
  for (const [k, list] of it) {
    ht = fnv(ht, `${k}:${list.map((c) => c.index).join(",")};`);
    slots++;
  }
  const hex = (h: number) => h.toString(16).padStart(8, "0");
  return { all: hex(fnv(hc, hex(ht))), comments: hex(hc), timeline: hex(ht), n: nc.comments.length, slots };
}

/** Field paths where two values differ (first few). */
function diff(a: unknown, b: unknown, path = "", out: string[] = []): string[] {
  if (out.length >= 6) return out;
  if (typeof a !== typeof b || a === null || b === null || typeof a !== "object") {
    if (a !== b && !(Number.isNaN(a) && Number.isNaN(b))) out.push(`${path}: ${JSON.stringify(a)?.slice(0, 60)} ≠ ${JSON.stringify(b)?.slice(0, 60)}`);
    return out;
  }
  const keys = new Set([...Object.keys(a as object), ...Object.keys(b as object)]);
  for (const k of keys) diff((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k], `${path}.${k}`, out);
  return out;
}

const snapshot = (nc: Instance) => nc.comments.map((c) => JSON.parse(JSON.stringify({ posY: c.posY, width: c.width, height: c.height, long: c.long, comment: c.comment },
  (k, v) => (k === "ctx" || k === "renderer" || k === "image" ? undefined : v))));

/** mulberry32: a small seeded PRNG in [0, 1). */
export function seeded(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export async function layoutCheck(threads: V1Thread[] | null, build: (canvas: HTMLCanvasElement, threads: V1Thread[]) => Promise<Instance>): Promise<unknown> {
  if (!threads) return { error: "no comments" };
  const runs: ({ ms: number; longTaskMs: number; maxSlice: string; diffs?: string[] } & ReturnType<typeof fingerprint>)[] = [];
  let first: unknown[] | null = null;
  for (let i = 0; i < 3; i++) {
    const canvas = document.createElement("canvas");
    canvas.width = 1920;
    canvas.height = 1080;
    // The renderer's only randomness: a comment with no free row goes to a
    // random height (Math.random, as on Niconico). Seeded here, so runs compare.
    const random = Math.random;
    Math.random = seeded(20260925);
    // The longest stretch the main thread was held (long tasks, 50 ms and over).
    let longest = 0;
    const lt = new PerformanceObserver((l) => { for (const e of l.getEntries()) longest = Math.max(longest, e.duration); });
    lt.observe({ type: "longtask" });
    const t0 = performance.now();
    let nc: Instance;
    try {
      nc = await build(canvas, threads);
    } finally {
      Math.random = random;
    }
    const ms = performance.now() - t0;
    await new Promise((r) => setTimeout(r, 0));
    lt.disconnect();
    const snap = snapshot(nc);
    const diffs: string[] = [];
    if (first) for (let k = 0; k < snap.length && diffs.length < 6; k++) diff(first[k], snap[k], `#${k}`, diffs);
    else first = snap;
    const bs = (nc as unknown as { buildStats?: { maxSliceMs: number; maxSlicePhase: string } }).buildStats;
    runs.push({ ms: Math.round(ms), longTaskMs: Math.round(longest), maxSlice: bs ? `${Math.round(bs.maxSliceMs)} ${bs.maxSlicePhase}` : "", ...fingerprint(nc), diffs });
    nc.destroy();
    await new Promise((r) => setTimeout(r, 50));
  }
  return { deterministic: runs.every((r) => r.all === runs[0].all), runs };
}
