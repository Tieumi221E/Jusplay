package subs

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf16"
)

// The cases of the page's former parser (web/test/subformats.test.ts),
// with the same fixtures and results.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "subs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func same(t *testing.T, got, want []Line) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %#v\nwant %#v", got, want)
	}
}

func TestSRT(t *testing.T) {
	same(t, ParseFile(fixture(t, "tiny.srt"), "srt"), []Line{
		{Start: 0.1, End: 0.45, Text: "第一行字幕\n斜体"},
		{Start: 0.5, End: 0.9, Text: "Second line & more"},
	})
}

func TestASS(t *testing.T) {
	same(t, ParseFile(fixture(t, "tiny.ass"), "ass"), []Line{
		{Start: 0.1, End: 0.45, Text: "こんにちは\n二行目"},
		{Start: 0.5, End: 0.9, Text: "上に出る行", Top: true},
	})
}

func TestASSCommasDrawingsComments(t *testing.T) {
	src := "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,はい、そうです, まあ\n" +
		"Comment: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,note\n" +
		"Dialogue: 0,0:00:03.00,0:00:04.00,Sign,,0,0,0,,{\\p1}m 0 0 l 100 0 100 100\n"
	same(t, ParseASS(src), []Line{{Start: 1, End: 2.5, Text: "はい、そうです, まあ"}})
}

func TestWebVTT(t *testing.T) {
	src := "WEBVTT\n\nNOTE made by hand\n\nid1\n00:01.000 --> 00:02.000 align:start\n<v Anna>Hi <b>there</b>\n\n01:00:00.500 --> 01:00:01.000\nlate\n"
	same(t, ParseSRT(src), []Line{{Start: 1, End: 2, Text: "Hi there"}, {Start: 3600.5, End: 3601, Text: "late"}})
}

func TestEncodings(t *testing.T) {
	ja := "こんにちは、世界。今日は晴れです。"
	if got := DecodeText(append([]byte{0xef, 0xbb, 0xbf}, ja...)); got != ja {
		t.Errorf("UTF-8 BOM: %q", got)
	}
	u := utf16.Encode([]rune(ja))
	le := []byte{0xff, 0xfe}
	be := []byte{0xfe, 0xff}
	for _, c := range u {
		le = append(le, byte(c), byte(c>>8))
		be = append(be, byte(c>>8), byte(c))
	}
	if got := DecodeText(le); got != ja {
		t.Errorf("UTF-16LE: %q", got)
	}
	if got := DecodeText(be); got != ja {
		t.Errorf("UTF-16BE: %q", got)
	}
	// Legacy bytes, made with iconv.
	for want, h := range map[string]string{
		"こんにちは、":  "82b182f182c982bf82cd8141",     // Shift-JIS
		"今天天气很好，": "bdf1ccecccecc6f8badcbac3a3ac", // GBK
		"今天天氣很好，": "a4b5a4d1a4d1aef0abdca66ea141", // Big5
	} {
		b, _ := hex.DecodeString(h)
		if got := DecodeText(b); got != want {
			t.Errorf("%s: got %q", h, got)
		}
	}
}

func TestEmbedded(t *testing.T) {
	same(t, FromEmbedded("S_TEXT/ASS", []Event{
		{Start: 1, End: 2, Data: "0,0,Default,,0,0,0,,{\\i1}一行目{\\i0}\\N二行目"},
		{Start: 5, End: 5, Data: "1,0,Sign,,0,0,0,,{\\an8}看板, 標識"},
		{Start: 7, End: 7, Data: "2,0,Default,,0,0,0,,"},
	}), []Line{
		{Start: 1, End: 2, Text: "一行目\n二行目"},
		{Start: 5, End: 7, Text: "看板, 標識", Top: true},
	})
	same(t, FromEmbedded("S_TEXT/UTF8", []Event{{Start: 0, End: 1, Data: "<i>x</i>"}}), []Line{{Start: 0, End: 1, Text: "x"}})
}

func TestBetween(t *testing.T) {
	ls := []Line{{Start: 0, End: 2}, {Start: 3, End: 5}, {Start: 6, End: 9}}
	if got := Between(ls, 4, 6); len(got) != 1 || got[0].Start != 3 {
		t.Fatalf("%v", got)
	}
	if got := Between(ls, 1, 0); len(got) != 3 {
		t.Fatalf("to the end: %v", got)
	}
}
