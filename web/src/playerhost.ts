// The player lives over the library, in one iframe kept for the whole run
// (docs/design.md 五.3): opening an episode does not load a page, and the
// library stays underneath as it was (scroll, focus, filters). The player
// page is loaded once, at an idle moment after the library, and kept; on
// back only its episode ends (stream and renderer), so the next opening is
// immediate. The frame keeps the player's styles and scripts apart from the
// library's; `jusplay play` still opens the same page on its own.
//
// It is loaded when an episode is first pointed at or focused (warm), not
// as soon as the library is idle: the frame costs about 36 MB, and someone
// only browsing the library does not pay it. Between pointing and pressing
// there is time to load it (measured below, "player frame ready").
//
// The move between them is a zoom, both ways: the thumbnail grows from its
// card into the picture's place, and on back the picture shrinks into the
// card of the episode that was playing (the player may have switched), or
// towards the middle when that card is not on screen. The player shows the
// same thumbnail in the same place until its first frame, so the hand-over
// at the end of the zoom cannot be seen.

interface PlayerApi {
  open(id: string, thumb?: string): Promise<void>;
  close(): void;
}

const reduced = matchMedia("(prefers-reduced-motion: reduce)");

export interface PlayerHost {
  /** Open id, zooming from the element it was chosen from. */
  open(id: string, thumb: string | undefined, from: Element | null): void;
  /** An episode is pointed at or focused: have the player ready. */
  warm(): void;
  readonly playing: boolean;
}

export function playerHost(opts: {
  /** The thumbnail URL of an entry. */
  thumbOf(id: string): string | undefined;
  /** Where an entry is shown in the library now (its card's picture), for the way back. */
  sourceOf(id: string): HTMLElement | null;
  /** The player is gone (id: the episode it showed last). */
  onClosed(id: string): void;
}): PlayerHost {
  const frame = document.createElement("iframe");
  frame.id = "player-frame";
  frame.title = "player";
  let api: PlayerApi | null = null;
  let loaded: Promise<PlayerApi> | null = null;
  let playing = false;

  const load = () => (loaded ??= new Promise<PlayerApi>((resolve) => {
    const t0 = performance.now();
    addEventListener("message", function ready(e) {
      if (e.source !== frame.contentWindow || e.data?.type !== "jusplay:ready") return;
      removeEventListener("message", ready);
      fetch("api/log", { method: "POST", body: `player frame ready in ${Math.round(performance.now() - t0)} ms` }).catch(() => undefined);
      api = (frame.contentWindow as unknown as { jusplay: PlayerApi }).jusplay;
      resolve(api);
    });
    frame.src = "index.html";
    document.body.append(frame);
  }));

  // ---- the zoom ----
  const bg = document.createElement("div");
  bg.id = "zoom-bg";
  const box = document.createElement("div");
  box.id = "zoom";
  const img = document.createElement("img");
  img.alt = "";
  box.append(img);
  box.hidden = bg.hidden = true;
  document.body.append(bg, box);

  /** The picture's place when playing: the window, at 16:9, centred. */
  const stage = () => {
    const W = innerWidth, H = innerHeight;
    let w = W, h = w * 9 / 16;
    if (h > H) (h = H), (w = h * 16 / 9);
    return { x: (W - w) / 2, y: (H - h) / 2, w, h };
  };
  const toward = (r: DOMRect | null) => {
    const s = stage();
    if (!r || !r.width) {
      // Not on screen: towards the middle, small.
      const k = 0.3;
      return `translate(${s.x + (s.w * (1 - k)) / 2 - s.x}px, ${s.y + (s.h * (1 - k)) / 2 - s.y}px) scale(${k})`;
    }
    return `translate(${r.left - s.x}px, ${r.top - s.y}px) scale(${r.width / s.w})`;
  };
  const place = () => {
    const s = stage();
    Object.assign(box.style, { left: `${s.x}px`, top: `${s.y}px`, width: `${s.w}px`, height: `${s.h}px` });
  };
  const moved = () => new Promise<void>((done) => {
    const t = setTimeout(done, 700);
    box.addEventListener("transitionend", () => {
      clearTimeout(t);
      done();
    }, { once: true });
  });

  async function zoomIn(src: string | undefined, from: Element | null): Promise<void> {
    place();
    img.src = src ?? "";
    const r = from?.getBoundingClientRect() ?? null;
    box.hidden = false;
    bg.hidden = false;
    if (reduced.matches) {
      box.style.transform = "";
      bg.classList.add("on");
      return;
    }
    box.classList.remove("moving");
    box.style.transform = toward(r);
    void box.offsetWidth; // from the card
    box.classList.add("moving");
    box.style.transform = "";
    bg.classList.add("on");
    await moved();
  }

  async function zoomOut(src: string | undefined, to: HTMLElement | null): Promise<void> {
    place();
    img.src = src ?? "";
    box.hidden = false;
    box.classList.remove("moving");
    box.style.transform = "";
    void box.offsetWidth;
    bg.classList.remove("on");
    if (!reduced.matches) {
      box.classList.add("moving");
      box.style.transform = toward(to?.getBoundingClientRect() ?? null);
      box.classList.toggle("fade", !to);
      await moved();
    }
    box.hidden = true;
    bg.hidden = true;
    box.classList.remove("fade");
  }

  // ---- back from the player ----
  addEventListener("message", async (e) => {
    if (e.source !== frame.contentWindow || e.data?.type !== "jusplay:back" || !playing) return;
    const id = String(e.data.id ?? "");
    playing = false;
    document.body.classList.remove("playing"); // the library is drawn again: its cards can be measured
    api?.close();
    frame.blur();
    const to = opts.sourceOf(id);
    await zoomOut(opts.thumbOf(id), to);
    opts.onClosed(id);
  });

  return {
    get playing() {
      return playing;
    },
    warm: () => void load(),
    open(id, thumb, from) {
      if (playing) return;
      playing = true;
      const zoom = zoomIn(thumb, from);
      void load().then(async (p) => {
        // The player puts the thumbnail up at once and starts the episode
        // under it; the hand-over waits for the zoom only.
        void p.open(id, thumb);
        await zoom;
        // The player shows the same thumbnail in the same place: hand over.
        document.body.classList.add("playing");
        box.hidden = true;
        frame.focus();
        frame.contentWindow?.focus();
      });
    },
  };
}
