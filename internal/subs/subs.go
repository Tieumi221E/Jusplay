// Package subs makes subtitles from a video's own sound: the page decodes
// a stretch of audio to 16 kHz PCM, Split finds the lines in it, the
// recogniser writes each line down and the translator translates it
// (package ai). The lines are kept per video in its folder's records, so a
// stretch is heard once; watching later, or again, reads them back.
//
// Whether the page asks for the stretch just ahead of what plays (subtitles
// while watching) or for the whole file (made in advance) is the page's
// choice; this side only turns audio into lines.
package subs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unicode"

	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/safeswap"
)

// Schema names the file format.
const Schema = "jusplay-subtitles/0"

// Cue is one line: when it is said, in which language, what was heard,
// and its translations by target language.
type Cue struct {
	Start float64           `json:"start"`
	End   float64           `json:"end"`
	Lang  string            `json:"lang"`
	Text  string            `json:"text"`
	Tr    map[string]string `json:"tr,omitempty"`
}

// Track is a video's subtitles as far as they have been made. Covered are
// the stretches already heard (sorted, merged), so they are not sent again;
// ASR and MT say which models wrote them (machine-made text is marked as
// such, and a later, better model can be told apart).
type Track struct {
	Schema  string       `json:"schema"`
	ASR     string       `json:"asr"`
	MT      string       `json:"mt"`
	Made    time.Time    `json:"made"`
	Covered [][2]float64 `json:"covered"`
	Cues    []Cue        `json:"cues"`
}

// Maker makes subtitles with a component and keeps them in files.
type Maker struct {
	C  *ai.Engine
	mu sync.Mutex // one stretch at a time: the models run one request each
}

// Load reads path's track ("" or missing: an empty one).
func Load(path string) (*Track, error) {
	t := &Track{Schema: Schema}
	if path == "" {
		return t, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, t); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if t.Schema != Schema {
		return nil, fmt.Errorf("%s: unknown schema %q", filepath.Base(path), t.Schema)
	}
	return t, nil
}

// Save writes t to path, replacing the old file safely.
func (t *Track) Save(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", " ")
	if err != nil {
		return err
	}
	tmp := safeswap.Temp(path)
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := safeswap.Sync(tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.Rename(tmp, path)
	}
	return safeswap.Swap(tmp, path)
}

// cover adds [a, b] to the covered stretches.
func (t *Track) cover(a, b float64) {
	if b <= a {
		return
	}
	all := append(t.Covered, [2]float64{a, b})
	sort.Slice(all, func(i, j int) bool { return all[i][0] < all[j][0] })
	out := all[:0]
	for _, r := range all {
		// Stretches that touch (within 50 ms) are one.
		if n := len(out); n > 0 && r[0] <= out[n-1][1]+0.05 {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	t.Covered = out
}

// add puts c in time order, replacing a cue that starts within 0.3 s of it
// (the same line heard again from an overlapping stretch).
func (t *Track) add(c Cue) {
	i := sort.Search(len(t.Cues), func(i int) bool { return t.Cues[i].Start >= c.Start-0.3 })
	if i < len(t.Cues) && t.Cues[i].Start <= c.Start+0.3 {
		t.Cues[i] = c
		return
	}
	t.Cues = append(t.Cues, Cue{})
	copy(t.Cues[i+1:], t.Cues[i:])
	t.Cues[i] = c
}

// Result is what one stretch gave.
type Result struct {
	// Next is where the following stretch should start: the chunk's end,
	// or the start of a line the chunk cut off.
	Next float64 `json:"next"`
	Cues []Cue   `json:"cues"`
}

// Chunk hears pcm (16 kHz mono), which starts at time start (seconds) in
// the video, adds its lines to the track at path, translated into target
// ("" for none), and returns them. lang is the language spoken (one of
// ai.Languages), or "" to let the track decide (Spoken). last says the
// chunk reaches the end of the file.
func (m *Maker) Chunk(ctx context.Context, path string, pcm []int16, start float64, target, lang string, last bool) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := Load(path)
	if err != nil {
		return Result{}, err
	}
	if lang == "" {
		lang = t.Spoken()
	}
	// While the translator is still loading, the lines go out untranslated
	// (the page asks for their translation next) rather than waiting for it.
	if target != "" && !m.C.Ready("mt") {
		target = ""
	}
	segs, open := Split(pcm, last)
	res := Result{Next: start + float64(open)/Rate}
	for _, s := range segs {
		tr, err := m.C.Transcribe(ctx, pcm[s.Start:s.End], lang)
		if err != nil {
			return res, err
		}
		dur := float64(s.End-s.Start) / Rate
		if tr.Text == "" || !Plausible(tr.Text, dur) {
			continue
		}
		c := Cue{Start: start + float64(s.Start)/Rate, End: start + float64(s.End)/Rate, Lang: tr.Lang, Text: tr.Text}
		if err := m.translate(ctx, &c, target); err != nil {
			return res, err
		}
		t.add(c)
		res.Cues = append(res.Cues, c)
	}
	t.cover(start, res.Next)
	t.ASR, t.MT = m.C.Models()
	t.Made = time.Now().UTC().Truncate(time.Second)
	return res, t.Save(path)
}

// Spoken is the language most of the track's lines are in, once there are
// enough of them (8, 70 % in one language), else "" (decide per line). A
// video is nearly always in one language; deciding per line, a short "うん"
// or a laugh came out as Chinese or English.
func (t *Track) Spoken() string {
	n := map[string]int{}
	for _, c := range t.Cues {
		n[c.Lang]++
	}
	for l, k := range n {
		if len(t.Cues) >= 8 && float64(k) >= 0.7*float64(len(t.Cues)) {
			return l
		}
	}
	return ""
}

// Plausible tells a spoken line from what the recogniser made of music: a
// few words stretched over seconds. People speak several characters a
// second (Japanese 6–8 kana, English 12+ letters); under a song or an
// instrumental the recogniser tends to write one word for a long stretch.
func Plausible(text string, seconds float64) bool {
	n := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			n++
		}
	}
	return seconds < 3 || float64(n)/seconds >= 1
}

// Translate adds target translations to the cues in [from, to) that lack
// one (the target changed after they were made) and returns those cues.
func (m *Maker) Translate(ctx context.Context, path string, from, to float64, target string) ([]Cue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := Load(path)
	if err != nil {
		return nil, err
	}
	var done []Cue
	for i := range t.Cues {
		c := &t.Cues[i]
		if c.Start < from || c.Start >= to || c.Tr[target] != "" || ai.SameLanguage(c.Lang, target) {
			continue
		}
		if err := m.translate(ctx, c, target); err != nil {
			return done, err
		}
		done = append(done, *c)
		// Saved as it goes: a seek away cancels the rest.
		if err := t.Save(path); err != nil {
			return done, err
		}
	}
	return done, nil
}

// translate fills c.Tr[target].
func (m *Maker) translate(ctx context.Context, c *Cue, target string) error {
	if target == "" || ai.SameLanguage(c.Lang, target) {
		return nil
	}
	out, err := m.C.Translate(ctx, c.Text, target)
	if err != nil {
		return err
	}
	if c.Tr == nil {
		c.Tr = map[string]string{}
	}
	c.Tr[target] = out
	return nil
}
