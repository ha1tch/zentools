# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.14.4] - 2026-11-14

### Changed

- **Documentation brought in line with the code.**
  - The README describes the ZXMODE and palette-design work, adds ZXMODE
    and Spectrum Next examples, and lists all fourteen packages
    (it said ten, and left out `pkg/bdf`).
  - The library manual now documents the classic `pkg/scr` API (screens,
    attributes, conversion, cropping, asset collections), which it had
    left to the doc comments, as well as `tap.DecodeBlock` and
    `build.EncodeSZX`, `Overlay` and `CodeEnd`. Its overview now lists
    `pkg/bdf` and every dependency between packages.
  - The architecture document no longer lists a `.zxs` codec in
    `pkg/snapshot` (that format belongs to zenzx) or third-party
    dependencies in `cmd/`. It lists the snapshot functions that exist and
    the screen formats zentools owns.

### Removed

- `MANIFEST.sha256`, which had not been updated since 0.8.6. Each release
  zip still carries a fresh one, written by the archive step. This closes
  T-07 (see RESOLVED.md).

## [0.14.3] - 2026-11-14

### Changed

- The ZXMODE and Next handling manuals now use Giles Laurent's photograph
  *Chateau de Chillon and Dents du Midi* (CC BY-SA 4.0) instead of the
  Turner painting. It is a landscape that scores consistently near the
  top in every palette among the specification's 22 colour pictures,
  so it shows the tools at their best. Every figure, number and
  command output in the two manuals was redone with it.
- The Next manual now fits a HiRes picture at 512x384 and squeezes it to
  512x192 with `--resize stretch`, so it keeps its proportions; fitting
  straight to 512x192 cropped a wide strip and showed it stretched.

## [0.14.2] - 2026-11-14

### Added

- **[Next handling manual](docs/NEXT-HANDLING.md)**: a worked guide to
  `zx next` and `zx zx0`, making the graphics for a small game and
  packing them into a NEX file. It covers Next colour and transparency,
  a Layer 2 title screen, palettes and their layouts, 8- and 4-bit
  sprites, sprite attributes, tiles and tilemaps, LoRes and Timex
  screens, ZX0 and NEX files. The sprites and tiles were drawn for it.
  Every command in it was run to produce its figures and output.

### Fixed

- The CLI manual said the hardware compares the RGB332 colour of the
  transparent index for every layer. Layer 2 does (NextReg 0x14), but
  sprites compare the index itself (NextReg 0x4B, of which a 4-bit sprite
  uses the low four bits) and the tilemap its own index (NextReg 0x4C).

## [0.14.1] - 2026-11-14

### Added

- **[ZXMODE handling manual](docs/ZXMODE-HANDLING.md)**: a worked guide
  to the ZXMODE modes and ZXPAL palettes in `zx scr`, using a
  public-domain Turner painting. It covers fitting, encoding in each
  palette, the error measures, loaded registers, assets across palettes,
  text on ZXMODE screens, error diffusion, and palette design with
  `-banks`, `-cell` and `fitregs`. Every command in it was run to produce
  its figures and numbers. The other manuals, the README and the HTML
  navigation link to it.

## [0.14.0] - 2026-11-14

### Added

