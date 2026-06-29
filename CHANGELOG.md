# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.1] - 2026-06-29

### Added

- Continuous integration. A `test` workflow runs the build, `go vet`, and the
  race-enabled test suite natively on Linux, macOS, and Windows (x86-64 and
  arm64). A `cross-build` workflow verifies that the code cross-compiles for the
  targets without a hosted runner: the Raspberry Pi ARM variants (ARMv6, ARMv7,
  and arm64) and the BSDs (FreeBSD, OpenBSD, NetBSD, and DragonFly). The race
  detector is enabled everywhere it is supported and skipped on Windows arm64,
  where it is not.

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
