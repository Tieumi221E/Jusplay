// The comment report: what the analysis found (internal/danmaku), laid out
// as a document one could hand on — a summary in words first, then numbered
// figures with captions, the data's provenance and how every number was
// made. Charts are drawn here (SVG and plain elements, no chart library),
// by the rules of the data-visualisation method: a validated categorical
// palette in a fixed order, thin marks, a 2 px surface gap between fills,
// hairline solid grids, a legend wherever colour carries identity, and
// every value readable without hovering (hover adds detail, never hides it).

import { L } from "./i18n.ts";
import { fmtTime } from "./ui.ts";

// ---- the report as the server sends it (danmaku.Report) ----

export interface Count { name: string; count: number }
interface CatCount { name: string; count: number; phrases: Count[] | null }
interface Phrase { text: string; count: number; category: string; lang: string; peak: number; spread: number; hist: number[] | null }
interface Term { text: string; count: number; cohesion: number; entropy: number; hist?: number[] | null; topic: number }
interface Topic { terms: string[]; count: number; hist: number[]; peak: number }
interface Family { top: string; count: number; variants: string[] | null; size: number }
export interface HotspotR { Start: number; Peak: number; End: number; Count: number; Rate: number; Baseline: number; Z: number; phrases: Count[] | null; mix: Count[] | null }
interface Liked { text: string; at: number; nicoru: number }
export interface Report {
  comments: number;
  distinct: number;
  duration: number;
  perSecond: number[];
  bucketSec: number;
  catBuckets: number[][];
  hotspots: HotspotR[] | null;
  categories: CatCount[];
  languages: Count[];
  phrases: Phrase[] | null;
  terms: Term[] | null;
  families: Family[] | null;
  topics: Topic[] | null;
  users: { distinct: number; top1Share: number; top10Share: number; gini: number; median: number; anonymousShare: number; premiumShare: number };
  posting: { First?: string; Median?: string; Last?: string; firstDayShare: number; days: Count[] | null; hours: number[] };
  style: Count[];
  forks: Count[];
  length: Count[];
  nicoru: Liked[] | null;
  ngScore: Count[];
}

export interface ReportContext {
  title: string;
  subtitle: string;
  /** Seek to a Niconico time (seconds) and close the report. */
  seek: (nicoSeconds: number) => void;
  /** Comment offset in seconds (Niconico time − video time). */
  offset: number;
  /** Rows for the provenance table. */
  provenance: [string, string][];
}

// The server's category ids, in the report's own order (danmaku.CategoryNames).
const CAT_IDS = ["other", "laugh", "applause", "cute", "praise", "surprise", "sad", "tsukkomi", "agree", "ritual", "meme", "mention", "commentary", "art", "owner"];

// Colour groups: eight hued slots in the validated order (the order is what
// keeps adjacent fills apart for colour-blind readers), and "unclassified"
// in neutral grey. Categories past eight are folded into one slot, never
// given a ninth hue.
interface Group { id: string; cats: string[]; name: () => string; slot: number }
const GROUPS: Group[] = [
  { id: "laugh", cats: ["laugh"], name: () => L("笑", "笑い"), slot: 1 },
  { id: "surprise", cats: ["surprise"], name: () => L("惊讶", "驚き"), slot: 2 },
  { id: "love", cats: ["cute", "praise"], name: () => L("喜爱", "好き・称賛"), slot: 3 },
  { id: "tsukkomi", cats: ["tsukkomi"], name: () => L("吐槽", "ツッコミ"), slot: 4 },
  { id: "meme", cats: ["meme"], name: () => L("梗 / 常见说法", "ネタ・定番"), slot: 5 },
  { id: "mention", cats: ["mention"], name: () => L("提到角色 / 名词", "キャラ・用語"), slot: 6 },
  { id: "commentary", cats: ["commentary"], name: () => L("感想", "感想"), slot: 7 },
  { id: "misc", cats: ["agree", "sad", "applause", "ritual", "art", "owner"], name: () => L("其他反应", "その他の反応"), slot: 8 },
];
const OTHER = { id: "other", name: () => L("未分类", "未分類") };

const catName = (id: string): string => ({
  other: L("未分类", "未分類"), laugh: L("笑", "笑い"), applause: L("鼓掌", "拍手"), cute: L("可爱", "かわいい"), praise: L("称赞", "称賛"),
  surprise: L("惊讶", "驚き"), sad: L("感伤", "しんみり"), tsukkomi: L("吐槽", "ツッコミ"), agree: L("同意", "共感"), ritual: L("打招呼 / 仪式", "挨拶・定番"),
  meme: L("梗 / 常见说法", "ネタ・定番"), mention: L("提到角色 / 名词", "キャラ・用語"), commentary: L("感想", "感想"),
  art: L("评论画", "コメントアート"), owner: L("投稿者", "投稿者"),
} as Record<string, string>)[id] ?? id;

