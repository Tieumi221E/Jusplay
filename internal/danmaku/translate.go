package danmaku

import (
	"strings"
	"unicode/utf8"
)

// laughTails are endings that carry no meaning to translate; they are cut
// off before translating and put back after ("かわいいww" → "可爱ww").
var laughTails = []string{"(笑)", "ww", "w", "草", "笑"}

// TranslationCore splits a phrase key into what needs translating and what
// does not: the laughter at its end (tail) and a doubling of the whole line
// ("かわいいかわいい" → "かわいい" said twice). A translation of core is
// rendered with Render.
func TranslationCore(key string) (core, tail string, repeat int) {
	core, repeat = key, 1
	for changed := true; changed; {
		changed = false
		for _, t := range laughTails {
			if len(core) > len(t) && strings.HasSuffix(core, t) {
				core, tail = strings.TrimRight(core[:len(core)-len(t)], " "), t+tail
				changed = true
			}
		}
	}
	if n := utf8.RuneCountInString(core); n >= 4 && n%2 == 0 {
		rs := []rune(core)
		if string(rs[:n/2]) == string(rs[n/2:]) {
			core, repeat = string(rs[:n/2]), 2
		}
	}
	return core, tail, repeat
}

// Render puts a translated core back together with its tail and doubling.
func Render(tr, tail string, repeat int) string {
	return strings.Repeat(tr, max(1, repeat)) + tail
}

// ShapeKey is the form under which writings of one phrase share a
// translation: katakana as hiragana, small kana as large, long vowel marks
// gone ("カワイイ", "かわいー" and "かわいい" are one).
func ShapeKey(core string) string { return shapeFold(core) }

// Reactions translate the most common short reactions without a model, by
// target language, keyed by ShapeKey. Written for comments: short, spoken,
// in the tone a viewer would use.
var Reactions = map[string]map[string]string{
	"zh": {
		"かわいい": "可爱", "かわいそう": "好可怜", "かっこいい": "好帅", "すごい": "好厉害", "すげ": "好厉害", "すげえ": "好厉害",
		"きた": "来了", "きたきた": "来了来了", "くる": "要来了", "くるぞ": "要来了", "でた": "出现了", "うわでた": "哇，出现了",
		"それな": "确实", "それはそう": "确实如此", "わかる": "懂", "せやな": "是啊", "たしかに": "确实", "ですよね": "就是说啊", "なるほど": "原来如此",
		"うぽつ": "UP主辛苦了", "おつ": "辛苦了", "おつかれ": "辛苦了", "おかえり": "欢迎回来", "ただいま": "我回来了",
		"ありがとう": "谢谢", "ありがとうございます": "谢谢", "ありがとうございました": "谢谢", "おめでとう": "恭喜", "おめでとうございます": "恭喜",
		"ここすき": "喜欢这里", "すき": "喜欢", "だいすき": "最喜欢了",
		"え": "诶", "えっ": "诶？", "えぇ": "诶……", "は?": "哈？", "はぁ?": "哈？", "うそ": "骗人", "まじ": "真的假的", "まじか": "真的假的",
		"ほんとに": "真的吗", "なんで": "为什么", "どうして": "为什么", "やめろ": "住手", "やめて": "别这样", "やばい": "不妙", "やば": "不妙",
		"こわい": "好可怕", "こわ": "好可怕", "つらい": "好难受", "なける": "要哭了", "ないた": "哭了", "かなしい": "好难过",
		"うわ": "哇", "うわあ": "哇啊", "あ": "啊", "あっ": "啊", "ああ": "啊啊", "おお": "哦哦", "わあ": "哇", "あーあ": "唉",
		"いいね": "不错", "よかった": "太好了", "よし": "好", "やった": "太好了", "やったぜ": "好耶",
		"きれい": "好美", "うつくしい": "好美", "えらい": "了不起", "つよい": "好强", "よわい": "好弱", "でかい": "好大",
		"しってた": "早就知道了", "まって": "等等", "ちょっとまって": "等一下", "おい": "喂", "ひどい": "好过分",
		"いけ": "上啊", "がんばれ": "加油", "がんばって": "加油", "たすけて": "救命", "ねむい": "好困", "おいしそう": "看起来好好吃",
	},
}

// The table is written in natural spellings; lookups use ShapeKey.
func init() {
	for lang, m := range Reactions {
		folded := make(map[string]string, len(m))
		for k, v := range m {
			folded[shapeFold(k)] = v
		}
		Reactions[lang] = folded
	}
}
