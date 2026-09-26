// WOFF2 (https://www.w3.org/TR/WOFF2/), written by us. The tables go into
// one Brotli stream (Node's zlib has Brotli). For TrueType-outline fonts
// (the emoji font) glyf and loca get the WOFF2 glyf transform (points as
// triplets in separate streams, loca and computable bounding boxes left
// out) and hmtx the hmtx transform (left side bearings equal to xMin left
// out); CFF fonts (Jus Sans) have nothing to transform.
//
//   node assets/woff2.mjs in.otf|in.ttf out.woff2
//
// Every file written is decoded again and compared with the input: tables
// byte for byte, glyf and hmtx glyph by glyph (points, on-curve flags,
// contours, instructions, components, boxes, advances, bearings). A
// mismatch is an error, so a written file is a correct one.

import { brotliCompressSync, brotliDecompressSync, constants } from "node:zlib";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// The spec's known-tag table: index = the 6-bit tag code.
const KNOWN = ["cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post", "cvt ", "fpgm", "glyf", "loca", "prep", "CFF ", "VORG",
  "EBDT", "EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea", "vmtx", "BASE", "GDEF", "GPOS", "GSUB", "EBSC", "JSTF",
  "MATH", "CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt", "avar", "bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx",
  "fvar", "gvar", "hsty", "just", "lcar", "mort", "morx", "opbd", "prop", "trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill"];

const pad4 = (n) => (n + 3) & ~3;

/** The tables of an sfnt (TrueType or CFF-flavoured OpenType), sorted by tag. */
export function readSfnt(buf) {
  const flavor = buf.readUInt32BE(0);
  const n = buf.readUInt16BE(4);
  const tables = [];
  for (let i = 0; i < n; i++) {
    const r = 12 + 16 * i;
    const tag = buf.toString("latin1", r, r + 4);
    const off = buf.readUInt32BE(r + 8), len = buf.readUInt32BE(r + 12);
    if (off + len > buf.length) throw new Error(`table ${tag} runs past the end`);
    tables.push({ tag, data: buf.subarray(off, off + len) });
  }
  tables.sort((a, b) => (a.tag < b.tag ? -1 : a.tag > b.tag ? 1 : 0));
  return { flavor, tables };
}

// ---- variable-length integers ----

function base128(n) {
  const out = [];
  do {
    out.unshift(n & 0x7f);
    n = Math.floor(n / 128);
  } while (n > 0);
  for (let i = 0; i < out.length - 1; i++) out[i] |= 0x80;
  return out;
}

function u255(n, out) {
  if (n < 253) out.push(n);
  else if (n < 506) out.push(255, n - 253);
  else if (n < 762) out.push(254, n - 506);
  else out.push(253, n >> 8, n & 0xff);
}

class Reader {
  constructor(buf, pos = 0) { this.b = buf; this.p = pos; }
  u8() { return this.b[this.p++]; }
  u16() { const v = this.b.readUInt16BE(this.p); this.p += 2; return v; }
  i16() { const v = this.b.readInt16BE(this.p); this.p += 2; return v; }
  u32() { const v = this.b.readUInt32BE(this.p); this.p += 4; return v; }
  bytes(n) { const v = this.b.subarray(this.p, this.p + n); this.p += n; return v; }
  u255() {
    const c = this.u8();
    if (c === 253) return this.u16();
    if (c === 255) return 253 + this.u8();
    if (c === 254) return 506 + this.u8();
    return c;
  }
}

// ---- glyf: parse into glyphs ----

