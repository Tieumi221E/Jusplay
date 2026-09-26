// The AI subtitles' backends, in the settings panel (internal/ai): for
// speech recognition and for translation each, the recommended models on
// this computer, the user's own model files, or an online service
// (OpenAI-compatible). The recommended models are not part of Jusplay:
// they are downloaded here, when the user asks, from fixed sources and
// checked against fixed SHA-256 sums.
//
// Keys go to the app and stay there (encrypted for the Windows user); the
// page only learns that one is set.

import { L } from "./i18n.ts";
import { toast } from "./ui.ts";

interface Backend {
  kind: "recommended" | "local" | "api";
  model: string;
  proj: string;
  url: string;
  hasKey: boolean;
  key?: string;
  clearKey?: boolean;
}

interface Config {
  asr: Backend;
  mt: Backend;
  server: string;
  dir: string;
}

interface TaskStatus {
  ready: boolean;
  kind: string;
  name?: string;
  why?: string;
}

interface DownloadState {
  running: boolean;
  done: number;
  total: number;
  file: string;
  error?: string;
  missing: string[] | null;
  bytes: number;
}

interface AiInfo {
  asr: TaskStatus;
  mt: TaskStatus;
  config: Config;
  download: DownloadState;
}

declare global {
  interface Window {
    kpPickModel?: () => Promise<string>;
    kpPickProgram?: () => Promise<string>;
  }
}

const gb = (n: number) => `${(n / 1e9).toFixed(n >= 1e10 ? 0 : 1)} GB`;

function el<K extends keyof HTMLElementTagNameMap>(tag: K, props: Partial<HTMLElementTagNameMap[K]> & { class?: string } = {}, ...kids: (Node | string)[]): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  const { class: cls, ...rest } = props;
  Object.assign(e, rest);
  if (cls) e.className = cls;
  e.append(...kids);
  return e;
}

/**
 * Build the backends' settings into root. changed() runs after a change is
 * applied (the page reads the AI status again). Returns a function that
 * rewrites the texts in the current language.
 */
