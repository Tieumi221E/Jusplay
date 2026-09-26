import { Engine, type MediaInfo } from "./mse.ts";
import { Clock, Overlay, createRenderer } from "./overlay.ts";
import { applyFilters, type V1Thread, type FilterStats } from "./filter.ts";
import { merge, clamp, type Settings } from "./settings.ts";
import { $, buildPanel, buildQuick, fmtTime, toast } from "./ui.ts";
import { Layer, LayerStack } from "./layer.ts";
import { initTips } from "./tip.ts";
import { watchInteractions } from "./perf.ts";
import { SeekBar, bufferedAhead } from "./bar.ts";
import { episodeLabel, seriesKey } from "./keys.ts";
import { L, applyStatic, bindSwitches, onLang, onTheme, type StaticText } from "./i18n.ts";
import { ICONS } from "./icons.ts";
import { episodeLayer, type Episode } from "./episodes.ts";
import { onBack, setTitle, host, framed, BACK_KEYS } from "./nav.ts";

interface CommentsInfo {
  source: "manual" | "attachment" | "same-name" | "none";
  path?: string;
  problems?: string[];
  videoId?: string;
  snapshotId?: string;
  captureStatus?: string;
  capturedTo?: string | null;
  offsetMs: number;
  syncVerified: boolean;
  error?: string;
}

interface Session {
  id: string;
  media: MediaInfo & { width: number; height: number; frameRate: string };
  file: string;
  path: string;
  folder: string;
  series: string;
  season: number | null;
  episode: number | null;
  title: string;
  position: number;
  comments: CommentsInfo;
  key: string;
  /** The comment offset chosen for this file, kept in its folder's records. */
  offsetMs?: number;
  prev?: string;
  next?: string;
}

const firstId = new URLSearchParams(location.search).get("id") ?? "";

declare global {
  interface Window {
    kpFullscreen?: (on: boolean) => Promise<void>;
    kpPickComments?: () => Promise<string>;
    kpPickSnapshotFolder?: () => Promise<string>;
  }
}

// Texts written in index.html, in both languages. Buttons carry their
// label as aria-label, which the tooltips show with the key (data-key).
const PLAYER_TEXT: StaticText[] = [
  ["#back", "aria-label", "返回媒体库", "ライブラリに戻る"],
  ["#seek", "aria-label", "进度", "再生位置"],
  ["#play", "aria-label", "播放 / 暂停", "再生 / 一時停止"],
  ["#mute", "aria-label", "静音", "ミュート"],
  ["#volume", "aria-label", "音量", "音量"],
  ["#rate", "aria-label", "倍速", "再生速度"],
  ["#toggle-comments", "aria-label", "弹幕开关", "コメント表示"],
  ["#open-quick", "aria-label", "弹幕设置", "コメント設定"],
  ["#fullscreen", "aria-label", "全屏", "全画面"],
  ["#open-help", "textContent", "快捷键", "ショートカット"],
  ["#open-settings", "textContent", "更多设置", "詳細設定"],
  ["#panel header strong", "textContent", "设置", "設定"],
  ["#help-title", "textContent", "快捷键", "ショートカット"],
  ["#help-done", "textContent", "完成", "完了"],
  ["#source h3", "textContent", "弹幕", "コメント"],
  ["#source .lbl", "textContent", "来源", "読み込み元"],
  ["#pick-comments", "textContent", "选择弹幕文件…", "コメントファイルを選択…"],
  ["#pick-snapshot", "textContent", "快照文件夹…", "スナップショットフォルダー…"],
  ["#auto-comments", "textContent", "恢复自动", "自動に戻す"],
  ["#sync h3", "textContent", "同步", "同期"],
  ["#sync .lbl", "textContent", "评论偏移", "表示オフセット"],
  ["#info h3", "textContent", "信息", "情報"],
  ["#appearance h3", "textContent", "外观", "外観"],
  ["#lbl-lang", "textContent", "界面语言", "表示言語"],
  ["#copy-log", "textContent", "复制诊断日志", "診断ログをコピー"],
  ["#log-hint", "textContent", "日志只在内存里，关闭后即消失；出问题时复制给开发者即可。", "ログはメモリ上にだけあり、閉じると消えます。問題があればコピーして開発者に渡してください。"],
];
onLang(() => applyStatic(PLAYER_TEXT));
const setIcon = (el: Element, name: keyof typeof ICONS) => {
  if ((el as HTMLElement).dataset.shown !== name) {
    el.innerHTML = ICONS[name];
    (el as HTMLElement).dataset.shown = name;
  }
};
for (const el of document.querySelectorAll<HTMLElement>("[data-icon]")) setIcon(el, el.dataset.icon as keyof typeof ICONS);
bindSwitches($<HTMLButtonElement>("#theme-switch"), $<HTMLButtonElement>("#lang-switch"));
const tips = initTips();
watchInteractions("player");

// Arriving from the library, the clicked thumbnail (?thumb=) stands in for
// the picture: the page transition grows it into the stage (both carry
// view-transition-name: hero), and it fades out under the first real frame.
{
  const thumb = new URLSearchParams(location.search).get("thumb");
  const morph = $<HTMLImageElement>("#morph");
  if (thumb && /^api\/thumb\?/.test(thumb)) {
    morph.src = thumb;
    morph.hidden = false;
  }
}

// Centre feedback for playback actions: an icon and a short text, gone
// again after a moment (docs/design.md §二.3).
let hudTimer = 0;
function hud(icon: keyof typeof ICONS | null, text = ""): void {
  const h = $("#hud");
  const i = h.querySelector<HTMLElement>(".hud-icon")!;
  i.hidden = !icon;
  if (icon) i.innerHTML = ICONS[icon];
  h.querySelector(".hud-text")!.textContent = text;
  h.classList.toggle("icon-only", !text);
  h.classList.remove("show");
  void h.offsetWidth;
  h.classList.add("show");
  clearTimeout(hudTimer);
  hudTimer = window.setTimeout(() => h.classList.remove("show"), 650);
}

// Startup timeline (ms since navigation start), reported by the selftest.
const marks: Record<string, number> = {};
const mark = (name: string) => void (marks[name] ??= Math.round(performance.now()));
(window as unknown as { kpMarks: typeof marks }).kpMarks = marks;

const video = $<HTMLVideoElement>("#video");
video.addEventListener("loadeddata", () => mark("firstFrameData"), { once: true });
video.requestVideoFrameCallback(() => mark("firstFramePresented"));

