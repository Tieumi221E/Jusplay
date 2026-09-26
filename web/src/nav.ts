// One navigation model for both pages (docs/design.md, navigation): "back"
// is Alt+←, Backspace and the mouse's back button everywhere, and the
// on-screen back control shows those keys. Escape closes the innermost
// thing first (a layer, fullscreen); only then does it go back, and on the
// player it never leaves playback.

export const BACK_KEYS = "Alt ← / Backspace";

declare global {
  interface Window { kpTitle?: (t: string) => Promise<void> }
}

/** The page that has the shell's bindings (kpFullscreen, kpTitle, …): this
 * one, or the library when the player is framed in it. */
export const host: Window = window.parent !== window ? window.parent : window;
export const framed = host !== window;

/** The page's title, also on the window (the shell sets it only at start). */
export function setTitle(t: string): void {
  document.title = t;
  host.kpTitle?.(t).catch(() => undefined);
}

/** Calls back() for every back gesture outside text fields. */
export function onBack(back: () => void, signal?: AbortSignal): void {
  document.addEventListener("keydown", (e) => {
    const t = e.target as HTMLElement;
    const typing = t.matches("input:not([type=range]):not([type=checkbox]), textarea, select, [contenteditable]");
    const alt = e.altKey && !e.ctrlKey && !e.shiftKey && e.key === "ArrowLeft";
    const bs = e.key === "Backspace" && !e.altKey && !e.ctrlKey && !typing;
    if ((alt || bs || e.key === "BrowserBack") && !e.defaultPrevented) {
      e.preventDefault();
      back();
    }
  }, { signal });
  // The mouse's back button (button 3). The engine would otherwise go back
  // in history on its own, which is not always the same place.
  const side = (e: MouseEvent) => {
    if (e.button !== 3) return false;
    e.preventDefault();
    return true;
  };
  document.addEventListener("mousedown", side, { signal });
  document.addEventListener("mouseup", (e) => {
    if (side(e)) back();
  }, { signal });
}
