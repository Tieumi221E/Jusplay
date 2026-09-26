// Package embed puts comment data into an MKV in place: the new file is
// written next to the original, verified packet by packet, flushed to disk,
// and only then swapped in (safeswap); any failure leaves the original as it
// was. Attachments being replaced are first kept in a backup archive.
package embed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/bundle"
	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/safeswap"
)

type Options struct {
	VideoID string // for sources that carry none
	// OffsetMs overrides the sync offset; nil keeps the one already embedded
	// (with its verified flag), or 0 for a first embed.
	OffsetMs *int64
	// SyncVerified marks an overriding offset as checked against the picture.
	SyncVerified bool
	// BackupDir receives the attachments being replaced, as one archive per
	// embed (<video>.<time>.comments.zip), before the file is touched. ""
	// refuses to replace existing attachments: they may hold the only copy
	// of a complete capture.
	BackupDir string
}

type Result struct {
	Source       string
	Comments     int
	Replaced     bool   // the file already had jusplay attachments
	Backup       string // where they were kept
	Recovered    string // what an earlier interrupted embed left, and what was done
	OffsetMs     int64
	SyncVerified bool
	Verification *mkv.Verification
	Took         time.Duration
}

// Embed attaches source (a comment file, snapshot dir or ZIP; "" = the
// same-name .json next to video) to video, replacing earlier jusplay
// attachments.
func Embed(video, source string, o Options) (*Result, error) {
	start := time.Now()
	if !strings.EqualFold(filepath.Ext(video), ".mkv") {
		return nil, fmt.Errorf("%s: comments can only be embedded in MKV files", filepath.Base(video))
	}
	recovered, err := safeswap.Recover(video)
	if err != nil {
		return nil, err
	}
	if source == "" {
		if source = comments.SameName(video); source == "" {
			return nil, fmt.Errorf("%w next to %s", comments.ErrNone, filepath.Base(video))
		}
	}
	snap, err := comments.Open(source, o.VideoID)
	if err != nil {
		return nil, err
	}
	probe, err := mkv.ProbeFile(video)
	if err != nil {
		return nil, err
	}
	res := &Result{Source: source, Replaced: len(probe.Ours()) > 0, Recovered: recovered}
	if res.Replaced {
		if o.BackupDir == "" {
			return nil, fmt.Errorf("%s already has comments; replacing them needs a backup folder", filepath.Base(video))
		}
		if res.Backup, err = backup(video, o.BackupDir); err != nil {
			return nil, fmt.Errorf("keeping the current comments failed, nothing was changed: %w", err)
		}
	}
	if o.OffsetMs != nil {
		res.OffsetMs, res.SyncVerified = *o.OffsetMs, o.SyncVerified
	} else if res.Replaced {
		res.OffsetMs, res.SyncVerified = previousTiming(video)
	}
	atts, rep, err := bundle.Build(snap, probe, bundle.Options{
		Derive:       derive.Options{},
		OffsetMs:     res.OffsetMs,
		SyncVerified: res.SyncVerified,
		Renderer:     bundle.Renderer{Name: "niconicomments", Version: "0.4.1-jusplay", Mode: "default", Fonts: "Jus Sans 2.005"},
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	res.Comments = rep.Output

	tmp := safeswap.Temp(video)
	if res.Verification, err = mkv.Replace(video, tmp, atts); err != nil {
		return nil, err
	}
	if err := safeswap.Swap(tmp, video); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	res.Took = time.Since(start)
	return res, nil
}

// previousTiming reads the offset and verified flag of the attachments
// already in video; 0/false when there are none or they cannot be read.
func previousTiming(video string) (int64, bool) {
	files, err := mkv.Extract(video)
	if err != nil {
		return 0, false
	}
	var m bundle.Manifest
	if json.Unmarshal(files[bundle.ManifestName], &m) != nil || m.Timing.OffsetMs == nil {
		return 0, false
	}
	return *m.Timing.OffsetMs, m.Timing.Verified
}

// backup writes the jusplay attachments now in video to one archive in dir
// and flushes it to disk.
func backup(video, dir string) (string, error) {
	files, err := mkv.Extract(video)
	if err != nil {
		return "", err
	}
	z, err := detzip.Write(files)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	p := filepath.Join(dir, base+"."+time.Now().Format("20060102-150405")+".comments.zip")
	if err := os.WriteFile(p, z, 0o644); err != nil {
		return "", err
	}
	return p, safeswap.Sync(p)
}
