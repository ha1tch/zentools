# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