/** The stand-in thumbnail fades out under the episode's first real frame. */
function liftMorphOnFirstFrame(signal: AbortSignal): void {
  video.requestVideoFrameCallback(() => {
    if (signal.aborted) return;
    dispatchEvent(new CustomEvent("kp:firstframe"));
    const morph = $("#morph");
    if (!morph.hidden) {
      morph.classList.add("gone");
      setTimeout(() => {
        if (morph.classList.contains("gone")) morph.hidden = true;
      }, 400);
    }
  });
}

// Everything that goes wrong in the page also goes to the Go log file, so a
// crash that takes the window down still leaves a trail.
let logged = 0;
function logToHost(msg: string): void {
  if (++logged > 200) return;
  fetch("api/log", { method: "POST", body: msg.slice(0, 4000) }).catch(() => undefined);
}
addEventListener("error", (e) => logToHost(`uncaught: ${e.message} at ${e.filename}:${e.lineno}`));
addEventListener("unhandledrejection", (e) => logToHost(`unhandled rejection: ${(e.reason as Error)?.stack ?? e.reason}`));
document.addEventListener("webglcontextlost", (e) => logToHost(`webgl context lost on ${(e.target as HTMLElement)?.id || "canvas"}`), true);

function clearError(): void {
  const e = $("#error");
  e.hidden = true;
  e.textContent = "";
}

function showError(msg: string): void {
  const e = $("#error");
  if (e.textContent?.split("\n").includes(msg)) return;
  logToHost(`shown: ${msg}`);
  e.hidden = false;
  e.textContent = e.textContent ? `${e.textContent}\n${msg}` : msg;
}

/**
 * One episode, from its session to the controls. The page opens one at a
 * time; another episode (the episode layer, N/P, up next) aborts this one's
 * signal, which removes its listeners, observers and timers and stops its
 * stream and renderer, and runs main again on the same page: no reload,
 * so fonts, scripts and the window stay warm (switchTo, below).
 */
