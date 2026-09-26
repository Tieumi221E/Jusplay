package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// localTranscribe asks a local recogniser. A Qwen3-ASR model answers
// "language Japanese<asr_text>…"; told the language, its answer is begun
// with it so it only writes the text. Other audio models get an instruction.
func localTranscribe(ctx context.Context, s *server, p plan, pcm []int16, lang string) (Transcription, error) {
	content := []any{map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(wav(pcm)), "format": "wav"}}}
	if !p.qwen {
		instr := "Transcribe this audio verbatim in the language spoken. Output only the transcription."
		if lang != "" {
			instr = "Transcribe this " + lang + " audio verbatim. Output only the transcription."
		}
		content = append(content, map[string]any{"type": "text", "text": instr})
	}
	msgs := []any{map[string]any{"role": "user", "content": content}}
	prefix := ""
	if p.qwen && lang != "" {
		prefix = "language " + lang + "<asr_text>"
		msgs = append(msgs, map[string]any{"role": "assistant", "content": prefix})
	}
	out, err := s.chat(ctx, map[string]any{"messages": msgs, "temperature": 0, "max_tokens": 512})
	if err != nil {
		return Transcription{}, err
	}
	if !p.qwen {
		return Transcription{Lang: lang, Text: strings.TrimSpace(out)}, nil
	}
	// llama-server answers a begun reply with the whole of it; should it
	// give only the continuation, the prefix is put back.
	if prefix != "" && !strings.Contains(out, "<asr_text>") {
		out = prefix + out
	}
	return parseASR(out), nil
}

// parseASR reads Qwen3-ASR's "language Japanese<asr_text>…" answer.
func parseASR(out string) Transcription {
	lang, text, ok := strings.Cut(out, "<asr_text>")
	if !ok {
		return Transcription{Text: strings.TrimSpace(out)}
	}
	lang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lang), "language"))
	text = strings.TrimSpace(text)
	if strings.EqualFold(lang, "none") || text == "" {
		return Transcription{}
	}
	return Transcription{Lang: lang, Text: text}
}

// localTranslate asks a local translation model with Hy-MT2's plain
// translation prompt, one line at a time: given the lines before as
// background (its "Structured Data 2" prompt), the 1.8B model translated
// those too, into this line.
func localTranslate(ctx context.Context, s *server, text, targetName string, terms [][2]string) (string, error) {
	prompt := "将以下文本翻译为" + targetName + "，注意只需要输出翻译后的结果，不要额外解释：\n\n" + text
	if len(terms) > 0 {
		// The model card's terminology prompt: names keep one rendering.
		var b strings.Builder
		b.WriteString("参考下面的翻译：\n")
		for _, t := range terms {
			b.WriteString(t[0] + " 翻译成 " + t[1] + "\n")
		}
		prompt = b.String() + prompt
	}
	// The model card's recommended sampling for the 1.8B and 7B models.
	return s.chat(ctx, map[string]any{
		"messages":       []any{map[string]any{"role": "user", "content": prompt}},
		"temperature":    0.7,
		"top_p":          0.6,
		"top_k":          20,
		"repeat_penalty": 1.05,
		"max_tokens":     256,
	})
}

// wav wraps 16 kHz mono 16-bit PCM in a WAV header.
func wav(pcm []int16) []byte {
	var b bytes.Buffer
	n := uint32(len(pcm) * 2)
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, 36+n)
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, n)
	binary.Write(&b, binary.LittleEndian, pcm)
	return b.Bytes()
}

// ---- one llama-server ----

type server struct {
	cmd  *exec.Cmd
	base string
	done chan struct{}
}

func start(ctx context.Context, exe string, args []string, logf func(string, ...any)) (*server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	// -ngl 99: every layer on the GPU when there is one (the Vulkan build
	// falls back to the CPU when there is none). Context and slots come
	// with the plan.
	args = append(args, "--host", "127.0.0.1", "--port", fmt.Sprint(port), "-ngl", "99", "--no-webui")
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	var tail tailBuffer
	cmd.Stdout, cmd.Stderr = &tail, &tail
	hide(cmd)
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	adopt(cmd.Process)
	s := &server{cmd: cmd, base: fmt.Sprintf("http://127.0.0.1:%d", port), done: make(chan struct{})}
	go func() { cmd.Wait(); close(s.done) }()
	// Loading a model takes a few seconds (longer the first time it is read
	// from disk).
	deadline := time.After(3 * time.Minute)
	for {
		select {
		case <-s.done:
			return nil, fmt.Errorf("exited: %s", tail.String())
		case <-ctx.Done():
			s.stop()
			return nil, ctx.Err()
		case <-deadline:
			s.stop()
			return nil, errors.New("not ready after 3 minutes")
		case <-time.After(200 * time.Millisecond):
		}
		r, err := http.Get(s.base + "/health")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == http.StatusOK {
				logf("ai: %s ready in %s", filepath.Base(args[1]), time.Since(t0).Round(time.Millisecond))
				return s, nil
			}
		}
	}
}

func (s *server) alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *server) stop() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	<-s.done
}

func (s *server) chat(ctx context.Context, body any) (string, error) {
	return chat(ctx, s.base+"/v1/chat/completions", "", body)
}

// chat posts an OpenAI-style chat completion and returns the answer's text.
func chat(ctx context.Context, url, key string, body any) (string, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rb, err := do(req)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("bad answer: %.200s", rb)
	}
	return out.Choices[0].Message.Content, nil
}

// client serves both the loopback servers and services: no overall timeout
// (a model may think), the environment's proxy for services.
var client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 2 * time.Minute}}

func do(req *http.Request) ([]byte, error) {
	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %.300s", r.StatusCode, bytes.TrimSpace(rb))
	}
	return rb, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// tailBuffer keeps the last 4 KB a server wrote, for an error message.
type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.b))
}
