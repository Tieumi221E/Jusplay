// Library page: continue-watching rail, series grid, series detail.
// Text from the disk (names, paths) only ever goes through textContent.

import { episodeLabel, seriesKey } from "./keys.ts";
import { L, applyStatic, bindSwitches, onLang, prefs, setPref, type StaticText } from "./i18n.ts";
import { Layer, LayerStack } from "./layer.ts";
import { initTips } from "./tip.ts";
import { watchInteractions } from "./perf.ts";
import { onBack, setTitle, BACK_KEYS } from "./nav.ts";
import { makeThumb } from "./thumbnailer.ts";
import { playerHost } from "./playerhost.ts";

interface Probe {
  duration: number;
  width: number;
  height: number;
  videoCodec: string;
  profile?: string;
  audioCodec?: string;
  attached: boolean;
  error?: string;
}

interface Entry {
  id: string;
  path: string;
  folder: string;
  series: string;
  season: number | null;
  episode: number | null;
  title: string;
  subtitle?: string;
  size: number;
  modTime: string;
  probe: Probe | null;
  comments?: string;
  commentCount?: number;
  position: number;
  watched: boolean;
  lastPlayed?: string;
  missing: boolean;
  sameName: boolean;
  /** Another version of an episode: the file shown for it (library.Versions). */
  altOf?: string;
  versions?: string[];
  /** The thumbnail exists; otherwise the page makes it (thumbnailer.ts). */
  thumb: boolean;
}

interface State {
  /** error: why the folder's records stay in memory this run ("" when they are saved in it). */
  folders: { path: string; error?: string }[];
  entries: Entry[];
  scanning: boolean;
  progress: [number, number];
  lastScan: { Files: number; Added: number; Changed: number; Missing: number } | null;
}

interface Series {
  key: string;
  name: string;
  folder: string;
  eps: Entry[];
  lastPlayed: number;
  added: number;
}

declare global {
  interface Window {
    kpPickFolder?: () => Promise<string>;
    kpPickComments?: () => Promise<string>;
    kpReveal?: (path: string) => Promise<void>;
  }
}

const $ = <T extends HTMLElement = HTMLElement>(sel: string) => document.querySelector<T>(sel)!;
const view = $("#view");

function h<K extends keyof HTMLElementTagNameMap>(tag: K, attrs: Record<string, string | boolean | undefined> = {}, ...kids: (Node | string | null | undefined | false)[]): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === false) continue;
    if (k === "text") e.textContent = String(v);
    else e.setAttribute(k, v === true ? "" : v);
  }
  for (const k of kids) if (k) e.append(k);
  return e;
}

let toastTimer = 0;
function toast(msg: string, ms = 2600): void {
  const t = $("#toast");
  t.textContent = msg;
  t.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => t.classList.remove("show"), ms);
}

const post = (url: string, body: unknown) => fetch(url, { method: "POST", body: JSON.stringify(body) });

function fmtClock(s: number): string {
  s = Math.max(0, Math.floor(s));
  const hh = Math.floor(s / 3600), mm = Math.floor((s % 3600) / 60), ss = s % 60;
  return hh ? `${hh}:${String(mm).padStart(2, "0")}:${String(ss).padStart(2, "0")}` : `${mm}:${String(ss).padStart(2, "0")}`;
}
function fmtSize(b: number): string {
  return b >= 2 ** 30 ? `${(b / 2 ** 30).toFixed(1)} GB` : `${Math.round(b / 2 ** 20)} MB`;
}
/** Seasons are named only when the series has more than one. */
const multiSeason = (s: Series | undefined) => !!s && new Set(s.eps.map((e) => e.season ?? 1)).size > 1;
const epName = (e: Entry, withSeason = multiSeason(byKey.get(seriesKey(e.folder, e.series)))) =>
  episodeLabel(e.season, e.episode, withSeason) || e.title;
const quality = (p: Probe | null) => {
  if (!p || p.error) return "";
  const res = p.height >= 2000 ? "4K" : p.height ? `${p.height}p` : "";
  return [res, p.videoCodec?.toUpperCase(), p.profile === "Main 10" ? "10bit" : ""].filter(Boolean).join(" ");
};

const episodes = (n: number) => L(`${n} 集`, `全 ${n} 話`);
const watchedN = (n: number) => L(`已看 ${n}`, `${n} 話視聴済み`);

const LIBRARY_TEXT: StaticText[] = [
  ["#search", "placeholder", "搜索系列或文件名", "シリーズ名・ファイル名で検索"],
  ["#rescan", "aria-label", "重新扫描", "再スキャン"],
  ["#folders-btn", "aria-label", "媒体库文件夹", "ライブラリのフォルダー"],
  [".pop-title", "textContent", "媒体库文件夹", "ライブラリのフォルダー"],
  ["#add-folder", "textContent", "＋ 添加文件夹", "＋ フォルダーを追加"],
];

// ---- thumbnails: loaded when they scroll into view -------------------------