async function main(entryId: string, signal: AbortSignal): Promise<void> {
  const withId = (path: string) => `${path}${path.includes("?") ? "&" : "?"}id=${encodeURIComponent(entryId)}`;
  const canvas = $<HTMLCanvasElement>("#comments"); // a rebuild replaces the element: find it now
  clearError();
  // Comments are asked for at once: the server opens the session once for
  // both requests, and parsing overlaps with the video's first bytes.
  const commentsReq = fetch(withId("api/comments")).catch(() => null);
  const [sess, stored] = await Promise.all([
    fetch(withId("api/session")).then(async (r) => {
      if (!r.ok) throw new Error(await r.text());
      return r.json() as Promise<Session>;
    }),
    fetch("api/settings").then((r) => r.json()),
  ]);
  mark("session");
  const s: Settings = merge(stored);
  const setHeading = () => {
    const epLabel = episodeLabel(sess.season, sess.episode) || (sess.episode == null ? sess.title : "");
    setTitle(`${sess.series}${epLabel ? " " + epLabel : ""} — Jusplay`);
    $("#series").textContent = sess.series || sess.title;
    $("#episode").textContent = epLabel;
  };
  setHeading();
  $("#heading").title = sess.path;

  // Facts for the bottom info line; defined first because rebuild() uses them.
  const m = sess.media;
  const [fn, fd] = m.frameRate.split("/").map(Number);
  const fps = fd ? fn / fd : fn;
  const videoFact = `${m.height}p ${m.video.codec.toUpperCase()}${m.video.profile ? " " + m.video.profile : ""} · ${fps ? fps.toFixed(3).replace(/\.?0+$/, "") : "?"} fps`;
  const audioFact = () => m.audio ? `${m.audio.codec.toUpperCase()}${m.audio.profile ? " " + m.audio.profile : ""}` : L("无音频", "音声なし");
  const status = (st?: string) => ({
    "verified-visible": L("已核对", "照合済み"), partial: L("部分", "一部"), unknown: L("未知", "不明"),
  } as Record<string, string>)[st ?? ""] ?? "";
  const unsupported = Engine.unsupported(sess.media);
  if (unsupported) {
    showError(`${L("无法播放：", "再生できません：")}${unsupported}`);
    return;
  }
  // A track the engine cannot decode costs the sound, not the picture.
  const noAudio = Engine.audioProblem(sess.media);
  if (noAudio) {
    const what = /dolby|ac-?3/i.test(noAudio) ? L("杜比数字（AC-3）", "ドルビーデジタル（AC-3）") : noAudio;
    toast(L(`这条音轨（${what}）暂时无法播放，只播放画面`, `この音声トラック（${what}）は再生できないため、映像のみ再生します`), 6000);
  }
  const engine = new Engine(video, sess.media, sess.id);
  signal.addEventListener("abort", () => engine.destroy());
  // Resume where the last session stopped, unless that was the very start or
  // end. From the key frame before it when that is at most 10 s earlier: the
  // picture then appears as soon as the first bytes arrive, instead of after
  // decoding up to a whole GOP (12 s on long-GOP releases) to reach the exact
  // point, and a few seconds of run-up help to pick the story up again.
  let resume = sess.position > 30 && sess.position < sess.media.duration - 30 ? sess.position : 0;
  if (resume) {
    const k = sess.media.keyframes.filter((t) => t <= resume + 1e-6).pop();
    if (k !== undefined && resume - k <= 10) resume = k;
  }
  await engine.init(resume);
  if (signal.aborted) return;
  mark("sourceOpen");
  liftMorphOnFirstFrame(signal);
  if (resume) toast(L(`从 ${fmtTime(resume)} 继续`, `${fmtTime(resume)} から再開`), 2500);
  // Opening an episode means watching it (the selftests drive playback themselves).
  if (!new URLSearchParams(location.search).has("selftest")) video.play().catch(() => undefined);

  let threads: V1Thread[] | null = null;
  if (sess.comments.error) showError(`${L("弹幕数据未通过校验，已关闭弹幕：", "コメントデータの検証に失敗したため、コメントを無効にしました：")}${sess.comments.error}`);
  else if (sess.comments.source !== "none") {
    const r = await commentsReq;
    if (r?.ok) threads = await r.json();
    else showError(`${L("读取弹幕失败：", "コメントの読み込みに失敗しました：")}HTTP ${r?.status ?? "—"}`);
    mark("commentsParsed");
  }

  // The chosen offset lives with the file (its folder's records). Earlier
  // versions kept it in the settings under the video's key: moved over once.
  const legacy = s.videos[sess.key]?.offsetMs;
  if (sess.offsetMs == null && legacy != null) {
    sess.offsetMs = legacy;
    fetch("api/offset", { method: "POST", body: JSON.stringify({ id: sess.id, offsetMs: legacy }) })
      .then((r) => { if (r.ok) { delete s.videos[sess.key]; save(); } }).catch(() => undefined);
  }
  const offsetOf = () => sess.offsetMs ?? sess.comments.offsetMs;
  const clock = new Clock(video, signal);
  const overlay = new Overlay(video, canvas, clock, s.comments, offsetOf(), signal);
  let stats: FilterStats | null = null;
  const seekBar = new SeekBar($("#density"), $("#seektip"), $("#seek"), video, sess.media.duration, () => overlay.offset / 1000, signal);
  const rebuild = async () => {
    if (!threads) {
      await overlay.load(null, s.comments);
      panel.setStats(null);
      seekBar.setComments(null);
      return;
    }
    const r = applyFilters(threads, s.filters, s.comments, sess.media.duration);
    stats = r.stats;
    panel.setStats(stats);
    seekBar.setComments(r.threads);
    await overlay.load(r.threads, s.comments);
    if (overlay.error) showError(`${L("弹幕渲染器拒绝了数据：", "コメント描画エンジンがデータを受け付けませんでした：")}${overlay.error}`);
    updateMeta();
  };
  // Opacity, area, on/off and frame rate only restyle; everything else
  // changes what niconicomments lays out and needs a rebuild.
  const rendererKey = () => {
    const { opacity, area, enabled, frameRate, ...rest } = s.comments;
    void [opacity, area, enabled, frameRate];
    return JSON.stringify([rest, s.filters]);
  };

  // ---- persistence ----
  let saveTimer = 0;
  const save = () => {
    clearTimeout(saveTimer);
    saveTimer = window.setTimeout(() => {
      fetch("api/settings", { method: "PUT", body: JSON.stringify(s) }).catch((e) => showError(`${L("保存设置失败：", "設定の保存に失敗しました：")}${e}`));
    }, 400);
  };
  let rebuildTimer = 0;
  let lastKey = rendererKey();
  const onSettings = () => {
    const key = rendererKey();
    if (key !== lastKey) {
      lastKey = key;
      clearTimeout(rebuildTimer);
      rebuildTimer = window.setTimeout(rebuild, 150);
    } else overlay.restyle(s.comments);
    updateToggles();
    save();
  };

  const panel = buildPanel($("#panel-body"), s, onSettings);
  const quick = buildQuick($("#quick-body"), s, onSettings);
  // The layout is built in slices that hand the main thread back (nico/
  // README.md), so it starts at once and overlaps the video's start instead
  // of waiting for the first frame. Most of a first build is the browser
  // meeting the comment fonts and characters for the first time (text
  // measurement: 223 ms cold, 15 ms warm for 2881 comments).
  const firstBuilt = rebuild().then(() => mark("commentsBuilt"));
  signal.addEventListener("abort", () => {
    // Pending writes are done now, not dropped; timers go.
    if (saveTimer) fetch("api/settings", { method: "PUT", body: JSON.stringify(s) }).catch(() => undefined);
    for (const t of [saveTimer, rebuildTimer, scrubTimer, idle]) clearTimeout(t);
    if (offsetTimer) {
      clearTimeout(offsetTimer);
      fetch("api/offset", { method: "POST", body: JSON.stringify({ id: sess.id, offsetMs: sess.offsetMs ?? null }) }).catch(() => undefined);
    }
    saveProgress();
    // Panels and the episode layer close with their episode (choosing one is going there).
    for (const l of [quickLayer, panelLayer, helpLayer, episodes.layer]) l.hide();
  });

  // ---- comment source ----
  $("#source-path").textContent = sess.comments.path ?? "";
  $("#auto-comments").hidden = sess.comments.source !== "manual";
  for (const p of sess.comments.problems ?? []) showError(`${L("未能使用：", "使用できませんでした：")}${p}`);
  const useSource = async (path: string) => {
    const r = await fetch("api/comments/source", { method: "POST", body: JSON.stringify({ id: sess.id, path }) });
    if (!r.ok) {
      toast(`${L("无法加载：", "読み込めません：")}${(await r.text()).slice(0, 200)}`, 4000);
      return;
    }
    location.reload();
  };
  const pick = async (f?: () => Promise<string>) => {
    if (!f) return toast(L("这个窗口不能打开文件对话框", "このウィンドウではファイル選択ダイアログを開けません"));
    const path = await f();
    if (path) await useSource(path);
  };
  $("#pick-comments").onclick = () => pick(host.kpPickComments);
  // The log is never written to disk (memlog); this is how it leaves.
  $("#copy-log").onclick = async () => {
    try {
      const text = await fetch("api/log").then((r) => (r.ok ? r.text() : Promise.reject(new Error(`HTTP ${r.status}`))));
      await navigator.clipboard.writeText(text);
      toast(L("诊断日志已复制", "診断ログをコピーしました"));
    } catch (e) {
      toast(`${L("复制失败：", "コピーできません：")}${e}`, 4000);
    }
  };
  $("#pick-snapshot").onclick = () => pick(host.kpPickSnapshotFolder);
  $("#auto-comments").onclick = () => useSource("");

  // ---- sync offset ----
  const offsetInput = $<HTMLInputElement>("#offset");
  let offsetTimer = 0;
  const setOffset = (ms: number) => {
    ms = Math.round(clamp(ms, -600000, 600000));
    sess.offsetMs = ms;
    overlay.setOffset(ms);
    offsetInput.value = String(ms);
    seekBar.draw();
    updateMeta();
    clearTimeout(offsetTimer);
    offsetTimer = window.setTimeout(() => {
      fetch("api/offset", { method: "POST", body: JSON.stringify({ id: sess.id, offsetMs: ms }) }).catch(() => undefined);
    }, 300);
  };
  offsetInput.value = String(offsetOf());
  offsetInput.onchange = () => setOffset(Number(offsetInput.value) || 0);
  for (const b of document.querySelectorAll<HTMLButtonElement>("[data-offset]")) {
    b.onclick = () => setOffset(overlay.offset + Number(b.dataset.offset));
  }
  // ---- texts that depend on the language, also re-run on a switch ----
  const renderInfo = () => {
    $("#source-name").textContent = sourceName(sess.comments.source);
    $("#sync-hint").textContent = sess.comments.source === "none"
      ? L("没有弹幕数据", "コメントデータがありません")
      : L("快捷键 - / = 每次调整 100 ms", "ショートカット - / = で 100 ms ずつ調整");
    const info: [string, string][] = [
      [L("文件", "ファイル"), sess.file],
      [L("视频", "映像"), `${sess.media.width}×${sess.media.height} ${sess.media.video.codec} ${sess.media.frameRate}`],
      [L("音频", "音声"), sess.media.audio?.codec ?? L("无", "なし")],
      [L("弹幕来源", "コメントの読み込み元"), sourceName(sess.comments.source)],
      [L("视频 ID", "動画 ID"), sess.comments.videoId ?? "—"],
      [L("快照", "スナップショット"), sess.comments.snapshotId ?? "—"],
      [L("采集状态", "取得状態"), status(sess.comments.captureStatus) || "—"],
      [L("采集截至", "取得日時"), sess.comments.capturedTo ?? L("未知", "不明")],
      [L("渲染器", "描画エンジン"), "niconicomments 0.4.1"],
    ];
    $("#info-list").replaceChildren(...info.flatMap(([k, v]) => {
      const dt = document.createElement("dt"), dd = document.createElement("dd");
      dt.textContent = k;
      dd.textContent = v;
      return [dt, dd];
    }));
  };

  // ---- bottom info line ----
  // Function declaration: rebuild() calls it before this point.
  // The live facts (codec, comments near now, buffer) are for the settings
  // panel's information, not the bar: updated only while it is open.
  function updateMeta(): void {
    if (!document.getElementById("panel")?.classList.contains("open")) return;
    const parts: string[] = [videoFact, audioFact()];
    if (sess.comments.source === "none") parts.push(L("无弹幕", "コメントなし"));
    else if (!threads) parts.push(L("弹幕不可用", "コメント利用不可"));
    else if (stats) {
      const nico = video.currentTime + overlay.offset / 1000;
      const now = seekBar.countBetween(nico - 3, nico + 1);
      parts.push(`${L("弹幕", "コメント")} <b>${stats.shown}</b>${stats.shown !== stats.total ? `/${stats.total}` : ""} · ${L("附近", "付近")} ${now}`);
      if (sess.comments.videoId) parts.push(sess.comments.videoId);
      // The offset is shown only once it has been moved.
      const off = overlay.offset;
      if (off) parts.push(`${L("偏移", "オフセット")} ${off > 0 ? "+" : ""}${off} ms`);
    }
    parts.push(`${L("缓冲", "バッファ")} ${Math.round(bufferedAhead(video))} s`);
    $("#live-info").innerHTML = parts.map((p) => escapeKeepB(p)).join('<span class="sep">|</span>');
  }
  updateMeta();

  // ---- controls ----
  const play = $<HTMLButtonElement>("#play"), seek = $<HTMLInputElement>("#seek"), vol = $<HTMLInputElement>("#volume");
  const rate = $<HTMLSelectElement>("#rate"), mute = $<HTMLButtonElement>("#mute"), cbtn = $<HTMLButtonElement>("#toggle-comments");
  seek.max = String(sess.media.duration);
  video.volume = s.playback.volume;
  video.muted = s.playback.muted;
  video.playbackRate = s.playback.rate;
  vol.value = String(s.playback.volume);
  const volFill = () => vol.style.setProperty("--v", `${(video.muted ? 0 : video.volume) * 100}%`);
  video.addEventListener("volumechange", volFill, { signal });
  volFill();
  rate.value = String(s.playback.rate);
  const seekwrap = $("#seekwrap"), timeEl = $("#time");
  function setTime(t: number): void {
    seekwrap.style.setProperty("--p", String(clamp(t / sess.media.duration, 0, 1)));
    timeEl.innerHTML = `${fmtTime(t)} <i>/</i> ${fmtTime(sess.media.duration)}`;
  }
  const togglePlay = () => (video.paused ? video.play().catch((e) => showError(String(e))) : video.pause());
  play.onclick = togglePlay;
  // A click on the picture while a card is open only closes the card.
  video.onclick = () => {
    if (layers.consumePress(video)) return;
    togglePlay();
    hud(video.paused ? "play" : "pause");
  };
  // Dragging the slider fires input continuously; seeking on every event
  // would restart the remux for each pixel, so drags seek at most every
  // 150 ms and once more on release.
  let scrubbing = false, scrubTimer = 0;
  seek.oninput = () => {
    scrubbing = true;
    setTime(Number(seek.value));
    if (!scrubTimer) {
      scrubTimer = window.setTimeout(() => {
        scrubTimer = 0;
        video.currentTime = Number(seek.value);
      }, 150);
    }
  };
  seek.onchange = () => {
    clearTimeout(scrubTimer);
    scrubTimer = 0;
    video.currentTime = Number(seek.value);
    scrubbing = false;
  };
  vol.oninput = () => {
    video.volume = s.playback.volume = Number(vol.value);
    video.muted = s.playback.muted = false;
    save();
  };
  mute.onclick = () => {
    video.muted = s.playback.muted = !video.muted;
    save();
  };
  const setVolume = (v: number) => {
    video.volume = s.playback.volume = clamp(v, 0, 1);
    video.muted = s.playback.muted = false;
    vol.value = String(video.volume);
    hud(video.volume ? "volume" : "muted", `${Math.round(video.volume * 100)}%`);
    save();
  };
  rate.onchange = () => {
    video.playbackRate = s.playback.rate = Number(rate.value);
    save();
  };
  cbtn.onclick = () => {
    s.comments.enabled = !s.comments.enabled;
    panel.refresh();
    quick.refresh();
    onSettings();
    hud("comments", s.comments.enabled ? L("弹幕 开", "コメント オン") : L("弹幕 关", "コメント オフ"));
  };
  function updateToggles(): void {
    $("#toggle-comments").classList.toggle("off", !s.comments.enabled);
    setIcon($("#mute"), video.muted || video.volume === 0 ? "muted" : "volume");
  }
  video.addEventListener("volumechange", updateToggles, { signal });
  updateToggles();
  const tick = () => {
    setIcon(play, video.paused ? "play" : "pause");
    if (!scrubbing) seek.value = String(video.currentTime);
    if (!scrubbing) setTime(video.currentTime);
    updateMeta();
    const errs = engine.errors();
    if (errs.length) showError(errs.join("\n"));
  };
  // loadeddata: the total time and buffer before playback starts (none of
  // the others fire while the first frame sits paused).
  for (const ev of ["timeupdate", "play", "pause", "seeked", "loadeddata"]) video.addEventListener(ev, tick, { signal });
  setTime(video.currentTime);
  video.addEventListener("error", () => showError(`${L("视频错误：", "動画エラー：")}${video.error?.message ?? video.error?.code}`), { signal });

  // ---- cards: quick settings, full settings, shortcuts ----
  const panelEl = $("#panel");
  const layers = new LayerStack(signal);
  const quickLayer = new Layer($("#quick"), layers, $("#open-quick"));
  const panelLayer = new Layer(panelEl, layers);
  const helpLayer = new Layer($("#help"), layers);
  const toggleQuick = () => {
    if (!quickLayer.isOpen) {
      quick.refresh();
      episodes.layer.hide();
    }
    quickLayer.toggle();
  };
  const openPanel = () => {
    quickLayer.hide();
    episodes.layer.hide();
    panel.refresh();
    panelLayer.show();
    updateMeta();
  };
  const toggleHelp = () => {
    quickLayer.hide();
    helpLayer.toggle();
  };
  $("#open-quick").onclick = toggleQuick;
  $("#open-settings").onclick = openPanel;
  $("#open-help").onclick = toggleHelp;
  $("#help-done").onclick = () => helpLayer.hide();
  const renderHelp = () => {
    const keys: [string, string, string][] = [
      ["Space / K", "播放 / 暂停", "再生 / 一時停止"], ["← →", "后退 / 前进 5 秒（Shift：1 秒）", "5 秒戻る / 進む（Shift：1 秒）"],
      ["↑ ↓ / " + L("滚轮", "ホイール"), "音量", "音量"], ["0–9", "跳到 0%–90%", "0%–90% へ移動"],
      [", .", "暂停时逐帧", "一時停止中にコマ送り"], ["[ ]", "倍速", "再生速度"], ["M", "静音", "ミュート"],
      ["C", "弹幕开关", "コメント表示"], ["- =", "弹幕偏移 ±100 ms", "コメントのずれ ±100 ms"], ["S", "弹幕设置", "コメント設定"],
      ["F / " + L("双击", "ダブルクリック"), "全屏", "全画面"], ["N P", "下一集 / 上一集", "次の話 / 前の話"],
      ["E", "剧集", "エピソード"], [BACK_KEYS, "返回媒体库", "ライブラリに戻る"], ["?", "快捷键", "ショートカット"], ["Esc", "关闭 / 退出全屏", "閉じる / 全画面を終了"],
    ];
    $("#help-list").replaceChildren(...keys.flatMap(([k, zh, ja]) => {
      const dt = document.createElement("dt"), dd = document.createElement("dd");
      dt.append(...k.split(" / ").flatMap((part, i) => {
        const kbd = document.createElement("kbd");
        kbd.textContent = part;
        return i ? [" ", kbd] : [kbd];
      }));
      dd.textContent = L(zh, ja);
      return [dt, dd];
    }));
  };
  // Kept across episodes: the window may already be full screen.
  let fullscreen = document.body.classList.contains("fullscreen");
  setIcon($("#fullscreen"), fullscreen ? "exitFullscreen" : "fullscreen");
  const setFullscreen = async (on: boolean) => {
    fullscreen = on;
    setIcon($("#fullscreen"), on ? "exitFullscreen" : "fullscreen");
    document.body.classList.toggle("fullscreen", on);
    if (host.kpFullscreen) await host.kpFullscreen(on);
    else if (on) await document.documentElement.requestFullscreen().catch(() => undefined);
    else if (document.fullscreenElement) await document.exitFullscreen();
  };
  $("#fullscreen").onclick = () => setFullscreen(!fullscreen);
  video.ondblclick = () => setFullscreen(!fullscreen);
  let idle = 0;
  const wake = () => {
    setUiHidden(false);
    clearTimeout(idle);
    idle = window.setTimeout(() => {
      if (!video.paused && !layers.any) setUiHidden(true);
    }, 2500);
  };
  document.addEventListener("mousemove", wake, { signal });
  video.addEventListener("pause", wake, { signal });
  layers.onChange = wake;
  // Wheel over the picture: volume, one step per notch.
  $("#stage").addEventListener("wheel", (e) => {
    if (layers.any || !e.deltaY) return;
    setVolume(video.volume + (e.deltaY < 0 ? 0.05 : -0.05));
    wake();
  }, { passive: true, signal });

  // ---- progress, episodes ----
  const saveProgress = (beacon = false) => {
    const body = JSON.stringify({ id: sess.id, position: video.currentTime, duration: sess.media.duration });
    if (beacon) navigator.sendBeacon("api/progress", body);
    else fetch("api/progress", { method: "POST", body }).catch(() => undefined);
  };
  let lastSaved = 0;
  video.addEventListener("timeupdate", () => {
    if (!video.paused && Math.abs(video.currentTime - lastSaved) >= 5) {
      lastSaved = video.currentTime;
      saveProgress();
    }
  }, { signal });
  video.addEventListener("pause", () => saveProgress(), { signal });
  addEventListener("pagehide", () => saveProgress(true), { signal });
  // Another episode: its thumbnail stands in until the first frame, and
  // grows out of the row it was picked from (view-transition-name: hero).
  const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
  const go = (id?: string, _from: HTMLElement | null = null) => {
    if (!id || signal.aborted) return;
    void reducedMotion;
    void switchTo(id, episodes.find(id)?.thumb);
  };
  const episodes = episodeLayer({
    current: sess.id, stack: layers,
    go: (e: Episode, from) => go(e.id, from),
    onOpen: () => (quickLayer.hide(), panelLayer.hide(), helpLayer.hide()),
    signal,
    keep: [$("#topbar"), $("#bar")],
  });
  // Other episodes are chosen in the episode layer, or with N and P; the
  // top bar is only what is playing and the way back.
  const hasSiblings = !!(sess.prev || sess.next);
  $("#eps-tab").hidden = !hasSiblings;
  const back = $<HTMLButtonElement>("#back");
  const backTo = `library.html#/s/${encodeURIComponent(seriesKey(sess.folder, sess.series))}`;
  back.dataset.key = BACK_KEYS;
  // Framed in the library, back is the library's move (a zoom into the
  // card); on its own, the page goes there.
  const leave = () => {
    saveProgress(true);
    if (framed) host.postMessage({ type: "jusplay:back", id: sess.id }, location.origin);
    else location.href = backTo;
  };
  back.onclick = (e) => {
    e.preventDefault();
    leave();
  };
  onBack(leave, signal);
  // At the end, the episode layer opens with the next episode lit and
  // counting down. Watching on (a seek back) or closing the layer stops it.
  video.addEventListener("seeking", () => episodes.cancelUpNext(), { signal });
  video.addEventListener("ended", () => {
    saveProgress();
    const next = sess.next;
    if (next) void episodes.upNext(next, 5, () => go(next));
  }, { signal });

  // ---- keyboard ----
  const frame = 1 / (eval_fps(sess.media.frameRate) || 24);
  document.addEventListener("keydown", (e) => {
    keyHandled++;
    const t = e.target as HTMLElement;
    if (t.matches("input, textarea, select") && e.key !== "Escape") return;
    // Shortcuts are bare keys; with Ctrl, Alt or Meta a key is left alone
    // (Alt ← is "back", nav.ts).
    if ((e.ctrlKey || e.altKey || e.metaKey) && e.key !== "Escape") return;
    const step = e.shiftKey ? 1 : 5;
    if (/^[0-9]$/.test(e.key) && !e.ctrlKey && !e.altKey) {
      video.currentTime = (sess.media.duration * Number(e.key)) / 10;
      hud(null, fmtTime(video.currentTime));
      e.preventDefault();
      wake();
      return;
    }
    switch (e.key) {
      case " ": case "k": togglePlay(); hud(video.paused ? "play" : "pause"); break;
      case "ArrowLeft": video.currentTime = Math.max(0, video.currentTime - step); hud("rewind", `${step} s`); break;
      case "ArrowRight": video.currentTime = Math.min(sess.media.duration, video.currentTime + step); hud("forward", `${step} s`); break;
      case "ArrowUp": setVolume(video.volume + 0.05); break;
      case "ArrowDown": setVolume(video.volume - 0.05); break;
      case ",": if (video.paused) video.currentTime = Math.max(0, video.currentTime - frame); break;
      case ".": if (video.paused) video.currentTime = video.currentTime + frame; break;
      case "[": case "]": {
        const rates = [0.5, 0.75, 1, 1.25, 1.5, 2], i = rates.indexOf(video.playbackRate);
        const r = rates[clamp((i < 0 ? 2 : i) + (e.key === "]" ? 1 : -1), 0, rates.length - 1)];
        video.playbackRate = s.playback.rate = r; rate.value = String(r); hud(null, `${r}×`); save(); break;
      }
      case "-": setOffset(overlay.offset - 100); hud(null, `${L("弹幕偏移", "コメントのずれ")} ${overlay.offset > 0 ? "+" : ""}${overlay.offset} ms`); break;
      case "=": case "+": setOffset(overlay.offset + 100); hud(null, `${L("弹幕偏移", "コメントのずれ")} ${overlay.offset > 0 ? "+" : ""}${overlay.offset} ms`); break;
      case "m": mute.click(); hud(video.muted ? "muted" : "volume", video.muted ? L("静音", "ミュート") : `${Math.round(video.volume * 100)}%`); break;
      case "n": go(sess.next); break;
      case "p": go(sess.prev); break;
      case "c": cbtn.click(); break;
      case "s": toggleQuick(); break;
      case "e": if (hasSiblings) episodes.toggle(); break;
      case "?": toggleHelp(); break;
      case "f": setFullscreen(!fullscreen); break;
      case "Escape": if (!layers.closeTop() && fullscreen) setFullscreen(false); break;
      default: return;
    }
    e.preventDefault();
    wake();
  }, { signal });

  // A language switch rewrites the texts in place: the settings rows keep
  // their elements, state and scroll position.
  let firstLang = true;
  onLang(() => {
    renderInfo();
    updateMeta();
    renderHelp();
    if (firstLang) return void (firstLang = false);
    setHeading();
    panel.relabel();
    quick.relabel();
  }, signal);

  onTheme(() => seekBar.draw(), signal);

  const q = new URLSearchParams(location.search);
  if (q.has("selftest") && q.has("shots")) {
    // Frames of picture and comments at the given times, for comparing the
    // rendering (-shots / -shots-dir).
    await firstBuilt;
    video.muted = true;
    video.pause();
    const times = q.get("shots")!.split(",").map(Number).filter(Number.isFinite);
    const within = (p: Promise<unknown>, ms: number) => Promise.race([p, new Promise((r) => setTimeout(r, ms))]);
    for (const t of times) {
      const seeked = new Promise((r) => video.addEventListener("seeked", r, { once: true }));
      video.currentTime = t;
      await within(seeked, 5000);
      await new Promise((r) => setTimeout(r, 400));
      logToHost(`shot ${t}: at ${video.currentTime.toFixed(3)} s, readyState ${video.readyState}`);
      const blob = await new Promise<Blob>((r) => overlay.snapshot().toBlob((b) => r(b!), "image/png"));
      await fetch(`api/selftest/shot?name=${encodeURIComponent(t.toFixed(2))}.png`, { method: "POST", body: blob });
    }
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({ shots: times.length }) });
    return;
  }
  if (q.has("selftest") && q.has("switch")) {
    // Episode switching on the page (switchTo): the time from asking to the
    // new episode's first frame, for every episode of the series in turn,
    // then one key press that must act exactly once (a listener left by an
    // earlier episode would act again).
    const ep = await fetch(`api/episodes?id=${encodeURIComponent(sess.id)}`).then((r) => r.json());
    const ids: string[] = ep.episodes.map((e: { id: string }) => e.id).filter((id: string) => id !== sess.id);
    const hops: { ms: number }[] = [];
    for (const id of ids) {
      const t0 = performance.now();
      const shown = new Promise<number>((r) => addEventListener("kp:firstframe", () => r(performance.now()), { once: true }));
      void switchTo(id, ep.episodes.find((e: { id: string }) => e.id === id)?.thumb);
      hops.push({ ms: Math.round((await shown) - t0) });
      await new Promise((r) => setTimeout(r, 800));
    }
    const before = keyHandled;
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "m", bubbles: true }));
    await new Promise((r) => setTimeout(r, 100));
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({ hops, keyHandlers: keyHandled - before, title: document.title }) });
    return;
  }
  if (q.has("selftest") && q.has("dock")) {
    // Smoothness of the episode layer: open and close it six times while
    // playing, and time every frame for 700 ms after each.
    video.muted = true;
    await video.play().catch(() => undefined);
    await new Promise((r) => setTimeout(r, 1500));
    const warm = performance.now();
    await episodes.ready;
    const warmedAfterMs = Math.round(performance.now() - warm);
    const runs: { open: boolean; frames: number; maxMs: number; over: number; syncMs?: number }[] = [];
    const deltas: number[] = [];
    // Long animation frames during the run, with what they spent where.
    const loaf: unknown[] = [];
    try {
      new PerformanceObserver((l) => {
        for (const e of l.getEntries() as unknown as { startTime: number; duration: number; blockingDuration: number; renderStart: number; styleAndLayoutStart: number;
          scripts: { invoker: string; duration: number; sourceURL: string; sourceFunctionName: string; forcedStyleAndLayoutDuration: number }[] }[]) {
          loaf.push({ at: Math.round(e.startTime), ms: Math.round(e.duration), blocking: Math.round(e.blockingDuration),
            scriptEnd: Math.round((e.renderStart || e.startTime + e.duration) - e.startTime), styleLayoutMs: Math.round(e.styleAndLayoutStart ? e.startTime + e.duration - e.styleAndLayoutStart : 0),
            scripts: e.scripts.map((x) => `${x.invoker} ${Math.round(x.duration)}ms forced ${Math.round(x.forcedStyleAndLayoutDuration)}ms ${x.sourceFunctionName}`) });
        }
      }).observe({ type: "long-animation-frame", buffered: false });
    } catch { /* not supported */ }
    // First: the controls coming back after they hid (a mouse move), alone.
    const frames = (ms: number) => new Promise<number[]>((done) => {
      const ds: number[] = [];
      let last = performance.now();
      const t0 = last;
      const step = (t: number) => {
        ds.push(t - last);
        last = t;
        if (t - t0 < ms) requestAnimationFrame(step);
        else done(ds);
      };
      requestAnimationFrame(step);
    });
    // A move starts the hiding timer; after 2.5 s still they hide, and go.
    document.dispatchEvent(new MouseEvent("mousemove", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 3500));
    const wasGone = document.body.classList.contains("ui-gone");
    document.dispatchEvent(new MouseEvent("mousemove", { bubbles: true }));
    const wakeDs = await frames(700);
    const wake = { wasGone, maxMs: Math.round(Math.max(...wakeDs)) };
    await new Promise((r) => setTimeout(r, 300));
    const t00 = performance.now();
    for (let i = 0; i < 6; i++) {
      const open = i % 2 === 0;
      const ts = performance.now();
      episodes.toggle(open);
      const syncMs = performance.now() - ts;
      const ds: number[] = [];
      await new Promise<void>((done) => {
        let last = performance.now();
        const t0 = last;
        const step = (t: number) => {
          ds.push(t - last);
          last = t;
          if (t - t0 < 700) requestAnimationFrame(step);
          else done();
        };
        requestAnimationFrame(step);
      });
      deltas.push(...ds);
      runs.push({ open, frames: ds.length, maxMs: Math.max(...ds), over: 0, syncMs: Math.round(syncMs * 10) / 10 });
      runs[runs.length - 1].over = ds.length; // placeholder, set below once the refresh is known
      (runs[runs.length - 1] as unknown as { ds: number[] }).ds = ds;
      await new Promise((r) => setTimeout(r, 300));
    }
    // Presses: on the bars the layer stays (they are for using), on the picture it closes.
    episodes.toggle(true);
    await new Promise((r) => setTimeout(r, 500));
    $("#play").dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    // The glass at its open place, the tab showing it is open.
    const opened = { glass: getComputedStyle($("#bar-glass")).transform, tab: $("#eps-tab").getAttribute("aria-expanded"), clipped: $("#bar-body").classList.contains("moving") };
    // The comment density behind the seek bar is drawn (pixels painted).
    const dc = $<HTMLCanvasElement>("#density"), dd = dc.getContext("2d")!.getImageData(0, 0, dc.width, dc.height).data;
    let densityPx = 0;
    for (let i = 3; i < dd.length; i += 4) if (dd[i]) densityPx++;
    (opened as Record<string, unknown>).densityPx = densityPx;
    const stayOnBar = episodes.layer.isOpen;
    video.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    const closeOnPicture = !episodes.layer.isOpen;
    (wake as unknown as Record<string, boolean>).stayOnBar = stayOnBar;
    (wake as unknown as Record<string, boolean>).closeOnPicture = closeOnPicture;
    (wake as unknown as Record<string, unknown>).opened = opened;
    const sorted = [...deltas].sort((a, b) => a - b);
    const refresh = sorted[Math.floor(sorted.length / 2)];
    for (const r of runs) {
      const ds = (r as unknown as { ds: number[] }).ds;
      r.over = ds.filter((d) => d > refresh * 1.5).length;
      delete (r as unknown as { ds?: number[] }).ds;
    }
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({ refreshMs: refresh, warmedAfterMs, wake, runs, t0: Math.round(t00), loaf }) });
    return;
  }
  if (q.has("selftest") && q.has("trace")) {
    // Playback over a stretch (-trace FROM,SECONDS), for stalls.
    const [from, seconds] = q.get("trace")!.split(",").map(Number);
    video.muted = true;
    const events: string[] = [], t0 = performance.now();
    const at = () => `${((performance.now() - t0) / 1000).toFixed(1)}s@${video.currentTime.toFixed(2)}`;
    for (const ev of ["waiting", "stalled", "error", "seeking", "seeked", "playing", "pause", "ended"])
      video.addEventListener(ev, () => events.push(`${at()} ${ev}${ev === "error" ? " " + video.error?.message : ""}`));
    addEventListener("error", (e) => events.push(`${at()} page error ${e.message} ${e.filename}:${e.lineno}`));
    addEventListener("unhandledrejection", (e) => events.push(`${at()} rejection ${String(e.reason?.stack ?? e.reason)}`));
    const ranges = () => Array.from({ length: video.buffered.length }, (_, i) => `${video.buffered.start(i).toFixed(2)}-${video.buffered.end(i).toFixed(2)}`).join(" ");
    video.currentTime = from;
    await new Promise((r) => video.addEventListener("seeked", r, { once: true }));
    await video.play().catch((e) => events.push(`${at()} play() ${e}`));
    const samples: string[] = [];
    overlay.takeDrawStats();
    for (let i = 0; i < seconds * 2; i++) {
      await new Promise((r) => setTimeout(r, 500));
      const d = overlay.takeDrawStats();
      samples.push(`${at()} rs${video.readyState} ${video.paused ? "paused " : ""}[${ranges()}] draws ${d.frames} max ${d.maxMs.toFixed(1)}ms`);
    }
    const qa = video.getVideoPlaybackQuality();
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({ from, seconds, end: video.currentTime, frames: qa.totalVideoFrames, dropped: qa.droppedVideoFrames, events, samples }) });
    return;
  }
  if (q.has("selftest") && q.has("layout")) {
    // After the page's own first build: it draws from the same Math.random
    // the check seeds, and running alongside it shifted the first run.
    await firstBuilt;
    const { layoutCheck } = await import("./layoutcheck.ts");
    const f = threads ? applyFilters(threads, s.filters, s.comments, sess.media.duration).threads : null;
    const report = await layoutCheck(f, async (cv, th) => {
      const nc = createRenderer(cv, th, s.comments, true);
      await nc.build();
      return nc as never;
    });
    await fetch("api/selftest", { method: "POST", body: JSON.stringify(report) });
    return;
  }
  if (q.has("selftest") && q.has("bench")) {
    // Steady-state resource benchmark: one scenario per run, measured from
    // outside; the page only sets the scenario up and reports its own view.
    // "-hidden": the controls hidden, as while watching; otherwise visible (worst case).
    const name = q.get("bench")!, seconds = Number(q.get("seconds")) || 25;
    const scenario = name.replace(/-hidden$/, "");
    if (name !== scenario) setUiHidden(true);
    video.muted = true;
    s.comments.enabled = scenario !== "no-comments";
    if (scenario.startsWith("comments-")) s.comments.frameRate = scenario.slice(9) as Settings["comments"]["frameRate"];
    overlay.restyle(s.comments);
    video.currentTime = 790;
    await new Promise((r) => video.addEventListener("seeked", r, { once: true }));
    if (scenario !== "paused") await video.play();
    await new Promise((r) => setTimeout(r, 5000)); // warm-up, excluded
    await fetch("api/log", { method: "POST", body: `bench start ${scenario}` });
    overlay.takeDrawStats();
    const q0 = video.getVideoPlaybackQuality(), t0 = performance.now();
    await new Promise((r) => setTimeout(r, seconds * 1000));
    const q1 = video.getVideoPlaybackQuality(), d = overlay.takeDrawStats();
    const mem = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory;
    // Paused, the draw loop is stopped; a seek and a settings change must
    // still redraw (draw counts after each, expected ≥ 1).
    let extra: string | null = null;
    if (scenario === "paused") {
      const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
      video.currentTime = 800;
      await new Promise((r) => video.addEventListener("seeked", r, { once: true }));
      await wait(600);
      const seek = overlay.takeDrawStats().frames;
      overlay.restyle(s.comments);
      await wait(300);
      extra = `seek ${seek} restyle ${overlay.takeDrawStats().frames}`;
    }
    await fetch("api/selftest", { method: "POST", body: JSON.stringify({
      scenario: name, extra, seconds: (performance.now() - t0) / 1000, draws: d.frames, drawMeanMs: d.meanMs, drawMaxMs: d.maxMs, drawP99Ms: d.p99Ms, drawOver8: d.over8,
      frames: q1.totalVideoFrames - q0.totalVideoFrames, dropped: q1.droppedVideoFrames - q0.droppedVideoFrames,
      jsHeapMB: mem ? Math.round(mem.usedJSHeapSize / 2 ** 20) : null, dpr: devicePixelRatio,
    }) });
    return;
  }
  if (q.has("selftest") && q.has("stress")) {
    const { stress } = await import("./stress.ts");
    const report = await stress({ video, engine, overlay, clock, duration: sess.media.duration }, Number(q.get("stress")) || 60);
    await fetch("api/selftest", { method: "POST", body: JSON.stringify(report) });
  } else if (q.has("selftest")) {
    const { selftest } = await import("./selftest.ts");
    await selftest({ video, engine, overlay, clock, session: sess, settings: s, rebuild, stats: () => stats });
  }
}