// A glyph: {kind: "empty"} | {kind: "simple", bbox, endPts, instr, pts: [[x, y, on]], overlap}
//        | {kind: "composite", bbox, comp (raw component records), instr}
function parseGlyphs(glyf, loca, numGlyphs, longLoca) {
  const off = (i) => (longLoca ? loca.readUInt32BE(4 * i) : 2 * loca.readUInt16BE(2 * i));
  const glyphs = [];
  for (let g = 0; g < numGlyphs; g++) {
    const a = off(g), b = off(g + 1);
    if (b <= a) {
      glyphs.push({ kind: "empty" });
      continue;
    }
    const r = new Reader(glyf.subarray(a, b));
    const n = r.i16();
    const bbox = [r.i16(), r.i16(), r.i16(), r.i16()];
    // No contours: nothing to draw, stored as an empty glyph (as WOFF2 does).
    if (n === 0) {
      glyphs.push({ kind: "empty" });
      continue;
    }
    if (n > 0) {
      const endPts = [];
      for (let i = 0; i < n; i++) endPts.push(r.u16());
      const instr = r.bytes(r.u16());
      const count = n ? endPts[n - 1] + 1 : 0;
      const flags = [];
      while (flags.length < count) {
        const f = r.u8();
        flags.push(f);
        if (f & 8) for (let k = r.u8(); k > 0; k--) flags.push(f);
      }
      const pts = [];
      let x = 0;
      for (const f of flags) {
        if (f & 2) x += (f & 16 ? 1 : -1) * r.u8();
        else if (!(f & 16)) x += r.i16();
        pts.push([x, 0, f & 1]);
      }
      let y = 0;
      flags.forEach((f, i) => {
        if (f & 4) y += (f & 32 ? 1 : -1) * r.u8();
        else if (!(f & 32)) y += r.i16();
        pts[i][1] = y;
      });
      glyphs.push({ kind: "simple", bbox, endPts, instr, pts, overlap: count > 0 && (flags[0] & 0x40) !== 0 });
    } else {
      const start = r.p;
      let flags, instrs = false;
      do {
        flags = r.u16();
        r.u16(); // glyph index
        r.p += flags & 1 ? 4 : 2;
        if (flags & 8) r.p += 2;
        else if (flags & 0x40) r.p += 4;
        else if (flags & 0x80) r.p += 8;
        if (flags & 0x100) instrs = true;
      } while (flags & 0x20);
      const comp = r.b.subarray(start, r.p);
      const instr = instrs ? r.bytes(r.u16()) : null;
      glyphs.push({ kind: "composite", bbox, comp, instr });
    }
  }
  return glyphs;
}

function computedBox(pts) {
  if (!pts.length) return [0, 0, 0, 0];
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const [x, y] of pts) {
    if (x < x0) x0 = x;
    if (x > x1) x1 = x;
    if (y < y0) y0 = y;
    if (y > y1) y1 = y;
  }
  return [x0, y0, x1, y1];
}

const sameBox = (a, b) => a[0] === b[0] && a[1] === b[1] && a[2] === b[2] && a[3] === b[3];

// A point delta as a WOFF2 triplet: the flag byte (index into the spec's
// 128-entry table, on-curve in the high bit) and its 1-4 data bytes.
function triplet(dx, dy, on, flags, data) {
  const ax = Math.abs(dx), ay = Math.abs(dy);
  const oc = on ? 0 : 128;
  const xs = dx < 0 ? 0 : 1, ys = dy < 0 ? 0 : 1, xys = xs + 2 * ys;
  if (dx === 0 && ay < 1280) {
    flags.push(oc + ((ay & 0xf00) >> 7) + ys);
    data.push(ay & 0xff);
  } else if (dy === 0 && ax < 1280) {
    flags.push(oc + 10 + ((ax & 0xf00) >> 7) + xs);
    data.push(ax & 0xff);
  } else if (ax < 65 && ay < 65) {
    flags.push(oc + 20 + ((ax - 1) & 0x30) + (((ay - 1) & 0x30) >> 2) + xys);
    data.push((((ax - 1) & 0xf) << 4) | ((ay - 1) & 0xf));
  } else if (ax < 769 && ay < 769) {
    flags.push(oc + 84 + 12 * (((ax - 1) & 0x300) >> 8) + (((ay - 1) & 0x300) >> 6) + xys);
    data.push((ax - 1) & 0xff, (ay - 1) & 0xff);
  } else if (ax < 4096 && ay < 4096) {
    flags.push(oc + 120 + xys);
    data.push(ax >> 4, ((ax & 0xf) << 4) | (ay >> 8), ay & 0xff);
  } else {
    flags.push(oc + 124 + xys);
    data.push(ax >> 8, ax & 0xff, ay >> 8, ay & 0xff);
  }
}

