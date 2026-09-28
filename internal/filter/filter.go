// Package filter is playback-time comment filtering: which comments the
// player shows, and what each rule hid. It never changes the archive; it
// answers, per thread, the positions of the comments kept, and counts. The
// player page and the command line use this one reading (it was the page's
// own, web/src/filter.ts, until 0.4.0), so what `jusplay comments list`
// prints is what is on screen, and translation works on the same lines.
package filter

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// Settings are the player settings filtering reads: the "filters" object
// of the settings document, and from "comments" the forks shown and
// whether owner scripts run.
type Settings struct {
	NgWords      []string `json:"ngWords"` // substrings, or /regex/flags
	NgUsers      []string `json:"ngUsers"`
	NgCommands   []string `json:"ngCommands"`
	Hide         Hide     `json:"hide"`
	NgShare      string   `json:"ngShare"`      // none | weak | medium | strong
	MaxLength    int      `json:"maxLength"`    // hide non-art comments longer than this; 0 off
	PostedBefore string   `json:"postedBefore"` // only comments posted at or before this date(time); "" off
	// CapPerSecond is the total budget in comments per second of video (at
	// least 100 overall); 0 no cap. Applied after the other rules.
	CapPerSecond float64         `json:"capPerSecond"`
	Forks        map[string]bool `json:"-"`
	RunScripts   bool            `json:"-"`
}

type Hide struct {
	Naka      bool `json:"naka"`
	Ue        bool `json:"ue"`
	Shita     bool `json:"shita"`
	Colored   bool `json:"colored"`
	Big       bool `json:"big"`
	Small     bool `json:"small"`
	CA        bool `json:"ca"`
	Anonymous bool `json:"anonymous"`
}

// Defaults are the page's defaults (web/src/settings.ts, defaults()).
func Defaults() Settings {
	return Settings{NgShare: "none", CapPerSecond: 2, Forks: map[string]bool{"owner": true, "main": true, "easy": true}, RunScripts: true}
}

// FromDocument reads the settings from a settings document, keeping only
// values of the right type and range over the defaults (as the page's
// merge does).
func FromDocument(doc []byte) Settings {
	s := Defaults()
	var d struct {
		Comments struct {
			Forks      map[string]json.RawMessage `json:"forks"`
			RunScripts json.RawMessage            `json:"runScripts"`
		} `json:"comments"`
		Filters map[string]json.RawMessage `json:"filters"`
	}
	if json.Unmarshal(doc, &d) != nil {
		return s
	}
	for _, k := range []string{"owner", "main", "easy"} {
		var b bool
		if v, ok := d.Comments.Forks[k]; ok && json.Unmarshal(v, &b) == nil {
			s.Forks[k] = b
		}
	}
	var b bool
	if json.Unmarshal(d.Comments.RunScripts, &b) == nil && d.Comments.RunScripts != nil {
		s.RunScripts = b
	}
	f := d.Filters
	s.NgWords, s.NgUsers, s.NgCommands = strings_(f["ngWords"]), strings_(f["ngUsers"]), strings_(f["ngCommands"])
	var hide map[string]json.RawMessage
	if json.Unmarshal(f["hide"], &hide) == nil {
		for k, p := range map[string]*bool{"naka": &s.Hide.Naka, "ue": &s.Hide.Ue, "shita": &s.Hide.Shita, "colored": &s.Hide.Colored,
			"big": &s.Hide.Big, "small": &s.Hide.Small, "ca": &s.Hide.CA, "anonymous": &s.Hide.Anonymous} {
			var v bool
			if raw, ok := hide[k]; ok && json.Unmarshal(raw, &v) == nil {
				*p = v
			}
		}
	}
	var str string
	if json.Unmarshal(f["ngShare"], &str) == nil {
		if _, ok := shareThreshold[str]; ok || str == "none" {
			s.NgShare = str
		}
	}
	var n float64
	if json.Unmarshal(f["maxLength"], &n) == nil && n >= 0 && !math.IsInf(n, 0) {
		s.MaxLength = int(n)
	}
	if json.Unmarshal(f["postedBefore"], &str) == nil {
		s.PostedBefore = str
	}
	if json.Unmarshal(f["capPerSecond"], &n) == nil && !math.IsNaN(n) {
		s.CapPerSecond = math.Min(math.Max(n, 0), 50)
	}
	return s
}