/**
 * Hide or show the controls. Hidden, they fade out and then leave the
 * render tree (display:none): a hidden but laid-out control keeps its
 * compositing layers, measured at ~80 MB and a little GPU time per frame.
 */
let goneTimer = 0;
function setUiHidden(hidden: boolean): void {
  const b = document.body;
  clearTimeout(goneTimer);
  if (hidden) {
    tips.hide();
    b.classList.add("hide-ui");
    goneTimer = window.setTimeout(() => b.classList.add("ui-gone"), 300);
  } else if (b.classList.contains("hide-ui")) {
    b.classList.remove("ui-gone");
    void b.offsetWidth; // start the fade-in from the hidden state
    b.classList.remove("hide-ui");
  }
}

/** Escape text but keep the <b> markers updateMeta uses for emphasis. */
function escapeKeepB(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/&lt;b&gt;/g, "<b>").replace(/&lt;\/b&gt;/g, "</b>");
}

function sourceName(src: CommentsInfo["source"]): string {
  switch (src) {
    case "manual": return L("手动选择", "手動で選択");
    case "attachment": return L("MKV 附件", "MKV の添付ファイル");
    case "same-name": return L("同名 JSON", "同名の JSON");
    default: return L("无（可在此选择文件）", "なし（ここでファイルを選択できます）");
  }
}

