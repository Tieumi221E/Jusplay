// Package ai makes subtitles' text: speech recognition and translation, by
// models on this machine or by a service the user names.
//
// None of it is part of the app itself, and no model ships with it. Each
// of the two tasks has a backend (Config), chosen by the user:
//
//   - "recommended": the models Jusplay suggests, run on this machine.
//     They and their runtime (llama.cpp's llama-server) are a download of
//     their own into the component folder (Recommended, Download):
//     Qwen3-ASR-0.6B for recognition, Hy-MT2-1.8B for translation.
//   - "local": the user's own GGUF model files, run the same way.
//   - "api": an OpenAI-compatible service (/audio/transcriptions,
//     /chat/completions). Only this sends anything off the machine, and
//     only when the user has set it up.
//
// Each local model runs in its own llama-server on a loopback port,
// started on first use and stopped with the app (a Windows job object ends
// them even if the app crashes).
package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Engine runs the configured backends.
type Engine struct {
	// Dir is the component folder: where the recommended models and the
	// runtime are, or are downloaded to.
	Dir string
	Log func(format string, args ...any)

	cfgPath string
	mu      sync.Mutex
	cfg     Config
	servers map[string]*server     // running local servers by their arguments
	starts  map[string]*sync.Mutex // one start at a time per server
	dl      download
}

// New prepares an engine whose component folder is dir and whose
// configuration is kept in cfgPath ("" keeps it in memory).
func New(dir, cfgPath string) *Engine {
	e := &Engine{Dir: dir, cfgPath: cfgPath, servers: map[string]*server{}, starts: map[string]*sync.Mutex{}}
	e.cfg = loadConfig(cfgPath)
	return e
}

func (e *Engine) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(format, args...)
	}
}

// Close stops the local servers and a download.
func (e *Engine) Close() {
	e.CancelDownload()
	e.mu.Lock()
	all := e.servers
	e.servers = map[string]*server{}
	e.mu.Unlock()
	for _, s := range all {
		s.stop()
	}
}

// runtime is the llama-server to run local models with.
func (e *Engine) runtime(c Config) string {
	if c.Server != "" {
		return c.Server
	}
	return filepath.Join(e.Dir, "llama", "llama-server.exe")
}

// plan is how a task will run: a local server (exe and arguments) or a service.
type plan struct {
	kind   string // "local" or "api"
	name   string // the model, for the subtitles' provenance
	exe    string
	args   []string
	qwen   bool // a Qwen3-ASR model (its answer format, its language prompt)
	url    string
	model  string
	apiKey string
}

var errNotSet = errors.New("not set up")

// planFor resolves a task's backend to what to run, or says what is missing.
func (e *Engine) planFor(task string) (plan, error) {
	e.mu.Lock()
	c := e.cfg
	e.mu.Unlock()
	b := c.ASR
	if task == "mt" {
		b = c.MT
	}
	switch b.Kind {
	case "api":
		if b.URL == "" || b.Model == "" {
			return plan{}, fmt.Errorf("%w: the service needs an address and a model", errNotSet)
		}
		key, err := unprotect(b.KeyProt)
		if err != nil {
			return plan{}, fmt.Errorf("API key: %w", err)
		}
		return plan{kind: "api", name: b.Model, url: strings.TrimRight(b.URL, "/"), model: b.Model, apiKey: key}, nil
	case "local", "", "recommended":
		model, proj := b.Model, b.Proj
		if b.Kind != "local" {
			model = filepath.Join(e.Dir, recommendedModel[task])
			proj = ""
			if task == "asr" {
				proj = filepath.Join(e.Dir, recommendedProj)
			}
		}
		if model == "" {
			return plan{}, fmt.Errorf("%w: no model file chosen", errNotSet)
		}
		if task == "asr" && proj == "" {
			return plan{}, fmt.Errorf("%w: the recognition model needs its mmproj file", errNotSet)
		}
		exe := e.runtime(c)
		var missing []string
		for _, f := range []string{exe, model, proj} {
			if _, err := os.Stat(f); f != "" && err != nil {
				missing = append(missing, filepath.Base(f))
			}
		}
		if len(missing) > 0 {
			return plan{}, fmt.Errorf("%w: missing %s", ErrMissing, strings.Join(missing, ", "))
		}
		args := []string{"-m", model}
		if task == "asr" {
			args = append(args, "--mmproj", proj)
		}
		name := strings.TrimSuffix(filepath.Base(model), ".gguf")
		return plan{kind: "local", name: name, exe: exe, args: args, qwen: strings.Contains(strings.ToLower(name), "qwen3-asr")}, nil
	}
	return plan{}, fmt.Errorf("unknown backend %q", b.Kind)
}

// ErrMissing means a model or the runtime is not on disk.
var ErrMissing = errors.New("model files not found")