- **Palette design tools from the ZXMODE specification's evaluation**, the
  parts of its Python scripts that `zx` did not yet cover:
  - `zx scr encode -banks file.csv` encodes over any colour banks, and
    `-cell WxH` with cells of any size dividing the picture; the output is
    a PNG. It matches the specification's `zxcommon.render` and
    `zxfast.render_fast` pixel for pixel (4x4 cells, cells 4 wide and 8
    tall, a 64-colour bank at another penalty).
  - `-penalty p` sets the encoder's span penalty (reference 0.204), also
    for encodings written as screens.
  - `zx scr palette -banks` writes a palette's banks as the encoder pairs
    within them.
  - `zx scr fitregs` fits colours to add to ZXPAL3, ZXPAL4 or a one-bank
    table from pictures, as the specification's register search does.
    On its ten development pictures, cross-validated, it gives 10.14,
    9.23, 8.58 and 7.95 for 8, 16, 32 and 48 colours (the
    specification's run: 10.28, 9.42, 8.47, 7.91).
- Library: `Banks`, `PaletteBanks`, `BanksCSV`, `ParseBanksCSV`,
  `EncodeBanks`, `BankOptions`, `RefPenalty`, `ParseCellSize`,
  `FitRegisters`, `EncodeOptions.Penalty`.

## [0.13.0] - 2026-11-14

### Added

- **The ZXMODE specification's picture fit in `zx scr encode` and the new
  `zx scr fit`**: `-filter lanczos` (Pillow's Lanczos resampler,
  reproduced exactly), `-centering x,y` for `-resize fill`'s crop,
  `-trim n` for scan margins, and `-grey` (Pillow's `convert("L")`).
  `-resize fill -filter lanczos` gives the specification's fitted
  pictures byte for byte from the same pixels.
- **The specification's error measures in `zx scr compare -metric`**:
  `blur` (RMS after Pillow's Gaussian blur of 1.5), `light` (0.75),
  `blur=<sigma>`, `rms`, `ssim` (scikit-image's) and `all`; they match
  the specification's tools to 1e-9. Either side may be a screen file.
- **Loaded palette registers** (`-registers file.csv`) for encode, decode,
  crop, ocr, cut, paste, flip, convert and compare, and `zx scr palette
  -registers`, `-o file.csv`. The specification's register CSVs read as
  they are.
- **Cut and paste on ZXMODE screens**: `zx scr cut` reads `.zm1`/`.zm2`
  in any palette and keeps the attributes; `zx scr paste` writes into any
  screen and converts attributes when the asset's palette or registers
  differ from the target's.
- **ZCUT version 3**: an asset may carry its own register table; a
  collection is written at the lowest version that holds it (v1 and v2
  files are unchanged).
- **`zx scr convert`** rewrites the attributes of a collection, one asset
  or a screen for another palette and registers (and between ZXMODE0 and
  ZXMODE1), keeping the bitmap and choosing the nearest target colours.
- **`zx scr flip -h|-v`** for screens, collections and images.
- `zx info` describes `.zm1`, `.zm2` and `.cut` files; `zx scr crop` (with
  `-auto`) and `zx scr ocr` read ZXMODE screens.
- **Experimental encoder options** from Part 4 of the specification:
  `-diffuse f` (cell-level error diffusion), `-flat n` and
  `-linear-pairs`, with measurements in the CLI manual. Diffusing half the
  error lowered the error by 13-28%; the other two made it worse.
- Library: `Registers`, `ParseRegistersCSV`, `EncodePictureWith`,
  `FitToWith`, `ResizeLanczos`, `FitLanczos`, `Trim`, `Grey`,
  `BlurredRMSE`, `SSIM`, `ConvertAsset`, `CutPicture`, `PastePicture`,
  `Asset.Flip`, `Picture.Flip`, `RecognizePicture`.

### Changed

- **JPEG decoding, for every command**: colour is rebuilt with
  libjpeg-turbo's chroma upsampling and colour tables instead of the
  nearest chroma sample, so JPEGs decode within a few levels of what
  Pillow and other libjpeg-based tools give (previously up to 68 levels
  apart). The remaining difference is filed as T-08.
- `Paste` onto a classic screen now converts an asset in another palette
  to ZXPAL0 instead of refusing it.
- `zx scr compare` accepts screen files on either side.

## [0.12.0] - 2026-11-14

### Added

- **The rest of the Spectrum Next graphics formats in `pkg/zxn` and
  `zx next`**, each checked byte for byte against fixtures the community
  tools wrote (gfx2next, nextbmp, nextraw):
  - Palette layouts beyond the 2-byte `.nxp`: 1-byte RGB332, 2-byte GBR444
    and 1-byte BGR222 (`-layout`, `zx next palette convert`); the Next's
    standard palette (`palette std`, `-std`); palette minimising (`-min`)
    (closes T-03, with the embedded palettes below).
  - A fourth rounding mode, `distance`, which is gfx2next's default.
  - Layer 2 bitmaps (`zx next bitmap`): 256x192, 320x256 and 640x256, row
    or column order, with an embedded or separate palette; LoRes 128x96 at
    8 and 4 bits.
  - Timex HiRes and HiColour screens (`zx next timex`), HiColour encoding
    choosing each 8x1 span's colours.
  - Tiles, tilemaps and blocks (`zx next tiles`): 8-, 4- and 1-bit tiles of
    any size, dropping repeated, mirrored and turned tiles with the
    matching attribute bits, 8- and 16-bit maps in either order, tile and
    palette offsets, and blocks of tiles; `tiles decode` draws a map with
    its attributes applied.
  - Hardware sprite attributes (`zx next attrs`, `SpriteAttr`): anchor and
    relative sprites, four or five bytes each, to and from JSON (closes
    T-02).
  - Sprite sheets: `-dedup` and `-pad` on encode, `-range` and `-split`
    (one PNG per pattern) on decode.
- **A colour reducer**, `zxn.Reduce`: an image with more colours than fit is
  reduced by median cut over its RGB333 histogram, optionally with
  Floyd-Steinberg dithering (`-dither fs`), and transparent pixels can take
  a chosen index (`-transparent`); `-match` maps an image onto a given
  palette instead (closes T-01).
- Register items T-04 (layouts still to check on hardware), T-05 (ZX0
  compression speed), T-06 (gfx2next options not yet covered) and T-07
  (MANIFEST.sha256 is not regenerated).
- **`pkg/nex` and `zx next nex info|extract|build`**: NEX files, V1.2 and
  V1.3 (loading screens and their palette, copper code, checksum, appended
  data). Six files sjasmplus wrote are read and rebuilt byte for byte, from
  the package and from `extract` followed by `build`.
- **`pkg/zx0` and `zx zx0`**: ZX0 compression and decompression, v2,
  classic (as gfx2next writes), backwards and quick, ported from the
  reference compressor and producing its bytes.
- **BMP input** for every image-reading command: `pkg/scr` registers a
  decoder for uncompressed BMPs at 1, 4, 8, 16, 24 and 32 bits, and an
  indexed BMP keeps its palette.
- `zx info` recognises NEX files by signature, and `.nxp`, `.spr`, `.nxi`,
  `.sl2` and `.slr` files by extension.
- `scr.ULAColor`, the Spectrum colour of an index at a brightness.

### Changed

- `zx next sprite encode -palette` now always writes the palette. For an
  image that is not indexed it used to name a palette to match against;
  that is now `-match`, which maps to the nearest colours instead of
  refusing colours the palette lacks. An image that is not indexed no
  longer needs a palette at all: its colours are chosen.
- `zx next palette encode` takes the shared colour flags (`-layout`, `-std`,
  `-min`, `-dither`, ...) and chooses colours for an image with too many.

### Fixed

- The `pkg/zxn` documentation called nearest rounding gfx2next's default;
  gfx2next's default is the distance search, now `RoundDistance`.

## [0.11.0] - 2026-11-14

### Added

- **`pkg/zxn` and `zx next`: ZX Spectrum Next palettes (`.nxp`) and
  hardware sprite patterns (`.spr`)**, folded in from zenpal, a separate
  palette and sprite tool that was never published. `zx next palette
  info|decode|encode` and `zx next sprite info|decode|encode` read, draw and
  write them; an indexed image keeps its own palette and pixel indices,
  any other image is matched against a palette.
  - The output matches the community tools byte for byte, checked against
    fixtures they wrote: gfx2next for palettes and 8- and 4-bit sprites,
    nextbmp/nextraw for palettes rounded down or up (gfx2next reduces a
    palette colour twice in those modes, moving it a further step).
  - New over zenpal: making sprites from an image (`sprite encode`), the
    rounding modes, and a rendered sprite sheet with a transparent index.
- **A work register** (`docs/TRACKING.md`, `docs/RESOLVED.md`), starting
  with zenpal's three open items: a colour reducer for images over 256
  colours (T-01), sprite attribute tables (T-02), other palette layouts
  (T-03).

### Fixed (relative to zenpal)

- 8-bit channels were reduced to 3 bits by truncation, which brightens:
  0xC8 became level 6 (219) instead of the nearest, 5 (182). They are now
  rounded to the nearest level by default.
- Reading a palette dropped bit 7 of each entry's second byte, the Layer 2
  priority bit, so a round trip cleared it. It is now kept.

## [0.10.0] - 2026-11-14

### Added

- `zx scr palette <zxpalN> [-o file.csv|file.png]`: a ZXPAL palette's
  registers as a table, a CSV, or a PNG of swatches.
- `zx scr compare <a> <b>`: counts the pixels where two images differ and
  exits non-zero if any do; a 256x192 image is doubled to compare with a
  512x384 one.
- `--resize fill` (alias `cover`), and `ResizeFill` in `pkg/scr`: scale to
  cover and crop the overflow equally from both sides; shrinking averages
  the source area under each target pixel instead of sampling it.

## [0.9.0] - 2026-11-14

### Added

- **ZXMODE screen modes and ZXPAL palettes** in `pkg/scr` and `zx scr`,
  for the ZXMODE video specification for the Spectrum family: the classic
  mode ZXMODE0, the linear modes ZXMODE1 (256x192) and ZXMODE2 (512x384),
  and the five palettes ZXPAL0 to ZXPAL4, including ZXPAL3 and ZXPAL4 with
  two attribute bytes per cell.
  - `Palette` generates each palette's registers from the specification's
    rules; a test checks ZXPAL4 against the published register table.
  - `Picture`, `EncodePicture`, `DecodePicture`, `PictureLen`: a screen in
    any mode and palette, its stored form (`.scr` for ZXMODE0, the linear
    page layout for ZXMODE1 and ZXMODE2) and rendering.
  - `EncodePicture` is the specification's reference encoder (best pair of
    register colours per cell, dithered in linear light). On the
    specification's test scene and 22 photographs it gives the same picture
    as the specification's Python reference, pixel for pixel, in every
    palette, about ten times faster. Linear-light values are tabulated,
    because Go's `math.Pow` differs from the reference in the last bit for
    88 of the 256 codes, and products are kept from fusing into
    multiply-adds, which Go does on arm64.
  - `zx scr encode --mode --palette` writes `.scr`, `.zm1`, `.zm2`, or
    appends to a `.cut`; `zx scr decode` reads them, with `--palette` and
    `--flash`; `zx scr cut --palette` tags assets cut from a ZXPAL1 or
    ZXPAL2 screen; `zx scr ls` shows the palette.
  - `FitTo` resizes to any size; `Fit` is `FitTo` at 256x192.
- **ZCUT version 2**: each asset records its palette, and an asset in
  ZXPAL3 or ZXPAL4 stores two attribute planes (ink, then paper). A
  collection of ZXPAL0 assets is still written as version 1, byte for byte
  as before, and version 1 files read as ZXPAL0. `Paste` refuses a
  two-byte asset, which a classic screen cannot hold.

### Unchanged

- `zx scr encode` without `--mode` or `--palette` uses the classic
  converter as before. That converter posterises before choosing colours,
  which turns normal colours (0xC8) into bright ones; the reference encoder
  with `--palette zxpal0` keeps both.

## [0.8.6] - 2026-09-02

### Fixed

- **A second, real half of the keyword-spacing bug, found by an
  independent tester (Jim Blimey) directly on real 128K/+3 hardware.**
  0.8.5 only ever removed a space stored *after* a keyword token; a
  space immediately *before* one -- exactly what a keyword right
  after a colon (`POKE 1,2: OUT 3,4`) or a closing quote
  (`LOAD "" CODE 100`) hits -- was still being stored and still
  doubling up on screen. Confirmed against the same real ZX Spectrum
  +3 fixture used for the first half of this fix: `loader.tok` has
  zero bytes between the colon and `PAPER`, zero between the closing
  quote and `CODE`. `pkg/basic/tokenise.go` now drops one space on
  either side of a keyword match, symmetrically.
- **A second, independent bug in `pkg/basic/detokenise.go`**, found
  while fixing the above: it only ever synthesised a space *after* a
  rendered token, never before, so a keyword with nothing stored on
  its leading side rendered running into whatever preceded it (`FOR
  I=1 TO 5` came back as `I=1TO 5`). Fixed symmetrically alongside the
  tokeniser fix, and re-verified: the exact tape reported as broken
  now detokenises clean, single-spaced throughout, matching real ROM
  `LIST` output on both 48K and (once its own separate, real
  `LOAD "T:"` vs `LOAD ""` quirk was accounted for) `+3` hardware.

Both fixes affect anywhere `pkg/basic.Tokenise`/`Detokenise` are used:
`totap`, `zx basic tokenise`/`detokenise`, and `zx tap make --basic`.

## [0.8.5] - 2026-09-01

### Fixed

- **A real, silent mis-tokenisation bug in `pkg/basic`**: `matchToken` was
  a bare prefix check with no word-boundary awareness, so any variable
  name that happened to start with a keyword -- `TOTAL` (contains
  `TO`), `FORMAT` (contains `FOR`), and many more -- was torn apart
  during tokenisation regardless of spacing. Fixed with a boundary
  check: a keyword match immediately followed by another letter is now
  treated as the start of a longer identifier, not a real keyword
  occurrence. A following digit is unaffected (confirmed against a
  real ZX Spectrum +3 fixture: `BORDER`/`PAPER` both precede a bare
  digit with no separator at all). The one genuinely ambiguous case --
  two keyword-shaped runs with no separator at all, e.g. `LETTOTAL` --
  is left as literal text rather than guessed wrong either way.
- **Tape listings no longer show doubled spaces after keywords.** A
  single source-typed space immediately after a keyword token was
  being stored, and real ROM `LIST` rendering always supplies its own
  space after a keyword token regardless of what's stored -- the two
  combined into a visible double space (`LET  TOTAL` instead of
  `LET TOTAL`). Exactly one following space is now dropped at
  tokenise time; a second, deliberate one is still kept. Verified
  against real emulator `LIST` output via `zx scr ocr` (pixel-exact
  ROM font matching, not eyeballed), not just this package's own round
  trip.

Both fixes affect anywhere `pkg/basic.Tokenise` is used: `totap`,
`zx basic tokenise`, and `zx tap make --basic`.

## [0.8.4] - 2026-09-01

### Added

- **`zx tap make --basic`**: tokenises a BASIC source file straight into a
  Program-block TAP, the `zx`-native equivalent of a standalone
  BASIC-to-tape tool. `--autostart LINE`, `--case-sensitive` (default
  case-insensitive, matching `zx basic tokenise`'s own convention).
- **`Makefile`**: `build`/`install`/`test`/`vet`/`fmt`/`lint`/`cover`/
  `tidy`/`verify`/`clean`/`release`/`cross-build`/`help`. The project
  never had one before.

### Fixed

- **A real autostart bug**, in both `totap` and the new `zx tap make
  --basic`: `--autostart` defaulted to `0`, but `0` is a genuinely
  valid BASIC line number, not a "no autostart" sentinel -- the real
  one, confirmed against the TAP spec and this package's own
  `EncodeProgram` test fixtures, is `32768`. Every BASIC tape made
  without an explicit `--autostart` was silently encoding "run from
  line 0," and Sinclair BASIC's own line-lookup falls through to the
  next existing line on no exact match -- so tapes were likely
  auto-running when nobody asked for that. `--autostart 0` is now a
  real, distinct choice from omitting the flag.
- **Tape block names now respect the real ZX Spectrum character set.**
  £ and © -- confirmed against the actual Spectrum charset, not
  assumed -- map to their genuine Sinclair code points (`0x60`,
  `0x7F`, replacing ASCII's grave accent and DEL at those positions)
  on encode, and decode back to the same characters rather than the
  ASCII bytes that happen to share those code points. Any other
  non-ASCII character becomes a plain asterisk instead of being
  silently corrupted by byte-level truncation.
- **`release.sh`/`syncver.sh` no longer duplicate `repoman relcore`/
  `repoman syncver`.** Both are now thin wrappers delegating entirely
  to gorepoman, so there's exactly one implementation of each, not two
  that can drift apart. `.repoman.json` populated for real (it was
  empty); the vendored, outdated (`0.8.0` vs. gorepoman's current
  `0.13.4`) Python `repoman/` copy removed, confirmed unreferenced by
  anything first. Along the way: `repoman relcore`'s archive builtin
  caught a real bug neither `release.sh` nor the prior `.repoman.json`
  exclude list caught -- a `.git/*` exclude pattern that only matches
  one path segment, silently packing the entire `.git` history into
  every checkpoint zip built this cycle. Fixed by switching to
  explicit archive sources instead of excluding after the fact.

### Removed

- **`docs/MIGRATION-STATUS.md`.** A point-in-time progress report for a
  consolidation effort its own text declared complete, several months
  stale, and not linked from anywhere a new reader would find it.

### Documentation

- **`docs/LIBRARY.md`**: `pkg/pzx`, `pkg/szx`, `pkg/rzx` now have their
  own sections (previously undocumented despite being released since
  0.7.0); the overview table and TOC list all ten packages correctly.
- **`docs/ARCHITECTURE.md`**: package tree and the `cmd/zx` subcommand
  list brought current (both were missing everything added since
  0.7.0: `pzx`, `rzx`, `scr`, `edit`, `build`).
- **`README.md`**: the Makefile is now mentioned.
- HTML renderings of all manuals regenerated to match.

## [0.8.3] - 2026-08-31

### Fixed

- `docs/SPRITE-HANDLING.md`'s compositing section rewritten to use only
  `zx scr` commands -- the previous version fell back to Python/PIL for
  image-level compositing, which had no place in a manual for this
  project's own tool. Tracing `pkg/scr`'s actual `Paste` implementation
  showed the fallback was unnecessary in the first place: a `color`
  chunk (the default `cut` output) pasted at a cell-aligned `--at`
  position (`x%8==0 && y%8==0`) writes its own attributes to the
  target directly. The earlier "attribute clash" example had paste
  positions that weren't cell-aligned, which is what actually produced
  the invisible result, not a real limitation of `paste` itself.
- Two more composited example screens added, reusing the same
  `assets.cut` collection with different `paste` arrangements, and a
  `--bitmap-only` mono-chunk example demonstrating the real,
  intentional case for attribute inheritance.
- The former `booth` chunk renamed to `window_red`, with a matching
  `window_white` cut alongside it -- both single-window tiles meant to
  be repeated in a grid, the same construction the reference artwork's
  own building actually uses. The third example scene rebuilt around
  this: two houses, one bigger than the other, each assembled from
  repeated window tiles rather than one building-sized chunk.

## [0.8.2] - 2026-08-31

### Added

- **`Makefile`**: `build`/`install` (all five commands), `test`
  (`-race`, matching CI exactly), `vet`/`fmt`/`fmt-check`/`lint`,
  `cover`, `tidy`/`verify`, `clean`, `release` (wraps `release.sh`),
  `cross-build`/`dist` (all 15 targets `cross-build.yml` covers), and
  a self-documenting `help`. The project never had one before.

### Fixed

- The three remaining `gofmt` issues (`pkg/build/build_test.go`,
  `pkg/snapshot/sna_test.go`, `pkg/snapshot/z80_test.go`) -- purely
  mechanical (struct alignment, blank-line removal, single-line-if
  expansion), no semantic change. The repo is now genuinely
  `gofmt`-clean throughout.
- `release.sh`'s own checkpoint-zip exclude list never excluded
  `dist/`, so running `make cross-build` before a release swept all
  75 cross-compiled binaries into the checkpoint (a 149MB zip instead
  of the usual ~600KB). Never triggered before `dist/` existed
  locally; fixed now.
- `docs/SPRITE-HANDLING.md`: removed a numpy/`scipy.ndimage`-based
  boundary-measurement section that had no business being in a
  user-facing manual -- that was a description of how this session
  measured tile boundaries while writing the tutorial, not an
  instruction for zentools' own users, who already know where they
  put their own graphics. The `zx scr` command examples themselves
  (`cut`, `atlas`, `paste`, the compositing walkthrough) are
  unchanged.

## [0.8.1] - 2026-08-31

### Added

- **`zx edit`**: `list` (plain and `--json`, sharing `zx convert
  --outdir`'s own manifest shape), `extract` (a block's payload, or
  `--raw` for the full flag+payload+checksum block), `delete`
  (single or comma-separated indices, against the tape's original
  numbering), `import` (a fresh `code`/`program` header+data pair, or
  `--raw` for an already-encoded block, at `--at N` or appended), and
  `append` (concatenate two or more tape files' own block lists, any
  mix of `tap`/`tzx`/`pzx`, into one output). All built on the same
  `[]tap.Block` list and the same encoders `zx convert`'s own tape
  normalisation already uses.
- **`zx build`**: build a multi-block tape from a JSON specification
  (`code`/`program`/`raw` block kinds; `file` paths resolve relative
  to the spec's own directory) -- the declarative counterpart to `zx
  edit`'s incremental operations.
- **`zx tap append`**: concatenate TAP files by raw byte
  concatenation -- TAP has no container structure of its own, so this
  needs no decode/re-encode round trip, unlike the general `zx edit
  append`.
- **Two new manuals**: [`docs/TAPE-HANDLING.md`](docs/TAPE-HANDLING.md),
  a full tutorial assembling a multi-block tape from BASIC, code, and
  tile/screen parts; [`docs/SPRITE-HANDLING.md`](docs/SPRITE-HANDLING.md),
  a tutorial on `zx scr`'s cut/paste/atlas workflow, including a
  numpy-based precise-boundary-measurement technique and a from-first-
  principles look at Spectrum attribute clash. Both illustrated with
  real screenshots in `docs/images/`.
- **HTML renderings** of all six manuals (`docs/*.html`, parallel to
  their `.md` sources), styled in the genuine Spectrum 8-colour
  palette, built with `docs/template.html`/`docs/zsp.css`.

### Fixed

- `docs/CLI.md` brought current: `zx pzx`, `zx rzx`, `zx edit`, `zx
  build` were entirely undocumented; the `zx convert` reference was
  missing most of its own flags (`--block`, `--org`, `--length`,
  `--bank`, `--origin`, `--name`, `--outdir`); the conversions-in-depth
  matrix only covered four of the now eight source/target kinds.

## [0.8.0] - 2026-08-31

### Added

- **`zx convert`**: `RZX → tap`/`tzx`/`pzx`, extracting the first embedded
  snapshot's user RAM the same way `sna`/`z80`/`szx → tape` already does
  (same registers/interrupt-state loss, same warning). The one remaining
  buildable gap from the RZX/PZX/SZX work: everything else an RZX could
  reasonably convert to was already wired up.
- **`zx convert --outdir`**: explode a multi-block tape (`tap`/`tzx`/`pzx`)
  into a directory -- one `.bin` per data block (named after its header
  when it has one, index-only for a bare/headerless block), plus
  `manifest.json` describing every block, header and data alike, in
  order with checksum status and header attribution.
- **`zx convert`**: `bin` as a source, not just a target -- `bin → tap`/
  `tzx`/`pzx`/`sna`/`z80`/`szx` via `--origin`/`--name`/`--start`,
  reachable through the same dispatch as every other conversion rather
  than only through the dedicated `make` commands.
- **`zx convert`**: a `PAGING` block (2 bytes: port `7FFD`, then port
  `1FFD`) alongside the existing per-bank blocks when a 128K-family
  snapshot converts to a tape format, so which bank was actually paged
  in survives the trip -- a tape format has no field of its own for it,
  but nothing stops a small dedicated metadata block, the same
  convention TZX itself uses for non-audio information.
- **`zx convert`**: a warning when a real (non-zero) port `1FFD` value is
  about to be silently dropped converting to `sna` specifically -- `.sna`
  genuinely has no field for the +3's secondary paging port (confirmed
  directly against `pkg/snapshot`'s own encoder/decoder), so the loss
  itself isn't fixable, but it no longer happens silently.

### Fixed

- **`zx convert`**: `PZX → tap`/`tzx`/`sna`/`z80`/`szx` no longer requires
  the source to use this package's own exact standard-timing pulse
  constants. A `DATA` block's payload is already fully-resolved bytes by
  the time PZX's own decoder produces it, regardless of what pulse
  timing represented them physically on tape -- a real, well-formed
  turbo-loader capture converts now, where it was previously silently
  dropped. The one place that already worked this way, `PZX → bin`,
  is what proved the fix.
- **`zx convert`**: `TZX → tap`/`pzx`/`sna`/`z80`/`szx`/`bin` now reads
  `0x11` (Turbo Speed Data) and `0x14` (Pure Data) blocks, not only
  `0x10` -- `pkg/tzx`'s own doc comment on `Block.Data` already said
  `0x11` "holds the raw payload bytes themselves, the same field `0x10`
  uses", which the conversion code simply hadn't acted on. Any real TZX
  using a turbo loader -- a large fraction of commercial tapes, which is
  the entire reason turbo loaders exist -- previously lost its data
  blocks converting to anything else.
- **`zx convert`**: same-format "conversion" (source and target the same
  format) is now rejected outright rather than silently copying the file
  or silently re-processing it through lossy normalisation. The latter
  was a real bug (`TZX → TZX`, and, before an adjacent fix, `PZX → PZX`,
  both went through timing-based filtering even though nothing should be
  lost for a same-format no-op) that a same-format identity pass would
  have papered over without actually fixing.
- **`zx convert`**: `TAP`/`SNA`/`Z80`/`SZX → pzx` no longer silently
  produces a TZX-encoded file under a `.pzx` name. Two independent
  instances of the identical bug -- `convertTapeToTape` and
  `convertSnapToTape` each had a `case "tap"` and otherwise assumed
  `"tzx"`, with no case for `"pzx"` even though it had been a valid tape
  format for several releases.

## [0.7.0] - 2026-08-31

### Added

- **`pkg/szx`: read and write `.szx` (zx-state) snapshots**, spec v1.5
  (https://www.spectaculator.com/docs/zx-state/intro.html). Decodes into
  and encodes from the same neutral `snapshot.MachineState` the SNA and
  `.z80` codecs already use -- a third codec for one type, not a separate
  representation. Covers the four blocks every snapshot needs
  (`ZXSTCREATOR`, `ZXSTZ80REGS`, `ZXSTSPECREGS`, `ZXSTRAMPAGE`, the last
  zlib-compressed); the spec's thirty-odd further peripheral block types
  (AY sound, joysticks, disk controllers, speech synthesis, and so on)
  are skipped on read rather than modelled, per the spec's own rule for
  unrecognised blocks. A file naming a machine model
  `snapshot.Model` doesn't represent (Pentagon 512/1024, Scorpion, Timex
  variants) or a RAM page beyond 7 is rejected with a clear error rather
  than partially read.
- **`pkg/rzx`: read and write `.rzx` input-recording files**, spec v0.13
  (https://worldofspectrum.net/RZXformat.html). Blocks decode into an
  ordered `[]Block`, not separated by type, since a real multiload
  recording legitimately interleaves Snapshot and Recording blocks and
  that sequence is part of the file's meaning. Creator, Snapshot
  (zlib-compressed), and Recording blocks (the actual frame-by-frame CPU
  `IN`-result log, zlib-compressed, with the "repeated frame" idle-frame
  optimisation) are fully supported. The DSA Security Information and
  Security Signature blocks used for tournament submissions are decoded
  into their raw fields (OpenPGP multi-precision-integer format for the
  signature) and re-encoded verbatim, but this package does not
  implement DSA signing or verification -- the spec's own security
  chapter calls itself "obsolete info, needs updating to DSA", and
  verifiable cryptographic signing is a different undertaking from
  reading and writing the container.
- **`pkg/pzx`: read and write `.pzx` tape files**, spec v1.0
  (http://zxds.raxoft.cz/docs/pzx.txt), Patrik Rak's simpler successor to
  TZX. All four mandatory block types (`PZXT`, `PULS`, `DATA`, `PAUS`)
  plus the two the spec calls out as should-be-supported (`BRWS`,
  `STOP`) are implemented, including the bit-packed pulse/duration
  encoding (an optional repeat-count prefix, and an extended-duration
  form for values over 15 bits) exactly as the spec's own decode
  pseudocode defines it -- including a real, easy-to-miss edge case the
  spec itself warns about and an early version of this implementation
  got wrong: a duration needing the full extended form must carry an
  explicit repeat-count prefix even when the count is 1, or the decoder
  cannot distinguish the extension prefix from a genuine repeat count.
  Caught by a round-trip test on the format's maximum duration value,
  not by inspection. Unrecognised block tags (the spec reserves
  lowercase tags for custom extensions) are preserved as raw bytes on
  decode and re-emitted verbatim, rather than dropped.

## [0.6.1] - 2026-08-31

### Fixed

- **`pkg/snapshot`: `EncodeSNA128` no longer silently produces a
  malformed image when `Paging.Port7FFD` selects bank 2 or bank 5 for
  the `0xC000` window** -- a real, valid hardware state (nothing in the
  128K paging hardware prevents aliasing an always-fixed bank into the
  switchable window too), but one the fixed 131103-byte `.sna128`
  layout has no documented way to represent losslessly: the format's
  own "remaining five banks" trailer section is only five banks wide,
  which assumes the paged bank is distinct from banks 5 and 2. When it
  isn't, the previous implementation wrote that bank's content twice
  (once as the fixed bank, once again as "the paged bank") and produced
  an image exactly one 16K bank too large (147487 bytes instead of
  131103) -- which `DecodeSNA128`'s own strict length check would then
  reject as "wrong size", a confusing failure two steps removed from
  its actual cause. `EncodeSNA128` now returns a clear error naming the
  aliased bank and address instead of guessing at an undocumented
  corner of a community-reverse-engineered format. Found and reported
  via real-world use in `github.com/ha1tch/zendis`.

## [0.6.0] - 2026-08-23

### Added

- **`pkg/tzx`: full coverage of all 25 SpecIDE-documented TZX block
  types** (previously a subset). Notable additions: 0x18 CSW Recording
  (zlib decompression via Go's stdlib `compress/zlib`, no external
  dependency, plus CSW RLE pulse decoding), 0x19 Generalized Data
  (structure/length validated; alphabet-driven pulse decoding
  deliberately deferred as an acknowledged scope boundary -- never
  observed in the newdiv 303-game/413-TZX validation corpus), 0x24/0x25
  Loop Start/End (real control-flow expansion at decode time -- the
  loop body is repeated inline in the decoded block stream), 0x23
  Jump/0x26 Call/0x27 Return (recognised, safe no-op, matching
  libspectrum's own stub-level support for these). Bounds-checking gaps
  in five original handlers (`idArchiveInfo`, `idTextDesc`,
  `idStopThe48K`, `idHardwareTyp`, `idGroupStart`) found and fixed via
  libspectrum's own `invalid-hardwareinfo.tzx` test fixture.
  `DecodeBlock(raw []byte) (Block, error)` exported from `pkg/tap`,
  parsing a single raw flag+payload+checksum block with no length
  prefix -- used both for `.tap` files and for a TZX 0x10 block's own
  payload, which is structurally identical.
- **`pkg/scr`: OCR** (`ocr.go`) -- reads text from a rendered ZX
  Spectrum screen by matching each 8x8 character cell against the real
  ROM font, for driving emulators through screenshots without a human
  reading images by eye. Built on a new, standalone **`pkg/bdf`**
  package: a BDF (Glyph Bitmap Distribution Format) bitmap font reader,
  parsing glyph blocks and rasterising each to a full-cell RGBA pixmap.
  `zx scr ocr` added as a new CLI subcommand.
- Validated against the newdiv corpus (303 commercial games, 413 TZX +
  404 TAP files, downloaded from archive.org's ZX Spectrum Top 100
  collection): TZX decode 100% clean (413/413); TAP decode 99.5%
  (401/403 -- the two failures are both Operation Wolf variants with a
  trailing 1-byte garbage block, correctly rejected).

## [0.5.0] - 2026-06-30

### Added

- `pkg/scr`: a new package for ZX Spectrum screens (`.scr`, the 6912-byte
  display file) and a companion asset-collection format. It provides:
  - `Screen` encode/decode with the authoritative palette (dim `0xC8`, bright
    `0xFF`), an image converter (`FromImage`/`ToImage`), and PNG/JPEG/GIF
    decoding with `none`/`stretch`/`bestfit`/`centre` resize modes.
  - An attribute-aware image reducer that posterises, snaps to the Spectrum
    palette, and selects the two most frequent colours per cell by palette
    index. Tallying by index (rather than by index-and-brightness) keeps a
    feature whose antialiased pixels straddle both brightness sets from
    self-competing, and a uniform cell resolves to solid paper with no set bits.
  - `Crop`, `AutoExtent`, and `BitmapExtent` for trimming a screen or finding
    the bounding box of its set bits.
  - The ZCUT asset-collection format (`pkg/scr/cutout.go`): a container of N
    named, heterogeneous assets, each carrying a linear bitmap and optional
    attributes, with a validated chunk layout (`ZCUT` magic, per-asset `IMAG`
    chunks padded to an 8-byte stride). Files use the `.cut` extension (8.3-safe
    for the +3DOS filesystem); the in-file magic and the colloquial name remain
    ZCUT.
  - Asset operations: `CutRegion`/`CutCells` (attributes retained only on
    cell-aligned cuts), `Paste` with a `PasteOp` bit-operation mode
    (`PasteOR` the default, plus `PasteAND`, `PasteCOPY`, `PasteXOR`),
    `ApplyMask`, and `AssetToImage` for rendering a single asset.
- `cmd/zx`: a `scr` command group for the above.
  - `zx scr encode` / `decode` - convert an image to a `.scr` and back.
  - `zx scr crop` - trim a screen or image, with `--auto`, `--bits`,
    `--with-attributes`, and `--bitmap-only` modes.
  - `zx scr cut` / `paste` / `ls` - build a `.cut` collection by appending named
    regions, composite assets back onto a screen (with `--op or|and|copy|xor`
    and a `--set-attr` recolour), and list a collection. The `ls` listing leads
    with a 0-based chunk index and a `PIXMAP` column whose values are
    `color`, `mono`, or `mask`.
  - `zx scr atlas` - render a collection as a labelled contact sheet (a built-in
    5x7 bitmap font keeps the command dependency-free).
  - `zx scr fromsnap` - extract the display file from a `.sna` or `.z80`
    snapshot (48K or 128K, currently-paged screen) to a `.scr`.

### Changed

- The version package documentation referred to zenas (the project it was lifted
  from); corrected to zentools.

### Added

- Continuous integration. A `test` workflow runs the build, `go vet`, and the
  race-enabled test suite natively on the GitHub-hosted Linux runners (x86-64 and
  arm64). A `cross-build` workflow verifies that the code cross-compiles, with
  cgo disabled, for every other supported target: macOS (amd64 and arm64),
  Windows (amd64 and 386), 32-bit Linux, the Raspberry Pi ARM variants (ARMv6,
  ARMv7, and arm64), and the BSDs (FreeBSD, OpenBSD, NetBSD, and DragonFly).
- Release automation. A `release` workflow, triggered by a version tag (or run
  manually against an existing tag), cross-compiles every command for all
  supported targets, packages them one archive per platform (`.tar.gz`, or
  `.zip` for Windows), and attaches the archives to the GitHub release.
- CI actions pinned to their Node 24 era major versions (`actions/checkout@v5`,
  `actions/upload-artifact@v5`, `actions/download-artifact@v5`), clearing the
  Node 20 deprecation warning ahead of its removal from the runners.

## [0.4.0] - 2026-06-29

### Added

- `pkg/build`: new package, moved in from zenas. Turns machine-code bytes into
  loadable artifacts by overlaying them onto a real booted machine state (one
  embedded boot snapshot per model) and emitting tapes (`.tap`/`.tzx`, with an
  optional BASIC auto-run loader) and snapshots (`.sna`/`.z80`). This is the
  shared procedure behind both `zenas build` and `zx snap`.
- `cmd/zx`: a modern, unified command organised by format. Subcommands:
  - `zx tap`   - create and inspect TAP images (with optional auto-run loader).
  - `zx tzx`   - create and inspect TZX images.
  - `zx basic` - tokenise and detokenise ZX BASIC.
  - `zx snap`  - create runnable `.sna`/`.z80` snapshots from a binary, and
    inspect existing snapshots.
  - `zx convert` - convert between tape and snapshot formats (see below).
  - `zx info`  - auto-detect a file's format and summarise it.
  - Flags may appear in any position, not only before positional arguments.

### Conversions

`zx convert` covers the format matrix. Conversions within a kind are lossless:
`tap` and `tzx` interconvert exactly (standard-speed blocks; custom-loader and
structural blocks have no TAP equivalent and are dropped with a note), and
`sna` and `z80` interconvert as the same machine state. Across kinds they are
asymmetric: a tape carries no entry point, so `tape -> snapshot` requires a
`--start` address; a snapshot has no block structure, so `snapshot -> tape`
emits RAM as a CODE block (a memory dump, not a runnable program) and warns.

## [0.3.0] - 2026-06-29

### Added

- Command-line tools in `cmd/`, providing drop-in replacements for the
  zxgotools utilities on top of the zentools packages:
  - `maketap` - create a TAP file from a binary as a CODE block.
  - `totap` - convert a binary or a BASIC text file to TAP.
  - `loadtap` - read and analyse a TAP file (`-d` hex dump, `-r` raw output).
  - `tap2tzx` - convert one or more TAP files to a single TZX, with metadata,
    hardware-info, grouping, and multiload stop blocks.
- `pkg/tzx`: hardware-type (0x33) and group (0x21/0x22) blocks, per the TZX
  v1.20 specification, with the full hardware type/ID constant set. `EncodeOptions`
  gains `Hardware` and `Group`; `Decode` now reads these blocks, so the package
  round-trips everything it writes.
- `pkg/tzx`: a `Writer` type for assembling a TZX image block by block, for
  callers that interleave several tape images with structural blocks (e.g.
  multi-file concatenation). `EncodeFromTAP` is now implemented on top of it.

### Changed

- The `tap2tzx` configuration file (`-c`) is JSON rather than the YAML used by
  the zxgotools original. This keeps zentools dependency-free. The schema is
  otherwise equivalent (`metadata`, `hardware`, `blocks`).

### Fixed

- The `tap2tzx` and `maketap` CODE headers carry the standard param2 value of
  0x8000; the zxgotools originals wrote 0, contrary to their own documentation.
- `totap --basic` produces a valid Program tape; the zxgotools original hung on
  the BASIC conversion path.

## [0.2.0] - 2026-06-29

### Added

- `pkg/snapshot`: new package. Reads and writes ZX Spectrum snapshots via a
  neutral `MachineState` (registers, memory banks, paging, IO) with no emulator
  coupling. Supports:
  - `.sna` 48K (`EncodeSNA`/`DecodeSNA`) and 128K (`EncodeSNA128`/`DecodeSNA128`),
    including the 48K pushed-PC convention and the 128K paging trailer.
  - `.z80` v1 (`EncodeZ80`/`DecodeZ80`) with the v1 RLE scheme, and v2/v3
    extended-header reading plus v3 writing (`EncodeZ80v3`), covering 48K and
    128K machines and per-page compressed memory blocks.
  - Validated against real third-party files: z88dk `.sna` (48K and 128K), and
    `.z80` snapshots spanning v1 (Jet Set Willy), v2 (Manic Miner), and v3 128K.
    The v1 RLE decode is byte-identical to an independent decoder across a full
    48 KiB game image.

### Notes

- Per the format-ownership decision (see doc/ARCHITECTURE.md), zentools owns the
  portable interchange formats; `.zxs` remains native to zenzx and `.dsk` to
  plus3.

## [0.1.2] - 2026-06-28

### Added

- `pkg/basic`: new package. Tokenises and detokenises Sinclair BASIC programs
  (48K and 128K keywords). `Tokenise(src, opts...)`, `Detokenise(prog)`, and
  `LooksTokenised(data)`. Built on the plus3 tokeniser/detokeniser, decoupled
  from the disk-image type, with a case-sensitivity option. Verified against real
  +3 BASIC program fixtures and a tokenise/detokenise round trip. A leading minus
  on a number is correctly treated as the subtraction operator followed by a
  positive number (the ROM behaviour), not a signed single number.

## [0.1.1] - 2026-06-28

### Added

- `pkg/tap`: `Decode` parses a TAP image into typed blocks (header fields parsed,
  per-block checksum verified). Verified against pasmo-produced TAP files.
- `pkg/tzx`: new package. Reads and writes TZX tape images, dependency-free and
  in-memory. `EncodeFromTAP` wraps a TAP image's blocks in standard-speed (0x10)
  blocks with optional archive-info, text-description, and stop-the-48K metadata;
  `Decode` parses them back. Output is byte-identical to zxgotools' tap2tzx for
  the same input.
- `doc/ARCHITECTURE.md`: package layout, per-package API shape, dependency rules,
  source provenance, and the verification standard for the library.

## [0.1.0] - 2026-06-28

### Added

- `pkg/tap`: read and write ZX Spectrum TAP files. In-memory core (`EncodeCode`,
  `EncodeProgram`) returning complete TAP bytes, plus a `WriteCodeFile`
  file-to-file helper. The CODE-block encoding is verified byte-identical to
  pasmo's `--tap` output.
- Project scaffolding: versioning (VERSION + pkg/version, syncver.sh),
  release pipeline (release.sh), Apache-2.0 license, and a .gitignore that
  excludes build output (binaries are never committed).

### Notes

- The TAP encoding was scavenged from github.com/ha1tch/zxgotools, restructured
  around an in-memory API and corrected to write the conventional 0x8000 in a
  CODE block's second parameter.
