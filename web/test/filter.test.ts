import { test } from "node:test";
import assert from "node:assert/strict";
import { applyFilters, type V1Comment, type V1Thread } from "../src/filter.ts";
import { defaults, merge } from "../src/settings.ts";

let no = 0;
const c = (body: string, commands: string[] = [], extra: Partial<V1Comment> = {}): V1Comment => ({
  id: `c${++no}`, no, vposMs: no * 100, body, commands, userId: "u", isPremium: false, score: 0,
  postedAt: "2026-01-02T10:00:00+09:00", nicoruCount: 0, nicoruId: null, source: "trunk", isMyPost: false, ...extra,
});

const threads = (): V1Thread[] => [
  { id: "1", fork: "owner", commentCount: 2, comments: [c("@デフォルト", ["red"]), c("投稿者 NG語")] },
  { id: "2", fork: "main", commentCount: 8, comments: [
    c("普通"), c("NG語を含む"), c("上", ["ue"]), c("下", ["shita", "big"]), c("赤", ["#ff0000"]),
    c("■■\n■■", ["ca", "small"]), c("匿名", ["184"]), c("x", [], { userId: "bad", score: -5000, postedAt: "2026-02-01T00:00:00+09:00" }),
  ] },
  { id: "2", fork: "easy", commentCount: 1, comments: [c("かんたん")] },
];

const run = (edit: (s: ReturnType<typeof defaults>) => void) => {
  const s = defaults();
  s.filters.capPerSecond = 0; // the rule tests below are about the other filters
  edit(s);
  return applyFilters(threads(), s.filters, s.comments);
};

test("no settings hides nothing", () => {
  const { stats, threads: out } = run(() => {});
  assert.equal(stats.total, 11);
  assert.equal(stats.shown, 11);
  assert.deepEqual(out.map((t) => t.fork), ["owner", "main", "easy"]);
});

test("each rule removes its comments and is counted", () => {
  const { stats } = run((s) => {
    s.filters.ngWords = ["NG語"];
    s.filters.ngUsers = ["bad"];
    s.filters.hide.ue = true;
    s.filters.hide.colored = true;
    s.filters.hide.ca = true;
    s.filters.hide.anonymous = true;
    s.comments.forks.easy = false;
  });
  assert.deepEqual(stats.byRule, { word: 1, user: 1, ue: 1, colored: 1, ca: 1, anonymous: 1, fork: 1 });
  // Owner comments ignore viewer filters (投稿者 NG語 stays).
  assert.equal(stats.byFork.owner.shown, 2);
});

test("scripts, NG share, time cutoff, commands, regex", () => {
  assert.equal(run((s) => (s.comments.runScripts = false)).stats.byRule.script, 1);
  assert.equal(run((s) => (s.filters.ngShare = "medium")).stats.byRule.ngShare, 1);
  assert.equal(run((s) => (s.filters.ngShare = "weak")).stats.byRule.ngShare, undefined);
  assert.equal(run((s) => (s.filters.postedBefore = "2026-01-15")).stats.byRule.time, 1);
  assert.equal(run((s) => (s.filters.ngCommands = ["BIG"])).stats.byRule.command, 1);
  assert.equal(run((s) => (s.filters.ngWords = ["/^普/"])).stats.byRule.word, 1);
  const bad = run((s) => (s.filters.ngWords = ["/(/"]));
  assert.deepEqual(bad.stats.badPatterns, ["/(/"]);
  assert.equal(bad.stats.shown, 11);
});

test("hide scrolling keeps fixed comments", () => {
  const { stats } = run((s) => (s.filters.hide.naka = true));
  assert.equal(stats.byRule.naka, 7); // main without ue/shita (5) + easy (1) + ...
});

