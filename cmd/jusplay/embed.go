package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/embed"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// cmdEmbed puts comments into an MKV in place. Exit status 3 means there
// was no same-name comment file, so scripts can tell "nothing to do" from
// a failure.
func cmdEmbed(args []string) error {
	fs := flag.NewFlagSet("embed", flag.ExitOnError)
	videoID := fs.String("video-id", "", "Niconico video id for comment files that carry none")
	offset := fs.Int64("offset-ms", 0, "sync offset (nicoMs = localMs + offset); default keeps the embedded one")
	verified := fs.Bool("verified", false, "with -offset-ms: mark the offset as checked against the picture")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "jusplay embed <video.mkv> [comments.json|snapshot-dir|raw.zip|.xml]\n\n"+
			"Without a comment file, uses the .json next to the video with the same name.\n")
		fs.PrintDefaults()
	}
	var pos []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		pos, rest = append(pos, rest[0]), rest[1:]
	}
	if len(pos) < 1 || len(pos) > 2 {
		fs.Usage()
		os.Exit(2)
	}
	o := embed.Options{VideoID: *videoID}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "offset-ms" {
			o.OffsetMs, o.SyncVerified = offset, *verified
		}
	})
	video, source := pos[0], ""
	if len(pos) == 2 {
		source = pos[1]
	}
	comments.Version = version
	// Replaced comments are kept next to the video, in the folder's records.
	o.BackupDir = filepath.Join(filepath.Dir(video), library.VaultDir, "backup")
	r, err := embed.Embed(video, source, o)
	if errors.Is(err, comments.ErrNone) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if err != nil {
		return err
	}
	action := "embedded"
	if r.Replaced {
		action = "replaced embedded comments with"
	}
	sync := "unverified"
	if r.SyncVerified {
		sync = "verified"
	}
	fmt.Printf("%s: %s %s (%d comments), offset %d ms %s; %d packets verified in %s\n",
		filepath.Base(video), action, filepath.Base(r.Source), r.Comments, r.OffsetMs, sync,
		r.Verification.Packets, r.Took.Round(time.Millisecond))
	if r.Backup != "" {
		fmt.Println("the comments it had are kept in", r.Backup)
	}
	if r.Recovered != "" {
		fmt.Println("an earlier interrupted embed was put right:", r.Recovered)
	}
	return nil
}
