// Package legacyxml reads Niconico-style <packet><chat/></packet> XML
// exports and converts each <chat> to a V1 comment. The conversion is
// lossy by construction; every value the XML does not carry is either
// reported as assumed or given a neutral placeholder, never presented as a
// source fact.
package legacyxml

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Chat is one <chat> element with its attributes in document order.
type Chat struct {
	Attrs []xml.Attr
	Body  string
}

func (c Chat) Attr(name string) (string, bool) {
	for _, a := range c.Attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// Doc is a parsed XML export.
type Doc struct {
	Chats []Chat
	// Other counts non-chat child elements of <packet> by name.
	Other map[string]int
	// Leaf maps thread id -> count attribute of <leaf> / num_res of
	// <global_num_res>, as reported by the export (not verified).
	Leaf map[string]int64
}

// Parse reads the export strictly. Unknown elements are counted, nested
// elements inside <chat> are an error.
func Parse(b []byte) (*Doc, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	doc := &Doc{Other: map[string]int{}, Leaf: map[string]int64{}}
	depth := 0
	var cur *Chat
	var body bytes.Buffer
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case depth == 1:
				if t.Name.Local != "packet" {
					return nil, fmt.Errorf("root element is <%s>, want <packet>", t.Name.Local)
				}
			case depth == 2 && t.Name.Local == "chat":
				cur = &Chat{Attrs: append([]xml.Attr(nil), t.Attr...)}
				body.Reset()
			case depth == 2:
				doc.Other[t.Name.Local]++
				var thread, count string
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "thread":
						thread = a.Value
					case "count", "num_res":
						count = a.Value
					}
				}
				if (t.Name.Local == "leaf" || t.Name.Local == "global_num_res") && thread != "" {
					if n, err := strconv.ParseInt(count, 10, 64); err == nil {
						if _, seen := doc.Leaf[thread]; !seen || t.Name.Local == "global_num_res" {
							doc.Leaf[thread] = n
						}
					}
				}
			case cur != nil:
				return nil, fmt.Errorf("chat %d: nested <%s>", len(doc.Chats)+1, t.Name.Local)
			}
		case xml.CharData:
			if cur != nil {
				body.Write(t)
			}
		case xml.EndElement:
			if depth == 2 && cur != nil {
				cur.Body = body.String()
				doc.Chats = append(doc.Chats, *cur)
				cur = nil
			}
			depth--
		}
	}
	return doc, nil
}

// V1Comment mirrors the V1 comment fields in API order.
type V1Comment struct {
	ID          string   `json:"id"`
	No          int64    `json:"no"`
	VposMs      int64    `json:"vposMs"`
	Body        string   `json:"body"`
	Commands    []string `json:"commands"`
	UserID      string   `json:"userId"`
	IsPremium   bool     `json:"isPremium"`
	Score       int64    `json:"score"`
	PostedAt    string   `json:"postedAt"`
	NicoruCount int64    `json:"nicoruCount"`
	NicoruID    *string  `json:"nicoruId"`
	Source      string   `json:"source"`
	IsMyPost    bool     `json:"isMyPost"`
}

// Source marks every comment converted from XML.
const Source = "jusplay:legacy-xml"

// ForkSource says how a chat's fork was determined.
type ForkSource string

const (
	ForkFromAttr    ForkSource = "attr"      // fork="1" or other explicit value
	ForkNoUserID    ForkSource = "no-userid" // no user_id: owner, as niconicomments assumes
	ForkAssumedMain ForkSource = "assumed"   // nothing recorded: main
)

var knownAttrs = map[string]bool{
	"thread": true, "no": true, "vpos": true, "date": true, "date_usec": true,
	"premium": true, "anonymity": true, "user_id": true, "mail": true,
	"fork": true, "score": true, "nicoru": true, "deleted": true, "leaf": true,
}

var jst = time.FixedZone("JST", 9*60*60)

// Converted is one chat as a V1 comment plus conversion facts.
type Converted struct {
	ThreadID     string
	Fork         string
	ForkSource   ForkSource
	Comment      V1Comment
	Deleted      bool
	UnknownAttrs []string
}

// Convert maps a chat to V1. Missing no/vpos or malformed numbers are
// errors: they cannot be placed on the timeline without guessing.
func Convert(c Chat) (Converted, error) {
	var out Converted
	get := func(n string) string { v, _ := c.Attr(n); return v }
	intAttr := func(n string, required bool, min, max int64) (int64, error) {
		v, ok := c.Attr(n)
		if !ok || v == "" {
			if required {
				return 0, fmt.Errorf("missing %s", n)
			}
			return 0, nil
		}
		x, err := strconv.ParseInt(v, 10, 64)
		if err != nil || x < min || x > max {
			return 0, fmt.Errorf("bad %s=%q", n, v)
		}
		return x, nil
	}
	const maxSafe = 1<<53 - 1
	no, err := intAttr("no", true, 0, maxSafe)
	if err != nil {
		return out, err
	}
	vpos, err := intAttr("vpos", true, -maxSafe/10, maxSafe/10)
	if err != nil {
		return out, err
	}
	date, err := intAttr("date", true, 0, maxSafe)
	if err != nil {
		return out, err
	}
	usec, err := intAttr("date_usec", false, 0, 999_999)
	if err != nil {
		return out, err
	}
	score, err := intAttr("score", false, -maxSafe, maxSafe)
	if err != nil {
		return out, err
	}
	nicoru, err := intAttr("nicoru", false, 0, maxSafe)
	if err != nil {
		return out, err
	}
	deleted, err := intAttr("deleted", false, 0, maxSafe)
	if err != nil {
		return out, err
	}

	out.ThreadID = get("thread")
	userID := get("user_id")
	switch f, ok := c.Attr("fork"); {
	case ok && f == "1":
		out.Fork, out.ForkSource = "owner", ForkFromAttr
	case ok && f != "":
		out.Fork, out.ForkSource = f, ForkFromAttr
	case userID == "":
		out.Fork, out.ForkSource = "owner", ForkNoUserID
	default:
		out.Fork, out.ForkSource = "main", ForkAssumedMain
	}

	// Unicode whitespace, like the JS /\s+/ niconicomments uses (includes U+3000).
	commands := strings.Fields(get("mail"))
	if commands == nil {
		commands = []string{}
	}

	out.Deleted = deleted != 0
	for _, a := range c.Attrs {
		if !knownAttrs[a.Name.Local] || a.Name.Space != "" {
			out.UnknownAttrs = append(out.UnknownAttrs, a.Name.Local)
		}
	}
	out.Comment = V1Comment{
		ID:          "",
		No:          no,
		VposMs:      vpos * 10,
		Body:        c.Body,
		Commands:    commands,
		UserID:      userID,
		IsPremium:   get("premium") == "1",
		Score:       score,
		PostedAt:    time.Unix(date, usec*1000).In(jst).Format(time.RFC3339Nano),
		NicoruCount: nicoru,
		NicoruID:    nil,
		Source:      Source,
		IsMyPost:    false,
	}
	return out, nil
}

// MarshalComment encodes a V1 comment compactly without HTML escaping.
func MarshalComment(c V1Comment) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
