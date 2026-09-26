package ai

import "testing"

func TestParseASR(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Transcription
	}{
		{"language Japanese<asr_text>こんにちは。", Transcription{"Japanese", "こんにちは。"}},
		{"language None<asr_text>", Transcription{}},
		{"language Chinese<asr_text>  ", Transcription{}},
		{"plain text", Transcription{"", "plain text"}},
	} {
		if got := parseASR(c.in); got != c.want {
			t.Errorf("parseASR(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestWavHeader(t *testing.T) {
	b := wav([]int16{1, -1})
	if len(b) != 48 || string(b[:4]) != "RIFF" || string(b[36:40]) != "data" || b[40] != 4 {
		t.Errorf("header % x", b[:44])
	}
}
