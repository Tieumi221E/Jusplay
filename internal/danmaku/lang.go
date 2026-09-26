package danmaku

import "unicode"

// Languages a phrase can be told apart as, by its script.
const (
	LangNone = "none" // no letters: "88", "!?", marks, digits, emoji
	LangJa   = "ja"   // kana
	LangZh   = "zh"   // Chinese characters with a Chinese-only one
	LangHan  = "han"  // Chinese characters only: Chinese or Japanese
	LangKo   = "ko"   // hangul
	LangEn   = "en"   // Latin letters
)

// zhOnly are common simplified characters Japanese does not use, and
// Chinese function words; jaOnly are characters only Japanese uses.
var zhOnly, jaOnly = runeSet("这们说为么个吗吧呢啊没对还让给过发会来时后头进经现样点实种话学问题见觉关开长东车书门马鸟龙电爱国听写买卖读谁怎哪很"),
	runeSet("々〆込畑峠枠働辻匂栃塀")

func runeSet(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, r := range s {
		m[r] = true
	}
	return m
}

// Lang tells a phrase's language by its script: kana is Japanese, hangul
// Korean, Latin letters English (or at least Latin); Chinese characters
// alone are Chinese when one is used only in Chinese, Japanese when one is
// used only in Japanese, and otherwise undecided (LangHan) — "草" and
// "最高" are both. Lines of just "w" are laughter, not English.
func Lang(key string) string {
	var kana, hangul, latin, han, zh, ja int
	onlyW := true
	for _, r := range key {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana) || r == 'ー':
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Han, r):
			han++
			if zhOnly[r] {
				zh++
			}
			if jaOnly[r] {
				ja++
			}
		case r < 0x250 && unicode.IsLetter(r):
			latin++
			if r != 'w' {
				onlyW = false
			}
		}
	}
	switch {
	case kana > 0 || ja > 0:
		return LangJa
	case hangul > 0:
		return LangKo
	case han > 0 && zh > 0:
		return LangZh
	case han > 0:
		return LangHan
	case latin > 0 && !onlyW:
		return LangEn
	}
	return LangNone
}