function eval_fps(r: string): number {
  const [a, b] = r.split("/").map(Number);
  return b ? a / b : a;
}

let current: AbortController | null = null;
/** Key presses the player's handler saw (the switch selftest: one press, one count). */
let keyHandled = 0;

/** Open an episode on this page, replacing the one playing. */
async function open(id: string): Promise<void> {
  current?.abort();
  const ac = new AbortController();
  current = ac;
  try {
    await main(id, ac.signal);
  } catch (e) {
    if (!ac.signal.aborted) showError(`${L("启动失败：", "起動に失敗しました：")}${(e as Error)?.stack ?? e}`);
  }
}

/**
 * Another episode, without leaving the page: its thumbnail fades in over
 * the picture, the playing one is taken down under it, and the thumbnail
 * lifts at the new episode's first frame. The address follows (a reload
 * reopens this episode).
 */
async function switchTo(id: string, thumb?: string): Promise<void> {
  const morph = $<HTMLImageElement>("#morph");
  if (thumb) {
    morph.src = thumb;
    morph.classList.add("gone");
    morph.hidden = false;
    void morph.offsetWidth;
    morph.classList.remove("gone");
    await new Promise((r) => setTimeout(r, 140));
  }
  history.replaceState(null, "", `index.html?id=${encodeURIComponent(id)}`);
  await open(id);
}

if (firstId) void open(firstId);

// Framed in the library: it opens and closes episodes; nothing is shown
// until the first one.
if (framed) {
  (window as unknown as { jusplay: unknown }).jusplay = {
    async open(id: string, thumb?: string) {
      const morph = $<HTMLImageElement>("#morph");
      if (thumb) {
        morph.src = thumb;
        morph.classList.remove("gone");
        morph.hidden = false;
      }
      history.replaceState(null, "", `index.html?id=${encodeURIComponent(id)}`);
      await open(id);
    },
    close() {
      current?.abort();
      current = null;
      video.pause();
      video.removeAttribute("src");
      video.load(); // lets the decoder go
      $("#morph").hidden = true;
    },
  };
  host.postMessage({ type: "jusplay:ready" }, location.origin);
}
