package danmaku

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fold is a comment's text in one width and case: full-width ASCII to
// ASCII, half-width katakana to full-width (with their voicing marks
// joined), the ideographic space to a space, "…" to "..", letters lower
// case, runs of white space to one space. It is what Key and the n-gram
// statistics work on; the standard library has no Unicode normalisation,
// and only these foldings matter for comments.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	rs := []rune(s)
	space := false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			r -= 0xFF01 - 0x21
		case r == 0x3000 || r == '\n' || r == '\r' || r == '\t':
			r = ' '
		case r == '…':
			b.WriteString("..")
			space = false
			continue
		case r >= 0xFF61 && r <= 0xFF9F:
			k := halfKana[r-0xFF61]
			if i+1 < len(rs) && (rs[i+1] == 0xFF9E || rs[i+1] == 0xFF9F) {
				if v, ok := voiced(k, rs[i+1] == 0xFF9F); ok {
					k = v
					i++
				}
			}
			r = k
		}
		if r == ' ' {
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimRight(b.String(), " ")
}

// halfKana maps U+FF61–FF9F to full-width forms.
var halfKana = []rune("。「」、・ヲァィゥェォャュョッーアイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワン゛゜")

// voiced joins a katakana with a (semi-)voicing mark: カ+゛→ガ, ハ+゜→パ.
// A table, not arithmetic: ッ sits between チ and ツ and breaks the pattern.
func voiced(k rune, semi bool) (rune, bool) {
	m := voicedKana
	if semi {
		m = semiVoicedKana
	}
	v, ok := m[k]
	return v, ok
}

var voicedKana, semiVoicedKana = pairs("カキクケコサシスセソタチツテトハヒフヘホウ", "ガギグゲゴザジズゼゾダヂヅデドバビブベボヴ"), pairs("ハヒフヘホ", "パピプペポ")

func pairs(from, to string) map[rune]rune {
	f, t := []rune(from), []rune(to)
	m := make(map[rune]rune, len(f))
	for i := range f {
		m[f[i]] = t[i]
	}
	return m
}

// cutRepeats cuts a unit of two or three characters said three or more
// times running to two: "!?!?!?" → "!?!?", "wkwkwk" → "wkwk".
func cutRepeats(s string) string {
	rs := []rune(s)
	out := make([]rune, 0, len(rs))
	for i := 0; i < len(rs); {
		cut := false
		for u := 2; u <= 3 && !cut; u++ {
			n := 1
			for i+(n+1)*u <= len(rs) && slices.Equal(rs[i+n*u:i+(n+1)*u], rs[i:i+u]) {
				n++
			}
			if n >= 3 {
				out = append(out, rs[i:i+2*u]...)
				i += n * u
				cut = true
			}
		}
		if !cut {
			out = append(out, rs[i])
			i++
		}
	}
	return string(out)
}

// trailing are marks that vary between writings of the same line.
const trailing = "!?.,。、~〜ー・♪☆★♡♥❤ 　!！?？"

// Key is the phrase a comment says, so that its writings count together:
// folded, runs of three or more of a character cut to two ("wwww" → "ww",
// "8888" → "88", "ああああ" → "ああ"), and the marks at the end dropped
// ("行くよ〜" and "行くよ" are one phrase; "隠したのさ!!" and "…さ!"
// too). A comment that is only marks keeps them (cut to two): "!?" is a
// reaction of its own.
func Key(body string) string {
	f := Fold(body)
	if f == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(f))
	var prev rune
	run := 0
	for _, r := range f {
		if r == prev {
			run++
		} else {
			prev, run = r, 1
		}
		if run <= 2 {
			b.WriteRune(r)
		}
	}
	k := cutRepeats(b.String())
	if t := strings.TrimRight(k, trailing); t != "" {
		// A lone trailing mark after the cut ("は?" → "は") would merge a
		// question with a statement: keep "?" when the line is short.
		if last, _ := utf8.DecodeLastRuneInString(k); (last == '?' || last == '!') && utf8.RuneCountInString(t) <= 2 {
			return t + string(last)
		}
		return t
	}
	return k
}
