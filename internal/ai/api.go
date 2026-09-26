package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
)

// apiTranscribe sends a line's audio to an OpenAI-compatible
// /audio/transcriptions (OpenAI, Groq, SiliconFlow, a local whisper
// server, …). lang is an ISO 639-1 code or "".
func apiTranscribe(ctx context.Context, p plan, wavData []byte, lang string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "line.wav")
	if err != nil {
		return "", err
	}
	fw.Write(wavData)
	mw.WriteField("model", p.model)
	mw.WriteField("response_format", "json")
	if lang != "" && lang != "yue" {
		mw.WriteField("language", lang)
	}
	mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	rb, err := do(req)
	if err != nil {
		return "", err
	}
	var out struct{ Text string }
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", fmt.Errorf("bad answer: %.200s", rb)
	}
	return out.Text, nil
}

// apiTranslate asks an OpenAI-compatible /chat/completions to translate one line.
func apiTranslate(ctx context.Context, p plan, text, target string, terms [][2]string) (string, error) {
	system := "You translate video subtitles. Translate the user's line into " + target + ". Keep the tone of speech. Output only the translation, nothing else."
	if len(terms) > 0 {
		var b strings.Builder
		for _, t := range terms {
			b.WriteString(t[0] + " → " + t[1] + "; ")
		}
		system += " Render these names and terms this way: " + b.String()
	}
	return chat(ctx, p.url+"/chat/completions", p.apiKey, map[string]any{
		"model": p.model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": text},
		},
		"temperature": 0.3,
	})
}
