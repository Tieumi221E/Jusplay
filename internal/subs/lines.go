package subs

// Subtitle files and tracks as plain timed lines — the one reading of them,
// for the player page and the command line alike (it was the page's own,
// web/src/subformats.ts, until 0.4.0). Styling is not kept: Jusplay draws
// every subtitle in its own style; only what a line says, when, and whether
// it belongs at the top (ASS \an7–9, used for signs and notes) survive. ASS
// vector drawings are dropped.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Tieumi221E/Jus/textenc"
)

// Line is one subtitle line.
type Line struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	Top   bool    `json:"top,omitempty"`
}

// ---- text encoding ----

// DecodeText reads subtitle bytes in whatever encoding their maker used
// (the Jus module's textenc: UTF-8, UTF-16, Shift-JIS, GBK/GB18030, Big5).
func DecodeText(b []byte) string { return textenc.Decode(b) }

// ---- times ----

var timeRE = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{1,2})(?:[.,](\d{1,3}))?$`)

// parseTime reads "01:02:03,450", "1:02:03.45", "02:03.450" as seconds.
func parseTime(s string) (float64, bool) {
	m := timeRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	n := func(x string) float64 { f, _ := strconv.ParseFloat(x, 64); return f }
	t := n(m[1])*3600 + n(m[2])*60 + n(m[3])
	if m[4] != "" {
		f, _ := strconv.ParseFloat(m[4], 64)
		for range m[4] {
			f /= 10
		}
		t += f
	}
	return t, true
}

// ---- SRT and WebVTT ----

var (
	anTop     = regexp.MustCompile(`\{\\an[789]\}`)
	braceTags = regexp.MustCompile(`\{\\[^}]*\}`)
	angleTags = regexp.MustCompile(`<[^>]*>`)
	blankRuns = regexp.MustCompile(`\n{2,}`)
	entities  = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&nbsp;", " ")
)

// plain is SRT/VTT text without its markup: <i>, <font …>, <c.x>, <v Name>,
// <00:01.000>, {\an8}.
func plain(text string) (string, bool) {
	top := anTop.MatchString(text)
	t := angleTags.ReplaceAllString(braceTags.ReplaceAllString(text, ""), "")
	return strings.TrimSpace(entities.Replace(t)), top
}

func newlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// ParseSRT reads SRT, and WebVTT (the same blocks with a header, optional
// ids and NOTE/STYLE blocks).
func ParseSRT(src string) []Line {
	out := []Line{}
	for _, block := range blankRuns.Split(newlines(src), -1) {
		rows := strings.Split(block, "\n")
		i := -1
		for j, r := range rows {
			if strings.Contains(r, "-->") {
				i = j
				break
			}
		}
		if i < 0 {
			continue
		}
		a, b, _ := strings.Cut(rows[i], "-->")
		start, ok1 := parseTime(a)
		end, ok2 := parseTime(firstField(b))
		if !ok1 || !ok2 || !(end > start) {
			continue
		}
		text, top := plain(strings.Join(rows[i+1:], "\n"))
		if text != "" {
			out = append(out, Line{Start: start, End: end, Text: text, Top: top})
		}
	}
	sortLines(out)
	return out
}

func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// ---- ASS / SSA ----

var (
	assDrawing = regexp.MustCompile(`\{[^}]*\\p[1-9]`)
	assAnTop   = regexp.MustCompile(`\{[^}]*\\an[789]`)
	assATop    = regexp.MustCompile(`\{[^}]*\\a(?:5|6|7|9|10|11)(?:[^0-9]|$)`)
	assTags    = regexp.MustCompile(`\{[^}]*\}`)
	assBreaks  = strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ")
)

// assText is an ASS event's text: override tags dropped, line breaks kept;
// ok is false for drawings and empty events.
func assText(raw string) (text string, top, ok bool) {
	if assDrawing.MatchString(raw) {
		return "", false, false // vector drawing
	}
	top = assAnTop.MatchString(raw) || assATop.MatchString(raw)
	text = strings.TrimSpace(assBreaks.Replace(assTags.ReplaceAllString(raw, "")))
	return text, top, text != ""
}

// ParseASS reads a whole ASS/SSA script: the [Events] Dialogue lines.
func ParseASS(src string) []Line {
	out := []Line{}
	var format []string
	inEvents := false
	for _, row := range strings.Split(newlines(src), "\n") {
		r := strings.TrimSpace(row)
		low := strings.ToLower(r)
		if strings.HasPrefix(r, "[") {
			inEvents = low == "[events]"
			continue
		}
		if !inEvents {
			continue
		}
		if strings.HasPrefix(low, "format:") {
			format = nil
			for _, x := range strings.Split(r[7:], ",") {
				format = append(format, strings.ToLower(strings.TrimSpace(x)))
			}
			continue
		}
		if !strings.HasPrefix(low, "dialogue:") || format == nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(r[9:]), ",")
		n := len(format)
		var fields []string
		if len(parts) >= n {
			fields = append(append(fields, parts[:n-1]...), strings.Join(parts[n-1:], ","))
		} else {
			fields = parts
		}
		get := func(k string) string {
			for i, f := range format {
				if f == k && i < len(fields) {
					return fields[i]
				}
			}
			return ""
		}
		start, ok1 := parseTime(get("start"))
		end, ok2 := parseTime(get("end"))
		text, top, ok := assText(get("text"))
		if !ok1 || !ok2 || !(end > start) || !ok {
			continue
		}
		out = append(out, Line{Start: start, End: end, Text: text, Top: top})
	}
	sortLines(out)
	return out
}

// ParseFile reads a subtitle file's lines by its format ("srt", "vtt", "ass").
func ParseFile(b []byte, format string) []Line {
	src := DecodeText(b)
	if format == "ass" {
		return ParseASS(src)
	}
	return ParseSRT(src)
}

// ---- Matroska text tracks ----

// Event is one block of a Matroska text track (media.SubtitleEvent).
type Event struct {
	Start, End float64
	Data       string
}

// FromEmbedded reads a Matroska text track's events. In ASS/SSA tracks a
// block holds the event without its times: "ReadOrder,Layer,Style,Name,
// MarginL,MarginR,MarginV,Effect,Text" (SSA: "…,Marked,Style,…"); the text
// is what follows the eighth comma. An event without a length lasts until
// the next one (at most 10 s).
func FromEmbedded(codec string, events []Event) []Line {
	ass := codec == "S_TEXT/ASS" || codec == "S_TEXT/SSA"
	out := []Line{}
	for i, e := range events {
		var text string
		var top, ok bool
		if ass {
			parts := strings.Split(e.Data, ",")
			if len(parts) > 8 {
				text, top, ok = assText(strings.Join(parts[8:], ","))
			}
		} else {
			text, top = plain(e.Data)
			ok = text != ""
		}
		if !ok {
			continue
		}
		end := e.End
		if !(e.End > e.Start) {
			end = e.Start + 10
			if i+1 < len(events) && events[i+1].Start < end {
				end = events[i+1].Start
			}
		}
		out = append(out, Line{Start: e.Start, End: end, Text: text, Top: top})
	}
	sortLines(out)
	return out
}

// sortLines orders by start, keeping the order of lines that start together
// (as the page's stable sort did).
func sortLines(ls []Line) {
	sort.SliceStable(ls, func(i, j int) bool { return ls[i].Start < ls[j].Start })
}

// Between keeps the lines shown at some moment in [from, to) (to 0: to the end).
func Between(ls []Line, from, to float64) []Line {
	out := []Line{}
	for _, l := range ls {
		if l.End > from && (to <= 0 || l.Start < to) {
			out = append(out, l)
		}
	}
	return out
}
