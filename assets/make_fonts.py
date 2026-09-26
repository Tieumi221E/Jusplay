"""Makes Jusplay's one font, Jus Sans (web/src/fonts), from Adobe's Source
Han Sans 2.005, under the SIL Open Font License 1.1. One family, one
version, two weights; it sets the whole interface in both languages and the
comments. Nothing else is bundled or named.

  Jus Sans Regular <- Source Han Sans (Japanese default, full glyph set) Regular
  Jus Sans Bold    <- Source Han Sans Bold

Source: the release's super OTC, 01_SourceHanSans.ttc.zip from
https://github.com/adobe-fonts/source-han-sans/releases/tag/2.005R (checked:
the faces must say "Version 2.005"). Pass the unzipped SourceHanSans.ttc:

  python assets/make_fonts.py path/to/SourceHanSans.ttc

Chinese and Japanese forms of the same character differ; Source Han Sans
carries both and picks them by language (the locl feature, from the html
lang or the canvas's lang). Only the Japanese and Simplified Chinese forms
are kept: the Korean and traditional ones are made unreachable before the
subset, so their glyphs go.

Characters: the common ones (ASCII, Latin-1 and Latin Extended, Greek,
Cyrillic, JIS X 0208 and JIS X 0213 plane 1, GB 2312, kana, the symbol, arrow,
box and shape blocks, fullwidth forms). Measured on a real comment set
(9 292 comments, 147 874 characters): 6 characters were in the font but
outside this set; the full set (44 853 characters) would triple the size
for them. What the font does not have at all is mostly emoji.

Added: the spaces comment art is laid out with, which Source Han Sans lacks
(SPACES below).

Dropped: hinting (the text is scaled onto a canvas or by the compositor),
vertical metrics and features (no vertical text), alternate-form features.
Kept: locl, kern, ccmp, liga, calt, mark. Not kept: the proportional and
alternate-width features (palt, halt, pwid, fwid, hwid): one font, set one
way throughout.
Digits are tabular in the font itself (555 units Regular, 590 Bold).

The subsets are renamed (OFL 1.1 section 3: a Modified Version may not use
the Reserved Font Name "Source") and written as WOFF2 by assets/woff2.mjs,
which checks each file by decoding it again.

Emoji, which Source Han Sans has none of, come from Google's Noto Color
Emoji in its vector form (COLRv1; Noto-COLRv1.ttf of the noto-emoji release
v2026-09-24-unicode18_0, Version 2.057, OFL 1.1), renamed Jus Emoji ("Noto"
is Google's trademark). It is split in two files by unicode-range, so each
is read only when a character of its own is drawn: the country flags (the
regional indicators: 26 characters, but 17 000 layer glyphs) and the rest.
Measured in the engine: text with only digits, (c) and arrows, which both
fonts have, loads neither; the keycaps need the digits inside the emoji
range (without them 1+U+FE0F+U+20E3 came out as two pieces).

  python assets/make_fonts.py path/to/SourceHanSans.ttc path/to/Noto-COLRv1.ttf
"""
import os, subprocess, sys, tempfile
from fontTools.ttLib import TTCollection, TTFont
from fontTools import subset

OUT = "web/src/fonts"
OFL = "assets/OFL.txt"  # the SIL Open Font License 1.1 text
VERSION = "Version 2.005"
KEEP_LANGS = ("DFLT", "JAN ", "ZHS ")
# No proportional widths (palt, halt, pwid): kana and punctuation keep one
# width everywhere, the interface and the comments alike.
FEATURES = ["locl", "kern", "ccmp", "liga", "calt", "mark"]
JOBS = [("Source Han Sans", "Jus Sans", "Regular", "jus-sans-400"),
        ("Source Han Sans Bold", "Jus Sans", "Bold", "jus-sans-700")]
EMOJI_VERSION = "Version 2.057"
FLAGS = set(range(0x1F1E6, 0x1F200))  # the regional indicators
CSS = "web/src/tokens.css"  # the emoji faces are written between these markers
CSS_BEGIN, CSS_END = "/* emoji faces: made by assets/make_fonts.py */", "/* end of emoji faces */"


def ranges(cps):
    """A unicode-range value for a set of code points."""
    cps = sorted(cps)
    out, start, prev = [], cps[0], cps[0]
    for c in cps[1:]:
        if c == prev + 1:
            prev = c
            continue
        out.append((start, prev))
        start = prev = c
    out.append((start, prev))
    return ",".join(f"U+{a:X}" if a == b else f"U+{a:X}-{b:X}" for a, b in out)


