package ai

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestKeyRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI is Windows only")
	}
	p, err := protect("sk-test-123")
	if err != nil || p == "" || strings.Contains(p, "sk-test") {
		t.Fatalf("protect: %q %v", p, err)
	}
	if k, err := unprotect(p); err != nil || k != "sk-test-123" {
		t.Errorf("unprotect: %q %v", k, err)
	}
	if _, err := unprotect("AAAA"); err == nil {
		t.Error("garbage decrypted")
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ai.json")
	e := New(dir, cfg)
	if c := e.Config(); c.ASR.Kind != "recommended" || c.MT.Kind != "recommended" {
		t.Fatalf("defaults %+v", c)
	}
	// Recommended models not there: not ready, and it says what is missing.
	if a, _ := e.Status(); a.Ready || !strings.Contains(a.Why, "Qwen3-ASR-0.6B-Q8_0.gguf") {
		t.Errorf("status %+v", a)
	}
	for _, bad := range []PublicConfig{
		{ASR: PublicBackend{Kind: "api", URL: "ftp://x"}, MT: PublicBackend{Kind: "recommended"}},
		{ASR: PublicBackend{Kind: "local", Model: "relative.gguf"}, MT: PublicBackend{Kind: "recommended"}},
		{ASR: PublicBackend{Kind: "magic"}, MT: PublicBackend{Kind: "recommended"}},
	} {
		if err := e.SetConfig(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	good := PublicConfig{ASR: PublicBackend{Kind: "recommended"}, MT: PublicBackend{Kind: "api", URL: "https://api.example.com/v1", Model: "m", Key: "sk-1"}}
	if runtime.GOOS != "windows" {
		good.MT.Key = ""
	}
	if err := e.SetConfig(good); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg)
	if bytes.Contains(b, []byte("sk-1")) {
		t.Error("key stored in the clear")
	}
	// The page never sees the key; a change without one keeps it.
	e2 := New(dir, cfg)
	c := e2.Config()
	if c.MT.Kind != "api" || c.MT.HasKey != (runtime.GOOS == "windows") || c.MT.Key != "" {
		t.Errorf("reloaded %+v", c.MT)
	}
	c.MT.Model = "m2"
	if err := e2.SetConfig(c); err != nil {
		t.Fatal(err)
	}
	if c := e2.Config(); c.MT.Model != "m2" || c.MT.HasKey != (runtime.GOOS == "windows") {
		t.Errorf("after change %+v", c.MT)
	}
	if _, m := e2.Status(); !m.Ready || m.Name != "m2" {
		t.Errorf("api status %+v", m)
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return b.Bytes()
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestDownload(t *testing.T) {
	model := []byte("model bytes")
	runtimeZip := zipOf(t, map[string]string{"llama-server.exe": "exe", "ggml.dll": "dll"})
	evil := zipOf(t, map[string]string{"../escape.txt": "x"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/model":
			w.Write(model)
		case "/runtime.zip":
			w.Write(runtimeZip)
		case "/evil.zip":
			w.Write(evil)
		}
	}))
	defer srv.Close()
	old := Recommended
	defer func() { Recommended = old }()
	run := func(files []RemoteFile) (string, DownloadState) {
		Recommended = files
		dir := t.TempDir()
		e := New(dir, "")
		if err := e.StartDownload(); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 200 && e.Download().Running; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		return dir, e.Download()
	}
	dir, st := run([]RemoteFile{
		{Name: "llama.zip", Unzip: "llama", URL: srv.URL + "/runtime.zip", Size: int64(len(runtimeZip)), SHA256: sum(runtimeZip)},
		{Name: "m.gguf", URL: srv.URL + "/model", Size: int64(len(model)), SHA256: sum(model)},
	})
	if st.Error != "" || len(st.Missing) != 0 || st.Done != int64(len(model)+len(runtimeZip)) {
		t.Fatalf("good download: %+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "llama", "llama-server.exe")); string(b) != "exe" {
		t.Error("runtime not unpacked")
	}
	if _, err := os.Stat(filepath.Join(dir, "llama.zip")); err == nil {
		t.Error("archive kept")
	}
	// A file that is not the expected one is refused and not kept.
	dir, st = run([]RemoteFile{{Name: "m.gguf", URL: srv.URL + "/model", Size: int64(len(model)), SHA256: sum([]byte("other"))}})
	if !strings.Contains(st.Error, "not the expected file") {
		t.Errorf("bad sum: %+v", st)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("left %v", ents)
	}
	// An archive with a path out of its folder is refused.
	_, st = run([]RemoteFile{{Name: "e.zip", Unzip: "llama", URL: srv.URL + "/evil.zip", Size: int64(len(evil)), SHA256: sum(evil)}})
	if !strings.Contains(st.Error, "unsafe path") {
		t.Errorf("evil zip: %+v", st)
	}
}

// The service backends speak the OpenAI API: a multipart WAV upload with
// the model and the language, a chat completion; the key as a bearer token.
func TestAPIBackends(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("keys are stored with DPAPI")
	}
	var gotLang, gotModel, gotAuth, gotPrompt string
	var gotWav int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			f, _, err := r.FormFile("file")
			if err == nil {
				b := new(bytes.Buffer)
				b.ReadFrom(f)
				gotWav = b.Len()
			}
			gotLang, gotModel = r.FormValue("language"), r.FormValue("model")
			w.Write([]byte(`{"text":" こんにちは "}`))
		case "/v1/chat/completions":
			b := new(bytes.Buffer)
			b.ReadFrom(r.Body)
			gotPrompt = b.String()
			w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e := New(t.TempDir(), "")
	err := e.SetConfig(PublicConfig{
		ASR: PublicBackend{Kind: "api", URL: srv.URL + "/v1/", Model: "whisper-x", Key: "sk-a"},
		MT:  PublicBackend{Kind: "api", URL: srv.URL + "/v1", Model: "chat-x", Key: "sk-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := e.Transcribe(t.Context(), make([]int16, 1600), "Japanese")
	if err != nil || tr.Text != "こんにちは" || tr.Lang != "Japanese" {
		t.Fatalf("transcribe %+v %v", tr, err)
	}
	if gotLang != "ja" || gotModel != "whisper-x" || gotAuth != "Bearer sk-a" || gotWav != 44+3200 {
		t.Errorf("transcription request: lang %q model %q auth %q wav %d", gotLang, gotModel, gotAuth, gotWav)
	}
	out, err := e.Translate(t.Context(), "こんにちは", "zh")
	if err != nil || out != "你好" || gotAuth != "Bearer sk-b" || !strings.Contains(gotPrompt, "Simplified Chinese") || !strings.Contains(gotPrompt, `"model":"chat-x"`) {
		t.Errorf("translate %q %v, auth %q, body %s", out, err, gotAuth, gotPrompt)
	}
	// A service that refuses says so.
	e.SetConfig(PublicConfig{ASR: PublicBackend{Kind: "api", URL: srv.URL + "/nope", Model: "m"}, MT: PublicBackend{Kind: "recommended"}})
	if _, err := e.Transcribe(t.Context(), make([]int16, 160), ""); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("refused: %v", err)
	}
}