// The spec's triplet table, as a decoder reads it: [bytes, xBits, yBits, dx, dy, xSign, ySign].
const TRIPLETS = (() => {
  const t = [];
  for (let i = 0; i < 10; i++) t.push([2, 0, 8, 0, (i >> 1) * 256, 0, i & 1 ? 1 : -1]);
  for (let i = 0; i < 10; i++) t.push([2, 8, 0, (i >> 1) * 256, 0, i & 1 ? 1 : -1, 0]);
  for (let hi = 0; hi < 4; hi++)
    for (let lo = 0; lo < 4; lo++)
      for (let s = 0; s < 4; s++) t.push([2, 4, 4, 1 + 16 * hi, 1 + 16 * lo, s & 1 ? 1 : -1, s & 2 ? 1 : -1]);
  for (let hi = 0; hi < 3; hi++)
    for (let lo = 0; lo < 3; lo++)
      for (let s = 0; s < 4; s++) t.push([3, 8, 8, 1 + 256 * hi, 1 + 256 * lo, s & 1 ? 1 : -1, s & 2 ? 1 : -1]);
  for (let s = 0; s < 4; s++) t.push([4, 12, 12, 0, 0, s & 1 ? 1 : -1, s & 2 ? 1 : -1]);
  for (let s = 0; s < 4; s++) t.push([5, 16, 16, 0, 0, s & 1 ? 1 : -1, s & 2 ? 1 : -1]);
  return t;
})();

function readTriplet(flag, r) {
  const [bytes, xb, yb, dx0, dy0, xs, ys] = TRIPLETS[flag & 0x7f];
  const d = [];
  for (let i = 1; i < bytes; i++) d.push(r.u8());
  let x = 0, y = 0;
  if (bytes === 2) {
    if (xb === 8) x = d[0];
    else if (yb === 8) y = d[0];
    else {
      x = d[0] >> 4;
      y = d[0] & 0xf;
    }
  } else if (bytes === 3) {
    x = d[0];
    y = d[1];
  } else if (bytes === 4) {
    x = (d[0] << 4) | (d[1] >> 4);
    y = ((d[1] & 0xf) << 8) | d[2];
  } else {
    x = (d[0] << 8) | d[1];
    y = (d[2] << 8) | d[3];
  }
  return [xs * (x + dx0), ys * (y + dy0), !(flag & 0x80)];
}

function transformGlyf(glyphs, indexFormat) {
  const n = glyphs.length;
  const nContour = Buffer.alloc(2 * n), nPoints = [], flags = [], glyph = [], composite = [], instructions = [];
  const bitmap = Buffer.alloc(4 * Math.floor((n + 31) / 32)), boxes = [];
  const overlap = Buffer.alloc(Math.floor((n + 7) / 8));
  let anyOverlap = false;
  glyphs.forEach((g, i) => {
    const explicit = (box) => {
      bitmap[i >> 3] |= 0x80 >> (i & 7);
      boxes.push(...box);
    };
    if (g.kind === "empty") return nContour.writeInt16BE(0, 2 * i);
    if (g.kind === "composite") {
      nContour.writeInt16BE(-1, 2 * i);
      composite.push(...g.comp);
      if (g.instr) {
        u255(g.instr.length, glyph);
        instructions.push(...g.instr);
      }
      return explicit(g.bbox);
    }
    nContour.writeInt16BE(g.endPts.length, 2 * i);
    let prev = -1;
    for (const e of g.endPts) {
      u255(e - prev, nPoints);
      prev = e;
    }
    let x = 0, y = 0;
    for (const [px, py, on] of g.pts) {
      triplet(px - x, py - y, on, flags, glyph);
      x = px;
      y = py;
    }
    u255(g.instr.length, glyph);
    instructions.push(...g.instr);
    if (!sameBox(computedBox(g.pts), g.bbox)) explicit(g.bbox);
    if (g.overlap) {
      overlap[i >> 3] |= 0x80 >> (i & 7);
      anyOverlap = true;
    }
  });
  const bbox = Buffer.alloc(bitmap.length + 2 * boxes.length);
  bitmap.copy(bbox);
  boxes.forEach((v, k) => bbox.writeInt16BE(v, bitmap.length + 2 * k));
  const streams = [nContour, Buffer.from(nPoints), Buffer.from(flags), Buffer.from(glyph), Buffer.from(composite), bbox, Buffer.from(instructions)];
  const head = Buffer.alloc(36);
  head.writeUInt16BE(0, 0);
  head.writeUInt16BE(anyOverlap ? 1 : 0, 2);
  head.writeUInt16BE(n, 4);
  head.writeUInt16BE(indexFormat, 6);
  streams.forEach((s, k) => head.writeUInt32BE(s.length, 8 + 4 * k));
  return Buffer.concat([head, ...streams, ...(anyOverlap ? [overlap] : [])]);
}