// TaskStatus says whether a task can run and with what.
type TaskStatus struct {
	Ready bool   `json:"ready"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Why   string `json:"why,omitempty"`
}

// Status says whether recognition and translation can run.
func (e *Engine) Status() (asr, mt TaskStatus) {
	st := func(task string) TaskStatus {
		p, err := e.planFor(task)
		e.mu.Lock()
		b := e.cfg.ASR
		if task == "mt" {
			b = e.cfg.MT
		}
		e.mu.Unlock()
		kind := b.Kind
		if kind == "" {
			kind = "recommended"
		}
		if err != nil {
			return TaskStatus{Kind: kind, Why: err.Error()}
		}
		return TaskStatus{Ready: true, Kind: kind, Name: p.name}
	}
	return st("asr"), st("mt")
}

// Models names the models in use, for the subtitles' provenance.
func (e *Engine) Models() (asr, mt string) {
	a, m := e.Status()
	return a.Name, m.Name
}

// local returns the running server for p, starting it if needed.
func (e *Engine) local(ctx context.Context, p plan) (*server, error) {
	key := p.exe + "\x00" + strings.Join(p.args, "\x00")
	e.mu.Lock()
	lock := e.starts[key]
	if lock == nil {
		lock = &sync.Mutex{}
		e.starts[key] = lock
	}
	e.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	e.mu.Lock()
	s := e.servers[key]
	e.mu.Unlock()
	if s != nil && s.alive() {
		return s, nil
	}
	s, err := start(ctx, p.exe, p.args, e.logf)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", p.name, err)
	}
	e.mu.Lock()
	e.servers[key] = s
	e.mu.Unlock()
	return s, nil
}

// Warm starts the local servers now: recognition first, then translation.
// Loading both at once on an integrated GPU took as long as one after the
// other (8.5 s against 4 + 4.7 s), and the first lines need only the
// recogniser: they are shown untranslated until the translator is up.
func (e *Engine) Warm() {
	go func() {
		for _, task := range []string{"asr", "mt"} {
			p, err := e.planFor(task)
			if err != nil || p.kind != "local" {
				continue
			}
			if _, err := e.local(context.Background(), p); err != nil {
				e.logf("ai: %v", err)
			}
		}
	}()
}

// Ready reports whether a task can answer now without loading a model
// (a service is always ready; a local model once its server is up).
func (e *Engine) Ready(task string) bool {
	p, err := e.planFor(task)
	if err != nil {
		return false
	}
	if p.kind == "api" {
		return true
	}
	e.mu.Lock()
	s := e.servers[p.exe+"\x00"+strings.Join(p.args, "\x00")]
	e.mu.Unlock()
	return s != nil && s.alive()
}

// stopUnused stops local servers the configuration no longer uses.
func (e *Engine) stopUnused() {
	used := map[string]bool{}
	for _, task := range []string{"asr", "mt"} {
		if p, err := e.planFor(task); err == nil && p.kind == "local" {
			used[p.exe+"\x00"+strings.Join(p.args, "\x00")] = true
		}
	}
	e.mu.Lock()
	var stop []*server
	for k, s := range e.servers {
		if !used[k] {
			stop = append(stop, s)
			delete(e.servers, k)
		}
	}
	e.mu.Unlock()
	for _, s := range stop {
		s.stop()
	}
}

// Transcription is what the recogniser heard.
type Transcription struct {
	Lang string // e.g. "Japanese"; "" when unknown or when it heard no speech
	Text string
}

// Languages the recogniser can be told to expect, by the names Qwen3-ASR
// uses, with their ISO 639-1 codes for services.
var Languages = map[string]string{"Japanese": "ja", "Chinese": "zh", "English": "en", "Korean": "ko", "Cantonese": "yue",
	"French": "fr", "German": "de", "Spanish": "es", "Russian": "ru"}

// Transcribe recognises 16 kHz mono PCM. lang, when not "", is the
// language to expect (a key of Languages); left to itself a recogniser
// decides per line, and a short "うん" or a laugh came out as Chinese or English.
func (e *Engine) Transcribe(ctx context.Context, pcm []int16, lang string) (Transcription, error) {
	p, err := e.planFor("asr")
	if err != nil {
		return Transcription{}, err
	}
	if p.kind == "api" {
		text, err := apiTranscribe(ctx, p, wav(pcm), Languages[lang])
		return Transcription{Lang: lang, Text: strings.TrimSpace(text)}, err
	}
	s, err := e.local(ctx, p)
	if err != nil {
		return Transcription{}, err
	}
	return localTranscribe(ctx, s, p, pcm, lang)
}

// Targets are the languages subtitles can be translated into: code → the
// name in the translation prompt (Chinese, for Hy-MT2's prompts) and in
// English (for services).
var Targets = map[string][2]string{"zh": {"中文", "Simplified Chinese"}, "zh-Hant": {"繁体中文", "Traditional Chinese"},
	"ja": {"日语", "Japanese"}, "en": {"英语", "English"}, "ko": {"韩语", "Korean"}}

// SameLanguage reports whether the recogniser's language name is the target.
func SameLanguage(lang, target string) bool {
	switch target {
	case "zh", "zh-Hant":
		return lang == "Chinese" || lang == "Cantonese"
	case "ja":
		return lang == "Japanese"
	case "en":
		return lang == "English"
	case "ko":
		return lang == "Korean"
	}
	return false
}

// Translate translates one line into target (a key of Targets).
func (e *Engine) Translate(ctx context.Context, text, target string) (string, error) {
	names, ok := Targets[target]
	if !ok {
		return "", fmt.Errorf("unknown target language %q", target)
	}
	p, err := e.planFor("mt")
	if err != nil {
		return "", err
	}
	if p.kind == "api" {
		out, err := apiTranslate(ctx, p, text, names[1])
		return strings.TrimSpace(out), err
	}
	s, err := e.local(ctx, p)
	if err != nil {
		return "", err
	}
	out, err := localTranslate(ctx, s, text, names[0])
	return strings.TrimSpace(out), err
}
