// Interaction latency, always on (docs/design.md §二.1 and §二.6). The
// browser reports only interactions that took 16 ms or more from input to
// the next frame (Event Timing), so watching costs next to nothing when the
// page is fast. Each interaction over 50 ms goes to the Go log at once; a
// summary (count, how many took a frame or more, the slowest) when the page
// is left.

export function watchInteractions(page: string): void {
  let total = 0, slow = 0, worst = 0, worstName = "";
  const seen = new Set<number>();
  const count = () => total++;
  const keys: [number, string][] = []; // time stamp and key of recent presses, to name slow ones
  addEventListener("keydown", (e) => {
    total++;
    keys.push([e.timeStamp, e.key]);
    if (keys.length > 40) keys.shift();
  }, true);
  addEventListener("keyup", (e) => {
    keys.push([e.timeStamp, e.key]);
    if (keys.length > 40) keys.shift();
  }, true);
  addEventListener("pointerdown", count, true);
  const log = (msg: string, beacon = false) => {
    if (beacon) navigator.sendBeacon("api/log", msg);
    else fetch("api/log", { method: "POST", body: msg }).catch(() => undefined);
  };
  try {
    new PerformanceObserver((list) => {
      for (const e of list.getEntries() as PerformanceEventTiming[]) {
        if (!e.interactionId || seen.has(e.interactionId)) continue;
        seen.add(e.interactionId);
        slow++;
        if (e.duration > worst) (worst = e.duration), (worstName = e.name);
        if (e.duration > 50) {
          log(`${page}: slow interaction ${e.name}${e.name.startsWith("key") ? ` "${keys.find(([t]) => Math.abs(t - e.startTime) < 1)?.[1] ?? "?"}"` : ""} ${Math.round(e.duration)} ms (input delay ${Math.round(e.processingStart - e.startTime)}, handlers ${Math.round(e.processingEnd - e.processingStart)})`);
        }
      }
    }).observe({ type: "event", durationThreshold: 16, buffered: true } as PerformanceObserverInit);
  } catch {
    return; // no Event Timing: nothing to report
  }
  addEventListener("pagehide", () => {
    if (total) log(`${page}: ${total} inputs, ${slow} interactions took a frame or more (≥16 ms), slowest ${Math.round(worst)} ms${worstName ? ` (${worstName})` : ""}`, true);
  });
}
