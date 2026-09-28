import { test } from "node:test";
import assert from "node:assert/strict";
import { defaults, merge } from "../src/settings.ts";

// Filtering itself moved to the window (internal/filter, with the page's
// former cases and an equivalence set); the settings document stays here.

test("merge keeps valid values and repairs the rest", () => {
  const s = merge({ comments: { opacity: 7, mode: "weird", forks: { owner: false }, scale: "big" }, filters: { ngWords: ["a", 3] }, videos: { k: { offsetMs: 120 }, bad: { offsetMs: "x" } }, junk: 1 });
  assert.equal(s.comments.opacity, 1);
  assert.equal(s.comments.mode, "default");
  assert.equal(s.comments.forks.owner, false);
  assert.equal(s.comments.forks.main, true);
  assert.equal(s.comments.scale, 1);
  assert.deepEqual(s.filters.ngWords, ["a"]);
  assert.deepEqual(s.videos, { k: { offsetMs: 120 } });
  assert.deepEqual(merge(null), defaults());
});

test("keepCA is on by default and a version 1 document takes it once", () => {
  assert.equal(defaults().comments.keepCA, true);
  assert.equal(merge({ v: 1, comments: { keepCA: false } }).comments.keepCA, true, "v1: the stored false was the old default");
  assert.equal(merge({ comments: { keepCA: false } }).comments.keepCA, true, "no version: as v1");
  assert.equal(merge({ v: 2, comments: { keepCA: false } }).comments.keepCA, false, "v2: the user's choice");
  assert.equal(merge({ v: 1 }).v, 2);
});

test("the notebook for 记一笔 is kept; its file defaults to Jusplay.md", () => {
  const s = merge({ notes: { notebook: "D:\notes" } });
  assert.equal(s.notes.notebook, "D:\notes");
  assert.equal(s.notes.file, "Jusplay.md");
  assert.deepEqual(merge({ notes: { notebook: 5 } }).notes, defaults().notes);
});
