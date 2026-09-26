// Controls, settings panel and info panel. The panel is built from a
// declarative list so every setting has one definition.

import type { Settings } from "./settings.ts";
import type { FilterStats, Rule } from "./filter.ts";
import { L, sep } from "./i18n.ts";

export const $ = <T extends HTMLElement = HTMLElement>(sel: string, root: ParentNode = document): T => {
  const e = root.querySelector<T>(sel);
  if (!e) throw new Error(`missing ${sel}`);
  return e;
};

function el<K extends keyof HTMLElementTagNameMap>(tag: K, props: Partial<HTMLElementTagNameMap[K]> & { class?: string } = {}, ...kids: (Node | string)[]): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  const { class: cls, ...rest } = props;
  Object.assign(e, rest);
  if (cls) e.className = cls;
  e.append(...kids);
  return e;
}

export function fmtTime(s: number): string {
  if (!Number.isFinite(s)) return "--:--";
  s = Math.max(0, s);
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = Math.floor(s % 60);
  const mm = String(m).padStart(h ? 2 : 1, "0"), ss = String(sec).padStart(2, "0");
  return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

let toastTimer = 0;
export function toast(msg: string, ms = 1600): void {
  const t = $("#toast");
  t.textContent = msg;
  t.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => t.classList.remove("show"), ms);
}

// ---- settings panel -------------------------------------------------------

type Getter<T> = (s: Settings) => T;
type Setter<T> = (s: Settings, v: T) => void;

type Field = { id: string } & (
  | { kind: "toggle"; label: string; get: Getter<boolean>; set: Setter<boolean>; hint?: string }
  | { kind: "range"; label: string; min: number; max: number; step: number; fmt: (v: number) => string; get: Getter<number>; set: Setter<number>; hint?: string }
  | { kind: "select"; label: string; options: [string, string, boolean?][]; get: Getter<string>; set: Setter<string>; hint?: string }
  | { kind: "color"; label: string; get: Getter<string>; set: Setter<string> }
  | { kind: "lines"; label: string; placeholder: string; get: Getter<string[]>; set: Setter<string[]>; hint?: string }
  | { kind: "datetime"; label: string; get: Getter<string>; set: Setter<string>; hint?: string }
  | { kind: "checks"; label: string; items: [string, Getter<boolean>, Setter<boolean>][]; hint?: string });

const pct = (v: number) => `${Math.round(v * 100)}%`;

export function ruleLabel(r: Rule): string {
  const labels: Record<Rule, [string, string]> = {
    fork: ["图层关闭", "レイヤー非表示"], script: ["投稿者脚本", "投稿者スクリプト"], time: ["投稿时间", "投稿日時"],
    ngShare: ["NG 共享", "NG 共有"], user: ["NG 用户", "NG ユーザー"], word: ["NG 词", "NG ワード"],
    command: ["NG 命令", "NG コマンド"], naka: ["滚动", "流れる"], ue: ["上固定", "上固定"], shita: ["下固定", "下固定"],
    colored: ["彩色", "色付き"], big: ["大字", "大"], small: ["小字", "小"], ca: ["评论画", "コメントアート"],
    anonymous: ["184 匿名", "184（匿名）"], length: ["过长", "長すぎる"], cap: ["总量上限", "総量上限"],
  };
  return L(...labels[r]);
}

const unlimited = () => L("不限", "無制限");

