# niconicomments (vendored)

From niconicomments 0.4.1 by xpadev-net (MIT, `LICENSE`), `dist/bundle.js`
and `dist/bundle.d.ts`, taken in as Jusplay's own code so it can be
optimised and, piece by piece, rewritten.

Every change must keep the layout fingerprint of the reference set
unchanged (`jusplay play <video> -selftest -layout`; web/src/layoutcheck.ts):
for each comment its computed properties, position and box, and for each
time slot the order of its comments. The only randomness in the original is
the height of a comment that finds no free row (Math.random, as on
Niconico); the check seeds it.

## Changes

1. ES module instead of the UMD wrapper.
2. Time slots are sorted when first read (`_slot`), not all at the end of
   the build: sorting 143 390 slots for 2881 comments took ~75 ms. The order
   (owner comments last, then by index) is total, so the arrays are the same.
   (Superseded by 5.)
3. `deferBuild` and `build(budgetMs)`: the layout is built in slices that
   hand the main thread back (scheduler.yield) after ~6 ms each; the same
   steps in the same order, so the same layout. The player's overlay builds
   this way and keeps drawing the old renderer until the new one is ready.

4. v1 input is checked and converted directly (`fastFromV1`), with the
   same acceptance rules as the schema; only input that fails them goes
   through the schema library (which also copied the whole input), so an
   invalid file still fails with the original error.
5. The timeline is `IntervalTimeline`: each comment is stored once as the
   span of slots it is on screen, in buckets of 128 slots, and `slot(v)`
   returns the sorted list the original kept for slot v. The original pushed
   every comment into every slot of its span (about 600 for a scrolling
   comment; 1.4 million entries for 2881 comments). The collision tables,
   which the layout reads, are unchanged.

Measured with the reference set (4 episodes x 3 settings, all fingerprints
unchanged): the longest main-thread block during a build went from the whole
build (500-1700 ms) to 0 in most runs; 53-101 ms remain in a few, from the
parts not yet sliced (parsing and validating the input, the keepCA pass) or
from garbage collection of the previous instance.

After 4 and 5 (all fingerprints unchanged): a warm rebuild of 2881 comments
134-167 ms (original 213-276 ms); the build's own slices at most 13-28 ms;
while playing, JS heap 40 MB (was 52-57 MB), drawing 0.16 ms per frame as
before.

6. build() hands the main thread back at background priority
   (scheduler.postTask): scheduler.yield() resumes ahead of other tasks and,
   with the build overlapping the video's start, held the first video
   appends back by ~0.5 s. The player starts the build as soon as the
   comments arrive, in 3 ms slices: comments ready at ~0.72 s, first frame
   at ~0.84 s (the build used to wait for the first frame: comments at
   ~1.23 s). Most of a first build is the browser meeting the comment fonts
   and characters for the first time: measuring text took 223 ms cold and
   15 ms warm for 2881 comments.

7. The two schema checks run for every comment while measuring
   (is(ZMeasureInput), is(array(ZCommentMeasuredContentItem))) are written
   out with the same rules (`fastIsMeasureInput`, `fastIsMeasuredItemArray`):
   12-22 ms of 2881 comments. A warm rebuild of 2881 comments is now
   117-147 ms (original 213-276 ms).

8. Every 2D context is Japanese (`jaCtx`: `ctx.lang = "ja"`). A canvas not
   in the page takes the document's language, and the interface can be
   Chinese; the one font (Jus Sans) carries both forms of Han characters and
   picks them by language, so without this a Chinese interface drew the
   comments' kanji in their Chinese forms. Measured in the engine: a detached
   canvas drew 直 with the Chinese form by default and the Japanese form with
   `lang = "ja"`.

Not solved: about one frame in 15-20 s where drawing the comments takes
8-12 ms (over one refresh at 120 Hz), always a frame in which new comments
are drawn for the first time. Not garbage collection (JS heap unchanged) and
not the texture sweep (_gcTextures). Making the text images ahead in idle
time, and also uploading them as textures ahead, did not remove them (and
cost ~50 MB), so both were taken out again. Video frames are not affected.
