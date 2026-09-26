// Line icons on a 20×20 grid, drawn with currentColor (styled in CSS:
// `.icon svg`). Filled shapes carry class="f".

const svg = (body: string) => `<svg viewBox="0 0 20 20" aria-hidden="true">${body}</svg>`;

export const ICONS = {
  play: svg('<path class="f" d="M6.5 4.3v11.4a.8.8 0 0 0 1.2.7l9-5.7a.8.8 0 0 0 0-1.4l-9-5.7a.8.8 0 0 0-1.2.7z"/>'),
  pause: svg('<rect class="f" x="5" y="4" width="3.4" height="12" rx="1"/><rect class="f" x="11.6" y="4" width="3.4" height="12" rx="1"/>'),
  volume: svg('<path d="M3.5 8h2.8L10 4.8v10.4L6.3 12H3.5z"/><path d="M13.3 7.3a3.8 3.8 0 0 1 0 5.4M15.6 5.2a6.8 6.8 0 0 1 0 9.6"/>'),
  muted: svg('<path d="M3.5 8h2.8L10 4.8v10.4L6.3 12H3.5z"/><path d="m13.5 8 4 4m0-4-4 4"/>'),
  comments: svg('<path d="M4 4.5h12a1.5 1.5 0 0 1 1.5 1.5v7a1.5 1.5 0 0 1-1.5 1.5H9.5l-3.7 2.8v-2.8H4A1.5 1.5 0 0 1 2.5 13V6A1.5 1.5 0 0 1 4 4.5z"/><path d="M6 8.2h8M6 11h5"/>'),
  subtitles: svg('<rect x="2.5" y="4" width="15" height="12" rx="2"/><path d="M5.5 10.5h4M11.5 10.5h3M5.5 13h2.5M10 13h4.5"/>'),
  settings: svg('<path d="M3.5 6h7.5M15 6h1.5M3.5 14h1.5M9 14h7.5"/><circle cx="13" cy="6" r="2"/><circle cx="7" cy="14" r="2"/>'),
  fullscreen: svg('<path d="M3.5 7.5v-4h4M12.5 3.5h4v4M16.5 12.5v4h-4M7.5 16.5h-4v-4"/>'),
  exitFullscreen: svg('<path d="M7.5 3.5v4h-4M16.5 7.5h-4v-4M12.5 16.5v-4h4M3.5 12.5h4v4"/>'),
  back: svg('<path d="M12 4.5 6.5 10l5.5 5.5"/>'),
  prev: svg('<path d="M5.5 4.5v11"/><path class="f" d="M15.5 5.2v9.6a.7.7 0 0 1-1.1.6L8 10.6a.7.7 0 0 1 0-1.2l6.4-4.8a.7.7 0 0 1 1.1.6z"/>'),
  next: svg('<path d="M14.5 4.5v11"/><path class="f" d="M4.5 5.2v9.6a.7.7 0 0 0 1.1.6l6.4-4.8a.7.7 0 0 0 0-1.2L5.6 4.6a.7.7 0 0 0-1.1.6z"/>'),
  list: svg('<rect x="3" y="4" width="5.5" height="4" rx="1"/><rect x="3" y="12" width="5.5" height="4" rx="1"/><path d="M11 5.2h6M11 7.2h4M11 13.2h6M11 15.2h4"/>'),
  close: svg('<path d="m5 5 10 10M15 5 5 15"/>'),
  rewind: svg('<path class="f" d="M9.5 5.3v9.4a.6.6 0 0 1-1 .5L2.8 10.5a.6.6 0 0 1 0-1l5.7-4.7a.6.6 0 0 1 1 .5z"/><path class="f" d="M17 5.3v9.4a.6.6 0 0 1-1 .5l-5.7-4.7a.6.6 0 0 1 0-1L16 4.8a.6.6 0 0 1 1 .5z"/>'),
  forward: svg('<path class="f" d="M10.5 5.3v9.4a.6.6 0 0 0 1 .5l5.7-4.7a.6.6 0 0 0 0-1l-5.7-4.7a.6.6 0 0 0-1 .5z"/><path class="f" d="M3 5.3v9.4a.6.6 0 0 0 1 .5l5.7-4.7a.6.6 0 0 0 0-1L4 4.8a.6.6 0 0 0-1 .5z"/>'),
};