const io = new IntersectionObserver((items) => {
  for (const it of items) {
    if (!it.isIntersecting) continue;
    const img = it.target as HTMLImageElement;
    io.unobserve(img);
    const e = byId.get(img.dataset.id ?? "");
    if (e && !e.thumb) {
      // Made in the page, then shown (thumbnailer.ts).
      void makeThumb(e.id).then((ok) => {
        if (!ok) return img.remove();
        e.thumb = true;
        img.src = img.dataset.src!;
      });
      continue;
    }
    img.src = img.dataset.src!;
    // Already in the memory cache (coming back from the player): show it
    // as is, without replaying the fade-in.
    if (img.complete && img.naturalWidth) img.classList.add("loaded", "instant");
  }
}, { rootMargin: "300px" });

const thumbURL = (e: Entry) => `api/thumb?id=${e.id}&v=${e.size}-${Date.parse(e.modTime)}`;

// ---- motion: page and view transitions (docs/design.md §二.3) ----------------

const reduced = matchMedia("(prefers-reduced-motion: reduce)");

/**
 * Open an episode. The clicked thumbnail is named "hero" so the cross-page
 * transition grows it into the player's stage, where the player shows the
 * same image (?thumb=) until its first frame.
 */
function play(e: Entry, from?: Element | null): void {
  const t = from?.querySelector<HTMLElement>(".thumb") ?? (from as HTMLElement | null);
  sessionStorage.setItem("jus-home-scroll", String(scrollY));
  player.open(e.id, thumbURL(e), t);
}

// The player, over the library (playerhost.ts). Back zooms into the card of
// the episode it showed last; the library then shows its new progress.
const player = playerHost({
  thumbOf: (id) => {
    const e = byId.get(id);
    return e?.thumb ? thumbURL(e) : undefined;
  },
  sourceOf: (id) => {
    const e = byId.get(id);
    const own = view.querySelector<HTMLElement>(`[data-id="${id}"] .thumb`);
    if (own) return visible(own);
    // Its series' card on the home page.
    const s = e && view.querySelector<HTMLElement>(`[data-key="${CSS.escape(seriesKey(e.folder, e.series))}"] .thumb`);
    return s ? visible(s) : null;
  },
  onClosed: (id) => {
    setTitle("Jusplay");
    void refresh(true).then(() => view.querySelector<HTMLElement>(`[data-id="${id}"]`)?.focus({ preventScroll: true }));
  },
});
const visible = (el: HTMLElement) => {
  const r = el.getBoundingClientRect();
  return r.bottom > 0 && r.top < innerHeight && r.width ? el : null;
};

/** Run a view change as a same-page transition where supported. */
function transition(update: () => void): void {
  if (!document.startViewTransition || reduced.matches) return update();
  const vt = document.startViewTransition(update);
  vt.finished.finally(() => {
    for (const el of document.querySelectorAll<HTMLElement>("[style*=view-transition-name]")) el.style.viewTransitionName = "";
  });
}

/** A backdrop of e's thumbnail, once it exists. */
function backdrop(el: HTMLElement, e: Entry): void {
  const set = () => (el.style.backgroundImage = `url("${thumbURL(e)}")`);
  if (e.thumb) return void set();
  void makeThumb(e.id).then((ok) => {
    if (ok) {
      e.thumb = true;
      set();
    }
  });
}

function thumb(e: Entry | undefined, opts: { progress?: number; watched?: boolean; play?: boolean } = {}): HTMLElement {
  const box = h("div", { class: "thumb" });
  if (e && !e.missing) {
    const img = h("img", { alt: "", decoding: "async", "data-src": thumbURL(e), "data-id": e.id });
    img.onload = () => img.classList.add("loaded");
    img.onerror = () => img.remove();
    box.append(img);
    io.observe(img);
  }
  if (opts.play) box.append(h("div", { class: "play" }, h("span", { text: "▶" })));
  if (opts.watched) box.append(h("div", { class: "watched-mark", title: L("已看完", "視聴済み"), text: "✓" }));
  if (opts.progress && opts.progress > 0) {
    const bar = h("div", { class: "progress" }, h("i"));
    (bar.firstChild as HTMLElement).style.width = `${Math.min(100, opts.progress * 100)}%`;
    box.append(bar);
  }
  return box;
}

function commentChip(e: Entry): HTMLElement | null {
  const word = L("弹幕", "コメント");
  if (e.probe?.attached) return h("span", { class: "chip on", title: L("弹幕已打包进 MKV", "コメントは MKV に格納済み"), text: e.commentCount != null ? `${word} ${e.commentCount.toLocaleString()}` : word });
  if (e.comments) return h("span", { class: "chip on", title: e.comments, text: L("手动弹幕", "手動コメント") });
  if (e.sameName) return h("span", { class: "chip pending", title: L("旁边有同名 JSON，可以打包", "同名の JSON があり、格納できます"), text: L("同名 JSON", "同名 JSON") });
  return null;
}

// ---- data -------------------------------------------------------------------

let state: State = { folders: [], entries: [], scanning: false, progress: [0, 0], lastScan: null };
let seriesList: Series[] = [];
let byKey = new Map<string, Series>();
let byId = new Map<string, Entry>();