const groupOf = (cat: string): Group | null => GROUPS.find((g) => g.cats.includes(cat)) ?? null;
const colorOf = (cat: string): string => {
  const g = groupOf(cat);
  return g ? `var(--rc${g.slot})` : "var(--rc0)";
};

const langName = (id: string): string => ({
  ja: L("日语", "日本語"), zh: L("中文", "中国語"), han: L("仅汉字（中日难分）", "漢字のみ（中日判別不可）"), ko: L("韩语", "韓国語"),
  en: L("英语", "英語"), none: L("无文字（符号、数字）", "文字なし（記号・数字）"),
} as Record<string, string>)[id] ?? id;

const styleName = (id: string): string => ({
  anonymous: L("匿名（184）", "匿名（184）"), colored: L("彩色", "色付き"), big: L("大字", "大"), small: L("小字", "小"),
  ue: L("顶端固定", "上固定"), shita: L("底端固定", "下固定"), switch: "Switch", art: L("评论画", "コメントアート"),
} as Record<string, string>)[id] ?? id;

// ---- small helpers ----

type Kid = Node | string | null | undefined | false;
function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls = "", ...kids: Kid[]): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  for (const k of kids) if (k) e.append(k);
  return e;
}
const NS = "http://www.w3.org/2000/svg";
function svgEl<K extends keyof SVGElementTagNameMap>(tag: K, attrs: Record<string, string | number> = {}): SVGElementTagNameMap[K] {
  const e = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, String(v));
  return e;
}
/** A comment shortened to n characters, whole in its title. */
function clip(text: string, n = 18): HTMLElement {
  const chars = [...text];
  const span = el("span", "rp-clip", chars.length > n ? chars.slice(0, n).join("") + "…" : text);
  if (chars.length > n) span.title = text;
  return span;
}
const nf = () => new Intl.NumberFormat(L("zh-CN", "ja-JP"));
const num = (n: number) => nf().format(Math.round(n));
const pct = (x: number, digits = 1) => `${(x * 100).toFixed(digits)}%`;
const dec = (x: number, d = 1) => new Intl.NumberFormat(L("zh-CN", "ja-JP"), { maximumFractionDigits: d, minimumFractionDigits: d }).format(x);

// ---- the tooltip, shared by every chart ----

class Tip {
  constructor(private readonly box: HTMLElement) {}
  show(e: MouseEvent, lines: (string | [string, string])[]): void {
    this.box.replaceChildren(...lines.map((l) => (typeof l === "string" ? el("div", "tip-head", l) : el("div", "tip-row", el("span", "", l[0]), el("b", "", l[1])))));
    this.box.hidden = false;
    const pad = 14, w = this.box.offsetWidth, h = this.box.offsetHeight;
    let x = e.clientX + pad, y = e.clientY + pad;
    if (x + w > innerWidth - 8) x = e.clientX - w - pad;
    if (y + h > innerHeight - 8) y = e.clientY - h - pad;
    this.box.style.transform = `translate(${x}px, ${y}px)`;
  }
  hide(): void {
    this.box.hidden = true;
  }
}

// ---- figures ----

let figNo = 0;
function figure(title: string, caption: string, ...body: Kid[]): HTMLElement {
  figNo++;
  return el("figure", "rp-fig",
    el("figcaption", "", el("span", "rp-fig-no", `${L("图", "図")} ${figNo}`), el("span", "rp-fig-title", title)),
    ...body,
    caption ? el("p", "rp-caption", caption) : null);
}

function section(title: string, ...body: Kid[]): HTMLElement {
  return el("section", "rp-section", el("h2", "", title), ...body);
}

/** Horizontal bars: one row per item, label, bar, value; bar colours given per row. */
function hbars(tip: Tip, rows: { label: string; value: number; color?: string; note?: string; detail?: [string, string][]; onClick?: () => void }[], opts: { total?: number; valueText?: (v: number) => string } = {}): HTMLElement {
  const max = Math.max(1, ...rows.map((r) => r.value));
  const box = el("div", "rp-hbars");
  for (const r of rows) {
    const bar = el("div", "rp-bar");
    bar.style.setProperty("--w", `${(r.value / max) * 100}%`);
    bar.style.setProperty("--c", r.color ?? "var(--rc1)");
    const value = opts.valueText ? opts.valueText(r.value) : num(r.value);
    const share = opts.total ? pct(r.value / opts.total) : "";
    const row = el("div", r.onClick ? "rp-hrow link" : "rp-hrow",
      el("span", "rp-hlabel", r.label),
      el("span", "rp-htrack", bar),
      el("span", "rp-hval", value, share ? el("small", "", ` ${share}`) : null));
    if (r.note) row.append(el("span", "rp-hnote", r.note));
    row.addEventListener("mousemove", (e) => tip.show(e, [r.label, [L("数量", "件数"), value], ...(share ? [[L("占比", "割合"), share] as [string, string]] : []), ...(r.detail ?? [])]));
    row.addEventListener("mouseleave", () => tip.hide());
    if (r.onClick) row.addEventListener("click", r.onClick);
    box.append(row);
  }
  return box;
}

