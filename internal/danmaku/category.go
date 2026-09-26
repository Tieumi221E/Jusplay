package danmaku

import "strings"

// Category is a kind of reaction. Comments are short and conventional, so
// a small rule list, checked in order (specific before broad), covers the
// common reactions and can be read and argued with. It does not know a
// series' own catchphrases or ordinary sentences: those are Other, and how
// many is reported, not hidden (Report.Categories). On two real episodes
// the rules covered about a quarter and a sixth of the comments; the rest
// is left to the phrase, term and family analyses (and, later, a model).
type Category uint8

const (
	Other      Category = iota
	Laugh               // 草, w, 笑
	Applause            // 88, 拍手, おめでとう
	Cute                // かわいい, 尊い
	Praise              // すごい, 神回, 最高, ここすき, きれい
	Surprise            // !?, は?, え, うわ, でた
	Sad                 // 泣, つらい, かわいそう
	Tsukkomi            // なんで, そうはならん, おかしい
	Agree               // それな, それはそう, わかる
	Ritual              // うぽつ, 初見, おかえり, 見納め, 時報
	Meme                // a line many repeat that is no common reaction (Analyze)
	Mention             // names a term the analysis found: a character, a place (Analyze)
	Commentary          // a sentence of its own (Analyze)
	ArtCat              // comment art
	OwnerCat            // the owner's comments
	numCategories
)

// CategoryNames are the categories' identifiers in reports.
var CategoryNames = [numCategories]string{"other", "laugh", "applause", "cute", "praise", "surprise", "sad", "tsukkomi", "agree", "ritual",
	"meme", "mention", "commentary", "art", "owner"}

// rule matches a phrase key (Key: folded, runs cut to two, end marks
// dropped) by literals: the whole key, its start, its end, or anywhere in
// it. Literals, not regular expressions: a few string tests per rule, fast
// over millions of distinct phrases (Go's regexp took 228 ms for 13 000
// phrases), and a literal says exactly what it catches. Unanchored "おい"
// and "きゃわ" matched inside unrelated words; such short ones are anchored.
type rule struct {
	cat                        Category
	exact, prefix, suffix, any []string
}

var rules = []rule{
	{cat: Ritual, exact: []string{"うぽつ", "upつ", "初見", "おかえり", "ただいま", "見納め", "おつ", "乙", "おつかれ", "時報", "いってらっしゃい", "ありがとうございました"},
		prefix: []string{"うぽつ", "初見です", "見納め"}},
	{cat: Applause, exact: []string{"88", "8", "パチパチ", "ぱちぱち", "拍手", "👏", "👏👏"}, any: []string{"おめでと", "888", "88 88"}},
	{cat: Laugh, exact: []string{"w", "ww", "草", "草草", "笑", "わろ", "ワロ", "wkwk", "lol"},
		suffix: []string{"ww", "草", "(笑)", "笑"}, any: []string{"ワロタ", "わろた", "大草原", "爆笑", "くそわろ", "草生える", "草不可避"}},
	// Sad before Cute: かわいそう (pitiful) contains かわい.
	{cat: Sad, any: []string{"泣", "涙", ";;", ";_;", "つらい", "辛い", "かなしい", "悲し", "切ない", "せつない", "かわいそう", "可哀想", "可哀そう", "感動", "うるっと", "しんどい"}},
	{cat: Cute, exact: []string{"かわ", "かわい", "cute"}, any: []string{"かわいい", "かわいー", "可愛", "かわよ", "カワイイ", "かあいい", "尊い", "てぇてぇ", "キュート"}},
	{cat: Agree, exact: []string{"それな", "それはそう", "わかる", "せやな", "たしかに", "確かに", "ほんとそれ", "それ", "それよ", "ですよね", "ほんまそれ", "同意"},
		prefix: []string{"それはそう", "わかる", "せやな"}},
	{cat: Surprise, exact: []string{"!?", "?!", "!!", "え", "えっ", "えぇ", "ええ", "は?", "はぁ?", "うそ", "嘘", "マジ", "まじ", "ファッ", "なに", "何", "ん?", "あ", "あっ", "おお", "おぉ", "うわ", "うわぁ", "うわあ", "でた", "出た", "!?!?", "え?", "えっ?"},
		suffix: []string{"!?", "?!", "でた", "出た"}, prefix: []string{"うわ", "まじか", "マジか", "嘘だろ", "うそだろ"}},
	{cat: Praise, exact: []string{"神", "すき", "好き", "つよい", "強い", "えらい", "偉い", "nice", "good", "gj", "いいね", "天才", "有能"},
		prefix: []string{"神回", "神作画", "神曲", "神演出"},
		any:    []string{"すごい", "すげ", "凄", "最高", "天才", "上手", "うまい", "美し", "うつくし", "きれい", "綺麗", "ここすき", "かっこいい", "カッコイイ", "かっけ", "格好いい", "イケメン", "有能", "良い", "いいぞ"}},
	{cat: Tsukkomi, prefix: []string{"おい", "なんで", "なぜ", "何故", "やめろ"},
		any:    []string{"そうはならん", "おかしい", "何これ", "なにこれ", "なんだこれ", "どういうこと", "意味わからん", "ひどい", "酷い", "なんでや"},
		suffix: []string{"?"}},
}

// Categorize returns a comment's category from its key and flags.
func Categorize(key string, flags uint16) Category {
	if flags&Owner != 0 {
		return OwnerCat
	}
	if flags&Art != 0 {
		return ArtCat
	}
	k := strings.TrimSpace(key)
	for i := range rules {
		if rules[i].match(k) {
			return rules[i].cat
		}
	}
	return Other
}

func (r *rule) match(k string) bool {
	for _, x := range r.exact {
		if k == x {
			return true
		}
	}
	for _, x := range r.prefix {
		if strings.HasPrefix(k, x) {
			return true
		}
	}
	for _, x := range r.suffix {
		if strings.HasSuffix(k, x) {
			return true
		}
	}
	for _, x := range r.any {
		if strings.Contains(k, x) {
			return true
		}
	}
	return false
}