function group(entries: Entry[]): void {
  const m = new Map<string, Series>();
  byId = new Map(entries.map((e) => [e.id, e]));
  // One row per episode; its other versions are in the row's menu.
  for (const e of entries) {
    if (e.altOf) continue;
    const key = seriesKey(e.folder, e.series);
    let s = m.get(key);
    if (!s) m.set(key, (s = { key, name: e.series, folder: e.folder, eps: [], lastPlayed: 0, added: 0 }));
    s.eps.push(e);
    s.lastPlayed = Math.max(s.lastPlayed, e.lastPlayed ? Date.parse(e.lastPlayed) : 0);
    s.added = Math.max(s.added, Date.parse(e.modTime) || 0);
  }
  seriesList = [...m.values()];
  byKey = m;
}

/** The episode to open next: in progress, else the first unwatched, else the first. */
function upNext(s: Series): Entry | undefined {
  const live = s.eps.filter((e) => !e.missing);
  return live.find((e) => e.position > 0 && !e.watched) ?? live.find((e) => !e.watched) ?? live[0];
}

let lastSig = "";
async function refresh(force = false): Promise<void> {
  const r = await fetch("api/library");
  const st: State = await r.json();
  // Re-render only when something changed, so scrolling and hover stay put.
  const sig = JSON.stringify([st.folders, st.entries.map((e) => [e.id, e.position, e.watched, e.missing, e.probe?.attached, e.commentCount, e.comments, e.sameName, e.size])]);
  state = st;
  renderScan();
  if (force || sig !== lastSig) {
    lastSig = sig;
    group(st.entries);
    route();
  }
  if (st.scanning) setTimeout(() => refresh(), 1500);
}

function renderScan(): void {
  const btn = $("#rescan"), label = $("#scan-status");
  btn.classList.toggle("spinning", state.scanning);
  const [done, total] = state.progress;
  label.textContent = state.scanning ? (total ? `${L("扫描中", "スキャン中")} ${done}/${total}` : L("扫描中…", "スキャン中…")) : "";
}

// ---- views ------------------------------------------------------------------

let search = "";

function route(): void {
  const m = /^#\/s\/(.+)$/.exec(location.hash);
  if (m) {
    const s = byKey.get(decodeURIComponent(m[1]));
    if (s) return renderSeries(s);
  }
  renderHome();
}

function renderHome(): void {
  const saved = sessionStorage.getItem("jus-home-scroll");
  view.replaceChildren();
  if (!state.folders.length) {
    view.append(h("div", { class: "empty-state" },
      h("div", { class: "big", text: L("把动画文件夹加进来", "アニメのフォルダーを追加しましょう") }),
      h("p", { text: L("按系列整理并记住进度，不会移动或修改文件。", "シリーズごとに整理して続きを覚えます。ファイルは変更しません。") }),
      Object.assign(h("button", { class: "primary", text: L("添加文件夹", "フォルダーを追加") }), { onclick: addFolder })));
    return;
  }
  const q = search.trim().toLowerCase();
  const match = (s: Series) => !q || s.name.toLowerCase().includes(q) || s.eps.some((e) => e.path.toLowerCase().includes(q));

  const cont = state.entries.filter((e) => !e.missing && e.position > 0 && !e.watched && e.lastPlayed)
    .sort((a, b) => Date.parse(b.lastPlayed!) - Date.parse(a.lastPlayed!)).slice(0, 13);
  // The first screen is the episode being watched: large, one press away.
  if (cont.length && !q) view.append(homeHero(cont[0]));
  if (cont.length > 1 && !q) {
    view.append(h("h2", { text: L("继续观看", "続きから見る") }));
    const rail = h("div", { class: "rail" });
    for (const e of cont.slice(1)) {
      const d = e.probe?.duration || 0;
      const card = episodeCard(e, e.series, `${epName(e)}${d ? L(` · 剩 ${Math.max(1, Math.round((d - e.position) / 60))} 分钟`, ` · 残り ${Math.max(1, Math.round((d - e.position) / 60))} 分`) : ""}`, d ? e.position / d : 0);
      rail.append(card);
    }
    view.append(rail);
  }

  const order = prefs.sort;
  const sorted = seriesList.filter((s) => s.eps.some((e) => !e.missing) && match(s)).sort((a, b) =>
    order === "recent" ? b.lastPlayed - a.lastPlayed || a.name.localeCompare(b.name, "ja")
      : order === "added" ? b.added - a.added
        : a.name.localeCompare(b.name, "ja"));
  const seg = h("span", { class: "seg", role: "group", "aria-label": L("排序", "並べ替え") });
  for (const [k, label] of [["name", L("名称", "名前")], ["recent", L("最近播放", "最近再生")], ["added", L("最近添加", "最近追加")]]) {
    const b = h("button", { "aria-pressed": String(order === k), text: label });
    b.onclick = () => {
      setPref("sort", k);
      renderHome();
    };
    seg.append(b);
  }
  view.append(h("h2", {}, q ? L("搜索结果", "検索結果") : L("全部系列", "すべてのシリーズ"), h("span", { class: "sub", text: L(`${sorted.length} 个系列`, `${sorted.length} シリーズ`) }), h("span", { class: "toolbar" }, seg)));
  if (!sorted.length) {
    view.append(h("div", { class: "none", text: q ? L("没有匹配的系列。", "一致するシリーズはありません。") : state.scanning ? L("正在扫描…", "スキャン中…") : L("这些文件夹里还没有找到视频。", "これらのフォルダーにはまだ動画が見つかりません。") }));
    return;
  }
  const grid = h("div", { class: "grid" });
  for (const s of sorted) grid.append(seriesCard(s));
  view.append(grid);
  // At once, not on the next frame: a view transition captures this state.
  if (saved) window.scrollTo(0, Number(saved));
  if (coverFrom) {
    const c = view.querySelector<HTMLElement>(`[data-key="${CSS.escape(coverFrom)}"] .thumb`);
    if (c) c.style.viewTransitionName = "cover";
    coverFrom = "";
  }
}