/** Columns: equal bands, one hue (magnitude), labels under every nth band. */
function columns(tip: Tip, items: { label: string; value: number; tipLabel?: string }[], every: number, height = 150): HTMLElement {
  const max = Math.max(1, ...items.map((i) => i.value));
  const box = el("div", "rp-cols");
  box.style.setProperty("--h", `${height}px`);
  items.forEach((it, i) => {
    const col = el("div", "rp-col", el("i"));
    (col.firstChild as HTMLElement).style.height = `${(it.value / max) * 100}%`;
    col.append(el("span", "rp-col-label", i % every === 0 ? it.label : ""));
    col.addEventListener("mousemove", (e) => tip.show(e, [it.tipLabel ?? it.label, [L("弹幕", "コメント"), num(it.value)]]));
    col.addEventListener("mouseleave", () => tip.hide());
    box.append(col);
  });
  // The scale: the tallest column's value, so no value depends on hovering.
  box.append(el("span", "rp-cols-max", num(max)));
  return box;
}

function tile(label: string, value: string, note = ""): HTMLElement {
  return el("div", "rp-tile", el("span", "rp-tile-label", label), el("span", "rp-tile-value", value), note ? el("span", "rp-tile-note", note) : null);
}

function legend(items: { name: string; color: string }[]): HTMLElement {
  return el("div", "rp-legend", ...items.map((i) => {
    const sw = el("i");
    sw.style.background = i.color;
    return el("span", "", sw, i.name);
  }));
}

/** A count with a short bar in a fixed track (the bar never leaves its cell). */
function countCell(value: number, max: number): HTMLElement {
  const bar = el("i");
  bar.style.width = `${(value / Math.max(1, max)) * 100}%`;
  return el("span", "rp-countcell", el("span", "", num(value)), el("span", "rp-track", bar));
}

/** Where in the episode something was said: an area over the time stretches. */
function sparkline(hist: number[] | null | undefined, color = "var(--rc1)", w = 120, h = 22): SVGElement {
  const svg = svgEl("svg", { width: w, height: h, viewBox: `0 0 ${w} ${h}`, class: "rp-spark", "aria-hidden": "true" });
  const hs = hist ?? [];
  if (!hs.length) return svg;
  const max = Math.max(1, ...hs), step = w / hs.length;
  const pts = hs.map((v, i) => `${(i + 0.5) * step},${h - 1 - (v / max) * (h - 3)}`);
  svg.append(svgEl("polygon", { points: `0,${h - 1} ${pts.join(" ")} ${w},${h - 1}`, fill: color, "fill-opacity": 0.22 }));
  svg.append(svgEl("polyline", { points: pts.join(" "), fill: "none", stroke: color, "stroke-width": 1.5, "stroke-linejoin": "round" }));
  return svg;
}

const topicFill = (i: number) => (i >= 0 && i < 8 ? `var(--rc${i + 1})` : "var(--rc0)");
const topicText = (i: number) => (i >= 0 && i < 8 ? `var(--rt${i + 1})` : "var(--muted)");
const topicName = (tp: Topic) => tp.terms.slice(0, 2).join(" · ");