function untransformGlyf(t) {
  const r = new Reader(t);
  r.u16();
  const option = r.u16(), n = r.u16(), indexFormat = r.u16();
  const sizes = [];
  for (let k = 0; k < 7; k++) sizes.push(r.u32());
  let p = r.p;
  const s = sizes.map((len) => {
    const b = t.subarray(p, p + len);
    p += len;
    return new Reader(b);
  });
  const [nc, np, fl, gl, co, bb, ins] = s;
  const bitmapLen = 4 * Math.floor((n + 31) / 32);
  bb.p = bitmapLen;
  const overlap = option & 1 ? t.subarray(p, p + Math.floor((n + 7) / 8)) : null;
  const glyphs = [];
  for (let i = 0; i < n; i++) {
    const c = nc.i16();
    const hasBox = (bb.b[i >> 3] & (0x80 >> (i & 7))) !== 0;
    const box = () => [bb.i16(), bb.i16(), bb.i16(), bb.i16()];
    if (c === 0) {
      if (hasBox) throw new Error(`glyph ${i}: an empty glyph with a box`);
      glyphs.push({ kind: "empty" });
    } else if (c < 0) {
      const start = co.p;
      let flags, instrs = false;
      do {
        flags = co.u16();
        co.u16();
        co.p += flags & 1 ? 4 : 2;
        if (flags & 8) co.p += 2;
        else if (flags & 0x40) co.p += 4;
        else if (flags & 0x80) co.p += 8;
        if (flags & 0x100) instrs = true;
      } while (flags & 0x20);
      const comp = co.b.subarray(start, co.p);
      const instr = instrs ? ins.bytes(gl.u255()) : null;
      if (!hasBox) throw new Error(`glyph ${i}: a composite glyph without a box`);
      glyphs.push({ kind: "composite", bbox: box(), comp, instr });
    } else {
      const endPts = [];
      let e = -1;
      for (let k = 0; k < c; k++) endPts.push((e += np.u255()));
      const pts = [];
      let x = 0, y = 0;
      for (let k = 0; k <= e; k++) {
        const [dx, dy, on] = readTriplet(fl.u8(), gl);
        x += dx;
        y += dy;
        pts.push([x, y, on ? 1 : 0]);
      }
      const instr = ins.bytes(gl.u255());
      const bbox = hasBox ? box() : computedBox(pts);
      glyphs.push({ kind: "simple", bbox, endPts, instr, pts, overlap: overlap ? (overlap[i >> 3] & (0x80 >> (i & 7))) !== 0 : false });
    }
  }
  return { glyphs, indexFormat };
}

function sameGlyph(a, b) {
  if (a.kind !== b.kind) return false;
  if (a.kind === "empty") return true;
  if (!sameBox(a.bbox, b.bbox)) return false;
  if (a.kind === "composite") return a.comp.equals(b.comp) && (a.instr ? !!b.instr && a.instr.equals(b.instr) : !b.instr);
  return a.overlap === b.overlap && a.instr.equals(b.instr) && a.endPts.join() === b.endPts.join() &&
    a.pts.length === b.pts.length && a.pts.every((p, k) => p[0] === b.pts[k][0] && p[1] === b.pts[k][1] && !!p[2] === !!b.pts[k][2]);
}

// ---- hmtx ----

function readHmtx(hmtx, numHMetrics, numGlyphs) {
  const adv = [], lsb = [];
  for (let i = 0; i < numHMetrics; i++) {
    adv.push(hmtx.readUInt16BE(4 * i));
    lsb.push(hmtx.readInt16BE(4 * i + 2));
  }
  for (let i = numHMetrics; i < numGlyphs; i++) lsb.push(hmtx.readInt16BE(4 * numHMetrics + 2 * (i - numHMetrics)));
  return { adv, lsb };
}

const xMin = (g) => (g.kind === "empty" ? 0 : g.bbox[0]);

/** The hmtx transform, or null when no bearing can be left out. */
function transformHmtx(hmtx, numHMetrics, glyphs) {
  const { adv, lsb } = readHmtx(hmtx, numHMetrics, glyphs.length);
  const propOk = lsb.slice(0, numHMetrics).every((v, i) => v === xMin(glyphs[i]));
  const monoOk = lsb.slice(numHMetrics).every((v, i) => v === xMin(glyphs[numHMetrics + i]));
  if (!propOk && !monoOk) return null;
  const parts = [Buffer.from([(propOk ? 1 : 0) | (monoOk ? 2 : 0)])];
  const w = (vals, signed) => {
    const b = Buffer.alloc(2 * vals.length);
    vals.forEach((v, i) => (signed ? b.writeInt16BE(v, 2 * i) : b.writeUInt16BE(v, 2 * i)));
    return b;
  };
  parts.push(w(adv, false));
  if (!propOk) parts.push(w(lsb.slice(0, numHMetrics), true));
  if (!monoOk) parts.push(w(lsb.slice(numHMetrics), true));
  return Buffer.concat(parts);
}