def emoji(path):
    """Writes the two emoji files and their @font-face rules (into CSS)."""
    ver = TTFont(path)["name"].getDebugName(5)
    if not ver.startswith(EMOJI_VERSION):
        sys.exit(f"{path} is {ver!r}, expected {EMOJI_VERSION}")
    every = set(TTFont(path).getBestCmap())
    faces = []
    for name, keep in (("jus-emoji", every - FLAGS), ("jus-emoji-flags", every & FLAGS)):
        font = TTFont(path)
        o = subset.Options()
        o.layout_features = ["*"]
        o.drop_tables += ["vhea", "vmtx", "DSIG"]
        o.name_IDs = ["*"]
        o.name_languages = ["*"]
        o.notdef_outline = True
        o.glyph_names = False
        o.hinting = False
        s = subset.Subsetter(o)
        s.populate(unicodes=keep)
        s.subset(font)
        n = font["name"]
        for rec in list(n.names):
            if rec.nameID not in (0, 13, 14):
                n.removeNames(nameID=rec.nameID)
        for nid, v in ((1, "Jus Emoji"), (2, "Regular"), (3, f"jusplay:{name}"), (4, "Jus Emoji"),
                       (5, EMOJI_VERSION + " (Jusplay subset)"), (6, "JusEmoji-Regular")):
            n.setName(v, nid, 3, 1, 0x409)
        n.setName("Subset of Noto Color Emoji (COLRv1, https://github.com/googlefonts/noto-emoji), renamed, for Jusplay.", 10, 3, 1, 0x409)
        with tempfile.TemporaryDirectory() as tmp:
            ttf = os.path.join(tmp, name + ".ttf")
            font.save(ttf)
            subprocess.run(["node", "assets/woff2.mjs", ttf, os.path.join(OUT, name + ".woff2")], check=True)
        faces.append(f'@font-face {{ font-family: "Jus Emoji"; src: url(fonts/{name}.woff2) format("woff2"); font-display: block;'
                     f' unicode-range: {ranges(font.getBestCmap())}; }}')
        print(f"{name}: {len(font.getBestCmap())} characters, {font['maxp'].numGlyphs} glyphs")
    css = open(CSS, encoding="utf-8").read()
    a, b = css.index(CSS_BEGIN) + len(CSS_BEGIN), css.index(CSS_END)
    with open(CSS, "w", encoding="utf-8", newline="") as f:
        f.write(css[:a] + "\n" + "\n".join(faces) + "\n" + css[b:])


def chars():
    out = set(range(0x20, 0x7F)) | set(range(0xA0, 0x250))
    # JIS X 0208, GB 2312 and JIS X 0213 plane 1 (two-byte EUC; plane 1 has
    # the level-3 kanji, e.g. 𠮟, and the extra symbols). Plane 2 (2 436
    # level-4 kanji, +1.6 MB for the two weights) is left out.
    for enc in ("euc_jp", "gb2312", "euc_jis_2004"):
        for a in range(0xA1, 0xFF):
            for b in range(0xA1, 0xFF):
                try:
                    s = bytes([a, b]).decode(enc)
                except UnicodeDecodeError:
                    continue
                out.update(ord(c) for c in s)
    for lo, hi in [(0x250, 0x2FF), (0x300, 0x36F), (0x370, 0x3FF), (0x400, 0x4FF),
                   (0x2000, 0x206F), (0x2070, 0x209F), (0x20A0, 0x20CF), (0x2100, 0x218F), (0x2190, 0x21FF),
                   (0x2200, 0x22FF), (0x2300, 0x23FF), (0x2460, 0x24FF), (0x2500, 0x259F), (0x25A0, 0x25FF),
                   (0x2600, 0x26FF), (0x2700, 0x27BF), (0x27C0, 0x27FF), (0x2800, 0x28FF), (0x2900, 0x297F),
                   (0x2980, 0x29FF), (0x2B00, 0x2BFF), (0x3000, 0x303F), (0x3040, 0x30FF), (0x3190, 0x319F),
                   (0x31F0, 0x31FF), (0x3200, 0x33FF), (0xFE10, 0xFE1F), (0xFE30, 0xFE4F), (0xFF00, 0xFFEF)]:
        out.update(range(lo, hi + 1))
    return out


def drop_langs(font):
    """Keep only the Japanese and Simplified Chinese language systems, so the
    other languages' locl alternates are unreachable and subset away."""
    for tag in ("GSUB", "GPOS"):
        if tag in font:
            for sr in font[tag].table.ScriptList.ScriptRecord:
                sr.Script.LangSysRecord = [l for l in sr.Script.LangSysRecord if l.LangSysTag in KEEP_LANGS]
                sr.Script.LangSysCount = len(sr.Script.LangSysRecord)


# Spaces the font lacks, which comment art is laid out with (U+2004 and
# friends; docs/architecture.md, comment fonts: without them a line of art
# came out a median 21% off). Widths in font units (1000 per em): U+2000/1
# are the en and em quads (Unicode: the same as U+2002/3), U+2007 is a
# digit wide and U+2008 a full stop wide, as Unicode defines them; the rest
# are the widths the comment-art comparison measured a 0.0% median error
# with (the Sarasa Gothic ones).
SPACES = {0x2000: 500, 0x2001: 1000, 0x2004: 333, 0x2005: 250, 0x2006: 166, 0x2007: "0", 0x2008: ".",
          0x2009: 181, 0x200A: 90, 0x200B: 0, 0x205F: 281}