export function buildAiConfig(root: HTMLElement, changed: () => void, signal: AbortSignal): () => void {
  let info: AiInfo | null = null;
  let draft: Config | null = null;
  let poll = 0;
  signal.addEventListener("abort", () => clearTimeout(poll));

  const render = () => {
    root.replaceChildren();
    if (!info || !draft) {
      root.append(el("div", { class: "stats" }, L("正在读取 AI 设置…", "AI 設定を読み込み中…")));
      return;
    }
    root.append(el("h4", {}, L("模型与接口", "モデルと接続先")));
    root.append(downloadBlock(info.download));
    root.append(taskBlock("asr", L("语音识别", "音声認識"), draft.asr, info.asr));
    root.append(taskBlock("mt", L("翻译", "翻訳"), draft.mt, info.mt));
    if (draft.asr.kind === "local" || draft.mt.kind === "local") {
      root.append(pathRow(L("llama-server", "llama-server"), draft.server, `${draft.dir}\\llama\\llama-server.exe`, (v) => (draft!.server = v), window.kpPickProgram));
    }
    const apply = el("button", { class: "link" }, L("应用", "適用"));
    apply.onclick = save;
    root.append(el("div", { class: "row actions" }, apply));
  };

  const downloadBlock = (d: DownloadState): HTMLElement => {
    const box = el("div", { class: "stats" });
    if (d.running) {
      const pct = d.total ? Math.floor((d.done / d.total) * 100) : 0;
      const cancel = el("button", { class: "link" }, L("取消", "キャンセル"));
      cancel.onclick = () => fetch("api/ai/download/cancel", { method: "POST" }).then(load);
      box.append(el("div", {}, `${L("正在下载推荐模型", "推奨モデルをダウンロード中")} ${pct}%（${gb(d.done)} / ${gb(d.total)}）${d.file}`), cancel);
      return box;
    }
    if (!d.missing?.length) {
      box.append(el("div", {}, L("推荐模型已下载：Qwen3-ASR-0.6B（识别）、Hy-MT2-1.8B（翻译）", "推奨モデルはダウンロード済み：Qwen3-ASR-0.6B（認識）、Hy-MT2-1.8B（翻訳）")));
      return box;
    }
    const go = el("button", { class: "link" }, `${L("下载推荐模型", "推奨モデルをダウンロード")}（${gb(d.bytes)}）`);
    go.onclick = () => fetch("api/ai/download", { method: "POST" }).then(load);
    box.append(
      el("div", {}, L("推荐模型：Qwen3-ASR-0.6B（识别）与 Hy-MT2-1.8B（翻译），由 llama.cpp 在本机运行。不随 Jusplay 附带，需要时从 GitHub 与 Hugging Face 下载，校验 SHA-256 后放在：",
        "推奨モデル：Qwen3-ASR-0.6B（認識）と Hy-MT2-1.8B（翻訳）。llama.cpp でこの PC 上で動かします。Jusplay には含まれず、必要なときに GitHub と Hugging Face からダウンロードし、SHA-256 を確認して次に置きます：")),
      el("div", { class: "path" }, info!.config.dir),
    );
    if (d.error) box.append(el("div", { class: "err" }, `${L("上次下载失败：", "前回のダウンロードに失敗：")}${d.error}`));
    box.append(go);
    return box;
  };

  const taskBlock = (task: "asr" | "mt", title: string, b: Backend, st: TaskStatus): HTMLElement => {
    const box = el("div", { class: "ai-task" });
    const kind = el("select");
    for (const [v, t] of [["recommended", L("推荐模型（本机）", "推奨モデル（この PC）")], ["local", L("自己的模型文件（本机）", "自分のモデルファイル（この PC）")], ["api", L("在线接口（OpenAI 兼容）", "オンライン API（OpenAI 互換）")]]) {
      kind.append(el("option", { value: v, textContent: t }));
    }
    kind.value = b.kind;
    kind.onchange = () => {
      b.kind = kind.value as Backend["kind"];
      render();
    };
    box.append(el("div", { class: "row" }, el("label", { class: "lbl", textContent: title }), kind));
    if (b.kind === "local") {
      box.append(pathRow(L("模型", "モデル"), b.model, "*.gguf", (v) => (b.model = v), window.kpPickModel));
      if (task === "asr") box.append(pathRow("mmproj", b.proj, "mmproj-*.gguf", (v) => (b.proj = v), window.kpPickModel));
      box.append(el("div", { class: "hint" }, task === "asr"
        ? L("llama.cpp 支持的音频模型，如 Qwen3-ASR、Voxtral；需要对应的 mmproj 文件", "llama.cpp が対応する音声モデル（Qwen3-ASR、Voxtral など）。対応する mmproj ファイルが必要です")
        : L("llama.cpp 能运行的对话模型（GGUF）", "llama.cpp で動く対話モデル（GGUF）")));
    }
    if (b.kind === "api") {
      box.append(textRow(L("地址", "アドレス"), b.url, "https://api.openai.com/v1", (v) => (b.url = v)));
      box.append(textRow(L("模型", "モデル"), b.model, task === "asr" ? "whisper-1" : "gpt-4o-mini", (v) => (b.model = v)));
      const key = el("input", { type: "password", placeholder: b.hasKey ? L("已保存（留空则不变）", "保存済み（空欄なら変更なし）") : "API Key", autocomplete: "off", spellcheck: false });
      key.oninput = () => (b.key = key.value);
      const row = el("div", { class: "row" }, el("label", { class: "lbl", textContent: "Key" }), key);
      if (b.hasKey) {
        const clear = el("button", { class: "link" }, L("清除", "削除"));
        clear.onclick = () => {
          b.clearKey = true;
          b.hasKey = false;
          render();
        };
        row.append(clear);
      }
      box.append(row);
      box.append(el("div", { class: "hint warn" }, task === "asr"
        ? L("每句台词的声音会发送到这个服务。", "セリフごとの音声がこのサービスに送られます。")
        : L("每句台词的文字会发送到这个服务。", "セリフごとのテキストがこのサービスに送られます。")));
    }
    box.append(el("div", { class: st.ready ? "hint" : "hint err" }, st.ready ? `${L("可用：", "使用可能：")}${st.name}` : `${L("不可用：", "使用不可：")}${st.why ?? ""}`));
    return box;
  };

  const textRow = (label: string, value: string, placeholder: string, set: (v: string) => void) => {
    const i = el("input", { type: "text", value, placeholder, spellcheck: false });
    i.oninput = () => set(i.value);
    return el("div", { class: "row" }, el("label", { class: "lbl", textContent: label }), i);
  };

  const pathRow = (label: string, value: string, placeholder: string, set: (v: string) => void, pick?: () => Promise<string>) => {
    const i = el("input", { type: "text", value, placeholder, spellcheck: false });
    i.oninput = () => set(i.value);
    const row = el("div", { class: "row" }, el("label", { class: "lbl", textContent: label }), i);
    if (pick) {
      const b = el("button", { class: "link" }, L("选择…", "選択…"));
      b.onclick = async () => {
        const p = await pick();
        if (p) {
          i.value = p;
          set(p);
        }
      };
      row.append(b);
    }
    return row;
  };

  async function save(): Promise<void> {
    const r = await fetch("api/ai/config", { method: "PUT", body: JSON.stringify(draft) });
    if (!r.ok) {
      toast(`${L("无法应用：", "適用できません：")}${(await r.text()).slice(0, 200)}`, 5000);
      return;
    }
    toast(L("AI 设置已应用", "AI 設定を適用しました"));
    await load();
    changed();
  }

  async function load(): Promise<void> {
    clearTimeout(poll);
    try {
      const r = await fetch("api/ai", { signal });
      if (!r.ok) {
        root.replaceChildren(el("div", { class: "stats" }, await r.text()));
        return;
      }
      const wasRunning = info?.download.running;
      info = (await r.json()) as AiInfo;
      // The draft keeps what is being edited; only a fresh load replaces it.
      if (!draft || !wasRunning) draft = structuredClone(info.config);
      render();
      if (info.download.running) poll = window.setTimeout(load, 1000);
      else if (wasRunning) changed(); // the models just arrived
    } catch {
      /* the page is going */
    }
  }

  void load();
  return render;
}
