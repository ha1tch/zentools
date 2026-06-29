# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
