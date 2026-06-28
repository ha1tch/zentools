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

## Requirements

- Go 1.25 or later

## Building

```sh
go build ./...
go test ./...
```

## License

Licensed under the Apache License, Version 2.0. See the LICENSE file.
