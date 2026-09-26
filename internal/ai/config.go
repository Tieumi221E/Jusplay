package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Backend is how one task runs.
type Backend struct {
	// Kind is "recommended" (default), "local" or "api".
	Kind string `json:"kind"`
	// Model: for "local" a .gguf file; for "api" the service's model name.
	Model string `json:"model,omitempty"`
	// Proj is a local recognition model's audio projector (mmproj .gguf).
	Proj string `json:"proj,omitempty"`
	// URL is the service's base address, e.g. https://api.openai.com/v1.
	URL string `json:"url,omitempty"`
	// KeyProt is the service's key, encrypted for this Windows user (DPAPI);
	// it is never given to the page.
	KeyProt string `json:"keyProt,omitempty"`
}

// Config is the two tasks' backends and the runtime for local models.
type Config struct {
	ASR Backend `json:"asr"`
	MT  Backend `json:"mt"`
	// Server is the llama-server for local models ("" = the component's).
	Server string `json:"server,omitempty"`
}

func loadConfig(path string) Config {
	var c Config
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &c)
		}
	}
	for _, b := range []*Backend{&c.ASR, &c.MT} {
		if b.Kind == "" {
			b.Kind = "recommended"
		}
	}
	return c
}

// PublicBackend is a backend as the page sees it: the key only as "set".
type PublicBackend struct {
	Kind   string `json:"kind"`
	Model  string `json:"model"`
	Proj   string `json:"proj"`
	URL    string `json:"url"`
	HasKey bool   `json:"hasKey"`
	// Key, in a change: a new key; "" keeps the old one unless ClearKey.
	Key      string `json:"key,omitempty"`
	ClearKey bool   `json:"clearKey,omitempty"`
}

// PublicConfig is the configuration as the page sees and changes it.
type PublicConfig struct {
	ASR    PublicBackend `json:"asr"`
	MT     PublicBackend `json:"mt"`
	Server string        `json:"server"`
	// Dir is the component folder (read only).
	Dir string `json:"dir"`
}

// Config returns the configuration without keys.
func (e *Engine) Config() PublicConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	pub := func(b Backend) PublicBackend {
		return PublicBackend{Kind: b.Kind, Model: b.Model, Proj: b.Proj, URL: b.URL, HasKey: b.KeyProt != ""}
	}
	return PublicConfig{ASR: pub(e.cfg.ASR), MT: pub(e.cfg.MT), Server: e.cfg.Server, Dir: e.Dir}
}

// SetConfig checks and stores a changed configuration; local servers it
// no longer uses are stopped.
func (e *Engine) SetConfig(p PublicConfig) error {
	e.mu.Lock()
	old := e.cfg
	e.mu.Unlock()
	next := Config{Server: strings.TrimSpace(p.Server)}
	for i, pb := range []PublicBackend{p.ASR, p.MT} {
		ob := []Backend{old.ASR, old.MT}[i]
		b := Backend{Kind: pb.Kind, Model: strings.TrimSpace(pb.Model), Proj: strings.TrimSpace(pb.Proj), URL: strings.TrimSpace(pb.URL), KeyProt: ob.KeyProt}
		switch b.Kind {
		case "recommended":
		case "local":
			for _, f := range []string{b.Model, b.Proj} {
				if f != "" && !filepath.IsAbs(f) {
					return fmt.Errorf("%q is not a full path", f)
				}
			}
		case "api":
			u, err := url.Parse(b.URL)
			if b.URL != "" && (err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "") {
				return fmt.Errorf("%q is not an http(s) address", b.URL)
			}
		default:
			return fmt.Errorf("unknown backend %q", b.Kind)
		}
		if pb.ClearKey {
			b.KeyProt = ""
		}
		if k := strings.TrimSpace(pb.Key); k != "" {
			prot, err := protect(k)
			if err != nil {
				return fmt.Errorf("storing the key: %w", err)
			}
			b.KeyProt = prot
		}
		if i == 0 {
			next.ASR = b
		} else {
			next.MT = b
		}
	}
	if e.cfgPath != "" {
		b, _ := json.MarshalIndent(next, "", " ")
		tmp := e.cfgPath + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, e.cfgPath); err != nil {
			return errors.Join(err, os.Remove(tmp))
		}
	}
	e.mu.Lock()
	e.cfg = next
	e.mu.Unlock()
	e.stopUnused()
	return nil
}