// ---- warm-up on intent: resting the pointer on an episode for a moment
// opens its session on the server (media index, comments), so the click
// that usually follows starts playing sooner. One at a time, each entry once.
const warmed = new Set<string>();
let warming = false;
function warmOnHover(el: HTMLElement, id: string): void {
  let timer = 0;
  el.addEventListener("focusin", () => player.warm());
  el.addEventListener("pointerenter", () => {
    player.warm();
    if (warmed.has(id)) return;
    timer = window.setTimeout(async () => {
      if (warming || warmed.has(id)) return;
      warming = true;
      warmed.add(id);
      await fetch(`api/session?id=${encodeURIComponent(id)}`).catch(() => undefined);
      warming = false;
    }, 250);
  });
  el.addEventListener("pointerleave", () => clearTimeout(timer));
}

function episodeCard(e: Entry, title: string, meta: string, progress: number): HTMLElement {
  const a = h("div", { class: "card", role: "link", tabindex: "0", title: e.path, "data-id": e.id },
    thumb(e, { progress, play: true }),
    h("div", { class: "title", text: title }),
    h("div", { class: "meta", text: meta }));
  a.onclick = () => play(e, a);
  a.onkeydown = (ev) => { if (ev.key === "Enter") play(e, a); };
  warmOnHover(a, e.id);
  return a;
}

/**
 * The home page's first screen: the episode last watched, over its own
 * picture blurred large (a 480 px thumbnail would be soft at that size; a
 * blur of a still image costs one drawing, not one per scrolled frame).
 */
function homeHero(e: Entry): HTMLElement {
  const d = e.probe?.duration || 0;
  const left = d ? Math.max(1, Math.round((d - e.position) / 60)) : 0;
  const bd = h("div", { class: "backdrop" });
  backdrop(bd, e);
  const cover = h("div", { class: "cover", role: "link", tabindex: "0", "data-id": e.id, "aria-label": `${e.series} ${epName(e)}` },
    thumb(e, { progress: d ? e.position / d : 0, play: true }));
  const go = () => play(e, cover);
  cover.onclick = go;
  cover.onkeydown = (ev) => { if (ev.key === "Enter") go(); };
  warmOnHover(cover, e.id);
  const primary = Object.assign(h("button", { class: "primary", text: L(`▶ 继续播放 ${epName(e)}`, `▶ ${epName(e)} を続きから`) }), { onclick: go });
  warmOnHover(primary, e.id);
  const series = Object.assign(h("button", { text: L("全部剧集", "エピソード一覧") }), {
    onclick: () => (location.hash = `#/s/${encodeURIComponent(seriesKey(e.folder, e.series))}`),
  });
  return h("section", { class: "hero home" }, bd,
    h("div", { class: "hero-inner" }, cover,
      h("div", {}, h("div", { class: "kicker", text: L("继续观看", "続きから見る") }), h("h1", { text: e.series }),
        h("div", { class: "facts" }, epName(e), left ? h("span", { class: "dot" }) : null, left ? L(`剩 ${left} 分钟`, `残り ${left} 分`) : null),
        h("div", { class: "actions" }, primary, series))));
}

function seriesCard(s: Series): HTMLElement {
  const live = s.eps.filter((e) => !e.missing);
  const watched = live.filter((e) => e.watched).length;
  const next = upNext(s);
  const withComments = live.filter((e) => e.probe?.attached || e.comments).length;
  const meta = h("div", { class: "meta" }, episodes(live.length));
  if (watched) meta.append(h("span", { class: "dot" }), watched === live.length ? L("已看完", "視聴済み") : watchedN(watched));
  if (withComments) meta.append(h("span", { class: "chip on", title: L(`${withComments} 集有弹幕`, `${withComments} 話にコメントあり`), text: L("弹幕", "コメント") }));
  const a = h("div", { class: "card", role: "link", tabindex: "0", title: s.folder, "data-key": s.key },
    thumb(live[0] ?? next, { watched: watched === live.length && live.length > 0 }),
    h("div", { class: "title", text: s.name }), meta);
  a.onclick = () => {
    sessionStorage.setItem("jus-home-scroll", String(scrollY));
    // The cover grows into the series page's header.
    const t = a.querySelector<HTMLElement>(".thumb");
    if (t && !reduced.matches) t.style.viewTransitionName = "cover";
    location.hash = `#/s/${encodeURIComponent(s.key)}`;
  };
  a.onkeydown = (ev) => { if (ev.key === "Enter") a.click(); };
  return a;
}

