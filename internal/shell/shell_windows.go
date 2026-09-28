//go:build windows

// Package shell opens the player page in a WebView2 window.
package shell

import (
	"os"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
)

// Window is an open player window.
type Window struct {
	w      webview2.WebView
	hwnd   uintptr
	saved  windowPlacement
	style  uintptr
	isFull bool
	icons  [2]uintptr // the icons on the window now (small, big), destroyed when replaced
}

var (
	procSetDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem        = user32.NewProc("GetDpiForSystem")
)

// dpiAwarenessPerMonitorV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2.
const dpiAwarenessPerMonitorV2 = ^uintptr(3) // -4

// Without this the process is DPI-unaware: Windows renders the window at
// 96 DPI and stretches it, so text, video and comments are blurred on a
// scaled display (the page saw devicePixelRatio 1 at 175 % scaling).
func init() {
	procSetDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
}

// scaled converts a size in 96-DPI units to pixels at the system DPI.
func scaled(v int) int {
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return v
	}
	return v * int(dpi) / 96
}

// Open creates the window; call Run to show it. profile is WebView2's data folder: the app
// puts it in its temporary folder and removes it when the window has
// closed, since it holds the engine's history (page titles name what was
// watched) and caches.
func Open(title, url string, dark bool, profile string) (*Window, error) {
	data := profile
	if data != "" {
		// Also as the environment variable, which WebView2 prefers to the
		// argument. go-webview2 passes the argument as a temporary UTF-16
		// buffer that nothing keeps alive while WebView2 reads it on
		// another thread; built as a GUI program the buffer was already
		// reused and WebView2 got a garbled relative path ("cannot create
		// data directory" under bin\).
		os.Setenv("WEBVIEW2_USER_DATA_FOLDER", data)
	}
	// Picking an episode is the gesture: it plays at once, with sound, on
	// the page it opens (Chromium would otherwise ask for a click there).
	if os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--autoplay-policy=no-user-gesture-required")
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  data,
		WindowOptions: webview2.WindowOptions{
			Title: title, Width: uint(scaled(1280)), Height: uint(scaled(760)), Center: true,
			IconId: appIcon,
		},
	})
	if w == nil {
		return nil, errNoWebView
	}
	win := &Window{w: w, hwnd: uintptr(w.Window())}
	win.setIcons(dark)
	win.setDarkFrame(dark)
	if err := w.Bind("kpFullscreen", func(on bool) error {
		w.Dispatch(func() { win.setFullscreen(on) })
		return nil
	}); err != nil {
		w.Destroy()
		return nil, err
	}
	if err := w.Bind("kpTheme", func(dark bool) error {
		w.Dispatch(func() {
			win.setDarkFrame(dark)
			win.setIcons(dark)
		})
		return nil
	}); err != nil {
		w.Destroy()
		return nil, err
	}
	// The window's title follows the page's (a switch to another episode).
	if err := w.Bind("kpTitle", func(t string) error {
		w.Dispatch(func() { w.SetTitle(t) })
		return nil
	}); err != nil {
		w.Destroy()
		return nil, err
	}
	// To the front (a video opened from Explorer while the window was open:
	// the second process allowed it, AllowForeground).
	if err := w.Bind("kpFront", func() error {
		w.Dispatch(win.front)
		return nil
	}); err != nil {
		w.Destroy()
		return nil, err
	}
	if err := win.bindDialogs(); err != nil {
		w.Destroy()
		return nil, err
	}
	w.Navigate(url)
	return win, nil
}

