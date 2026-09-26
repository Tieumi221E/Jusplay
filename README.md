# Jusplay

**一个小而快、完全离线的本地动画播放器，忠实还原 Niconico 弹幕。**
A small, fast, fully offline anime player for Windows that renders Niconico comments faithfully.
小さく速い、完全オフラインのアニメプレイヤー。ニコニコのコメントを忠実に再現します。

- **打开就用**：一个约 18 MB 的 exe，解压即可运行，不需要安装，也不需要另装任何程序（Windows 11 自带所需的 WebView2 运行时）。
- **Niconico 式弹幕**：普通弹幕、顶端/底端、职人弹幕（弹幕艺术）、投稿者弹幕，按官方的排布规则绘制；可调透明度、字号、显示区域、同屏上限、NG 过滤与时间偏移。
- **媒体库**：自动识别系列、季和集数；记住每一集的进度；首页直接从上次看到的地方继续。
- **流畅**：播放中切集不重新加载页面；剧集列表从播放条上长出来；界面动画按 120 Hz 调校。
- **隐私**：完全离线，不联网、不上传任何东西。每个媒体文件夹的记录保存在它自己隐藏的 `.jusplay` 文件夹里；程序的设置放在 exe 旁边；日志只在内存里。

版本：**0.1.0**（首个公开版本）。

## 下载与运行

1. 从 Releases 下载 `Jusplay-0.1.0-windows-x64.zip`，解压到任意文件夹。
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
| 日志 | 只在内存里；需要时在设置 →「信息」里复制 |

删除 `jusplay-data` 和各文件夹里的 `.jusplay` 就会清除 Jusplay 留下的全部痕迹。

## 从源码构建

需要 Go 1.27+ 和 Node 22+。

```text
npm --prefix web ci
./build.ps1            # 生成 bin\jusplay.exe
go test ./...          # 测试不依赖任何外部程序
```


## 许可证

Jusplay 以 MIT 许可证发布（[LICENSE](LICENSE)）。exe 内包含的第三方软件与字体（Go 运行时、go-webview2、niconicomments 等，以及思源黑体、Noto Color Emoji 两款 SIL OFL 1.1 字体）的许可证见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。
