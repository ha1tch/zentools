# zentools — resolved items

Closed register items, newest first, each with its text as it stood at closure and the version that closed it.

## [0.14.4] T-07 — The tracked MANIFEST.sha256 is stale; only the release archive gets a fresh one (v0.14.4, 2026-11-14)

Theme: release · closed 0.14.4 · 2026-11-14
**Trigger:** noticed during the 0.12.0 release pass (2026-11-14).

**Scope:** MANIFEST.sha256 lists a sha256 for each file. The release archive step writes a fresh, correct MANIFEST.sha256 into the zip it builds, but the copy tracked in the repository was last written in the 0.8.6 commit and 0.9.0 onwards changed many of its files. (First filed as "no release step regenerates it", which was wrong: the archive step does.) Either have the release write the fresh manifest back into the tree before committing, or stop tracking it and leave it to the archive.

**Resolution:** stopped tracking it. MANIFEST.sha256 was removed from the repository; the release archive step still writes a fresh, correct one into each release zip, which is the only place a file manifest is useful. See CHANGELOG 0.14.4.

Cross-ref: CHANGELOG 0.14.4.

## [0.12.0] T-03 — No handling yet for Next palette file variants beyond the 2-byte .nxp layout (v0.12.0, 2026-11-14)

Theme: next · closed 0.12.0 · 2026-11-14
**Trigger:** filed in zenpal at its first commit (2026-09-10) and moved here when zenpal was folded into zentools (2026-11-14).

**Scope:** only the 2-byte-per-entry .nxp layout is implemented. gfx2next also writes 1-byte RGB332, GBR444 and BGR222 palettes, and palettes are embedded in other files (an .nxi with its palette prepended, a NEX file's palette block). None of these is read or written yet.

**Resolution:** `zxn.EncodePalette`, `DecodePalette` and `DecodePaletteRGBA` read and write the RGB332, GBR444 and BGR222 layouts, checked against gfx2next; `zx next bitmap` reads and writes a palette embedded before an .nxi's pixels; `pkg/nex` reads and writes a NEX file's palette block. See CHANGELOG 0.12.0.

Cross-ref: CHANGELOG 0.12.0.

## [0.12.0] T-02 — No handling yet for Next sprite attribute tables (v0.12.0, 2026-11-14)

Theme: next · closed 0.12.0 · 2026-11-14
**Trigger:** filed in zenpal at its first commit (2026-09-10) and moved here when zenpal was folded into zentools (2026-11-14).

**Scope:** pkg/zxn models only the raw 16x16 pattern data. The Next's hardware sprites also carry attribute bytes (position, palette offset, mirror and rotate flags, anchor and relative sprites); no type or encoding for them exists yet.

**Resolution:** `zxn.SpriteAttr` with `Bytes`, `ParseSpriteAttr`, `EncodeAttrs` and `DecodeAttrs` covers anchor, composite and unified relative sprites at four or five bytes each (or a fixed five), and `zx next attrs encode|decode` converts tables to and from JSON. The layout follows the Next developer guide's register descriptions; checking it on hardware is T-04. See CHANGELOG 0.12.0.

Cross-ref: CHANGELOG 0.12.0.

## [0.12.0] T-01 — Images with more than 256 RGB333 colours need a colour reducer before zx next can use them (v0.12.0, 2026-11-14)

Theme: next · closed 0.12.0 · 2026-11-14
**Trigger:** filed in zenpal at its first commit (2026-09-10) and moved here when zenpal was folded into zentools as pkg/zxn (2026-11-14).

**Scope:** `zxn.PaletteFromImage` refuses a true-colour image with more than 256 distinct RGB333 colours rather than reducing them, so `zx next palette encode` and `zx next sprite encode` need an indexed image or a prepared palette. Typical source art exceeds 256 colours. Needs a colour reducer (median cut or similar) that picks the palette and maps the pixels, ideally with dithering; the same reducer would serve Layer 2 images (.nxi, .sl2) and LoRes when those are added.

**Resolution:** `zxn.Reduce` keeps an image's colours exactly when they fit and otherwise chooses them by median cut over the RGB333 histogram (splitting the group with the largest squared error first, at the least-error point), with optional Floyd-Steinberg dithering, a transparent index, and mapping onto a given palette. Every `zx next` encoder (palette, sprite, bitmap, tiles) uses it through the shared colour flags. See CHANGELOG 0.12.0.

Cross-ref: CHANGELOG 0.12.0.

