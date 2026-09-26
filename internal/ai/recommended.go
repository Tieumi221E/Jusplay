package ai

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The recommended models (Apache 2.0) and runtime (MIT), fixed to exact
// files: a download is checked against these sizes and SHA-256 sums, and
// nothing else is fetched. Updating them is a code change.
var Recommended = []RemoteFile{
	{Name: "llama-b11193-bin-win-vulkan-x64.zip", Unzip: "llama", Size: 32469216,
		SHA256: "a106346b30d79883626bd9cf9f0d787ff949eeaf3feaf6a4f31d41a2ecf9086b",
		URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b11193/llama-b11193-bin-win-vulkan-x64.zip"},
	{Name: "Qwen3-ASR-0.6B-Q8_0.gguf", Size: 804749248,
		SHA256: "bca259818b50ca7c4c05e9bdb35a5dc04fa039653a6d6f3f0f331f96f6aa1971",
		URL:    "https://huggingface.co/ggml-org/Qwen3-ASR-0.6B-GGUF/resolve/main/Qwen3-ASR-0.6B-Q8_0.gguf"},
	{Name: "mmproj-Qwen3-ASR-0.6B-Q8_0.gguf", Size: 214392480,
		SHA256: "41a342b5e4c514e968cb756de6cd1b7be39eff43c44c57a2ef5fc6522e36603d",
		URL:    "https://huggingface.co/ggml-org/Qwen3-ASR-0.6B-GGUF/resolve/main/mmproj-Qwen3-ASR-0.6B-Q8_0.gguf"},
	{Name: "Hy-MT2-1.8B-Q4_K_M.gguf", Size: 1133080448,
		SHA256: "dc5f44fcf1fa496ee7ad725982c0c8c553a4de00259b53af84c4b89fb0c06699",
		URL:    "https://huggingface.co/tencent/Hy-MT2-1.8B-GGUF/resolve/main/Hy-MT2-1.8B-Q4_K_M.gguf"},
}

var (
	recommendedModel = map[string]string{"asr": "Qwen3-ASR-0.6B-Q8_0.gguf", "mt": "Hy-MT2-1.8B-Q4_K_M.gguf"}
	recommendedProj  = "mmproj-Qwen3-ASR-0.6B-Q8_0.gguf"
)

// RemoteFile is one file of the recommended component.
type RemoteFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	// Unzip, when set, is the folder the archive is unpacked into (the
	// archive itself is not kept).
	Unzip string `json:"unzip,omitempty"`
}

// present reports whether f is already in dir (a model of the right size,
// or the unpacked runtime).
func (f RemoteFile) present(dir string) bool {
	if f.Unzip != "" {
		_, err := os.Stat(filepath.Join(dir, f.Unzip, "llama-server.exe"))
		return err == nil
	}
	st, err := os.Stat(filepath.Join(dir, f.Name))
	return err == nil && st.Size() == f.Size
}

// DownloadState is how a download of the recommended component stands.
type DownloadState struct {
	Running bool     `json:"running"`
	Done    int64    `json:"done"`  // bytes, over all files
	Total   int64    `json:"total"` // bytes still missing when it began
	File    string   `json:"file"`
	Error   string   `json:"error,omitempty"`
	Missing []string `json:"missing"` // files not yet in the folder
	Bytes   int64    `json:"bytes"`   // their total size
}

type download struct {
	mu     sync.Mutex
	st     DownloadState
	cancel context.CancelFunc
}

// Download reports the recommended component's download state.
func (e *Engine) Download() DownloadState {
	e.dl.mu.Lock()
	st := e.dl.st
	e.dl.mu.Unlock()
	st.Missing, st.Bytes = nil, 0
	for _, f := range Recommended {
		if !f.present(e.Dir) {
			st.Missing = append(st.Missing, f.Name)
			st.Bytes += f.Size
		}
	}
	return st
}

// StartDownload fetches the missing recommended files into the component
// folder, in the background. Only on the user's request: the app fetches
// nothing by itself.
func (e *Engine) StartDownload() error {
	e.dl.mu.Lock()
	defer e.dl.mu.Unlock()
	if e.dl.st.Running {
		return nil
	}
	var todo []RemoteFile
	var total int64
	for _, f := range Recommended {
		if !f.present(e.Dir) {
			todo = append(todo, f)
			total += f.Size
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if err := os.MkdirAll(e.Dir, 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.dl.cancel = cancel
	e.dl.st = DownloadState{Running: true, Total: total}
	go func() {
		defer cancel()
		var err error
		for _, f := range todo {
			if err = e.fetch(ctx, f); err != nil {
				break
			}
		}
		e.dl.mu.Lock()
		e.dl.st.Running, e.dl.st.File = false, ""
		if err != nil {
			e.dl.st.Error = err.Error()
			if errors.Is(err, context.Canceled) {
				e.dl.st.Error = "cancelled"
			}
		}
		e.dl.mu.Unlock()
		e.logf("ai: download finished: %v", err)
	}()
	return nil
}

// CancelDownload stops a running download; the finished files stay.
func (e *Engine) CancelDownload() {
	e.dl.mu.Lock()
	if e.dl.cancel != nil {
		e.dl.cancel()
	}
	e.dl.mu.Unlock()
}

// fetch downloads f to a temporary file, checks its size and sum, and only
// then puts it in place (or unpacks it).
func (e *Engine) fetch(ctx context.Context, f RemoteFile) error {
	e.dl.mu.Lock()
	e.dl.st.File = f.Name
	e.dl.mu.Unlock()
	part := filepath.Join(e.Dir, f.Name+".part")
	defer os.Remove(part)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	dl := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: time.Minute}}
	r, err := dl.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", f.Name, r.StatusCode)
	}
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h, progress{e}), io.LimitReader(r.Body, f.Size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("%s: not the expected file (%d bytes, SHA-256 differs)", f.Name, n)
	}
	if f.Unzip != "" {
		return unzip(part, filepath.Join(e.Dir, f.Unzip))
	}
	return os.Rename(part, filepath.Join(e.Dir, f.Name))
}

type progress struct{ e *Engine }

func (p progress) Write(b []byte) (int, error) {
	p.e.dl.mu.Lock()
	p.e.dl.st.Done += int64(len(b))
	p.e.dl.mu.Unlock()
	return len(b), nil
}

// unzip unpacks archive into dir (made fresh), refusing paths that leave it.
func unzip(archive, dir string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	tmp := dir + ".part"
	os.RemoveAll(tmp)
	for _, f := range zr.File {
		p := filepath.Join(tmp, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(p, tmp+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(p)
		if err == nil {
			_, err = io.Copy(w, rc)
			if cerr := w.Close(); err == nil {
				err = cerr
			}
		}
		rc.Close()
		if err != nil {
			return err
		}
	}
	os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}
