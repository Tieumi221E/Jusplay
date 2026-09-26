// Package bundle builds the three MKV attachments from a snapshot and
// checks a set of extracted attachments layer by layer.
package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
	"github.com/Tieumi221E/Jusplay/schema"
)

const (
	RawName      = "jusplay.nico.raw.zip"
	PlaybackName = "jusplay.nico.playback.json"
	ManifestName = "jusplay.nico.manifest.json"
)

type Asset struct {
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type MediaStream struct {
	Index    int     `json:"index"`
	Type     string  `json:"type"`
	Codec    string  `json:"codec"`
	Language *string `json:"language"`
}

type Timing struct {
	Mode     string `json:"mode"`
	OffsetMs *int64 `json:"offsetMs,omitempty"`
	Source   string `json:"source"`
	Verified bool   `json:"verified"`
}

type Renderer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
	Fonts   string `json:"fonts"`
}

type Manifest struct {
	Schema             string   `json:"schema"`
	VideoID            string   `json:"videoId"`
	SnapshotID         string   `json:"snapshotId"`
	SnapshotCapturedTo *string  `json:"snapshotCapturedTo"`
	CaptureStatus      string   `json:"captureStatus"`
	Forks              []string `json:"forks"`
	Assets             []Asset  `json:"assets"`
	Media              struct {
		DurationMs *int64        `json:"durationMs"`
		Streams    []MediaStream `json:"streams"`
	} `json:"media"`
	Timing    Timing   `json:"timing"`
	Renderer  Renderer `json:"renderer"`
	CreatedAt string   `json:"createdAt"`
}

type Options struct {
	Derive   derive.Options
	OffsetMs int64
	// SyncVerified records that the offset was checked against the picture.
	SyncVerified bool
	Renderer     Renderer
	CreatedAt    string
}

// Build derives, packs and describes a snapshot. probe describes the video
// the attachments are meant for.
func Build(s *snapshot.Snapshot, probe *mkv.Probe, opt Options) ([]mkv.Attachment, *derive.Report, error) {
	playback, rep, err := derive.Derive(s, opt.Derive)
	if err != nil {
		return nil, nil, err
	}
	raw, err := detzip.Write(s.Files())
	if err != nil {
		return nil, nil, err
	}
	m := Manifest{
		Schema:             "jusplay-attachment/0",
		VideoID:            s.Capture.VideoID,
		SnapshotID:         s.Capture.SnapshotID,
		SnapshotCapturedTo: s.Capture.CapturedTo,
		CaptureStatus:      s.Report.Status,
		Forks:              append([]string{}, opt.Derive.Forks...),
		Assets: []Asset{
			{RawName, "application/zip", snapshot.SHA256Hex(raw), int64(len(raw))},
			{PlaybackName, "application/json", snapshot.SHA256Hex(playback), int64(len(playback))},
		},
		Timing:    Timing{Mode: "offset", OffsetMs: &opt.OffsetMs, Source: "manual", Verified: opt.SyncVerified},
		Renderer:  opt.Renderer,
		CreatedAt: opt.CreatedAt,
	}
	if probe.Duration > 0 {
		ms := int64(math.Round(probe.Duration * 1000))
		m.Media.DurationMs = &ms
	}
	m.Media.Streams = []MediaStream{}
	for _, st := range probe.Media() {
		ms := MediaStream{Index: st.Index, Type: st.CodecType, Codec: st.CodecName}
		// As ffprobe reported it before: any language but "und".
		if l := st.Language; l != "" && l != "und" {
			ms.Language = &l
		}
		m.Media.Streams = append(m.Media.Streams, ms)
	}
	mb, err := snapshot.MarshalDoc(m)
	if err != nil {
		return nil, nil, err
	}
	if err := schema.Validate(schema.Manifest, mb); err != nil {
		return nil, nil, err
	}
	return []mkv.Attachment{
		{Name: RawName, MIME: "application/zip", Data: raw},
		{Name: PlaybackName, MIME: "application/json", Data: playback},
		{Name: ManifestName, MIME: "application/json", Data: mb},
	}, rep, nil
}

// Check verifies extracted attachments: manifest schema, asset hashes,
// the snapshot inside the ZIP, and that re-deriving the snapshot with the
// recorded forks reproduces the playback file byte for byte.
func Check(files map[string][]byte) (*Manifest, error) {
	for name := range files {
		if name != RawName && name != PlaybackName && name != ManifestName {
			return nil, fmt.Errorf("unknown attachment %s", name)
		}
	}
	mb, ok := files[ManifestName]
	if !ok {
		return nil, errors.New("no manifest attachment")
	}
	if err := schema.Validate(schema.Manifest, mb); err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, err
	}
	var errs []error
	for _, a := range m.Assets {
		b, ok := files[mkv.Canonical(a.Name)]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%s missing", a.Name))
		case int64(len(b)) != a.Bytes || snapshot.SHA256Hex(b) != a.SHA256:
			errs = append(errs, fmt.Errorf("%s: hash or size does not match manifest", a.Name))
		}
	}
	if len(errs) > 0 {
		return &m, errors.Join(errs...)
	}
	zfs, err := detzip.Open(files[RawName])
	if err != nil {
		return &m, fmt.Errorf("raw zip: %w", err)
	}
	s, err := snapshot.Load(zfs)
	if err != nil {
		return &m, fmt.Errorf("raw zip snapshot: %w", err)
	}
	if s.Capture.SnapshotID != m.SnapshotID || s.Capture.VideoID != m.VideoID {
		return &m, errors.New("manifest identity does not match the snapshot")
	}
	var forks []string
	if len(m.Forks) > 0 {
		forks = m.Forks
	}
	playback, _, err := derive.Derive(s, derive.Options{Forks: forks})
	if err != nil {
		return &m, err
	}
	if !bytes.Equal(playback, files[PlaybackName]) {
		return &m, errors.New("playback file is not what the snapshot derives to (derivation changed, or file edited)")
	}
	return &m, nil
}