// Functions, not constants: the labels are read in the current language
// each time the panel is built.
const display = (): Field[] => [
  { id: "enabled", kind: "toggle", label: L("显示弹幕", "コメントを表示"), get: (s) => s.comments.enabled, set: (s, v) => (s.comments.enabled = v), hint: L("快捷键 C", "ショートカット C") },
  {
    id: "forks", kind: "checks", label: L("图层", "レイヤー"), items: [
      ["投稿者", (s) => s.comments.forks.owner, (s, v) => (s.comments.forks.owner = v)],
      [L("普通", "通常"), (s) => s.comments.forks.main, (s, v) => (s.comments.forks.main = v)],
      ["かんたん", (s) => s.comments.forks.easy, (s, v) => (s.comments.forks.easy = v)],
    ],
  },
  { id: "runScripts", kind: "toggle", label: L("执行投稿者脚本", "投稿者スクリプトを実行"), get: (s) => s.comments.runScripts, set: (s, v) => (s.comments.runScripts = v), hint: L("@デフォルト、@逆、@置換 等", "@デフォルト、@逆、@置換 など") },
  { id: "opacity", kind: "range", label: "不透明度", min: 0.05, max: 1, step: 0.05, fmt: pct, get: (s) => s.comments.opacity, set: (s, v) => (s.comments.opacity = v) },
  { id: "scale", kind: "range", label: L("字号", "文字サイズ"), min: 0.5, max: 2, step: 0.05, fmt: pct, get: (s) => s.comments.scale, set: (s, v) => (s.comments.scale = v), hint: L("按比例缩放，碰撞排布随之重算", "比率で拡大縮小し、重なりを避ける配置を計算し直します") },
  { id: "area", kind: "range", label: L("显示区域", "表示範囲"), min: 10, max: 100, step: 5, fmt: (v) => L(`上方 ${v}%`, `上から ${v}%`), get: (s) => s.comments.area, set: (s, v) => (s.comments.area = v), hint: L("只裁切下方，不重新排布", "下側を切り取るだけで、配置は変えません") },
  { id: "limit", kind: "range", label: L("同屏上限", "同時表示の上限"), min: 0, max: 300, step: 5, fmt: (v) => (v ? L(`${v} 条`, `${v} 件`) : unlimited()), get: (s) => s.comments.limit, set: (s, v) => (s.comments.limit = v) },
  { id: "limitOrder", kind: "select", label: L("超出上限时", "上限を超えたとき"), options: [["asc", L("先隐藏较旧的", "古いものから隠す")], ["desc", L("先隐藏较新的", "新しいものから隠す")]], get: (s) => s.comments.limitOrder, set: (s, v) => (s.comments.limitOrder = v as "asc" | "desc") },
  { id: "strokeColor", kind: "color", label: L("描边颜色", "縁取りの色"), get: (s) => s.comments.strokeColor, set: (s, v) => (s.comments.strokeColor = v) },
  { id: "strokeOpacity", kind: "range", label: L("描边不透明度", "縁取りの不透明度"), min: 0, max: 1, step: 0.05, fmt: pct, get: (s) => s.comments.strokeOpacity, set: (s, v) => (s.comments.strokeOpacity = v) },
  { id: "strokeWidth", kind: "range", label: L("描边粗细", "縁取りの太さ"), min: 0, max: 3, step: 0.1, fmt: pct, get: (s) => s.comments.strokeWidth, set: (s, v) => (s.comments.strokeWidth = v) },
  { id: "mode", kind: "select", label: L("渲染模式", "描画モード"), options: [["default", L("自动（按投稿时间）", "自動（投稿日時で判定）")], ["html5", "HTML5"], ["flash", "Flash"]], get: (s) => s.comments.mode, set: (s, v) => (s.comments.mode = v as Settings["comments"]["mode"]) },
  { id: "keepCA", kind: "toggle", label: L("评论画位置修正", "コメントアート位置補正"), get: (s) => s.comments.keepCA, set: (s, v) => (s.comments.keepCA = v), hint: "niconicomments keepCA" },
  { id: "frameRate", kind: "select", label: L("动画帧率", "描画フレームレート"), options: [["display", L("跟随显示器", "ディスプレイに合わせる")], ["60", L("60 帧", "60 fps")], ["video", L("跟随视频帧", "動画のフレームに合わせる")]], get: (s) => s.comments.frameRate, set: (s, v) => (s.comments.frameRate = v as "display" | "60" | "video") },
];

