# zentools

A library of ZX Spectrum binary-format tools written in Go: tape images,
snapshots, and related formats, packaged as reusable, dependency-free packages
that other tools (assemblers, emulators, disk utilities) can import.

## Status

Early. The first package is `pkg/tap`, with TZX, BASIC tokenisation, and
snapshot formats to follow.

## Packages

### pkg/tap

Reads and writes ZX Spectrum TAP files. The core works in memory:

```go
import "github.com/ha1tch/zentools/pkg/tap"

// Wrap assembled bytes as a CODE block loaded at 0x8000.
image := tap.EncodeCode("mygame", code, 0x8000)
os.WriteFile("mygame.tap", image, 0o644)
```

`EncodeCode` and `EncodeProgram` return the complete TAP bytes; `WriteCodeFile`
is a file-to-file convenience wrapper. The CODE-block encoding is verified
byte-identical to pasmo's `--tap` output.

### pkg/build

Turns machine-code bytes into loadable artifacts. It overlays the code onto a
real booted machine state — one embedded boot snapshot per model — and emits
tapes (`.tap`/`.tzx`, optionally with a BASIC auto-run loader) and snapshots
(`.sna`/`.z80`) that load and run at a given entry point. This is the shared
procedure behind both `zenas build` and `zx snap`.

## Commands

The `cmd/` directory provides command-line tools built on the packages:

- `maketap`, `totap`, `loadtap`, `tap2tzx` - drop-in replacements for the
  zxgotools utilities of the same names, preserving their interfaces while
  running on the zentools packages.
- `zx` - a modern, unified front-end organised by format, with subcommands
  `tap`, `tzx`, `basic`, `snap`, `convert`, and `info`. It adds capabilities
  the older tools lacked, such as creating runnable snapshots from a binary and
  converting between tape and snapshot formats.

## Requirements

- Go 1.25 or later

## Building

```sh
go build ./...
go test ./...
```

## License

Licensed under the Apache License, Version 2.0. See the LICENSE file.
