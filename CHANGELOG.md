# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