var (
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

func (win *Window) front() {
	const swRestore = 9
	if r, _, _ := procIsIconic.Call(win.hwnd); r != 0 {
		procShowWindow.Call(win.hwnd, swRestore)
	}
	procSetForegroundWindow.Call(win.hwnd)
}

// AllowForeground lets process pid bring its window to the front. A
// process started by the user (a video opened from Explorer) may; the
// already running window may not on its own.
func AllowForeground(pid int) {
	procAllowSetForegroundWindow.Call(uintptr(pid))
}

// appIcon and nightIcon are the icon groups in the executable's resources
// (cmd/jusplay/rsrc_windows_amd64.syso, made by tools/winres): the day one,
// which Explorer shows, and the night one, which the window wears in the
// dark theme so it matches the page.
const (
	appIcon   = 1
	nightIcon = 2
)

var (
	procGetModuleHandle     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procGetSystemMetricsDpi = user32.NewProc("GetSystemMetricsForDpi")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
)

const (
	wmSetIcon  = 0x0080
	iconSmall  = 0
	iconBig    = 1
	imageIcon  = 1
	smCxIcon   = 11
	smCxSmIcon = 49
)

// setIcons loads the title bar and taskbar icons for the theme at the
// window's DPI, so Windows picks the matching image instead of scaling one
// size.
func (win *Window) setIcons(dark bool) {
	group := uintptr(appIcon)
	if dark {
		group = nightIcon
	}
	inst, _, _ := procGetModuleHandle.Call(0)
	dpi, _, _ := procGetDpiForWindow.Call(win.hwnd)
	if dpi == 0 {
		dpi = 96
	}
	for i, v := range []struct{ which, metric uintptr }{{iconSmall, smCxSmIcon}, {iconBig, smCxIcon}} {
		n, _, _ := procGetSystemMetricsDpi.Call(v.metric, dpi)
		if h, _, _ := procLoadImage.Call(inst, group, imageIcon, n, n, 0); h != 0 {
			procSendMessage.Call(win.hwnd, wmSetIcon, v.which, h)
			if win.icons[i] != 0 {
				procDestroyIcon.Call(win.icons[i])
			}
			win.icons[i] = h
		}
	}
}

// Run blocks until the window closes.
func (win *Window) Run() {
	defer win.w.Destroy()
	win.w.Run()
}

// Close ends Run from any goroutine.
func (win *Window) Close() {
	win.w.Dispatch(win.w.Terminate)
}

type errString string

func (e errString) Error() string { return string(e) }

const errNoWebView = errString("could not create a WebView2 window (is the WebView2 runtime installed?)")

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procGetWindowPlace   = user32.NewProc("GetWindowPlacement")
	procSetWindowPlace   = user32.NewProc("SetWindowPlacement")
	procMonitorFromWin   = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

const (
	gwlStyle           = ^uintptr(15) // -16
	wsOverlappedWindow = 0x00CF0000
	monitorDefaultNear = 2
	swpNoMove          = 0x0002
	swpNoSize          = 0x0001
	swpNoZOrder        = 0x0004
	swpNoOwnerZOrder   = 0x0200
	swpFrameChanged    = 0x0020
)

type rect struct{ Left, Top, Right, Bottom int32 }

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	MinPos, MaxPos         [2]int32
	Normal                 rect
}

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

var procDwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// dwmwaUseImmersiveDarkMode is DWMWA_USE_IMMERSIVE_DARK_MODE (Windows 10
// 20H1 and later): a dark title bar and frame, following the page's theme
// instead of the system's app mode.
const dwmwaUseImmersiveDarkMode = 20

// DWMWA_CAPTION_COLOR and DWMWA_TEXT_COLOR (Windows 11): the title bar in
// the page's own background colour, also when "accent colour on title bars"
// is on. Ignored by Windows 10, which keeps the dark/light frame above.
const (
	dwmwaCaptionColor = 35
	dwmwaTextColor    = 36
)

// colorref is a Win32 COLORREF (0x00BBGGRR).
func colorref(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

func (win *Window) setDarkFrame(dark bool) {
	var v int32
	caption, text := colorref(0xf5, 0xf6, 0xf8), colorref(0x1a, 0x1c, 0x21)
	if dark {
		v = 1
		caption, text = colorref(0x0e, 0x0f, 0x12), colorref(0xe9, 0xea, 0xee)
	}
	procDwmSetWindowAttribute.Call(win.hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	procDwmSetWindowAttribute.Call(win.hwnd, dwmwaCaptionColor, uintptr(unsafe.Pointer(&caption)), unsafe.Sizeof(caption))
	procDwmSetWindowAttribute.Call(win.hwnd, dwmwaTextColor, uintptr(unsafe.Pointer(&text)), unsafe.Sizeof(text))
}

// setFullscreen switches between a borderless window covering the monitor
// and the saved normal window (the usual Win32 recipe).
func (win *Window) setFullscreen(on bool) {
	if on == win.isFull {
		return
	}
	h := win.hwnd
	if on {
		win.style, _, _ = procGetWindowLongPtr.Call(h, gwlStyle)
		win.saved.Length = uint32(unsafe.Sizeof(win.saved))
		procGetWindowPlace.Call(h, uintptr(unsafe.Pointer(&win.saved)))
		mon, _, _ := procMonitorFromWin.Call(h, monitorDefaultNear)
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
		procSetWindowLongPtr.Call(h, gwlStyle, win.style&^wsOverlappedWindow)
		r := mi.Monitor
		procSetWindowPos.Call(h, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			swpNoOwnerZOrder|swpFrameChanged)
	} else {
		procSetWindowLongPtr.Call(h, gwlStyle, win.style|wsOverlappedWindow)
		procSetWindowPlace.Call(h, uintptr(unsafe.Pointer(&win.saved)))
		procSetWindowPos.Call(h, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpFrameChanged)
	}
	win.isFull = on
}
