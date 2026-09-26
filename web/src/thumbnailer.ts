// Thumbnails are made here, in the page: the server has no decoder
// (internal/player/thumbs.go). For a video without one, the keyframe a
// third of the way in comes as a one-sample stream (api/thumbsrc), the
// engine decodes it (Media Source Extensions, the same hardware path as
// playback), and it is drawn 480 px wide and sent back as WebP at quality
// 0.75 (api/thumb, POST), where it is kept in the video's folder records.
//
// One at a time; the most recently asked for first (what just came into
// view). While a video plays over the library the queue waits.

type Waiter = (ok: boolean) => void;

const made = new Set<string>();
const failed = new Set<string>();
const waiting = new Map<string, Waiter[]>();
const queue: string[] = [];
let busy = false;

/** Make id's thumbnail if needed; true once it exists on the server. */
export function makeThumb(id: string): Promise<boolean> {
  if (made.has(id)) return Promise.resolve(true);
  if (failed.has(id)) return Promise.resolve(false);
  return new Promise((resolve) => {
    const w = waiting.get(id);
    if (w) w.push(resolve);
    else waiting.set(id, [resolve]);
    const i = queue.indexOf(id);
    if (i >= 0) queue.splice(i, 1);
    queue.push(id);
    void pump();
  });
}

async function pump(): Promise<void> {
  if (busy) return;
  busy = true;
  while (queue.length) {
    // Not while a video plays over the library (its decoder comes first).
    while (document.body.classList.contains("playing")) await new Promise((r) => setTimeout(r, 1000));
    const id = queue.pop()!;
    let ok = false;
    try {
      ok = await make(id);
    } catch (e) {
      ok = false;
      fetch("api/log", { method: "POST", body: `thumbnail ${id}: ${e instanceof Error ? e.message : e}` }).catch(() => undefined);
    }
    (ok ? made : failed).add(id);
    for (const r of waiting.get(id) ?? []) r(ok);
    waiting.delete(id);
  }
  busy = false;
}

async function make(id: string): Promise<boolean> {
  const res = await fetch(`api/thumbsrc?id=${encodeURIComponent(id)}`);
  if (!res.ok) return false;
  const mime = res.headers.get("X-Jusplay-Mime") ?? "";
  const offset = Number(res.headers.get("X-Jusplay-Offset"));
  const start = Number(res.headers.get("X-Jusplay-Start"));
  const data = await res.arrayBuffer();
  if (!("MediaSource" in window) || !MediaSource.isTypeSupported(mime)) return false;
  // A detached element: it decodes without being laid out. (The page's
  // CSP must allow blob: media, library.html.)
  const video = document.createElement("video");
  video.muted = true;
  video.preload = "auto";
  const ms = new MediaSource();
  const url = URL.createObjectURL(ms);
  video.src = url;
  try {
    await new Promise((r) => ms.addEventListener("sourceopen", r, { once: true }));
    const sb = ms.addSourceBuffer(mime);
    sb.timestampOffset = offset;
    await new Promise<void>((resolve, reject) => {
      sb.addEventListener("updateend", () => resolve(), { once: true });
      sb.addEventListener("error", () => reject(new Error("append failed")), { once: true });
      sb.appendBuffer(data);
    });
    ms.endOfStream();
    video.currentTime = start;
    await new Promise<void>((resolve, reject) => {
      const t = setTimeout(() => reject(new Error("no frame")), 4000);
      const check = () => {
        if (video.readyState >= 2 && !video.seeking) {
          clearTimeout(t);
          resolve();
        }
      };
      video.addEventListener("seeked", check);
      video.addEventListener("loadeddata", check);
      video.addEventListener("error", () => reject(new Error("decode failed")), { once: true });
      check();
    });
    if (!video.videoWidth) return false;
    const w = 480;
    const h = Math.max(2, Math.round((w * video.videoHeight) / video.videoWidth / 2) * 2);
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    canvas.getContext("2d")!.drawImage(video, 0, 0, w, h);
    const blob = await new Promise<Blob | null>((r) => canvas.toBlob(r, "image/webp", 0.75));
    if (!blob || blob.type !== "image/webp") return false;
    const put = await fetch(`api/thumb?id=${encodeURIComponent(id)}`, { method: "POST", body: blob });
    return put.ok;
  } finally {
    video.removeAttribute("src");
    video.load();
    URL.revokeObjectURL(url);
  }
}
