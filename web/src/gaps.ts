// Holes in a media buffer: how much is buffered ahead across small ones,
// and where to jump when playback is stuck in front of one (mse.ts).

/**
 * Holes in the buffer this small are played over, as other players do:
 * files have them where a track's packets are missing (a download that
 * lost 61 ms of audio at the start is enough to stop playback there).
 */
export const MAX_GAP = 1;

export type Range = [number, number];

export function rangesOf(b: TimeRanges): Range[] {
  const out: Range[] = [];
  for (let i = 0; i < b.length; i++) out.push([b.start(i), b.end(i)]);
  return out;
}

/**
 * Seconds buffered ahead of t, counting across holes up to MAX_GAP (and a
 * hole t itself is in, when the buffer resumes within MAX_GAP): a track
 * whose playhead sits in a small hole must still pause reading, or it
 * appends until the SourceBuffer is full.
 */
export function aheadAcross(ranges: Range[], t: number, maxGap = MAX_GAP): number {
  let reach = t;
  for (const [s, e] of ranges) {
    if (e <= reach) continue; // behind
    if (s > reach + maxGap) break; // a hole too big to play over
    reach = e;
  }
  return reach - t;
}

/**
 * Where to jump when playback is stuck at t: the start of the next buffered
 * range, when t is in a hole (or at a range's end) and that range begins
 * within MAX_GAP; null otherwise (nothing to play over: wait for data).
 */
export function gapTarget(ranges: Range[], t: number, maxGap = MAX_GAP): number | null {
  for (const [s, e] of ranges) {
    if (s <= t + 0.05 && t < e - 0.2) return null; // playable here: not a hole
    if (s > t && s - t <= maxGap) return s + 0.01;
  }
  return null;
}