/**
 * The subtitle tracks of the video playing, for the two track rows: set by
 * the player page (subs.ts), since they are the video's, not settings.
 */
export const subSlots = {
  options: (): [string, string, boolean?][] => [["off", L("关", "オフ")]],
  get: (_slot: 0 | 1): string => "off",
  set: (_slot: 0 | 1, _key: string): void => undefined,
};

// Subtitles: which tracks, how big; AI: what is spoken, into what language.
const subtitleFields = (): Field[] => [
  {
    id: "subPick", kind: "select", label: L("字幕", "字幕"), options: subSlots.options(),
    get: () => subSlots.get(0), set: (_s, v) => subSlots.set(0, v),
    hint: L("外挂、内嵌或 AI 生成的字幕；快捷键 T 显示 / 隐藏", "外部・内蔵・AI 生成の字幕。ショートカット T で表示 / 非表示"),
  },
  {
    id: "subPick2", kind: "select", label: L("第二字幕", "第 2 字幕"), options: subSlots.options(),
    get: () => subSlots.get(1), set: (_s, v) => subSlots.set(1, v),
    hint: L("显示在主字幕下方，较小，例如原文", "メイン字幕の下に小さく表示（原文など）"),
  },
  { id: "subScale", kind: "range", label: L("字幕字号", "字幕の文字サイズ"), min: 0.5, max: 2, step: 0.05, fmt: pct, get: (s) => s.subtitles.scale, set: (s, v) => (s.subtitles.scale = v) },
  {
    id: "subAi", kind: "toggle", label: L("没有字幕时用 AI 生成", "字幕がなければ AI で作る"), get: (s) => s.subtitles.ai, set: (s, v) => (s.subtitles.ai = v),
    hint: L("视频没有外挂或内嵌字幕时，自动显示 AI 译文；也可随时在上方选择 AI 字幕", "外部・内蔵字幕がない動画では AI 翻訳を自動で表示します。上でいつでも AI 字幕を選べます"),
  },
  {
    id: "subSource", kind: "select", label: L("原声语言", "音声の言語"),
    options: [["", L("自动（按开头的台词判断）", "自動（最初のセリフで判断）")], ["Japanese", "日本語"], ["Chinese", "中文"], ["English", "English"], ["Korean", "한국어"], ["Cantonese", "粵語"], ["French", "Français"], ["German", "Deutsch"], ["Spanish", "Español"], ["Russian", "Русский"]],
    get: (s) => s.subtitles.source, set: (s, v) => (s.subtitles.source = v),
    hint: L("指定后逐句按这种语言识别，不再逐句判断；已生成的字幕不变", "指定すると、1 行ずつ判断せずこの言語として認識します。作成済みの字幕は変わりません"),
  },
  {
    id: "subTarget", kind: "select", label: L("翻译为", "翻訳先"),
    options: [["", L("跟随界面语言", "表示言語に合わせる")], ["zh", "简体中文"], ["zh-Hant", "繁體中文"], ["ja", "日本語"], ["en", "English"], ["ko", "한국어"]],
    get: (s) => s.subtitles.target, set: (s, v) => (s.subtitles.target = v),
  },
];