function untransformHmtx(t, numHMetrics, glyphs) {
  const r = new Reader(t);
  const f = r.u8();
  const adv = [], lsb = [];
  for (let i = 0; i < numHMetrics; i++) adv.push(r.u16());
  for (let i = 0; i < numHMetrics; i++) lsb.push(f & 1 ? xMin(glyphs[i]) : r.i16());
  for (let i = numHMetrics; i < glyphs.length; i++) lsb.push(f & 2 ? xMin(glyphs[i]) : r.i16());
  return { adv, lsb };
}

// ---- the file ----

export function encode(sfnt) {
  const { flavor, tables } = readSfnt(sfnt);
  const get = (tag) => tables.find((t) => t.tag === tag)?.data;
  const out = tables.map((t) => ({ tag: t.tag, orig: t.data, data: t.data, version: t.tag === "glyf" || t.tag === "loca" ? 3 : 0, transformed: false }));
  const glyf = get("glyf"), loca = get("loca"), head = get("head"), maxp = get("maxp"), hhea = get("hhea"), hmtx = get("hmtx");
  let upper = 0; // how large the decoder's glyf may come out, at most
  if (glyf && loca && head && maxp) {
    const numGlyphs = maxp.readUInt16BE(4), longLoca = head.readInt16BE(50) === 1;
    const glyphs = parseGlyphs(glyf, loca, numGlyphs, longLoca);
    const g = out.find((t) => t.tag === "glyf"), l = out.find((t) => t.tag === "loca");
    g.data = transformGlyf(glyphs, longLoca ? 1 : 0);
    g.version = 0;
    g.transformed = true;
    l.data = Buffer.alloc(0);
    l.version = 0;
    l.transformed = true;
    for (const x of glyphs) {
      if (x.kind === "simple") upper += pad4(12 + 2 * x.endPts.length + x.instr.length + 5 * x.pts.length);
      else if (x.kind === "composite") upper += pad4(12 + x.comp.length + (x.instr ? x.instr.length : 0));
    }
    if (hmtx && hhea) {
      const th = transformHmtx(hmtx, hhea.readUInt16BE(34), glyphs);
      if (th) {
        const h = out.find((t) => t.tag === "hmtx");
        h.data = th;
        h.version = 1;
        h.transformed = true;
      }
    }
  }
  // When glyf is transformed, loca follows it in the directory.
  const gi = out.findIndex((t) => t.tag === "glyf");
  if (gi >= 0 && out[gi].transformed) {
    const li = out.findIndex((t) => t.tag === "loca");
    const [l] = out.splice(li, 1);
    out.splice(out.findIndex((t) => t.tag === "glyf") + 1, 0, l);
  }
  const dir = [];
  for (const t of out) {
    const k = KNOWN.indexOf(t.tag);
    dir.push((t.version << 6) | (k >= 0 ? k : 63));
    if (k < 0) dir.push(...Buffer.from(t.tag, "latin1"));
    dir.push(...base128(t.orig.length));
    if (t.transformed) dir.push(...base128(t.data.length));
  }
  const stream = Buffer.concat(out.map((t) => t.data));
  const compressed = brotliCompressSync(stream, {
    params: {
      [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_FONT,
      [constants.BROTLI_PARAM_QUALITY]: 11,
      [constants.BROTLI_PARAM_LGWIN]: 24,
      [constants.BROTLI_PARAM_SIZE_HINT]: stream.length,
    },
  });
  // The decoded font's size: the tables as they come back, padded (glyf and
  // loca are rebuilt by the decoder; an upper bound for those).
  let totalSfntSize = 12 + 16 * out.length;
  for (const t of out) {
    if (t.tag === "glyf" && t.transformed) totalSfntSize += Math.max(pad4(t.orig.length), upper);
    else totalSfntSize += pad4(t.orig.length);
  }
  const headerLen = 48;
  const length = pad4(headerLen + dir.length + compressed.length);
  const w = Buffer.alloc(length);
  w.write("wOF2", 0, "latin1");
  w.writeUInt32BE(flavor, 4);
  w.writeUInt32BE(length, 8);
  w.writeUInt16BE(out.length, 12);
  w.writeUInt16BE(0, 14);
  w.writeUInt32BE(totalSfntSize, 16);
  w.writeUInt32BE(compressed.length, 20);
  w.writeUInt16BE(1, 24); // font version, informational
  w.writeUInt16BE(0, 26);
  Buffer.from(dir).copy(w, headerLen);
  compressed.copy(w, headerLen + dir.length);
  return w;
}

/** Decode a WOFF2: its tables by tag, transformed ones as written (with version and original length). */
export function decode(woff2) {
  if (woff2.toString("latin1", 0, 4) !== "wOF2") throw new Error("not WOFF2");
  if (woff2.readUInt32BE(8) !== woff2.length) throw new Error("length field does not match the file");
  const n = woff2.readUInt16BE(12);
  const compLen = woff2.readUInt32BE(20);
  let p = 48;
  const entries = [];
  const b128 = () => {
    let v = 0;
    for (let b; ; ) {
      b = woff2[p++];
      v = v * 128 + (b & 0x7f);
      if (!(b & 0x80)) return v;
    }
  };
  for (let i = 0; i < n; i++) {
    const flags = woff2[p++];
    let tag = KNOWN[flags & 0x3f];
    if ((flags & 0x3f) === 63) {
      tag = woff2.toString("latin1", p, p + 4);
      p += 4;
    }
    const version = flags >> 6;
    const orig = b128();
    const transformed = tag === "glyf" || tag === "loca" ? version === 0 : version !== 0;
    entries.push({ tag, version, orig, len: transformed ? b128() : orig, transformed });
  }
  const stream = brotliDecompressSync(woff2.subarray(p, p + compLen));
  const out = new Map();
  let o = 0;
  for (const e of entries) {
    out.set(e.tag, { ...e, data: stream.subarray(o, o + e.len) });
    o += e.len;
  }
  if (o !== stream.length) throw new Error("table lengths do not add up to the stream");
  return out;
}

/** Encode, then decode and compare with the input: throws on any difference. */
export function encodeChecked(sfnt) {
  const w = encode(sfnt);
  const back = decode(w);
  const { tables } = readSfnt(sfnt);
  if (back.size !== tables.length) throw new Error(`round trip: ${back.size} tables, expected ${tables.length}`);
  const get = (tag) => tables.find((t) => t.tag === tag)?.data;
  let glyphs = null;
  for (const t of tables) {
    const b = back.get(t.tag);
    if (!b) throw new Error(`round trip: table ${t.tag} missing`);
    if (b.orig !== t.data.length) throw new Error(`round trip: table ${t.tag} has the wrong original length`);
    if (!b.transformed) {
      if (!b.data.equals(t.data)) throw new Error(`round trip: table ${t.tag} differs`);
      continue;
    }
    if (t.tag === "glyf") {
      const head = get("head"), maxp = get("maxp");
      const numGlyphs = maxp.readUInt16BE(4), longLoca = head.readInt16BE(50) === 1;
      const want = parseGlyphs(t.data, get("loca"), numGlyphs, longLoca);
      const got = untransformGlyf(b.data);
      if (got.glyphs.length !== want.length || got.indexFormat !== (longLoca ? 1 : 0)) throw new Error("round trip: glyph count or loca format differs");
      want.forEach((g, i) => {
        if (!sameGlyph(g, got.glyphs[i])) throw new Error(`round trip: glyph ${i} differs`);
      });
      glyphs = want;
    }
  }
  const hm = back.get("hmtx");
  if (hm?.transformed) {
    const hhea = get("hhea"), numHMetrics = hhea.readUInt16BE(34);
    const want = readHmtx(get("hmtx"), numHMetrics, glyphs.length);
    const got = untransformHmtx(hm.data, numHMetrics, glyphs);
    if (want.adv.join() !== got.adv.join() || want.lsb.join() !== got.lsb.join()) throw new Error("round trip: hmtx differs");
  }
  return w;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [, , input, output] = process.argv;
  if (!input || !output) {
    console.error("usage: node assets/woff2.mjs in.otf|in.ttf out.woff2");
    process.exit(2);
  }
  const src = readFileSync(input);
  const w = encodeChecked(src);
  writeFileSync(output, w);
  console.log(`${output}: ${w.length} bytes (${src.length} in), round trip checked`);
}
