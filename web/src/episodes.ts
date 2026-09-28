// The player's episode layer: as on tvOS, the
// bottom bar itself grows upward and shows the series' episodes in a row,
// the current one lit and centred. The seek bar and buttons stay where they
// are and keep working; the picture never moves.
//
// The "Episodes ⌃" tab on the bar's top edge (where the layer appears) and E
// open and close it; resting the pointer on the tab for 400 ms opens it too
// (intent), a press opens it at once. Esc or a press on the picture closes
// it; presses on the bars do not. Inside, ← → Home End move, Enter plays;
// resting on an episode opens its session in the background, so starting
// it is quick. Choosing one switches in place, and the layer folds away.
//
// At the end of an episode the layer opens by itself with the next one lit
// and counting down (up next); closing the layer or seeking stops it.
//
// Motion is transform and opacity only: the bar's glass is one surface,
// taller than the bar, moved up inside the bar's rounded clip; the tab
// rides on its top edge; the row fades in over it.

import { Layer, type LayerStack } from "./layer.ts";
import { episodeLabel } from "./keys.ts";
import { L, onLang } from "./i18n.ts";
import { fmtTime } from "./ui.ts";
import { makeThumb } from "./thumbnailer.ts";

export interface Episode {
  id: string;
  season: number | null;
  episode: number | null;
  title: string;
  subtitle?: string;
  duration: number;
  position: number;
  watched: boolean;
  comments?: number;
  /** Other files of this episode (library.Versions). */
  versions?: string[];
  thumb: string;
  /** The thumbnail exists yet (otherwise it is made here, thumbnailer.ts). */
  hasThumb: boolean;
}

export interface EpisodeLayer {
  readonly layer: Layer;
  toggle(open?: boolean): void;
  /** The list, once loaded (for the thumbnail of N/P). */
  find(id: string): Episode | undefined;
  /** Open with this episode lit and counting down; then done(). False when it is not in the list. */
  upNext(id: string, seconds: number, done: () => void): Promise<boolean>;
  /** Stop a countdown (a seek back: watching on). */
  cancelUpNext(): void;
  /** Resolves once the layer has been drawn unseen (the first opening is then like any other). */
  readonly ready: Promise<void>;
}

const OPEN_AFTER = 400, WARM_DWELL = 250;

