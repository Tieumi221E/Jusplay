// Interface language and colour theme, shared by both pages. The values come
// from prefs.js (a blocking script in <head>, so the first paint is already
// right) and are saved back through api/prefs. Switching re-renders in place:
// no reload, so playback is not interrupted.

export type Lang = "zh" | "ja";
export type Theme = "dark" | "light";

export interface Prefs {
  lang: Lang;
  theme: Theme;
  sort: string;
}

declare global {
  interface Window {
    kpPrefs?: Partial<Prefs>;
    /** Native title bar colour (shell_windows.go). */
    kpTheme?: (dark: boolean) => Promise<void>;
  }
}

export const prefs: Prefs = { lang: "zh", theme: "dark", sort: "name", ...window.kpPrefs };

/** The text for the current language; both versions sit side by side at the call site. */
export const L = (zh: string, ja: string): string => (prefs.lang === "ja" ? ja : zh);

/** List separator: "，" in Chinese, "、" in Japanese. */
export const sep = (): string => L("，", "、");

const listeners: (() => void)[] = [];

/** Runs fn now and after every language change (until signal aborts). */
export function onLang(fn: () => void, signal?: AbortSignal): void {
  listeners.push(fn);
  signal?.addEventListener("abort", () => listeners.splice(listeners.indexOf(fn), 1));
  fn();
}

function applyDocument(): void {
  const d = document.documentElement;
  d.dataset.theme = prefs.theme;
  d.lang = prefs.lang === "ja" ? "ja" : "zh-CN";
  window.kpTheme?.(prefs.theme === "dark").catch(() => undefined);
}

const themeListeners: (() => void)[] = [];

/** Runs fn after every theme change (for canvas drawing that reads CSS colours). */
export function onTheme(fn: () => void, signal?: AbortSignal): void {
  themeListeners.push(fn);
  signal?.addEventListener("abort", () => themeListeners.splice(themeListeners.indexOf(fn), 1));
}

// The library and the player framed in it are two pages: a switch in one
// reaches the other at once.
const channel = "BroadcastChannel" in window ? new BroadcastChannel("jusplay-prefs") : null;
channel?.addEventListener("message", (e) => {
  const { k, v } = e.data as { k: keyof Prefs; v: never };
  if (k in prefs) apply(k, v);
});

function apply<K extends keyof Prefs>(k: K, v: Prefs[K]): boolean {
  if (prefs[k] === v) return false;
  prefs[k] = v;
  applyDocument();
  if (k === "lang") for (const fn of listeners) fn();
  if (k === "theme") for (const fn of themeListeners) fn();
  return true;
}

export function setPref<K extends keyof Prefs>(k: K, v: Prefs[K]): void {
  if (!apply(k, v)) return;
  channel?.postMessage({ k, v });
  fetch("api/prefs", { method: "PUT", body: JSON.stringify({ [k]: v }) }).catch(() => undefined);
}

const SUN = '<svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="3.6"/><path d="M10 2.2v1.9M10 15.9v1.9M2.2 10h1.9M15.9 10h1.9M4.5 4.5l1.3 1.3M14.2 14.2l1.3 1.3M4.5 15.5l1.3-1.3M14.2 5.8l1.3-1.3"/></svg>';
const MOON = '<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M16.2 12.6A6.6 6.6 0 0 1 7.4 3.8a6.6 6.6 0 1 0 8.8 8.8z"/></svg>';

/**
 * Wire the two switch buttons. The theme button shows the theme it switches
 * to; the language button names the other language in that language, so it
 * can be found by someone who cannot read the current one.
 */
export function bindSwitches(themeBtn: HTMLButtonElement, langBtn: HTMLButtonElement): void {
  const paint = () => {
    const dark = prefs.theme === "dark";
    themeBtn.innerHTML = dark ? SUN : MOON;
    themeBtn.title = dark ? L("切换到白天模式", "ライトモードに切り替え") : L("切换到夜间模式", "ダークモードに切り替え");
    themeBtn.setAttribute("aria-label", themeBtn.title);
    langBtn.textContent = prefs.lang === "ja" ? "中文" : "日本語";
    langBtn.title = prefs.lang === "ja" ? "切换到简体中文界面" : "日本語の表示に切り替え";
  };
  themeBtn.onclick = () => {
    setPref("theme", prefs.theme === "dark" ? "light" : "dark");
    paint();
  };
  langBtn.onclick = () => setPref("lang", prefs.lang === "ja" ? "zh" : "ja");
  onLang(paint);
  onTheme(paint);
}

/** Static texts of a page: [selector, property, zh, ja]. */
export type StaticText = [string, "textContent" | "title" | "placeholder" | "aria-label", string, string];

export function applyStatic(items: StaticText[]): void {
  for (const [sel, prop, zh, ja] of items) {
    const e = document.querySelector<HTMLElement>(sel);
    if (!e) continue;
    const v = L(zh, ja);
    if (prop === "textContent") e.textContent = v;
    else if (prop === "placeholder") (e as HTMLInputElement).placeholder = v;
    else e.setAttribute(prop, v);
  }
}

applyDocument();