def add_spaces(font):
    """Adds empty glyphs of the given widths to a CID-keyed CFF font and maps
    SPACES to them (the ones the font already maps are left alone)."""
    from fontTools.misc.psCharStrings import T2CharString
    cmap = font.getBestCmap()
    cff = font["CFF "].cff
    top = cff.topDictIndex[0]
    cs = top.CharStrings
    order = font.getGlyphOrder()
    space_fd = top.FDSelect[order.index(cmap[0x20])]
    private = top.FDArray[space_fd].Private
    nominal = getattr(private, "nominalWidthX", 0)
    next_cid = max(int(g[3:]) for g in order if g.startswith("cid")) + 1
    added = []
    for u, w in SPACES.items():
        if u in cmap:
            continue
        if isinstance(w, str):
            w = font["hmtx"][cmap[ord(w)]][0]
        name = f"cid{next_cid:05d}"
        next_cid += 1
        glyph = T2CharString(program=[w - nominal, "endchar"], private=private, globalSubrs=cff.GlobalSubrs)
        if cs.charStringsAreIndexed:
            cs.charStrings[name] = len(cs.charStringsIndex)
            cs.charStringsIndex.append(glyph)
        else:
            cs.charStrings[name] = glyph
        top.charset.append(name)
        top.FDSelect.append(space_fd)
        order.append(name)
        font["hmtx"][name] = (w, 0)
        for t in font["cmap"].tables:
            if t.isUnicode():
                t.cmap[u] = name
        added.append(f"U+{u:04X}={w}")
    font.setGlyphOrder(order)
    font["maxp"].numGlyphs = len(order)
    return added


def rename(font, family, style):
    full = family if style == "Regular" else f"{family} {style}"
    ps = f"{family.replace(' ', '')}-{style}"
    n = font["name"]
    for rec in list(n.names):
        if rec.nameID not in (0, 13, 14):  # keep the copyright and the license
            n.removeNames(nameID=rec.nameID)
    for nid, v in ((1, family), (2, style), (3, f"jusplay:{ps}"), (4, full), (5, VERSION + " (Jusplay subset)"), (6, ps)):
        n.setName(v, nid, 3, 1, 0x409)
    n.setName("Subset of Source Han Sans 2.005 (https://github.com/adobe-fonts/source-han-sans), renamed, for Jusplay.", 10, 3, 1, 0x409)
    n.setName("This Font Software is licensed under the SIL Open Font License, Version 1.1.", 13, 3, 1, 0x409)
    n.setName("https://openfontlicense.org", 14, 3, 1, 0x409)
    cff = font["CFF "].cff
    cff.fontNames = [ps]
    top = cff.topDictIndex[0]
    for k, v in (("FullName", full), ("FamilyName", family)):
        if hasattr(top, k):
            setattr(top, k, v)


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    coll = TTCollection(sys.argv[1])
    faces = {f["name"].getDebugName(4): f for f in coll.fonts}
    os.makedirs(OUT, exist_ok=True)
    text = chars()
    copyright = None
    for src, family, style, name in JOBS:
        font = faces.get(src) or sys.exit(f"{src} not in {sys.argv[1]}")
        ver = font["name"].getDebugName(5)
        if not ver.startswith(VERSION):
            sys.exit(f"{src} is {ver!r}, expected {VERSION}")
        copyright = font["name"].getDebugName(0)
        drop_langs(font)
        o = subset.Options()
        o.layout_features = FEATURES
        o.drop_tables += ["vhea", "vmtx", "VORG", "DSIG"]
        o.name_IDs = ["*"]
        o.name_languages = ["*"]
        o.notdef_outline = True
        o.glyph_names = False
        o.hinting = False
        s = subset.Subsetter(o)
        s.populate(unicodes=text)
        s.subset(font)
        added = add_spaces(font)
        rename(font, family, style)
        with tempfile.TemporaryDirectory() as tmp:
            otf = os.path.join(tmp, name + ".otf")
            font.save(otf)
            subprocess.run(["node", "assets/woff2.mjs", otf, os.path.join(OUT, name + ".woff2")], check=True)
        print(f"{name}: {src} {ver}, {len(font.getBestCmap())} characters, {len(font.getGlyphOrder())} glyphs; spaces added: {' '.join(added)}")
    emoji(sys.argv[2])
    body = open(OFL, encoding="utf-8").read()
    body = body[body.index("This Font Software is licensed"):]
    with open(os.path.join(OUT, "LICENSE.txt"), "w", encoding="utf-8", newline="\n") as f:
        f.write("Both fonts here are under the SIL Open Font License, Version 1.1 (below).\n\n"
                "Jus Sans (jus-sans-400.woff2, jus-sans-700.woff2) is a subset of Source Han Sans 2.005\n"
                "(https://github.com/adobe-fonts/source-han-sans), renamed as the license requires.\n"
                + copyright + "\n\n"
                "Jus Emoji (jus-emoji.woff2, jus-emoji-flags.woff2) is a subset of Noto Color Emoji 2.057,\n"
                "COLRv1 (https://github.com/googlefonts/noto-emoji), renamed (Noto is a trademark of Google Inc.).\n"
                "Copyright 2022 Google Inc.\n\n" + body)


main()