function renderSeries(s: Series): void {
  view.replaceChildren();
  window.scrollTo(0, 0);
  const live = s.eps.filter((e) => !e.missing);
  const watched = live.filter((e) => e.watched).length;
  const size = live.reduce((n, e) => n + e.size, 0);
  const next = upNext(s);
  const hero = h("section", { class: "hero" });
  const coverEl = h("div", { class: "cover" }, thumb(live[0]));
  if (!reduced.matches) (coverEl.firstChild as HTMLElement).style.viewTransitionName = "cover";
  // From the header, a play button grows the cover into the stage.
  const startEp = (e: Entry) => {
    (coverEl.firstChild as HTMLElement).style.viewTransitionName = "";
    play(e, e === live[0] ? coverEl : view.querySelector(`[data-id="${e.id}"]`));
  };
  const bd = h("div", { class: "backdrop" });
  if (live[0]) backdrop(bd, live[0]);
  const facts = h("div", { class: "facts" }, episodes(live.length));
  if (watched) facts.append(h("span", { class: "dot" }), watchedN(watched));
  facts.append(h("span", { class: "dot" }), fmtSize(size));
  const actions = h("div", { class: "actions" });
  if (next) {
    const resumeLabel = next.position > 0 && !next.watched
      ? L(`继续播放 ${epName(next)}（${fmtClock(next.position)}）`, `${epName(next)} の続きから（${fmtClock(next.position)}）`)
      : L(`播放 ${epName(next)}`, `${epName(next)} を再生`);
    const playBtn = h("button", { class: "primary", text: `▶ ${resumeLabel}` });
    playBtn.onclick = () => startEp(next);
    actions.append(playBtn);
    if (live[0] && live[0] !== next) {
      const first = h("button", { text: L("从头开始", "最初から") });
      first.onclick = () => startEp(live[0]);
      actions.append(first);
    }
  }
  hero.append(bd, h("div", { class: "hero-inner" },
    coverEl,
    h("div", {}, h("h1", { text: s.name }), facts, actions)));
  view.append(hero);
  $("#crumb-title").textContent = s.name;

  // Several seasons in one folder: a heading per season, specials last.
  const seasons = new Map<number, Entry[]>();
  for (const e of s.eps) {
    const k = e.season ?? 1;
    if (!seasons.has(k)) seasons.set(k, []);
    seasons.get(k)!.push(e);
  }
  const grouped = seasons.size > 1;
  for (const [season, eps] of [...seasons].sort((a, b) => a[0] - b[0])) {
    if (grouped) view.append(h("h2", {}, episodeLabel(season, null), h("span", { class: "sub", text: episodes(eps.filter((e) => !e.missing).length) })));
    const list = h("div", { class: "episodes", role: "list" });
    for (const e of eps) list.append(episodeRow(e));
    view.append(list);
  }
  const dir = s.eps[0]?.path.replace(/[\\/][^\\/]*$/, "");
  if (dir) view.append(h("div", { class: "none", title: dir, text: dir }));
}

function episodeRow(e: Entry): HTMLElement {
  const d = e.probe?.duration || 0;
  const metaBits = [d ? fmtClock(d) : "", quality(e.probe), e.probe?.audioCodec?.toUpperCase() ?? ""].filter(Boolean);
  const m = h("div", { class: "m" });
  metaBits.forEach((b, i) => {
    if (i) m.append(h("span", { class: "dot" }));
    m.append(b);
  });
  if (e.missing) m.replaceChildren(L("文件不见了", "ファイルが見つかりません"));
  else if (e.probe?.error) m.replaceChildren(h("span", { class: "chip bad", title: e.probe.error, text: L("无法读取", "読み込めません") }));
  const chips = h("div", { class: "chips" }, commentChip(e));
  const more = h("button", { class: "more", title: L("更多", "その他"), "aria-haspopup": "true", "aria-expanded": "false", text: "⋯" });
  const row = h("div", { class: `ep${e.watched ? " watched" : ""}${e.missing ? " missing" : ""}`, role: "listitem", tabindex: e.missing ? undefined : "0", title: e.path, "data-id": e.id },
    thumb(e, { progress: d && !e.watched ? e.position / d : 0, watched: e.watched, play: !e.missing }),
    h("div", { class: "num", text: e.episode != null ? String(e.episode) : "·" }),
    h("div", { class: "name" }, h("div", { class: "t", text: e.subtitle || epName(e) }), m),
    chips, more);
  const open = () => { if (!e.missing) play(e, row); };
  if (!e.missing) warmOnHover(row, e.id);
  row.onclick = (ev) => { if (!(ev.target as HTMLElement).closest(".more")) open(); };
  row.onkeydown = (ev) => { if (ev.key === "Enter") open(); };
  more.onclick = (ev) => {
    ev.stopPropagation();
    openMenu(more, e);
  };
  row.oncontextmenu = (ev) => {
    ev.preventDefault();
    openMenu(more, e, ev.clientX, ev.clientY);
  };
  return row;
}

// ---- episode menu -------------------------------------------------------------

const layers = new LayerStack();
const menuLayer = new Layer($("#menu"), layers);
const popLayer = new Layer($("#folders-pop"), layers, $("#folders-btn"));
function closeMenu(): void {
  menuLayer.hide();
}