const filters = (): Field[] => [
  {
    id: "cap", kind: "range", label: L("总量上限", "総量上限"), min: 0, max: 10, step: 0.5, fmt: (v) => (v ? L(`每秒 ${v} 条`, `毎秒 ${v} 件`) : unlimited()),
    get: (s) => s.filters.capPerSecond, set: (s, v) => (s.filters.capPerSecond = v),
    hint: L("按视频时长计的总条数（至少 100），同 danmk.py；投稿者评论与评论画不计入、不删减；每次抽中的是同一批",
      "動画の長さから総数を決めます（最低 100 件、danmk.py と同じ）。投稿者コメントとコメントアートは数えず、削りません。毎回同じコメントが選ばれます"),
  },
  { id: "ngWords", kind: "lines", label: L("NG 词", "NG ワード"), placeholder: L("每行一个；/正则/ 也可以", "1 行に 1 つ。/正規表現/ も使えます"), get: (s) => s.filters.ngWords, set: (s, v) => (s.filters.ngWords = v) },
  { id: "ngUsers", kind: "lines", label: L("NG 用户 ID", "NG ユーザー ID"), placeholder: L("每行一个 userId", "1 行に 1 つの userId"), get: (s) => s.filters.ngUsers, set: (s, v) => (s.filters.ngUsers = v) },
  { id: "ngCommands", kind: "lines", label: L("NG 命令", "NG コマンド"), placeholder: L("如 big、red、#ff0000", "例：big、red、#ff0000"), get: (s) => s.filters.ngCommands, set: (s, v) => (s.filters.ngCommands = v) },
  {
    id: "hide", kind: "checks", label: L("隐藏类型", "種類ごとに隠す"), items: [
      [ruleLabel("naka"), (s) => s.filters.hide.naka, (s, v) => (s.filters.hide.naka = v)],
      [ruleLabel("ue"), (s) => s.filters.hide.ue, (s, v) => (s.filters.hide.ue = v)],
      [ruleLabel("shita"), (s) => s.filters.hide.shita, (s, v) => (s.filters.hide.shita = v)],
      [ruleLabel("colored"), (s) => s.filters.hide.colored, (s, v) => (s.filters.hide.colored = v)],
      [ruleLabel("big"), (s) => s.filters.hide.big, (s, v) => (s.filters.hide.big = v)],
      [ruleLabel("small"), (s) => s.filters.hide.small, (s, v) => (s.filters.hide.small = v)],
      [ruleLabel("ca"), (s) => s.filters.hide.ca, (s, v) => (s.filters.hide.ca = v)],
      [ruleLabel("anonymous"), (s) => s.filters.hide.anonymous, (s, v) => (s.filters.hide.anonymous = v)],
    ],
  },
  {
    id: "ngShare", kind: "select", label: L("NG 共享等级", "NG 共有レベル"),
    options: [["none", L("无", "なし")], ["weak", "弱（score ≤ −10000）"], ["medium", "中（≤ −4800）"], ["strong", L("强（≤ −1000）", "強（≤ −1000）")]],
    get: (s) => s.filters.ngShare, set: (s, v) => (s.filters.ngShare = v as Settings["filters"]["ngShare"]),
    hint: L("阈值据公开资料，未与官网实测核对；旧 XML 导入的数据没有 score",
      "しきい値は公開情報に基づくもので、公式サイトでの実測とは照合していません。旧 XML から取り込んだデータには score がありません"),
  },
  { id: "maxLength", kind: "range", label: L("最大长度", "最大文字数"), min: 0, max: 200, step: 5, fmt: (v) => (v ? L(`${v} 字`, `${v} 文字`) : unlimited()), get: (s) => s.filters.maxLength, set: (s, v) => (s.filters.maxLength = v), hint: L("评论画不受此限", "コメントアートは対象外") },
  { id: "postedBefore", kind: "datetime", label: L("只显示此前投稿", "指定日時までの投稿"), get: (s) => s.filters.postedBefore, set: (s, v) => (s.filters.postedBefore = v), hint: L("重现某一时刻的弹幕；留空关闭", "ある時点のコメントを再現します。空欄で無効") },
];

interface Row {
  el: HTMLElement;
  refresh(): void;
  /** Take the same field's definition in the current language. */
  relabel(f: Field): void;
}

/**
 * One setting as a row. The row keeps its field definition and reads it on
 * every use, so a language switch swaps definitions and rewrites the texts
 * in place (the panel used to be rebuilt: 102 ms, over the 50 ms budget).
 */