test("cap keeps a stable, proportional subset and spares owner and CA", () => {
  const many = (): V1Thread[] => [
    { id: "1", fork: "owner", commentCount: 3, comments: [c("@デフォルト"), c("o1"), c("o2")] },
    { id: "2", fork: "main", commentCount: 2000, comments: Array.from({ length: 2000 }, (_, i) => c(i % 100 === 0 ? "■" : `m${i}`, i % 100 === 0 ? ["ca"] : [], { id: `m-${i}`, no: i, vposMs: i * 50, userId: i % 100 === 0 ? "artist" : `v${i}` })) },
  ];
  const s = defaults();
  s.filters.capPerSecond = 2; // 100 s video → limit 200
  const a = applyFilters(many(), s.filters, s.comments, 100);
  const b = applyFilters(many(), s.filters, s.comments, 100);
  assert.equal(a.stats.cap.limit, 200);
  assert.ok(Math.abs(a.stats.shown - 200) <= 30, `shown ${a.stats.shown}`);
  assert.deepEqual(a.threads[1].comments.map((x) => x.id), b.threads[1].comments.map((x) => x.id));
  assert.equal(a.threads[0].comments.length, 3);
  assert.equal(a.threads[1].comments.filter((x) => x.commands.includes("ca")).length, 20);
  // Density keeps its shape: every tenth of the video keeps about a tenth,
  // and kept ids are not clustered (longest run of consecutive kept ids).
  const kept = a.threads[1].comments.filter((x) => !x.commands.includes("ca"));
  for (let d = 0; d < 10; d++) {
    const n = kept.filter((x) => x.vposMs >= d * 10000 && x.vposMs < (d + 1) * 10000).length;
    assert.ok(n >= 8 && n <= 30, `tenth ${d}: ${n}`);
  }
  let run = 0, longest = 0, prev = -2;
  for (const x of kept) { run = x.no === prev + 1 ? run + 1 : 1; longest = Math.max(longest, run); prev = x.no; }
  assert.ok(longest <= 4, `longest run of consecutive kept comments ${longest}`);
  // Under the budget nothing is dropped; 0 disables the cap; minimum 100.
  assert.equal(applyFilters(many(), s.filters, s.comments, 2000).stats.byRule.cap, undefined);
  s.filters.capPerSecond = 0;
  assert.equal(applyFilters(many(), s.filters, s.comments, 100).stats.shown, 2003);
  s.filters.capPerSecond = 0.5;
  assert.equal(applyFilters(many(), s.filters, s.comments, 10).stats.cap.limit, 100);
});

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

// Artists rarely use a "ca" command: a picture is many comments at one
// moment, often single lines with full/ender/patissier, from one account.
// The cap must keep all of them, or the picture falls apart.
test("cap keeps every comment of a comment-art author, without a ca command", () => {
  const art: V1Comment[] = [];
  for (let i = 0; i < 12; i++) art.push(c(`■□■□■□■□ ${i}`, ["ue", "small", "full"], { userId: "artist", vposMs: 50000 }));
  const multi = c("■\n■\n■\n■", ["shita"], { userId: "other", vposMs: 60000 });
  const crowd: V1Comment[] = [];
  for (let i = 0; i < 400; i++) crowd.push(c(`普通 ${i}`, [], { userId: `v${i}`, vposMs: i * 1000 }));
  const s = defaults();
  s.filters.capPerSecond = 1;
  const { threads: out, stats } = applyFilters([{ id: "9", fork: "main", commentCount: 413, comments: [...art, multi, ...crowd] }], s.filters, s.comments, 120);
  const kept = new Set(out.flatMap((t) => t.comments.map((x) => x.id)));
  assert.ok(stats.byRule.cap! > 0, "the cap is in effect");
  assert.deepEqual(art.filter((x) => !kept.has(x.id)).map((x) => x.body), [], "all lines of the art are kept");
  assert.ok(kept.has(multi.id), "a comment of more than two lines is kept");
});

test("hide comment art and max length use the same recognition", () => {
  const art = c("■□■□■□■□■□■□■□■□■□■□■□■□", ["ue", "ender"], { userId: "artist2" });
  const plain = c("ふつうのながいコメントふつうのながいコメント", [], { userId: "p" });
  const s = defaults();
  s.filters.capPerSecond = 0;
  s.filters.maxLength = 10;
  let { threads: out } = applyFilters([{ id: "9", fork: "main", commentCount: 2, comments: [art, plain] }], s.filters, s.comments);
  assert.deepEqual(out.flatMap((t) => t.comments.map((x) => x.id)), [art.id], "max length spares the art");
  s.filters.maxLength = 0;
  s.filters.hide.ca = true;
  ({ threads: out } = applyFilters([{ id: "9", fork: "main", commentCount: 2, comments: [art, plain] }], s.filters, s.comments));
  assert.deepEqual(out.flatMap((t) => t.comments.map((x) => x.id)), [plain.id], "hide comment art hides it");
});

test("keepCA is on by default and a version 1 document takes it once", () => {
  assert.equal(defaults().comments.keepCA, true);
  assert.equal(merge({ v: 1, comments: { keepCA: false } }).comments.keepCA, true, "v1: the stored false was the old default");
  assert.equal(merge({ comments: { keepCA: false } }).comments.keepCA, true, "no version: as v1");
  assert.equal(merge({ v: 2, comments: { keepCA: false } }).comments.keepCA, false, "v2: the user's choice");
  assert.equal(merge({ v: 1 }).v, 2);
});
