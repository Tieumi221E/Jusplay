# Jusplay

**一个小而快、默认离线的本地动画播放器，忠实还原 Niconico 弹幕。**
A small, fast, offline-by-default anime player for Windows that renders Niconico comments faithfully.
小さく速い、既定でオフラインのアニメプレイヤー。ニコニコのコメントを忠実に再現します。

- **打开就用**：一个约 18 MB 的 exe，解压即可运行，不需要安装，也不需要另装任何程序（Windows 11 自带所需的 WebView2 运行时）。
- **Niconico 式弹幕**：普通弹幕、顶端/底端、职人弹幕（弹幕艺术）、投稿者弹幕，按官方的排布规则绘制；可调透明度、字号、显示区域、同屏上限、NG 过滤与时间偏移。
- **字幕**：外挂字幕（SRT、ASS、VTT）与 MKV 内嵌字幕，可同时显示两条（如译文 + 原文）；没有字幕的视频可以用 AI 从声音生成字幕并翻译（可选，见下文）。
- **媒体库**：自动识别系列、季和集数；记住每一集的进度；首页直接从上次看到的地方继续。
- **流畅**：播放中切集不重新加载页面；剧集列表从播放条上长出来；界面动画按 120 Hz 调校。
- **隐私**：默认完全离线，不联网、不上传任何东西；只有你主动下载 AI 模型，或自己填写在线接口时才会联网。每个媒体文件夹的记录保存在它自己隐藏的 `.jusplay` 文件夹里；程序的设置放在 exe 旁边；日志只在内存里。

版本：**0.2.0**（新增字幕与 AI 字幕）。

## 下载与运行

1. 从 Releases 下载 `Jusplay-0.2.0-windows-x64.zip`，解压到任意文件夹。
2. 双击 `jusplay.exe` 打开媒体库，点右上角的文件夹按钮添加你的动画文件夹。
   也可以把一个视频文件直接拖到 `jusplay.exe` 上播放。

需要：

- Windows 10 / 11（64 位）。
- Microsoft Edge WebView2 运行时：Windows 11 已自带；Windows 10 若缺少，启动时会提示。
- 播放 HEVC（H.265）视频需要 Windows 的「HEVC 视频扩展」（很多电脑已预装）。

exe 没有代码签名，第一次运行时 Windows SmartScreen 可能提示“已保护你的电脑”，点“更多信息 → 仍要运行”即可。

## 支持的格式

| | 支持 | 暂不支持 |
|---|---|---|
| 容器 | MKV、WebM、MP4 | AVI、TS 等 |
| 视频 | HEVC（8/10 bit）、H.264、AV1、VP9 | 10 bit H.264 等浏览器引擎不能硬解的格式 |
| 音频 | AAC、Opus、FLAC、MP3 | AC-3、E-AC-3、DTS、TrueHD（这类文件只播放画面，并给出提示） |

视频不重新编码：Jusplay 自己读取容器，把原始数据无损地交给系统的硬件解码器。

## 弹幕从哪里来

弹幕按以下顺序查找：

1. 在设置里手动选择的弹幕文件；
2. 封装在 MKV 里的弹幕（见下文）；
3. 视频旁边的同名 `.json` 文件（浏览器扩展「コメント増量」导出的格式），例如 `第01话.mkv` 旁的 `第01话.json`。

也支持旧的 Niconico XML。媒体库里可以把同名 JSON “打包”进 MKV：画面与声音逐包校验、一字节不动，原来的弹幕会先备份到 `.jusplay\backup`。

## 字幕

点播放条上的字幕按钮（或按 T 显示 / 隐藏），选择主字幕和第二字幕：

- **外挂字幕**：视频旁边的同名字幕文件会自动列出，例如 `第01话.srt`、`第01话.chs.ass`、`第01话[CHT].vtt`，文件名里的语言标记会被识别；也可以用“加载字幕文件…”选择任意字幕文件。UTF-8、UTF-16、GBK、Big5、Shift-JIS 编码都能自动识别。
- **内嵌字幕**：MKV 里的文字字幕轨（SRT、ASS、WebVTT）。图形字幕（PGS、VobSub）暂不支持。
- **显示方式**：所有字幕都用 Jusplay 自己的样式绘制；ASS 的字体、颜色与特效不还原，放在屏幕上方的行（招牌、注释）仍显示在上方。
- 每一集选了哪条字幕会记在该视频的记录里。