/** A word cloud of the terms: bigger for more mentions, coloured by topic. */
function wordCloud(terms: Term[], topics: Topic[], tip: Tip): HTMLElement {
  const wrap = el("div", "rp-cloud");
  const draw = () => {
    const W = Math.max(320, wrap.clientWidth), H = 300;
    const words = terms.slice(0, 60);
    if (!words.length) return;
    const max = Math.max(...words.map((t) => t.count)), min = Math.min(...words.map((t) => t.count));
    const size = (c: number) => 13 + (max === min ? 1 : Math.sqrt((c - min) / (max - min))) * 33;
    const g = document.createElement("canvas").getContext("2d")!;
    const font = getComputedStyle(wrap).fontFamily;
    const placed: [number, number, number, number][] = [];
    const svg = svgEl("svg", { width: W, height: H, viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": L("词云", "ワードクラウド") });
    for (const t of words) {
      const fs = size(t.count);
      g.font = `700 ${fs}px ${font}`;
      const w = g.measureText(t.text).width + 6, h = fs * 1.1;
      // Out from the centre along a spiral, flattened to the cloud's shape.
      for (let a = 0; a < 90; a += 0.12) {
        const x = W / 2 + a * 4.2 * Math.cos(a) * (W / H) * 0.62 - w / 2;
        const y = H / 2 + a * 4.2 * Math.sin(a) * 0.62 - h / 2;
        if (x < 2 || y < 2 || x + w > W - 2 || y + h > H - 2) continue;
        if (placed.some(([px, py, pw, ph]) => x < px + pw && x + w > px && y < py + ph && y + h > py)) continue;
        placed.push([x, y, w, h]);
        const text = svgEl("text", { x: x + w / 2, y: y + h * 0.8, "text-anchor": "middle", "font-size": fs, class: "rp-word" });
        text.style.fill = topicText(t.topic);
        text.textContent = t.text;
        text.addEventListener("mousemove", (e) => tip.show(e, [t.text, [L("提到的弹幕", "含むコメント"), num(t.count)],
          ...(t.topic >= 0 && topics[t.topic] ? [[L("话题", "話題"), topicName(topics[t.topic])] as [string, string]] : [])]));
        text.addEventListener("mouseleave", () => tip.hide());
        svg.append(text);
        break;
      }
    }
    wrap.replaceChildren(svg);
  };
  new ResizeObserver(() => draw()).observe(wrap);
  return wrap;
}

// ---- the timeline: reactions over the episode, stacked, with the hotspots ----

function timeline(rep: Report, tip: Tip, ctx: ReportContext, classifiedOnly = false): HTMLElement {
  const wrap = el("div", "rp-timeline");
  const draw = () => {
    const W = Math.max(320, wrap.clientWidth), H = classifiedOnly ? 200 : 250, padL = 44, padR = 12, padT = classifiedOnly ? 16 : 40, padB = 26;
    const pw = W - padL - padR, ph = H - padT - padB;
    const nb = rep.catBuckets.length, bs = rep.bucketSec;
    // Per bucket: the stack of the eight groups, then unclassified on top.
    const series = [...GROUPS.map((g) => g.cats.map((c) => CAT_IDS.indexOf(c))), ...(classifiedOnly ? [] : [[CAT_IDS.indexOf("other")]])];
    const stacks = rep.catBuckets.map((b) => {
      let acc = 0;
      return series.map((idx) => {
        const v = idx.reduce((s, i) => s + (b[i] ?? 0), 0);
        const lo = acc;
        acc += v;
        return [lo, acc] as [number, number];
      });
    });
    const perMin = (v: number) => (v / bs) * 60;
    const maxV = Math.max(1, ...stacks.map((s) => s[s.length - 1][1]));
    const niceMax = niceCeil(perMin(maxV));
    const x = (i: number) => padL + ((i + 0.5) / nb) * pw;
    const y = (v: number) => padT + ph - (perMin(v) / niceMax) * ph;
    const svg = svgEl("svg", { width: W, height: H, viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": L("弹幕反应随时间变化", "時間に沿ったコメントの反応") });
    // Grid and y labels: hairline, solid, recessive.
    for (let k = 0; k <= 4; k++) {
      const v = (niceMax * k) / 4, yy = padT + ph - (v / niceMax) * ph;
      svg.append(svgEl("line", { x1: padL, x2: W - padR, y1: yy, y2: yy, class: "rp-grid" }));
      const t = svgEl("text", { x: padL - 6, y: yy + 4, class: "rp-axis", "text-anchor": "end" });
      t.textContent = num(v);
      svg.append(t);
    }
    const unit = svgEl("text", { x: padL - 6, y: padT - 12, class: "rp-axis", "text-anchor": "end" });
    unit.textContent = L("条/分", "件/分");
    svg.append(unit);
    // Areas, bottom up; the 2 px surface gap between them is a stroke in the surface colour.
    series.forEach((_, si) => {
      const top = stacks.map((s, i) => `${x(i)},${y(s[si][1])}`);
      const bot = stacks.map((s, i) => `${x(i)},${y(s[si][0])}`).reverse();
      const fill = si < GROUPS.length ? `var(--rc${GROUPS[si].slot})` : "var(--rc0)";
      svg.append(svgEl("polygon", { points: [...top, ...bot].join(" "), fill, class: "rp-area" }));
    });
    // Time axis: a label every few minutes.
    const dur = rep.duration, step = dur > 3600 ? 600 : dur > 1200 ? 300 : 120;
    for (let t = 0; t <= dur; t += step) {
      const xx = padL + (t / dur) * pw;
      const tx = svgEl("text", { x: xx, y: H - 8, class: "rp-axis", "text-anchor": "middle" });
      tx.textContent = fmtTime(t);
      svg.append(tx);
    }
    // Hotspots: a numbered marker over each peak, strongest first; a marker
    // too close to one already placed goes up a row, so none overlap.
    const placed: { x: number; row: number }[] = [];
    (classifiedOnly ? [] : rep.hotspots ?? []).slice(0, 8).forEach((h, i) => {
      const xx = padL + (h.Peak / dur) * pw;
      let row = 0;
      while (placed.some((p) => p.row === row && Math.abs(p.x - xx) < 19)) row++;
      placed.push({ x: xx, row });
      const cy = padT - 12 - row * 18;
      svg.append(svgEl("line", { x1: xx, x2: xx, y1: cy + 8, y2: padT + ph, class: "rp-hot-line" }));
      svg.append(svgEl("circle", { cx: xx, cy, r: 8, class: "rp-hot-dot" }));
      const n = svgEl("text", { x: xx, y: cy + 3.5, class: "rp-hot-no", "text-anchor": "middle" });
      n.textContent = String(i + 1);
      svg.append(n);
    });
    // Hover: a crosshair and the bucket's breakdown; click: go there.
    const cross = svgEl("line", { y1: padT, y2: padT + ph, class: "rp-cross", visibility: "hidden" });
    svg.append(cross);
    const hit = svgEl("rect", { x: padL, y: 0, width: pw, height: H, fill: "transparent", class: "rp-hit" });
    const at = (e: MouseEvent) => Math.min(nb - 1, Math.max(0, Math.floor(((e.offsetX - padL) / pw) * nb)));
    hit.addEventListener("mousemove", (e) => {
      const i = at(e), b = rep.catBuckets[i];
      cross.setAttribute("x1", String(x(i)));
      cross.setAttribute("x2", String(x(i)));
      cross.setAttribute("visibility", "visible");
      const total = b.reduce((s, v) => s + v, 0);
      const rows: [string, string][] = GROUPS.map((g) => [g.name(), num(g.cats.reduce((s, c) => s + (b[CAT_IDS.indexOf(c)] ?? 0), 0))] as [string, string]).filter((r) => r[1] !== "0");
      tip.show(e, [`${fmtTime(Math.max(0, i * bs - ctx.offset))}–${fmtTime(Math.max(0, (i + 1) * bs - ctx.offset))} · ${num(total)} ${L("条", "件")}`, ...rows, [OTHER.name(), num(b[0] ?? 0)], L("点击跳到这里", "クリックでこの位置へ")]);
    });
    hit.addEventListener("mouseleave", () => {
      cross.setAttribute("visibility", "hidden");
      tip.hide();
    });
    hit.addEventListener("click", (e) => ctx.seek(at(e) * bs));
    svg.append(hit);
    wrap.replaceChildren(svg);
  };
  new ResizeObserver(() => draw()).observe(wrap);
  return el("div", "", legend([...GROUPS.map((g) => ({ name: g.name(), color: `var(--rc${g.slot})` })), ...(classifiedOnly ? [] : [{ name: OTHER.name(), color: "var(--rc0)" }])]), wrap);
}

function niceCeil(v: number): number {
  const p = 10 ** Math.floor(Math.log10(Math.max(v, 1)));
  for (const m of [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10]) if (m * p >= v) return m * p;
  return 10 * p;
}

// ---- the summary in words ----

function summary(rep: Report, ctx: ReportContext): HTMLElement {
  const N = rep.comments, mins = rep.duration / 60;
  const cats = new Map(rep.categories.map((c) => [c.name, c.count]));
  const unclassified = cats.get("other") ?? 0;
  const groups = GROUPS.map((g) => ({ g, n: g.cats.reduce((s, c) => s + (cats.get(c) ?? 0), 0) })).sort((a, b) => b.n - a.n);
  const classified = N - unclassified;
  const top = rep.hotspots?.[0];
  const phrase = rep.phrases?.[0];
  const termTexts = (rep.terms ?? []).filter((t) => t.text !== phrase?.text).slice(0, 3).map((t) => t.text);
  const quoted = (xs: string[]) => L(xs.map((x) => `“${x}”`).join("、"), xs.map((x) => `「${x}」`).join("、"));
  const lines: string[] = [];
  lines.push(L(`本集共有 ${num(N)} 条弹幕，平均每分钟 ${dec(N / Math.max(mins, 1 / 60))} 条，来自 ${num(rep.users.distinct)} 个账号，其中 ${pct(rep.users.anonymousShare, 0)} 以匿名（184）发出。`,
    `このエピソードのコメントは ${num(N)} 件、1 分あたり平均 ${dec(N / Math.max(mins, 1 / 60))} 件で、${num(rep.users.distinct)} アカウントから投稿され、うち ${pct(rep.users.anonymousShare, 0)} が匿名（184）です。`));
  if (top) {
    const said = top.phrases?.[0]?.name;
    lines.push(L(`最热闹的时刻在 ${fmtTime(top.Peak - ctx.offset)}：每秒约 ${dec(top.Rate)} 条，是前后两分钟常态的 ${dec(top.Rate / Math.max(top.Baseline, 0.5))} 倍${said ? `，观众集中在说“${said}”` : ""}。全集共识别出 ${rep.hotspots!.length} 个高能时刻。`,
      `最も盛り上がったのは ${fmtTime(top.Peak - ctx.offset)}：毎秒約 ${dec(top.Rate)} 件で、前後 2 分の平常時の ${dec(top.Rate / Math.max(top.Baseline, 0.5))} 倍${said ? `、「${said}」が集中しました` : ""}。盛り上がりどころは全部で ${rep.hotspots!.length} か所です。`));
  }
  if (classified > 0 && groups[0].n > 0) {
    const a = groups[0], b = groups[1];
    lines.push(L(`能归类的弹幕占 ${pct(classified / N, 0)}，其中以${a.g.name()}（${pct(a.n / classified, 0)}）${b && b.n ? `和${b.g.name()}（${pct(b.n / classified, 0)}）` : ""}最多。`,
      `分類できたのは ${pct(classified / N, 0)} で、多いのは${a.g.name()}（${pct(a.n / classified, 0)}）${b && b.n ? `と${b.g.name()}（${pct(b.n / classified, 0)}）` : ""}です。`));
  }
  if (phrase) {
    lines.push(L(`出现最多的一句是“${phrase.text}”（${num(phrase.count)} 次）${termTexts.length ? `；弹幕里反复出现的词还有 ${quoted(termTexts)}` : ""}。`,
      `最も多いコメントは「${phrase.text}」（${num(phrase.count)} 回）${termTexts.length ? `。よく出る語には ${quoted(termTexts)} もあります` : ""}。`));
  }
  const tp = rep.topics?.[0];
  if (tp && tp.terms.length >= 2) {
    lines.push(L(`讨论最多的话题围绕 ${quoted(tp.terms.slice(0, 3))}，集中在 ${fmtTime(Math.max(0, tp.peak - ctx.offset))} 前后。`,
      `最も話題になったのは ${quoted(tp.terms.slice(0, 3))} で、${fmtTime(Math.max(0, tp.peak - ctx.offset))} 前後に集中しています。`));
  }
  if (rep.posting.days && rep.posting.days.length > 1) {
    lines.push(L(`弹幕在 ${rep.posting.days.length} 天内陆续发出，其中 ${pct(rep.posting.firstDayShare, 0)} 集中在第一条弹幕后的 24 小时内。`,
      `コメントは ${rep.posting.days.length} 日にわたって投稿され、${pct(rep.posting.firstDayShare, 0)} が最初のコメントから 24 時間以内です。`));
  }
  return el("div", "rp-summary", ...lines.map((l) => el("p", "", l)));
}

// ---- the whole report ----

export function renderReport(root: HTMLElement, tipBox: HTMLElement, rep: Report, ctx: ReportContext): void {
  figNo = 0;
  const tip = new Tip(tipBox);
  const N = rep.comments;
  const at = (nico: number) => fmtTime(Math.max(0, nico - ctx.offset));
  const peak = rep.hotspots?.[0];
  const tiles = el("div", "rp-tiles",
    tile(L("弹幕总数", "コメント数"), num(N)),
    tile(L("平均密度", "平均密度"), dec(N / Math.max(rep.duration / 60, 1 / 60)), L("条 / 分钟", "件 / 分")),
    tile(L("峰值密度", "ピーク密度"), peak ? dec(peak.Rate) : "—", peak ? L(`条 / 秒 · ${at(peak.Peak)}`, `件 / 秒 · ${at(peak.Peak)}`) : ""),
    tile(L("参与账号", "アカウント"), num(rep.users.distinct), L(`人均中位 ${num(rep.users.median)} 条`, `中央値 ${num(rep.users.median)} 件`)),
    tile(L("不同的说法", "異なる言い回し"), num(rep.distinct), L("合并写法差异后", "表記ゆれをまとめた後")),
    tile(L("首日占比", "初日の割合"), pct(rep.posting.firstDayShare, 0), L("24 小时内发出", "24 時間以内")));

  const hotspots = (rep.hotspots ?? []).slice(0, 8);
  const hotList = el("ol", "rp-hotlist", ...hotspots.map((h, i) => {
    const li = el("li", "link",
      el("span", "rp-hot-badge", String(i + 1)),
      el("div", "",
        el("div", "rp-hot-time", `${at(h.Start)} – ${at(h.End)}`),
        el("div", "rp-hot-stat", L(`${num(h.Count)} 条 · 每秒 ${dec(h.Rate)} 条 · 常态的 ${dec(h.Rate / Math.max(h.Baseline, 0.5))} 倍`,
          `${num(h.Count)} 件 · 毎秒 ${dec(h.Rate)} 件 · 平常時の ${dec(h.Rate / Math.max(h.Baseline, 0.5))} 倍`)),
        el("div", "rp-chips", ...(h.phrases ?? []).slice(0, 4).map((p) => el("span", "rp-chip", clip(p.name, 14), el("small", "", ` ${num(p.count)}`))))));
    li.addEventListener("click", () => ctx.seek(h.Start));
    return li;
  }));

  // Categories, largest first; unclassified last, in grey.
  const cats = rep.categories.filter((c) => c.count > 0);
  const catRows = [...cats.filter((c) => c.name !== "other").sort((a, b) => b.count - a.count), ...cats.filter((c) => c.name === "other")];
  const catBars = hbars(tip, catRows.map((c) => ({
    label: catName(c.name), value: c.count, color: colorOf(c.name),
    note: (c.phrases ?? []).slice(0, 3).map((p) => ([...p.name].length > 12 ? [...p.name].slice(0, 12).join("") + "…" : p.name)).join(" · "),
    detail: (c.phrases ?? []).slice(0, 5).map((p) => [p.name, num(p.count)] as [string, string]),
  })), { total: N });

  const phrases = (rep.phrases ?? []).slice(0, 30);
  const maxP = Math.max(1, ...phrases.map((p) => p.count));
  const phraseTable = el("table", "rp-table",
    el("thead", "", el("tr", "", ...[L("排名", "順位"), L("弹幕", "コメント"), L("次数", "回数"), L("反应", "反応"), L("分布", "分布"), L("高峰", "ピーク"), L("集中度", "集中度")].map((h) => el("th", "", h)))),
    el("tbody", "", ...phrases.map((p, i) => {
      const dot = el("i", "rp-dot");
      dot.style.background = colorOf(p.category);
      const time = el("td", "link rp-num", at(p.peak));
      time.addEventListener("click", () => ctx.seek(Math.max(0, p.peak - 3)));
      return el("tr", "",
        el("td", "rp-num rp-muted", String(i + 1)),
        el("td", "rp-text", clip(p.text, 24)),
        el("td", "rp-num", countCell(p.count, maxP)),
        el("td", "rp-nowrap", dot, catName(p.category)),
        el("td", "", sparkline(p.hist)),
        time,
        el("td", "rp-muted", p.spread >= 0.6 ? L(`集中（${pct(p.spread, 0)}）`, `集中（${pct(p.spread, 0)}）`) : L(`分散（${pct(p.spread, 0)}）`, `分散（${pct(p.spread, 0)}）`)));
    })));

  const topics = rep.topics ?? [];
  const terms = (rep.terms ?? []).slice(0, 24);
  const maxT = Math.max(1, ...terms.map((t) => t.count));
  const termTable = el("table", "rp-table",
    el("thead", "", el("tr", "", ...[L("词", "言葉"), L("提到的弹幕", "含むコメント"), L("分布", "分布"), L("话题", "話題")].map((h) => el("th", "", h)))),
    el("tbody", "", ...terms.map((t) => {
      const dot = el("i", "rp-dot");
      dot.style.background = topicFill(t.topic);
      return el("tr", "",
        el("td", "rp-text", clip(t.text, 16)),
        el("td", "rp-num", countCell(t.count, maxT)),
        el("td", "", sparkline(t.hist, topicFill(t.topic))),
        el("td", "rp-muted rp-nowrap", ...(t.topic >= 0 && topics[t.topic] ? [dot, topicName(topics[t.topic])] : ["—"])));
    })));
  const maxTopic = Math.max(1, ...topics.map((tp) => tp.count));
  const topicList = el("div", "rp-topics", ...topics.map((tp, i) => {
    const sw = el("i", "rp-dot");
    sw.style.background = topicFill(i);
    const time = el("span", "link rp-num rp-topic-peak", at(tp.peak));
    time.addEventListener("click", () => ctx.seek(Math.max(0, tp.peak - 10)));
    return el("div", "rp-topic",
      el("div", "rp-topic-head", sw, el("b", "", tp.terms[0]), el("span", "rp-muted", tp.terms.slice(1).join(" · "))),
      el("div", "rp-topic-row", countCell(tp.count, maxTopic), sparkline(tp.hist, topicFill(i), 220, 26), time));
  }));

  const fams = (rep.families ?? []).slice(0, 12);
  const famList = el("div", "rp-families", ...fams.map((f) => el("div", "rp-family",
    el("div", "rp-family-head", el("span", "rp-text", clip(f.top, 20)), el("span", "rp-num", `${num(f.count)} ${L("条", "件")}`)),
    el("div", "rp-muted rp-variants", L(`${f.size} 种写法：`, `${f.size} 通り：`), (f.variants ?? []).map((v) => [...v].length > 16 ? [...v].slice(0, 16).join("") + "…" : v).join(" · ")))));

  // When comments were posted.
  const days = rep.posting.days ?? [];
  const dayItems = days.length > 60
    ? weekly(days)
    : days.map((d) => ({ label: d.name.slice(5), value: d.count, tipLabel: d.name }));
  const hours = rep.posting.hours.map((v, h) => ({ label: `${h}`, value: v, tipLabel: L(`${h}:00–${h}:59（日本时间）`, `${h}:00–${h}:59（日本時間）`) }));

  const style = hbars(tip, rep.style.filter((s) => s.count > 0).sort((a, b) => b.count - a.count).map((s) => ({ label: styleName(s.name), value: s.count })), { total: N });
  const langs = hbars(tip, rep.languages.map((l) => ({ label: langName(l.name), value: l.count })), { total: N });
  const length = hbars(tip, rep.length.map((l) => ({ label: L(`${l.name} 字`, `${l.name} 文字`), value: l.count })), { total: N });
  const ng = hbars(tip, rep.ngScore.map((s) => ({ label: s.name, value: s.count })), { total: N });

  const liked = el("ol", "rp-liked", ...(rep.nicoru ?? []).map((c) => {
    const li = el("li", "link", el("span", "rp-text", clip(c.text, 36)), el("span", "rp-muted rp-num", `${at(c.at)} · ${num(c.nicoru)} ${L("ニコる", "ニコる")}`));
    li.addEventListener("click", () => ctx.seek(Math.max(0, c.at - 3)));
    return li;
  }));

  const u = rep.users;
  const userTiles = el("div", "rp-tiles small",
    tile(L("参与账号", "アカウント"), num(u.distinct)),
    tile(L("最活跃账号占比", "最多アカウントの割合"), pct(u.top1Share)),
    tile(L("前 10 个账号占比", "上位 10 アカウント"), pct(u.top10Share)),
    tile(L("集中度（基尼系数）", "偏り（ジニ係数）"), dec(u.gini, 2), L("0 为人人均等，1 为全部出自一人", "0 は均等、1 は一人に集中")),
    tile(L("匿名占比", "匿名の割合"), pct(u.anonymousShare)),
    tile(L("高级会员占比", "プレミアムの割合"), pct(u.premiumShare)));

  const print = el("button", "link rp-print", L("打印 / 导出 PDF", "印刷 / PDF に書き出す"));
  print.addEventListener("click", () => window.print());

  root.replaceChildren(
    el("div", "rp-actions", print),
    section(L("概览", "概要"), summary(rep, ctx), tiles),
    section(L("什么时候最热闹", "いつ盛り上がったか"),
      figure(L("弹幕随时间的变化", "時間に沿ったコメントの推移"), L("每分钟弹幕数，点击可跳转", "1 分あたりのコメント数。クリックで移動"), timeline(rep, tip, ctx)),
      figure(L("已归类的反应", "分類できた反応"), "", timeline(rep, tip, ctx, true)),
      figure(L("高能时刻", "盛り上がりどころ"), L("点击可跳转", "クリックで移動"),
        hotspots.length ? hotList : el("p", "rp-muted", L("没有特别突出的时段", "特に目立つ区間はありません")))),
    section(L("大家在说什么", "何が言われていたか"),
      figure(L("反应类型", "反応の種類"), L("占全部弹幕的比例", "全コメントに対する割合"), catBars),
      figure(L("词云", "ワードクラウド"), L("字越大提到越多，同色为同一话题", "大きいほど多く、同じ色は同じ話題"), wordCloud(rep.terms ?? [], topics, tip)),
      topics.length ? figure(L("话题", "話題"), L("经常一起出现的词，右侧为讨论的时间分布", "よく一緒に出る言葉。右は話題の時間分布"), topicList) : null,
      figure(L("出现最多的弹幕", "多かったコメント"), L("写法差异已合并", "表記ゆれはまとめて集計"), phraseTable),
      figure(L("常出现的词", "よく出る言葉"), "", termTable),
      figure(L("同一句话的不同写法", "同じセリフの書き方の違い"), "", famList)),
    section(L("谁在发、什么时候发", "誰が、いつ投稿したか"),
      userTiles,
      days.length > 1 ? figure(L("每天的弹幕数", "日ごとのコメント数"), days.length > 60 ? L("按周合计", "週ごとの合計") : "",
        columns(tip, dayItems, Math.ceil(dayItems.length / 12))) : null,
      figure(L("一天中各时段的弹幕数", "時間帯ごとのコメント数"), L("日本时间", "日本時間"), columns(tip, hours, 3, 120))),
    section(L("弹幕的样子", "コメントの見た目"),
      el("div", "rp-grid2",
        figure(L("样式", "スタイル"), "", style),
        figure(L("语言", "言語"), "", langs),
        figure(L("长度", "長さ"), "", length),
        figure(L("NG 共享分数", "NG 共有スコア"), L("越低越容易被隐藏", "低いほど非表示になりやすい"), ng)),
      (rep.nicoru ?? []).length ? figure(L("ニコる最多的弹幕", "ニコるが多かったコメント"), "", liked) : null),
    section(L("说明", "注記"),
      el("table", "rp-table rp-prov", el("tbody", "", ...ctx.provenance.map(([k, v]) => el("tr", "", el("th", "", k), el("td", "", v))))),
      el("ul", "rp-method",
        el("li", "", L("统计全部弹幕，不受 NG 设置影响；时间已按弹幕偏移换算。", "全コメントを集計（NG 設定の影響なし）。時刻はコメントのずれを反映。")),
        el("li", "", L("反应类型和词语由程序自动判断，不显示任何账号。", "反応と言葉は自動判定。アカウントは表示しません。")))));
}

function weekly(days: Count[]): { label: string; value: number; tipLabel: string }[] {
  const out: { label: string; value: number; tipLabel: string }[] = [];
  for (let i = 0; i < days.length; i += 7) {
    const w = days.slice(i, i + 7);
    out.push({ label: w[0].name.slice(5), value: w.reduce((s, d) => s + d.count, 0), tipLabel: `${w[0].name} – ${w[w.length - 1].name}` });
  }
  return out;
}
