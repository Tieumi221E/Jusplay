// Package danmaku analyses a video's comments: when they come, what they
// say, how they react, what repeats. It is meant to scale well past one
// episode — a series, a library, millions of comments — so the comments are
// read as a stream into a columnar corpus (one slice per field, strings
// interned), and every analysis runs in bounded memory: frequent phrases by
// Space-Saving counting, near-duplicates by SimHash buckets rather than
// pairwise comparison, peaks by robust statistics.
//
// Nothing here needs a model or a dictionary. Comments are mostly short
// reactions and in-jokes that a dictionary tokenizer cuts wrongly, so words
// are discovered from the data (cohesion and boundary entropy of n-grams).
package danmaku

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Flag bits of a comment.
const (
	Anonymous uint16 = 1 << iota // 184
	Premium
	Owner   // the owner fork
	Colored // a colour command
	Big     // big
	Small   // small
	Top     // ue
	Bottom  // shita
	Art     // comment art (see markArt)
	Switch  // posted from a Nintendo Switch (device:Switch)
	Multiline
	ArtCommand // ca, patissier, ender or full
)

// Corpus is comments column by column: comment i is Vpos[i], Posted[i], …
// Bodies, phrase keys, users and forks are interned; the columns hold ids.
type Corpus struct {
	Vpos   []int32  // ms into the video
	Posted []int64  // Unix seconds (0 when unknown)
	Body   []uint32 // index into Bodies
	Key    []uint32 // index into Keys (the normalised phrase, see Key)
	User   []uint32 // index into Users
	Fork   []uint8  // index into Forks
	Flags  []uint16
	Nicoru []uint32
	Score  []int32

	Bodies, Keys, Users, Forks []string
	KeyCount                   []uint32 // comments per key

	bodyID, keyID, userID map[string]uint32
	forkID                map[string]uint8
}

// NewCorpus returns an empty corpus.
func NewCorpus() *Corpus {
	return &Corpus{bodyID: map[string]uint32{}, keyID: map[string]uint32{}, userID: map[string]uint32{}, forkID: map[string]uint8{}}
}

// Len is the number of comments.
func (c *Corpus) Len() int { return len(c.Vpos) }

func intern(m map[string]uint32, list *[]string, s string) uint32 {
	if id, ok := m[s]; ok {
		return id
	}
	id := uint32(len(*list))
	m[s] = id
	*list = append(*list, s)
	return id
}

// Comment is one comment as the V1 format has it (the fields used here).
type Comment struct {
	VposMs      int64    `json:"vposMs"`
	Body        string   `json:"body"`
	Commands    []string `json:"commands"`
	UserID      string   `json:"userId"`
	IsPremium   bool     `json:"isPremium"`
	Score       int64    `json:"score"`
	PostedAt    string   `json:"postedAt"`
	NicoruCount int64    `json:"nicoruCount"`
}

// Add appends one comment of fork.
func (c *Corpus) Add(fork string, cm *Comment) {
	f, ok := c.forkID[fork]
	if !ok {
		f = uint8(len(c.Forks))
		c.forkID[fork] = f
		c.Forks = append(c.Forks, fork)
	}
	var flags uint16
	if fork == "owner" {
		flags |= Owner
	}
	if cm.IsPremium {
		flags |= Premium
	}
	for _, x := range cm.Commands {
		switch strings.ToLower(x) {
		case "184":
			flags |= Anonymous
		case "big":
			flags |= Big
		case "small":
			flags |= Small
		case "ue":
			flags |= Top
		case "shita":
			flags |= Bottom
		case "device:switch":
			flags |= Switch
		case "ca", "patissier", "ender", "full":
			flags |= ArtCommand
		case "white", "default":
		default:
			if isColor(x) {
				flags |= Colored
			}
		}
	}
	if strings.Count(cm.Body, "\n") > 2 {
		flags |= Multiline
	}
	var posted int64
	if t, err := time.Parse(time.RFC3339, cm.PostedAt); err == nil {
		posted = t.Unix()
	}
	key := intern(c.keyID, &c.Keys, Key(cm.Body))
	if int(key) == len(c.KeyCount) {
		c.KeyCount = append(c.KeyCount, 0)
	}
	c.KeyCount[key]++
	c.Vpos = append(c.Vpos, int32(max(0, min(cm.VposMs, 1<<31-1))))
	c.Posted = append(c.Posted, posted)
	c.Body = append(c.Body, intern(c.bodyID, &c.Bodies, cm.Body))
	c.Key = append(c.Key, key)
	c.User = append(c.User, intern(c.userID, &c.Users, cm.UserID))
	c.Fork = append(c.Fork, f)
	c.Flags = append(c.Flags, flags)
	c.Nicoru = append(c.Nicoru, uint32(max(0, min(cm.NicoruCount, 1<<32-1))))
	c.Score = append(c.Score, int32(max(-1<<31, min(cm.Score, 1<<31-1))))
}