function openMenu(anchor: HTMLElement, e: Entry, x?: number, y?: number): void {
  const menu = $("#menu");
  const item = (label: string, fn: () => void, disabled = false) => {
    const b = h("button", { role: "menuitem", text: label, disabled });
    b.onclick = () => {
      closeMenu();
      fn();
    };
    return b;
  };
  const isMkv = /\.mkv$/i.test(e.path);
  menu.replaceChildren(
    item(e.watched ? L("标为未看", "未視聴にする") : L("标为已看", "視聴済みにする"), async () => {
      await post("api/library/watched", { id: e.id, watched: !e.watched });
      refresh(true);
    }),
    h("hr"),
    item(e.probe?.attached ? L("用同名 JSON 更新已打包的弹幕", "同名の JSON で格納済みのコメントを更新") : L("把同名 JSON 打包进 MKV", "同名の JSON を MKV に格納"), () => embed(e), !(e.sameName && isMkv)),
    item(L("选择弹幕文件…", "コメントファイルを選択…"), () => pickComments(e), !window.kpPickComments),
    item(L("恢复自动选择弹幕", "コメントの自動選択に戻す"), async () => {
      await post("api/comments/source", { id: e.id, path: "" });
      refresh(true);
    }, !e.comments),
    h("hr"),
    item(L("在资源管理器中显示", "エクスプローラーで表示"), () => window.kpReveal?.(e.path), !window.kpReveal || e.missing),
  );
  const alts = (e.versions ?? []).map((id) => byId.get(id)).filter((x): x is Entry => !!x && !x.missing);
  if (alts.length) {
    menu.append(h("hr"), h("div", { class: "menu-label", text: L("其他版本", "ほかのバージョン") }),
      ...alts.map((a) => item(`${a.path.split(/[\\/]/).slice(-2).join("\\")}${a.probe?.attached ? L(" · 有弹幕", " · コメントあり") : ""}`, () => play(a))));
  }
  if (menuLayer.isOpen && menuLayer.trigger !== anchor) menuLayer.hide();
  menu.hidden = false;
  const r = anchor.getBoundingClientRect();
  const mw = menu.offsetWidth, mh = menu.offsetHeight;
  const left = Math.min(innerWidth - mw - 8, x ?? r.right - mw), top = Math.min(innerHeight - mh - 8, y ?? r.bottom + 4);
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
  // Grow out of the pointer or the button.
  menu.style.transformOrigin = `${(x ?? r.right) - left}px ${(y ?? r.bottom) - top}px`;
  menuLayer.trigger = anchor;
  menuLayer.show();
  (menu.querySelector("button:not(:disabled)") as HTMLElement | null)?.focus({ preventScroll: true });
}


async function embed(e: Entry): Promise<void> {
  // Replacing comments already in the file: say so first, and where the
  // current ones go (the server keeps them before touching the file).
  if (e.probe?.attached && !confirm(L(
    `${epName(e)} 已有弹幕，用同名 JSON 替换吗？\n\n原弹幕会先备份到 .jusplay\\backup。`,
    `${epName(e)} にはコメントがあります。同名の JSON で置き換えますか？\n\n今のコメントは .jusplay\\backup に保存されます。`))) return;
  toast(L(`正在打包 ${epName(e)}…`, `${epName(e)} を格納中…`), 60000);
  const r = await post("api/library/embed", { id: e.id });
  if (!r.ok) toast(`${L("打包失败：", "格納に失敗しました：")}${(await r.text()).slice(0, 200)}`, 5000);
  else {
    const j = await r.json();
    const n = j.comments.toLocaleString(), sec = (j.tookMs / 1000).toFixed(1);
    toast(L(`已打包 ${n} 条弹幕（${sec} 秒）`, `コメント ${n} 件を格納しました（${sec} 秒）`));
  }
  refresh(true);
}

async function pickComments(e: Entry): Promise<void> {
  const path = await window.kpPickComments?.();
  if (!path) return;
  const r = await post("api/comments/source", { id: e.id, path });
  toast(r.ok ? L("已关联弹幕文件", "コメントファイルを関連付けました") : `${L("无法使用：", "使用できません：")}${(await r.text()).slice(0, 200)}`, r.ok ? 2600 : 5000);
  refresh(true);
}

// ---- folders ------------------------------------------------------------------

function togglePop(open = !popLayer.isOpen): void {
  if (open) renderFolders();
  popLayer.toggle(open);
}

function renderFolders(): void {
  const ul = $("#folder-list");
  ul.replaceChildren();
  if (!state.folders.length) ul.append(h("li", { class: "empty", text: L("还没有文件夹", "フォルダーがありません") }));
  for (const { path: f, error } of state.folders) {
    const n = state.entries.filter((e) => e.folder === f && !e.missing).length;
    const rm = h("button", { title: L("从媒体库移除（不删除文件）", "ライブラリから外す（ファイルは削除しません）"), text: L("移除", "外す") });
    rm.onclick = async () => {
      if (!confirm(L(`从媒体库移除这个文件夹？\n${f}\n\n文件不会被删除，重新添加即可恢复记录。`,
        `このフォルダーをライブラリから外しますか？\n${f}\n\nファイルは削除されず、追加し直すと記録も戻ります。`))) return;
      await post("api/library/folders", { path: f, remove: true });
      await refresh(true);
      renderFolders();
    };
    // Records that cannot be written into the folder (read-only share) are
    // kept for this run only; say so rather than lose them silently.
    const warn = error ? h("div", { class: "folder-warn", title: error, text: L("记录无法写入这个文件夹，只保留到关闭为止", "このフォルダーに記録を書き込めないため、閉じるまでしか残りません") }) : null;
    ul.append(h("li", {}, h("span", { class: "path", title: f, text: f }), h("span", { class: "count", text: `${n}` }), rm, warn));
  }
}