export function episodeLayer(opts: {
  current: string;
  stack: LayerStack;
  /** Play an episode; from is its thumbnail, for the transition. */
  go: (e: Episode, from: HTMLElement | null) => void;
  /** Called when the layer opens, to close panels that would cover it. */
  onOpen?: () => void;
  /** The episode's lifetime: listeners and warming stop with it. */
  signal?: AbortSignal;
  /** Presses on these (the bars) leave the layer open. */
  keep?: Element[];
}): EpisodeLayer {
  const signal = opts.signal;
  const el = document.querySelector<HTMLElement>("#eps")!;
  const tab = document.querySelector<HTMLButtonElement>("#eps-tab")!;
  const list = el.querySelector<HTMLOListElement>("#eps-list")!;
  const title = el.querySelector<HTMLElement>("#eps-title")!;
  const sub = el.querySelector<HTMLElement>("#eps-sub")!;
  const body = document.body;
  const barBody = document.querySelector<HTMLElement>("#bar-body")!;
  const layer = new Layer(el, opts.stack, tab);
  layer.keepOn = opts.keep ?? [];
  let eps: Episode[] | null = null;
  let series = "";
  let loading: Promise<void> | null = null;

  const load = () => (loading ??= fetch(`api/episodes?id=${encodeURIComponent(opts.current)}`)
    .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
    .then((d: { series: string; episodes: Episode[] }) => {
      eps = d.episodes;
      series = d.series;
      render();
    })
    .catch(() => {
      loading = null; // try again on the next opening
    }));

  // Thumbnails load when their card comes near.
  const io = new IntersectionObserver((es) => {
    for (const x of es) {
      if (!x.isIntersecting) continue;
      const img = x.target as HTMLImageElement;
      io.unobserve(img);
      if (img.dataset.has === "0") {
        void makeThumb(img.dataset.id!).then((ok) => (ok ? (img.src = img.dataset.src!) : img.remove()));
        continue;
      }
      img.src = img.dataset.src!;
    }
  }, { root: list, rootMargin: "0px 400px" });
  signal?.addEventListener("abort", () => io.disconnect());

  const multiSeason = () => new Set((eps ?? []).map((e) => e.season ?? 1)).size > 1;
  const isNow = (e: Episode) => e.id === opts.current || !!e.versions?.includes(opts.current);

  function card(e: Episode): HTMLLIElement {
    const li = document.createElement("li");
    const b = document.createElement("button");
    b.dataset.cap = "player.open";
    b.className = "ep-card";
    b.dataset.id = e.id;
    const now = isNow(e);
    if (now) b.setAttribute("aria-current", "true");
    const thumb = document.createElement("span");
    thumb.className = "ep-thumb";
    const img = document.createElement("img");
    img.alt = "";
    img.decoding = "async";
    img.dataset.src = e.thumb;
    img.dataset.id = e.id;
    img.dataset.has = e.hasThumb ? "1" : "0";
    img.onload = () => img.classList.add("loaded");
    img.onerror = () => img.remove();
    io.observe(img);
    thumb.append(img);
    const p = e.duration ? e.position / e.duration : 0;
    if (!e.watched && p > 0.01) {
      const bar = document.createElement("i");
      bar.className = "ep-progress";
      bar.style.setProperty("--p", String(Math.min(1, p)));
      thumb.append(bar);
    }
    if (now) {
      const eq = document.createElement("span");
      eq.className = "ep-now";
      eq.innerHTML = "<i></i><i></i><i></i>";
      thumb.append(eq);
    } else if (e.watched) {
      const w = document.createElement("span");
      w.className = "ep-watched";
      w.textContent = "✓";
      thumb.append(w);
    }
    // Up next: a bar that fills over the countdown.
    const count = document.createElement("i");
    count.className = "ep-count";
    thumb.append(count);
    const name = document.createElement("span");
    name.className = "ep-name";
    const label = episodeLabel(e.season, e.episode, multiSeason());
    name.textContent = label && e.subtitle ? `${label} ${e.subtitle}` : label || e.title;
    const meta = document.createElement("span");
    meta.className = "ep-meta";
    meta.textContent = metaText(e);
    b.append(thumb, name, meta);
    b.setAttribute("aria-label", [label || e.title, e.subtitle, meta.textContent].filter(Boolean).join("，"));
    b.onclick = () => {
      cancelUpNext();
      opts.go(e, thumb);
    };
    warmOnRest(b, e.id);
    li.append(b);
    return li;
  }

  function metaText(e: Episode): string {
    const p = e.duration ? e.position / e.duration : 0;
    if (isNow(e)) return L("正在播放", "再生中");
    if (e.watched) return L("已看完", "視聴済み");
    if (p > 0.01) return L(`剩 ${Math.max(1, Math.round((e.duration - e.position) / 60))} 分钟`, `残り ${Math.max(1, Math.round((e.duration - e.position) / 60))} 分`);
    return e.duration ? fmtTime(e.duration) : "";
  }

  function render(): void {
    if (!eps) return;
    title.textContent = series;
    const watched = eps.filter((e) => e.watched).length;
    sub.textContent = L(`${eps.length} 集${watched ? ` · 已看 ${watched}` : ""}`, `全 ${eps.length} 話${watched ? ` · ${watched} 話視聴済み` : ""}`);
    list.replaceChildren(...eps.map((e) => card(e)));
  }
  const tabLabel = () => {
    tab.querySelector("span")!.textContent = L("剧集", "エピソード");
  };
  tabLabel();
  onLang(() => (render(), tabLabel()), signal);

  // ---- warming ----
  const warmed = new Set<string>([opts.current]);
  function warmOnRest(b: HTMLElement, id: string): void {
    let t = 0;
    b.addEventListener("pointerenter", () => {
      if (warmed.has(id)) return;
      t = window.setTimeout(() => {
        warmed.add(id);
        fetch(`api/session?id=${encodeURIComponent(id)}`).catch(() => undefined);
      }, WARM_DWELL);
    }, { signal });
    b.addEventListener("pointerleave", () => clearTimeout(t), { signal });
  }

  // ---- open / close ----
  const cards = () => [...list.querySelectorAll<HTMLButtonElement>(".ep-card")];
  const centre = (c: HTMLElement | null) => {
    if (c) list.scrollLeft = c.offsetLeft - list.clientWidth / 2 + c.offsetWidth / 2;
  };
  // The bar's clip, only while the layer moves or is open (app.css): put on
  // with the glass already at its closed place (no transition), taken off
  // once the glass is back.
  let settleTimer = 0;
  const clip = () => {
    clearTimeout(settleTimer);
    if (barBody.classList.contains("moving")) return;
    barBody.classList.add("moving", "snap");
    void barBody.offsetHeight;
    barBody.classList.remove("snap");
  };
  const unclipLater = () => {
    clearTimeout(settleTimer);
    settleTimer = window.setTimeout(() => {
      if (!body.classList.contains("eps-open")) barBody.classList.remove("moving");
    }, 600);
  };
  layer.onClose = () => {
    body.classList.remove("eps-open");
    unclipLater();
    cancelUpNext();
    if (el.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
  };
  async function open(focus: boolean, lit?: HTMLElement | null): Promise<void> {
    opts.onOpen?.();
    if (!eps) await load();
    if (!eps || signal?.aborted) return;
    const cur = lit ?? list.querySelector<HTMLElement>("[aria-current]");
    // Opened while being warmed: shown for real from here.
    el.classList.remove("warm");
    el.style.opacity = "";
    el.style.pointerEvents = "";
    clip();
    body.classList.add("eps-open");
    layer.show();
    // Centred as it shows, without animation (it needs layout, so after show).
    centre(cur);
    if (focus) (cur ?? cards()[0])?.focus({ preventScroll: true });
  }
  const toggle = (want = !layer.isOpen) => (want ? void open(true) : layer.hide());

  let restTimer = 0;
  tab.addEventListener("click", () => {
    clearTimeout(restTimer);
    if (layer.isOpen) layer.hide();
    else void open(false);
  }, { signal });
  // Resting on the tab is intent enough; leaving first cancels.
  tab.addEventListener("pointerenter", () => {
    if (layer.isOpen) return;
    restTimer = window.setTimeout(() => void open(false), OPEN_AFTER);
  }, { signal });
  tab.addEventListener("pointerleave", () => clearTimeout(restTimer), { signal });
  signal?.addEventListener("abort", () => clearTimeout(restTimer));

  // Keys inside the row; the vertical wheel scrolls it sideways.
  el.addEventListener("keydown", (e) => {
    const cs = cards();
    const i = cs.indexOf(document.activeElement as HTMLButtonElement);
    const to = ({ ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: cs.length - 1, PageDown: i + 5, PageUp: i - 5 } as Record<string, number>)[e.key];
    if (to === undefined) return;
    e.preventDefault();
    e.stopPropagation();
    const t = cs[Math.max(0, Math.min(cs.length - 1, i < 0 ? 0 : to))];
    t?.focus({ preventScroll: true });
    t?.scrollIntoView({ inline: "nearest", block: "nearest" });
  }, { signal });
  list.addEventListener("wheel", (e) => {
    e.stopPropagation(); // not the volume
    if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
    e.preventDefault();
    list.scrollLeft += e.deltaY;
  }, { passive: false, signal });

  // ---- up next ----
  let upTimer = 0, upCard: HTMLElement | null = null;
  function cancelUpNext(): void {
    clearInterval(upTimer);
    if (!upCard) return;
    upCard.classList.remove("counting");
    const e = eps?.find((x) => x.id === upCard!.dataset.id);
    if (e) upCard.querySelector(".ep-meta")!.textContent = metaText(e);
    upCard = null;
  }
  signal?.addEventListener("abort", () => clearInterval(upTimer));
  async function upNext(id: string, seconds: number, done: () => void): Promise<boolean> {
    if (!eps) await load();
    const c = list.querySelector<HTMLElement>(`.ep-card[data-id="${CSS.escape(id)}"]`);
    if (!c || signal?.aborted) return false;
    cancelUpNext();
    await open(false, c);
    upCard = c;
    c.style.setProperty("--count", `${seconds}s`);
    const meta = c.querySelector(".ep-meta")!;
    let left = seconds;
    const say = () => (meta.textContent = L(`${left} 秒后播放`, `${left} 秒後に再生`));
    say();
    void c.offsetWidth;
    c.classList.add("counting");
    c.focus({ preventScroll: true }); // Enter plays it now
    upTimer = window.setInterval(() => {
      left--;
      say();
      if (left <= 0) {
        cancelUpNext();
        done();
      }
    }, 1000);
    return true;
  }

  // Fetched once the player is idle, so the first opening is immediate;
  // then shown once, all but invisible. A first opening otherwise held one
  // frame for 110-160 ms (120 Hz, -selftest -dock): the first rasterisation
  // and compositing. Opacity 0 or visibility: hidden does not help (the
  // engine skips drawing those); 0.01 is drawn.
  // An idle moment, but within 2 s: during playback idle time can be scarce.
  const idle = "requestIdleCallback" in window ? (f: () => void) => requestIdleCallback(f, { timeout: 2000 }) : (f: () => void) => setTimeout(f, 1500);
  let markWarm!: () => void;
  const ready = new Promise<void>((r) => (markWarm = r));
  idle(async () => {
    await load();
    if (!eps || layer.isOpen || signal?.aborted) return markWarm();
    el.style.opacity = "0.01";
    el.style.pointerEvents = "none";
    el.hidden = false;
    clip(); // the bar's clip drawn once too
    centre(list.querySelector<HTMLElement>("[aria-current]"));
    // The thumbnails near the current episode, loaded and decoded now.
    const imgs = [...list.querySelectorAll<HTMLImageElement>(`img[data-src][data-has="1"]`)];
    const at = Math.max(0, cards().findIndex((c) => c.hasAttribute("aria-current")));
    await Promise.all(imgs.slice(Math.max(0, at - 8), at + 9).map((img) => {
      if (!img.src) img.src = img.dataset.src!;
      return img.decode().catch(() => undefined);
    }));
    // At its open place too, by a class of its own (.open is the layer's state).
    el.classList.add("warm");
    await new Promise((r) => setTimeout(r, 400));
    el.classList.remove("warm");
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    if (!layer.isOpen) {
      unclipLater();
      el.hidden = true;
      el.style.opacity = "";
      el.style.pointerEvents = "";
    }
    markWarm();
  });

  return {
    layer,
    toggle,
    find: (id) => eps?.find((e) => e.id === id),
    upNext,
    cancelUpNext,
    ready,
  };
}
