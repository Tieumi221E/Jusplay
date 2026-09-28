// Every control that does something names the capability it uses
// (data-cap, on it or a container), so the window has no ability the
// command line lacks (Jus contract 17). "ui" marks what only moves the view
// (a panel, a dialog, scrolling). This check finds controls without a name,
// and names that are not in the window's capabilities (GET api/cap, the
// same list as `jusplay help -json`).

export interface Coverage {
  controls: number;
  /** Controls with no data-cap: tag#id.class "label". */
  missing: string[];
  /** Capability names used that the window does not have. */
  unknown: string[];
}

const CONTROLS = "button, input, select, textarea, [role=button], [role=menuitem], [role=link], [tabindex]:not([tabindex='-1']), a[href]:not([href^='#'])";

function describe(e: Element): string {
  const id = e.id ? `#${e.id}` : "";
  const cls = e.classList.length ? `.${[...e.classList].join(".")}` : "";
  const text = (e.getAttribute("aria-label") || e.getAttribute("title") || e.textContent || "").trim().slice(0, 30);
  return `${e.tagName.toLowerCase()}${id}${cls}${text ? ` "${text}"` : ""}`;
}

export async function capCoverage(docs: Document[] = [document]): Promise<Coverage> {
  const help = (await fetch("api/cap").then((r) => r.json())) as { commands: { id: string }[] };
  const known = new Set(help.commands.map((c) => c.id));
  const out: Coverage = { controls: 0, missing: [], unknown: [] };
  for (const doc of docs) {
    for (const e of doc.querySelectorAll(CONTROLS)) {
      out.controls++;
      const cap = e.closest("[data-cap]")?.getAttribute("data-cap");
      if (!cap) {
        out.missing.push(describe(e));
        continue;
      }
      for (const c of cap.split(/\s+/)) {
        if (c !== "ui" && !c.startsWith("ui:") && !known.has(c) && !out.unknown.includes(c)) out.unknown.push(c);
      }
    }
  }
  return out;
}