// strings_ keeps the strings of a JSON array (other elements dropped, as
// the page's merge does).
func strings_(raw json.RawMessage) []string {
	var xs []json.RawMessage
	if json.Unmarshal(raw, &xs) != nil {
		return nil
	}
	var out []string
	for _, x := range xs {
		var s string
		if json.Unmarshal(x, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}

// Thread and Comment are the fields of the V1 playback data filtering reads.
type Thread struct {
	ID       json.RawMessage `json:"id"`
	Fork     string          `json:"fork"`
	Comments []Comment       `json:"comments"`
}

type Comment struct {
	ID       string   `json:"id"`
	No       int64    `json:"no"`
	VposMs   int64    `json:"vposMs"`
	Body     string   `json:"body"`
	Commands []string `json:"commands"`
	UserID   string   `json:"userId"`
	Score    float64  `json:"score"`
	PostedAt string   `json:"postedAt"`
}

// Load reads the V1 playback data (a threads array).
func Load(playback []byte) ([]Thread, error) {
	var ts []Thread
	return ts, json.Unmarshal(playback, &ts)
}

// Stats is what filtering hid, the shape the page shows (FilterStats).
type Stats struct {
	Total  int                  `json:"total"`
	Shown  int                  `json:"shown"`
	ByRule map[string]int       `json:"byRule"`
	ByFork map[string]*ForkStat `json:"byFork"`
	// BadPatterns are NG word entries that are not valid regular expressions.
	BadPatterns []string `json:"badPatterns"`
	// Cap is the total budget in effect (0 none) and how many were eligible.
	Cap struct {
		Limit    int `json:"limit"`
		Eligible int `json:"eligible"`
	} `json:"cap"`
}

type ForkStat struct {
	Total int `json:"total"`
	Shown int `json:"shown"`
}

// Result is which comments are shown: Keep[i] lists the positions, in
// thread i's comments, of those kept.
type Result struct {
	Keep  [][]int `json:"keep"`
	Stats Stats   `json:"stats"`
}

// Niconico's shared-NG levels hide comments whose score is at or below the
// threshold. Values from public documentation; not yet checked against the
// live site.
var shareThreshold = map[string]float64{"weak": -10000, "medium": -4800, "strong": -1000}

var caCommands = map[string]bool{"ca": true, "patissier": true, "ender": true, "full": true}

func lineBreaks(body string) int {
	return strings.Count(strings.ReplaceAll(body, "\r\n", "\n"), "\n") + strings.Count(strings.ReplaceAll(body, "\r\n", ""), "\r")
}

// artRecogniser recognises comment art as the renderer does (niconicomments
// changeCALayer/getUsersScore): an account scores 5 for each comment with
// ca, patissier, ender or full, and half the line count for each comment of
// more than two lines; at 10 or more all its comments are art. A comment
// with one of those commands, or of more than two lines, is art by itself.
func artRecogniser(threads []Thread) func(fork string, c *Comment) bool {
	score := map[string]float64{}
	for _, t := range threads {
		for i := range t.Comments {
			c := &t.Comments[i]
			add := 0.0
			for _, x := range c.Commands {
				if caCommands[x] {
					add += 5
					break
				}
			}
			if n := lineBreaks(c.Body); n > 2 {
				add += float64(n) / 2
			}
			if add != 0 {
				score[c.UserID] += add
			}
		}
	}
	return func(fork string, c *Comment) bool {
		if fork == "owner" {
			return false
		}
		if score[c.UserID] >= 10 || lineBreaks(c.Body) > 2 {
			return true
		}
		for _, x := range c.Commands {
			if caCommands[strings.ToLower(x)] {
				return true
			}
		}
		return false
	}
}

// capLimit is the total budget for a video of durationSec.
func capLimit(durationSec, perSecond float64) int {
	if perSecond <= 0 {
		return 0
	}
	return max(100, int(math.Floor(durationSec*perSecond)))
}

// fnv1a is uniform in [0, 1): FNV-1a over the string's UTF-16 code units
// (as the page computed it), then murmur3's fmix32 so that ids differing
// only in their last characters do not land next to each other.
func fnv1a(s string) float64 {
	h := uint32(0x811c9dc5)
	for _, u := range utf16.Encode([]rune(s)) {
		h ^= uint32(u)
		h *= 0x01000193
	}
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return float64(h) / 0x100000000
}

var colors = map[string]bool{}

func init() {
	for _, c := range strings.Fields("white red pink orange yellow green cyan blue purple black white2 niconicowhite red2 truered pink2 orange2 passionorange yellow2 madyellow green2 elementalgreen cyan2 blue2 marinblue purple2 nobleviolet black2") {
		colors[c] = true
	}
}

var hexColor = regexp.MustCompile(`(?i)^#[0-9a-f]{3,8}$`)

func isColored(cmd string) bool {
	return (colors[cmd] && cmd != "white") || hexColor.MatchString(cmd)
}

var slashed = regexp.MustCompile(`^/(.+)/([a-z]*)$`)

// compileWords turns NG words into matchers: /regex/flags (i, m, s; g, y
// and u mean nothing here) or substrings. Regular expressions are RE2's
// (no look-around or back-references); one that does not compile is
// reported, not used.
func compileWords(words []string, bad *[]string) []func(string) bool {
	var out []func(string) bool
	for _, w := range words {
		if w == "" {
			continue
		}
		if m := slashed.FindStringSubmatch(w); m != nil {
			flags := ""
			ok := true
			for _, f := range m[2] {
				switch f {
				case 'i', 'm', 's':
					if !strings.ContainsRune(flags, f) {
						flags += string(f)
					}
				case 'g', 'y', 'u':
				default:
					ok = false
				}
			}
			expr := m[1]
			if flags != "" {
				expr = "(?" + flags + ")" + expr
			}
			re, err := regexp.Compile(expr)
			if !ok || err != nil {
				*bad = append(*bad, w)
				continue
			}
			out = append(out, re.MatchString)
			continue
		}
		out = append(out, func(b string) bool { return strings.Contains(b, w) })
	}
	return out
}

// parseDate reads a date as the page's Date.parse did: a date alone is
// UTC midnight, a date and time without an offset is local time.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999Z07:00", "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	for _, l := range []string{"2006-01-02T15:04:05.999", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func isScript(c *Comment) bool {
	return strings.HasPrefix(c.Body, "@") || strings.HasPrefix(c.Body, "＠")
}

// Apply filters threads by s for a video of durationSec.
func Apply(threads []Thread, s Settings, durationSec float64) Result {
	st := Stats{ByRule: map[string]int{}, ByFork: map[string]*ForkStat{}, BadPatterns: []string{}}
	words := compileWords(s.NgWords, &st.BadPatterns)
	users := map[string]bool{}
	for _, u := range s.NgUsers {
		if u != "" {
			users[u] = true
		}
	}
	ngCmds := map[string]bool{}
	for _, x := range s.NgCommands {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			ngCmds[x] = true
		}
	}
	cutoff, hasCutoff := time.Time{}, false
	if s.PostedBefore != "" {
		cutoff, hasCutoff = parseDate(s.PostedBefore)
	}
	shareMax, hasShare := shareThreshold[s.NgShare]
	isArt := artRecogniser(threads)
	h := s.Hide

	rule := func(fork string, c *Comment) string {
		if shown, ok := s.Forks[fork]; ok && !shown {
			return "fork"
		}
		if fork == "owner" {
			if isScript(c) && !s.RunScripts {
				return "script"
			}
			return "" // viewer filters never touch owner comments, as on the site
		}
		if hasCutoff {
			if t, ok := parseDate(c.PostedAt); ok && t.After(cutoff) {
				return "time"
			}
		}
		if hasShare && c.Score <= shareMax {
			return "ngShare"
		}
		if users[c.UserID] {
			return "user"
		}
		for _, m := range words {
			if m(c.Body) {
				return "word"
			}
		}
		cmds := make([]string, len(c.Commands))
		has := map[string]bool{}
		for i, x := range c.Commands {
			cmds[i] = strings.ToLower(x)
			has[cmds[i]] = true
		}
		for _, x := range cmds {
			if ngCmds[x] {
				return "command"
			}
		}
		ue, shita := has["ue"], has["shita"]
		switch {
		case h.Ue && ue:
			return "ue"
		case h.Shita && shita:
			return "shita"
		case h.Naka && !ue && !shita:
			return "naka"
		}
		if h.Colored {
			for _, x := range cmds {
				if isColored(x) {
					return "colored"
				}
			}
		}
		if h.Big && has["big"] {
			return "big"
		}
		if h.Small && has["small"] {
			return "small"
		}
		ca := isArt(fork, c)
		if h.CA && ca {
			return "ca"
		}
		if h.Anonymous && has["184"] {
			return "anonymous"
		}
		if s.MaxLength > 0 && !ca && utf8.RuneCountInString(c.Body) > s.MaxLength {
			return "length"
		}
		return ""
	}
	exempt := func(fork string, c *Comment) bool { return fork == "owner" || isArt(fork, c) }

	// Pass 1: the rules.
	kept := make([][]int, len(threads))
	eligible, exemptCount := 0, 0
	for ti, t := range threads {
		fs := st.ByFork[t.Fork]
		if fs == nil {
			fs = &ForkStat{}
			st.ByFork[t.Fork] = fs
		}
		kept[ti] = []int{}
		for ci := range t.Comments {
			c := &threads[ti].Comments[ci]
			st.Total++
			fs.Total++
			if r := rule(t.Fork, c); r != "" {
				st.ByRule[r]++
				continue
			}
			kept[ti] = append(kept[ti], ci)
			if exempt(t.Fork, c) {
				exemptCount++
			} else {
				eligible++
			}
		}
	}
	// Pass 2: the total budget, spent only on comments that survived.
	limit := capLimit(durationSec, s.CapPerSecond)
	room := max(0, limit-exemptCount)
	p := 1.0
	if limit > 0 && eligible > room {
		p = float64(room) / float64(eligible)
	}
	st.Cap.Limit, st.Cap.Eligible = limit, eligible
	for ti, t := range threads {
		if p < 1 {
			final := []int{}
			for _, ci := range kept[ti] {
				c := &threads[ti].Comments[ci]
				key := c.ID
				if key == "" {
					key = strconv.FormatInt(c.No, 10)
				}
				if exempt(t.Fork, c) || fnv1a(jsString(t.ID)+"/"+t.Fork+"/"+key) < p {
					final = append(final, ci)
				} else {
					st.ByRule["cap"]++
				}
			}
			kept[ti] = final
		}
		st.Shown += len(kept[ti])
		st.ByFork[t.Fork].Shown += len(kept[ti])
	}
	return Result{Keep: kept, Stats: st}
}

// jsString is a thread id as JavaScript's String() gives it: a string as
// it is, a number in its shortest form.
func jsString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if len(raw) == 0 {
		return "undefined"
	}
	return string(raw)
}