### AI 字幕（可选）

没有字幕的视频，可以让 AI 边播边从声音生成字幕并翻译：字幕会提前生成到播放位置前方约一分钟，也可以一次生成整集。生成过的字幕存在该视频文件夹的记录里，再看时不用重新生成；界面会标明是机器生成以及所用的模型。

模型**不随 Jusplay 附带**。在“更多字幕设置”里，语音识别与翻译可以分别选择：

| 方式 | 说明 |
|---|---|
| 推荐模型（本机） | Qwen3-ASR-0.6B（识别）+ Hy-MT2-1.8B（翻译），用 llama.cpp 在本机运行，不联网。点“下载推荐模型”才会下载（约 2.2 GB，来自 GitHub 与 Hugging Face，下载后校验 SHA-256），放在 exe 旁边的 `components\ai`。有独立显卡或核显时自动用 GPU（Vulkan）。 |
| 自己的模型文件（本机） | llama.cpp 能运行的 GGUF 模型：识别需要 Qwen3-ASR、Voxtral 等音频模型及其 mmproj 文件，翻译可用任意对话模型。 |
| 在线接口 | OpenAI 兼容的 `/audio/transcriptions` 与 `/chat/completions`。**每句台词的声音或文字会发送到你填写的服务。** API Key 用 Windows 加密保存，只有当前用户在这台电脑上能读取。 |

可以指定原声语言；默认按开头的台词自动判断，并在整集锁定这种语言。

## 快捷键

| 键 | 作用 |
|---|---|
| Space / K | 播放 / 暂停 |
| ← → | 后退 / 前进 5 秒（Shift：1 秒） |
| ↑ ↓ / 滚轮 | 音量 |
| 0–9 | 跳到 0%–90% |
| , . | 暂停时逐帧 |
| [ ] | 倍速 |
| M | 静音 |
| C | 弹幕开关 |
| T | 字幕显示 / 隐藏 |
| - = | 弹幕偏移 ±100 ms |
| S | 弹幕设置 |
| F / 双击 | 全屏 |
| N P | 下一集 / 上一集 |
| E | 剧集列表 |
| Alt ← / Backspace | 返回媒体库 |
| ? | 全部快捷键 |
| Esc | 关闭 / 退出全屏 |

## 数据放在哪里

| 什么 | 在哪里 |
|---|---|
| 某个媒体文件夹的记录（进度、已看、缩略图、索引） | 那个文件夹里隐藏的 `.jusplay\` |
| 文件夹列表、界面偏好、弹幕设置 | exe 旁边的 `jusplay-data\`（不可写时用 `%AppData%\jusplay`） |
| 字幕选择、AI 生成的字幕 | 那个文件夹里隐藏的 `.jusplay\`（`subtitles\`） |
| AI 设置 | exe 旁边的 `jusplay-data\ai.json`（API Key 加密保存） |
| 下载的 AI 模型与运行时 | exe 旁边的 `components\ai\`（不可写时放在 `jusplay-data` 里） |
| 日志 | 只在内存里；需要时在设置 →「信息」里复制 |

删除 `jusplay-data`、`components` 和各文件夹里的 `.jusplay` 就会清除 Jusplay 留下的全部痕迹。

## 从源码构建

需要 Go 1.27+ 和 Node 22+。

```text
npm --prefix web ci
./build.ps1            # 生成 bin\jusplay.exe
go test ./...          # 测试不依赖任何外部程序
```


## 许可证

Jusplay 以 MIT 许可证发布（[LICENSE](LICENSE)）。exe 内包含的第三方软件与字体（Go 运行时、go-webview2、niconicomments 等，以及思源黑体、Noto Color Emoji 两款 SIL OFL 1.1 字体）的许可证见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。

AI 字幕的推荐组件不包含在 Jusplay 里，由你选择下载：llama.cpp（MIT）、Qwen3-ASR-0.6B（Apache 2.0）、Hy-MT2-1.8B（Apache 2.0），各自遵循其许可证。
