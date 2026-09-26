"""Writes THIRD_PARTY_NOTICES.txt: the license of everything inside the
released jusplay.exe that Jusplay did not write. Run from the repository
root after changing a dependency:

  python tools/notices.py

Sources: the Go module cache (go env GOMODCACHE) for the Go modules and the
WebView2 loader they embed, the Go distribution (go env GOROOT) for the Go
runtime and standard library, and the repository for the vendored comment
renderer and the fonts.
"""
import os, subprocess

def go(*args):
    return subprocess.check_output(["go", *args], text=True).strip()

root = go("env", "GOROOT")
cache = go("env", "GOMODCACHE")
mods = {}
for line in go("list", "-deps", "-f", "{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}", "./cmd/jusplay").splitlines():
    p = line.split()
    if len(p) == 2:
        mods[p[0]] = p[1]


def mod_file(path, *names):
    d = os.path.join(cache, f"{path}@{mods[path]}")
    for n in names:
        f = os.path.join(d, n)
        if os.path.exists(f):
            return open(f, encoding="utf-8").read().strip()
    raise SystemExit(f"no license file in {d}")


parts = [
    ("Go runtime and standard library", "https://go.dev", "BSD-3-Clause", open(os.path.join(root, "LICENSE"), encoding="utf-8").read().strip()),
]
for path, what in [("github.com/jchv/go-webview2", "WebView2 bindings"), ("github.com/jchv/go-winloader", "DLL loader used by go-webview2"),
                   ("github.com/santhosh-tekuri/jsonschema/v6", "JSON Schema validation"),
                   ("golang.org/x/sys", "Windows system calls"), ("golang.org/x/text", "text encodings")]:
    if path in mods:
        parts.append((f"{path} {mods[path]} ({what})", "https://" + path, "", mod_file(path, "LICENSE", "LICENSE.md", "LICENSE.txt")))
if "github.com/jchv/go-webview2" in mods:
    parts.append(("Microsoft WebView2Loader.dll (embedded by go-webview2)", "https://aka.ms/webview2", "",
                  mod_file("github.com/jchv/go-webview2", "webviewloader/sdk/LICENSE.txt")))
parts.append(("niconicomments 0.4.1, modified (web/src/nico: the comment renderer)", "https://github.com/xpadev-net/niconicomments", "MIT",
              open("web/src/nico/LICENSE", encoding="utf-8").read().strip()))
parts.append(("valibot 1.x (bundled inside niconicomments)", "https://github.com/fabian-hiller/valibot", "MIT",
              "MIT License\n\nCopyright (c) Fabian Hiller\n\nPermission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the \"Software\"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE."))
parts.append(("Fonts: Jus Sans (Source Han Sans 2.005) and Jus Emoji (Noto Color Emoji 2.057), subsets, renamed", "https://github.com/adobe-fonts/source-han-sans, https://github.com/googlefonts/noto-emoji", "OFL-1.1",
              open("web/src/fonts/LICENSE.txt", encoding="utf-8").read().strip()))

out = ["Third-party software in Jusplay",
       "================================",
       "",
       "Jusplay itself is under the MIT License (LICENSE). jusplay.exe also contains",
       "the following, each under its own license, reproduced below.",
       ""]
for name, url, spdx, text in parts:
    out += ["-" * 78, name, url + (f"  ({spdx})" if spdx else ""), "-" * 78, "", text, ""]
with open("THIRD_PARTY_NOTICES.txt", "w", encoding="utf-8", newline="\n") as f:
    f.write("\n".join(out))
print(f"THIRD_PARTY_NOTICES.txt: {len(parts)} components")
