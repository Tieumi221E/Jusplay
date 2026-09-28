// Overlays that come and go. A layer is shown by
// adding .open and hidden by removing it; the CSS transition runs both ways
// and reverses from wherever it is when interrupted. After the exit, the
// element leaves the render tree (hidden), so a closed layer costs nothing.
//
// Open layers form a stack: Escape and a press outside close only the top
// one. A press outside that closed a layer is reported to the caller, which
// can then swallow the click (a click on the video closes the menu instead
// of also pausing).

export class Layer {
  private timer = 0;
  onClose: (() => void) | null = null;
  /** Presses on these do not count as outside (the drawer stays while the bars are used). */
  keepOn: Element[] = [];

  /** trigger: the button that opens it (aria-expanded; a press on it is not "outside"). Can change per opening. */
  constructor(readonly el: HTMLElement, private readonly stack: LayerStack | null = null, public trigger: HTMLElement | null = null) {}

  get isOpen(): boolean {
    return this.el.classList.contains("open");
  }

  show(): void {
    clearTimeout(this.timer);
    if (this.isOpen) return;
    if (this.el.hidden) {
      this.el.hidden = false;
      void this.el.offsetWidth; // start the transition from the closed state
    }
    this.el.classList.add("open");
    this.trigger?.setAttribute("aria-expanded", "true");
    this.stack?.push(this);
  }

  hide(): void {
    if (!this.isOpen) return;
    this.el.classList.remove("open");
    this.trigger?.setAttribute("aria-expanded", "false");
    this.stack?.remove(this);
    const ms = parseFloat(getComputedStyle(this.el).getPropertyValue("--layer-exit")) || 200;
    this.timer = window.setTimeout(() => {
      if (!this.isOpen) this.el.hidden = true;
    }, ms);
    this.onClose?.();
  }

  toggle(open = !this.isOpen): void {
    if (open) this.show();
    else this.hide();
  }
}

export class LayerStack {
  private layers: Layer[] = [];
  onChange: (() => void) | null = null;

  constructor(signal?: AbortSignal) {
    // Capture, so this runs before the pressed element's own handlers.
    document.addEventListener("pointerdown", (e) => {
      const top = this.top;
      if (!top) return;
      const t = e.target as Node;
      if (top.el.contains(t) || top.trigger?.contains(t) || top.keepOn.some((k) => k.contains(t))) return;
      top.hide();
      this.closedByPress = e.target as Element;
    }, { capture: true, signal });
  }

  /** The element whose press last closed a layer, until the next check. */
  private closedByPress: Element | null = null;

  /** True once, if the current click's press closed a layer. */
  consumePress(el: Element): boolean {
    const hit = this.closedByPress !== null && el.contains(this.closedByPress);
    this.closedByPress = null;
    return hit;
  }

  get top(): Layer | undefined {
    return this.layers[this.layers.length - 1];
  }

  get any(): boolean {
    return this.layers.length > 0;
  }

  push(l: Layer): void {
    this.remove(l);
    this.layers.push(l);
    this.onChange?.();
  }

  remove(l: Layer): void {
    const i = this.layers.indexOf(l);
    if (i >= 0) {
      this.layers.splice(i, 1);
      this.onChange?.();
    }
  }

  /** Close the top layer; false when there was none. */
  closeTop(): boolean {
    const top = this.top;
    if (!top) return false;
    top.hide();
    return true;
  }
}
