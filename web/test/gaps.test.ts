import { test } from "node:test";
import assert from "node:assert/strict";
import { aheadAcross, gapTarget, type Range } from "../src/gaps.ts";

// A file whose audio lost 61 ms right after its first packet: the buffer the
// browser plays from has a hole just after the start.
const holeAtStart: Range[] = [[0, 0.3], [0.4, 61.6]];

test("stuck in front of a small hole: jump to the range after it", () => {
  const near = (x: number | null, want: number) => assert.ok(x !== null && Math.abs(x - want) < 1e-9, `${x} ≠ ${want}`);
  near(gapTarget(holeAtStart, 0.26), 0.41);
  near(gapTarget(holeAtStart, 0.35), 0.41); // in the hole itself
});

test("no jump while the playhead can still play, or when the hole is big", () => {
  assert.equal(gapTarget(holeAtStart, 0.05), null); // 0.25 s still ahead of it
  assert.equal(gapTarget(holeAtStart, 10), null);
  assert.equal(gapTarget([[0, 5], [8, 20]], 4.9), null); // a 3 s hole: data is missing, wait for it
  assert.equal(gapTarget([], 0), null);
});

test("buffered-ahead counts across small holes, so reading pauses", () => {
  assert.ok(Math.abs(aheadAcross(holeAtStart, 0.26) - 61.34) < 1e-9);
  assert.ok(Math.abs(aheadAcross(holeAtStart, 0.35) - 61.25) < 1e-9); // playhead in the hole: not 0
  assert.equal(aheadAcross([[0, 5], [8, 20]], 1), 4); // a big hole stops the count
  assert.equal(aheadAcross([], 3), 0);
});