function makeRow(def: Field, s: Settings, changed: () => void): Row {
  let f = def;
  const label = el("label", { class: "lbl" });
  const r = el("div", { class: "row" }, label);
  let refresh = () => {};
  let texts = () => {};
  switch (f.kind) {
    case "toggle": {
      const i = el("input", { type: "checkbox" });
      i.onchange = () => ((f as Extract<Field, { kind: "toggle" }>).set(s, i.checked), changed());
      refresh = () => (i.checked = (f as Extract<Field, { kind: "toggle" }>).get(s));
      r.append(i);
      break;
    }
    case "range": {
      const g = () => f as Extract<Field, { kind: "range" }>;
      const i = el("input", { type: "range", min: String(g().min), max: String(g().max), step: String(g().step), class: "fill" });
      const out = el("span", { class: "val" });
      const fill = () => i.style.setProperty("--v", `${((Number(i.value) - g().min) / (g().max - g().min)) * 100}%`);
      i.oninput = () => {
        g().set(s, Number(i.value));
        out.textContent = g().fmt(Number(i.value));
        fill();
        changed();
      };
      refresh = () => {
        i.value = String(g().get(s));
        out.textContent = g().fmt(g().get(s));
        fill();
      };
      r.append(i, out);
      break;
    }
    case "select": {
      const g = () => f as Extract<Field, { kind: "select" }>;
      const i = el("select");
      // Options may change (a video's subtitle tracks): rebuilt when they do.
      texts = () => {
        const opts = g().options;
        if (i.options.length !== opts.length || opts.some(([v], k) => i.options[k].value !== v)) {
          i.replaceChildren(...opts.map(([v]) => el("option", { value: v })));
        }
        opts.forEach(([, t, off], k) => {
          i.options[k].textContent = t;
          i.options[k].disabled = !!off;
        });
      };
      i.onchange = () => (g().set(s, i.value), changed());
      refresh = () => (i.value = g().get(s));
      r.append(i);
      break;
    }
    case "color": {
      const g = () => f as Extract<Field, { kind: "color" }>;
      const i = el("input", { type: "color" });
      i.oninput = () => (g().set(s, i.value), changed());
      refresh = () => (i.value = g().get(s));
      r.append(i);
      break;
    }
    case "lines": {
      const g = () => f as Extract<Field, { kind: "lines" }>;
      const i = el("textarea", { rows: 3, spellcheck: false });
      texts = () => (i.placeholder = g().placeholder);
      i.onchange = () => (g().set(s, i.value.split(/\r?\n/).map((x) => x.trim()).filter(Boolean)), changed());
      refresh = () => (i.value = g().get(s).join("\n"));
      r.classList.add("wide");
      r.append(i);
      break;
    }
    case "datetime": {
      const g = () => f as Extract<Field, { kind: "datetime" }>;
      const i = el("input", { type: "datetime-local" });
      i.onchange = () => (g().set(s, i.value), changed());
      refresh = () => (i.value = g().get(s));
      r.append(i);
      break;
    }
    case "checks": {
      const g = () => f as Extract<Field, { kind: "checks" }>;
      const box = el("div", { class: "checks" });
      const names: Text[] = [];
      const inputs: HTMLInputElement[] = [];
      g().items.forEach((_, k) => {
        const i = el("input", { type: "checkbox" });
        i.onchange = () => (g().items[k][2](s, i.checked), changed());
        const t = document.createTextNode("");
        names.push(t);
        inputs.push(i);
        box.append(el("label", {}, i, t));
      });
      texts = () => g().items.forEach(([n], k) => (names[k].data = n));
      refresh = () => g().items.forEach(([, get], k) => (inputs[k].checked = get(s)));
      r.classList.add("wide");
      r.append(box);
      break;
    }
  }
  const setTexts = () => {
    label.textContent = f.label;
    // Explanations stay out of the way: shown on hover only.
    const hint = "hint" in f ? f.hint : undefined;
    label.title = hint ?? "";
    label.classList.toggle("has-hint", !!hint);
    texts();
  };
  setTexts();
  refresh();
  return {
    el: el("div", { class: "field" }, r),
    refresh: () => refresh(),
    relabel(nf) {
      f = nf;
      setTexts();
      refresh();
    },
  };
}