async function addFolder(): Promise<void> {
  if (!window.kpPickFolder) return toast(L("这个窗口不能打开文件夹对话框", "このウィンドウではフォルダー選択ダイアログを開けません"));
  const path = await window.kpPickFolder();
  if (!path) return;
  const r = await post("api/library/folders", { path });
  if (!r.ok) return toast(`${L("无法添加：", "追加できません：")}${await r.text()}`, 4000);
  toast(L("已添加，正在扫描…", "追加しました。スキャンしています…"));
  togglePop(false);
  refresh(true);
}

// ---- wiring -------------------------------------------------------------------

bindSwitches($<HTMLButtonElement>("#theme-switch"), $<HTMLButtonElement>("#lang-switch"));
// A language switch re-renders the current view in place and keeps the scroll.
let langReady = false;
onLang(() => {
  applyStatic(LIBRARY_TEXT);
  if (!langReady) return void (langReady = true);
  const y = scrollY;
  closeMenu();
  renderScan();
  route();
  if (popLayer.isOpen) renderFolders();
  window.scrollTo(0, y);
});

$("#folders-btn").onclick = () => togglePop();
$("#add-folder").onclick = addFolder;
$("#rescan").onclick = async () => {
  await post("api/library/scan", {});
  setTimeout(() => refresh(), 300);
};
const input = $<HTMLInputElement>("#search");
input.oninput = () => {
  search = input.value;
  if (location.hash.startsWith("#/s/")) location.hash = "#/";
  else renderHome();
};
// Home <-> series as a same-page transition; the series' cover is the shared
// element both ways.
let lastHash = location.hash;
let coverFrom = "";
addEventListener("hashchange", () => {
  const m = /^#\/s\/(.+)$/.exec(lastHash);
  if (m && !location.hash.startsWith("#/s/")) coverFrom = decodeURIComponent(m[1]);
  lastHash = location.hash;
  transition(route);
  markPage();
});
// The top-left place (library.css #home-slot): the mark at home; on a
// series page the way back, ‹ and the series, as over the player.
function markPage(): void {
  const sub = location.hash.startsWith("#/s/");
  $("#top").classList.toggle("sub", sub);
  $("#brand").inert = sub;
  $("#crumb").inert = !sub;
  $("#brand").setAttribute("aria-label", L("Jusplay 首页", "Jusplay ホーム"));
  const up = $("#up");
  up.setAttribute("aria-label", L("返回媒体库", "ライブラリに戻る"));
  up.dataset.key = `${BACK_KEYS} / Esc`;
}
markPage();
onLang(markPage);
initTips();
setTitle("Jusplay");

// The header, like an iOS tab bar, folds while the page scrolls down (only
// the mark and a round search button stay) and unfolds on the way back up,
// near the top, or when pointed at or focused. The content scrolls on under
// it; its lower edge fades instead of ending on a line.
{
  const top = $("#top");
  let lastY = scrollY, queued = false;
  const set = (compact: boolean) => top.classList.toggle("compact", compact);
  addEventListener("scroll", () => {
    if (queued) return;
    queued = true;
    requestAnimationFrame(() => {
      queued = false;
      const y = scrollY, dy = y - lastY;
      if (y < 60) set(false);
      else if (dy > 6 && !top.matches(":hover, :focus-within")) set(true);
      else if (dy < -6) set(false);
      if (Math.abs(dy) > 6 || y < 60) lastY = y;
    });
  }, { passive: true });
  top.addEventListener("pointerenter", () => set(false));
  top.addEventListener("focusin", () => set(false));
}
// No links with an address anywhere in the app: the engine would show it in
// a status bubble (and it is nobody's business). Cards and the brand are
// focusable elements that act on click and Enter.
{
  $("#up").onclick = () => (location.hash = "#/");
  const brand = $("#brand");
  brand.onclick = () => (location.hash = "#/");
  brand.onkeydown = (ev) => { if (ev.key === "Enter") brand.click(); };
}
// Back from a series page goes home, as Esc does (nav.ts: one model on both pages).
onBack(() => {
  if (layers.closeTop()) return;
  if (location.hash.startsWith("#/s/")) location.hash = "#/";
});
watchInteractions("library");
document.addEventListener("keydown", (ev) => {
  const t = ev.target as HTMLElement;
  if (ev.key === "Escape") {
    if (layers.closeTop()) return;
    if (t === input && input.value) {
      input.value = "";
      search = "";
      return renderHome();
    }
    if (location.hash.startsWith("#/s/")) location.hash = "#/";
    return;
  }
  if (t.matches("input, textarea")) return;
  if (ev.key === "/") {
    ev.preventDefault();
    input.focus();
    return;
  }
  const dir = ({ ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] } as Record<string, [number, number]>)[ev.key];
  if (dir && !layers.any && !ev.altKey && !ev.ctrlKey) {
    ev.preventDefault();
    moveFocus(dir[0], dir[1]);
  }
});