var colors = map[string]bool{"red": true, "pink": true, "orange": true, "yellow": true, "green": true, "cyan": true, "blue": true,
	"purple": true, "black": true, "white2": true, "niconicowhite": true, "red2": true, "truered": true, "pink2": true, "orange2": true,
	"passionorange": true, "yellow2": true, "madyellow": true, "green2": true, "elementalgreen": true, "cyan2": true, "blue2": true,
	"marinblue": true, "purple2": true, "nobleviolet": true, "black2": true}

func isColor(x string) bool {
	x = strings.ToLower(x)
	if colors[x] {
		return true
	}
	if len(x) == 7 && x[0] == '#' {
		return true
	}
	return false
}

// Load reads V1 threads (a JSON array of {fork, comments: […]}, as the
// player serves them) as a stream: one comment decoded at a time, so the
// raw text is never held whole. Comments without a body are skipped.
func Load(r io.Reader) (*Corpus, error) {
	c := NewCorpus()
	d := json.NewDecoder(r)
	if err := expect(d, json.Delim('[')); err != nil {
		return nil, err
	}
	for d.More() {
		if err := expect(d, json.Delim('{')); err != nil {
			return nil, err
		}
		fork := ""
		var pending []Comment // comments seen before "fork" (keys may come in any order)
		for d.More() {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t {
			case "fork":
				if err := d.Decode(&fork); err != nil {
					return nil, err
				}
				for i := range pending {
					c.Add(fork, &pending[i])
				}
				pending = nil
			case "comments":
				if err := expect(d, json.Delim('[')); err != nil {
					return nil, err
				}
				var cm Comment
				for d.More() {
					cm = Comment{}
					if err := d.Decode(&cm); err != nil {
						return nil, fmt.Errorf("comment %d: %w", c.Len(), err)
					}
					if cm.Body == "" {
						continue
					}
					if fork == "" {
						pending = append(pending, cm)
					} else {
						c.Add(fork, &cm)
					}
				}
				if err := expect(d, json.Delim(']')); err != nil {
					return nil, err
				}
			default:
				var skip json.RawMessage
				if err := d.Decode(&skip); err != nil {
					return nil, err
				}
			}
		}
		for i := range pending {
			c.Add(fork, &pending[i])
		}
		if err := expect(d, json.Delim('}')); err != nil {
			return nil, err
		}
	}
	c.markArt()
	return c, nil
}

func expect(d *json.Decoder, want json.Delim) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t != want {
		return fmt.Errorf("expected %v, found %v", want, t)
	}
	return nil
}

// markArt flags comment art the way the player recognises it
// (web/src/filter.ts artRecogniser): an account scoring 10 or more — 5 per
// ca/patissier/ender/full command, half the line count per comment of more
// than two lines — has all its comments counted as art, and so does any
// comment with such a command or more than two lines. Owner comments never.
func (c *Corpus) markArt() {
	score := map[uint32]float64{}
	for i, f := range c.Flags {
		if f&ArtCommand != 0 {
			score[c.User[i]] += 5
		}
		if f&Multiline != 0 {
			score[c.User[i]] += float64(strings.Count(c.Bodies[c.Body[i]], "\n")) / 2
		}
	}
	for i, f := range c.Flags {
		if f&Owner == 0 && (f&(Multiline|ArtCommand) != 0 || score[c.User[i]] >= 10) {
			c.Flags[i] |= Art
		}
	}
}