export interface Panel {
  refresh(): void;
  setStats(stats: FilterStats | null): void;
  /** Rewrite all texts in the current language. */
  relabel(): void;
}

const byId = (fields: Field[]) => new Map(fields.map((f) => [f.id, f]));

/** The full settings: display and filter sections with the filter statistics. */
export function buildPanel(root: HTMLElement, s: Settings, changed: () => void): Panel {
  root.replaceChildren(); // built again for each episode
  const rows = new Map<string, Row>();
  const titles: [HTMLElement, () => string][] = [];
  const section = (title: () => string, fields: Field[], id = "") => {
    const h = el("h3", {}, title());
    titles.push([h, title]);
    const box = el("section", {}, h);
    if (id) box.dataset.section = id;
    for (const f of fields) {
      const r = makeRow(f, s, changed);
      rows.set(f.id, r);
      box.append(r.el);
    }
    root.append(box);
    return box;
  };
  section(() => L("显示", "表示"), display());
  section(() => L("字幕", "字幕"), subtitleFields(), "subs");
  const filt = section(() => L("过滤", "フィルター"), filters());
  const stats = el("div", { class: "stats" });
  filt.append(stats);
  let last: FilterStats | null = null;
  const setStats = (st: FilterStats | null) => {
    last = st;
    if (!st) {
      stats.textContent = L("没有弹幕数据", "コメントデータがありません");
      return;
    }
    const hidden = st.total - st.shown;
    const parts = Object.entries(st.byRule).map(([r, n]) => `${ruleLabel(r as Rule)} ${n}`);
    const forks = Object.entries(st.byFork).map(([f, v]) => `${f} ${v.shown}/${v.total}`);
    stats.replaceChildren(
      el("div", {}, L(`显示 ${st.shown} / ${st.total} 条，隐藏 ${hidden} 条`, `表示 ${st.shown} / ${st.total} 件、非表示 ${hidden} 件`)),
      el("div", {}, parts.length ? parts.join(sep()) : L("没有规则生效", "適用中のルールはありません")),
      el("div", {}, `${L("图层：", "レイヤー：")}${forks.join(sep())}`),
      ...(st.badPatterns.length ? [el("div", { class: "err" }, `${L("无效正则：", "無効な正規表現：")}${st.badPatterns.join(sep())}`)] : []),
    );
  };
  return {
    refresh: () => rows.forEach((r) => r.refresh()),
    setStats,
    relabel() {
      const defs = byId([...display(), ...subtitleFields(), ...filters()]);
      rows.forEach((r, id) => r.relabel(defs.get(id)!));
      for (const [h, t] of titles) h.textContent = t();
      setStats(last);
    },
  };
}

/** The settings people change while watching, for the quick card. */
export const QUICK = ["enabled", "opacity", "scale", "area", "limit", "frameRate"];
/** The subtitle card (its own button): tracks, size, AI when there are none. */
export const SUB_QUICK = ["subPick", "subPick2", "subScale", "subAi"];

export interface Quick {
  refresh(): void;
  relabel(): void;
}

export function buildQuick(root: HTMLElement, s: Settings, changed: () => void, ids = QUICK): Quick {
  root.replaceChildren();
  const defs = byId([...display(), ...subtitleFields()]);
  const rows = ids.map((id) => {
    const r = makeRow(defs.get(id)!, s, changed);
    root.append(r.el);
    return [id, r] as const;
  });
  return {
    refresh: () => rows.forEach(([, r]) => r.refresh()),
    relabel() {
      const d = byId([...display(), ...subtitleFields()]);
      for (const [id, r] of rows) r.relabel(d.get(id)!);
    },
  };
}
