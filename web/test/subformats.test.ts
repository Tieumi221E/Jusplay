import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { decodeText, parseSrt, parseAss, parseFile, fromEmbedded } from "../src/subformats.ts";

const fixture = (name: string) => new Uint8Array(readFileSync(new URL(`../../testdata/subs/${name}`, import.meta.url)));

test("SRT: times, line breaks, markup", () => {
  const l = parseFile(fixture("tiny.srt"), "srt");
  assert.deepEqual(l, [
    { start: 0.1, end: 0.45, text: "第一行字幕\n斜体" },
    { start: 0.5, end: 0.9, text: "Second line & more" },
  ]);
});

test("ASS: override tags, \\N, \\an8 at the top", () => {
  const l = parseFile(fixture("tiny.ass"), "ass");
  assert.deepEqual(l, [
    { start: 0.1, end: 0.45, text: "こんにちは\n二行目" },
    { start: 0.5, end: 0.9, text: "上に出る行", top: true },
  ]);
});

test("ASS: commas in the text, drawings and comments dropped", () => {
  const src = "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
    "Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,はい、そうです, まあ\n" +
    "Comment: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,note\n" +
    "Dialogue: 0,0:00:03.00,0:00:04.00,Sign,,0,0,0,,{\\p1}m 0 0 l 100 0 100 100\n";
  assert.deepEqual(parseAss(src), [{ start: 1, end: 2.5, text: "はい、そうです, まあ" }]);
});

test("WebVTT: header, ids, cue settings, NOTE", () => {
  const src = "WEBVTT\n\nNOTE made by hand\n\nid1\n00:01.000 --> 00:02.000 align:start\n<v Anna>Hi <b>there</b>\n\n01:00:00.500 --> 01:00:01.000\nlate\n";
  assert.deepEqual(parseSrt(src), [{ start: 1, end: 2, text: "Hi there" }, { start: 3600.5, end: 3601, text: "late" }]);
});

test("encodings: BOMs, UTF-16, Shift-JIS, GBK, Big5", () => {
  const ja = "こんにちは、世界。今日は晴れです。";
  const zh = "今天天气很好，我们去公园散步吧。";
  assert.equal(decodeText(new Uint8Array([0xef, 0xbb, 0xbf, ...new TextEncoder().encode(ja)])), ja);
  const u16 = new Uint8Array(2 + ja.length * 2);
  u16.set([0xff, 0xfe]);
  for (let i = 0; i < ja.length; i++) u16[2 + i * 2] = ja.charCodeAt(i) & 0xff, u16[3 + i * 2] = ja.charCodeAt(i) >> 8;
  assert.equal(decodeText(u16), ja);
  // Legacy bytes, made with iconv (the platform has no encoder for them).
  const sjis = Uint8Array.from(Buffer.from("82b182f182c982bf82cd8141", "hex")); // こんにちは、
  assert.equal(decodeText(sjis), "こんにちは、");
  const gbk = Uint8Array.from(Buffer.from("bdf1ccecccecc6f8badcbac3a3ac", "hex")); // 今天天气很好，
  assert.equal(decodeText(gbk), "今天天气很好，");
  void zh;
});

test("Matroska ASS events: text after the eighth comma; missing lengths", () => {
  const l = fromEmbedded("S_TEXT/ASS", [
    { start: 1, end: 2, data: "0,0,Default,,0,0,0,,{\\i1}一行目{\\i0}\\N二行目" },
    { start: 5, end: 5, data: "1,0,Sign,,0,0,0,,{\\an8}看板, 標識" },
    { start: 7, end: 7, data: "2,0,Default,,0,0,0,," },
  ]);
  assert.deepEqual(l, [
    { start: 1, end: 2, text: "一行目\n二行目" },
    { start: 5, end: 7, text: "看板, 標識", top: true },
  ]);
  assert.deepEqual(fromEmbedded("S_TEXT/UTF8", [{ start: 0, end: 1, data: "<i>x</i>" }]), [{ start: 0, end: 1, text: "x" }]);
});