/**
 * Arrow keys move the focus to the nearest card or episode in that
 * direction (by position on screen, so it works for the grid, the rail and
 * the episode list alike). Enter opens it.
 */
function moveFocus(dx: number, dy: number): void {
  const items = [...view.querySelectorAll<HTMLElement>(".card, .ep[tabindex]")];
  if (!items.length) return;
  const cur = document.activeElement as HTMLElement | null;
  if (!cur || !items.includes(cur)) return void items[0].focus();
  const c = cur.getBoundingClientRect(), cx = c.left + c.width / 2, cy = c.top + c.height / 2;
  let best: HTMLElement | null = null, bestScore = Infinity;
  for (const it of items) {
    if (it === cur) continue;
    const r = it.getBoundingClientRect(), x = r.left + r.width / 2 - cx, y = r.top + r.height / 2 - cy;
    const along = x * dx + y * dy, across = Math.abs(x * dy - y * dx);
    if (along <= 1) continue;
    const score = along + 2.5 * across;
    if (score < bestScore) (best = it), (bestScore = score);
  }
  if (best) {
    best.focus({ preventScroll: true });
    best.scrollIntoView({ block: "nearest", inline: "nearest", behavior: reduced.matches ? "auto" : "smooth" });
  }
}

// -selftest -scroll: the home page scrolled down to its end and back at
// 1600 px/s (a brisk wheel), every frame timed, long animation frames kept.
// -selftest -thumbs: make every missing thumbnail (thumbnailer.ts), one at a
// time, and report how long each took and how large it came out.
if (new URLSearchParams(location.search).has("thumbs")) {
  void (async () => {
    for (let i = 0; i < 100 && !state.entries.length; i++) await new Promise((r) => setTimeout(r, 100));
    const todo = state.entries.filter((e) => !e.missing && !e.thumb);
    const runs: { ms: number; ok: boolean; bytes: number }[] = [];
    for (const e of todo) {
      const t0 = performance.now();
      const ok = await makeThumb(e.id);
      const ms = Math.round(performance.now() - t0);
      const r = ok ? await fetch(thumbURL(e) + "&check") : null;
      runs.push({ ms, ok, bytes: r && r.ok ? (await r.arrayBuffer()).byteLength : 0 });
    }
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({ entries: state.entries.length, todo: todo.length, runs }) });
  })();
}

if (new URLSearchParams(location.search).has("scroll")) {
  void (async () => {
    const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
    for (let i = 0; i < 100 && !view.querySelector(".grid"); i++) await wait(100);
    await wait(2500); // thumbnails in
    const loaf: unknown[] = [];
    try {
      new PerformanceObserver((l) => {
        for (const e of l.getEntries() as unknown as { duration: number; startTime: number; styleAndLayoutStart: number; renderStart: number;
          scripts: { invoker: string; duration: number; sourceFunctionName: string; forcedStyleAndLayoutDuration: number }[] }[])
          loaf.push({ ms: Math.round(e.duration), beforeRender: Math.round((e.renderStart || e.startTime + e.duration) - e.startTime),
            styleLayoutMs: Math.round(e.styleAndLayoutStart ? e.startTime + e.duration - e.styleAndLayoutStart : 0),
            scripts: e.scripts.map((x) => `${x.invoker} ${Math.round(x.duration)}ms forced ${Math.round(x.forcedStyleAndLayoutDuration)} ${x.sourceFunctionName}`) });
      }).observe({ type: "long-animation-frame" });
    } catch { /* not supported */ }
    const ds: number[] = [];
    const run = (dir: 1 | -1) => new Promise<void>((done) => {
      let last = performance.now();
      const step = (t: number) => {
        ds.push(t - last);
        const px = (1600 * (t - last)) / 1000;
        last = t;
        scrollBy(0, dir * px);
        const end = dir > 0 ? scrollY + innerHeight >= document.documentElement.scrollHeight - 1 : scrollY <= 0;
        if (end) done();
        else requestAnimationFrame(step);
      };
      requestAnimationFrame(step);
    });
    const height = document.documentElement.scrollHeight;
    await run(1);
    await wait(300);
    await run(-1);
    const sorted = ds.slice(1).sort((a, b) => a - b);
    const refresh = sorted[Math.floor(sorted.length / 2)];
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({
      pageHeight: height, frames: sorted.length, refreshMs: refresh, maxMs: Math.round(sorted[sorted.length - 1] * 10) / 10,
      p99Ms: Math.round(sorted[Math.floor(sorted.length * 0.99)] * 10) / 10, dropped: sorted.filter((d) => d > refresh * 1.5).length, loaf,
    }) });
  })();
}

refresh(true).catch((e) => {
  view.replaceChildren(h("div", { class: "none", text: `${L("读取媒体库失败：", "ライブラリの読み込みに失敗しました：")}${e}` }));
});
