# zentools

[![test](https://github.com/ha1tch/zentools/actions/workflows/test.yml/badge.svg)](https://github.com/ha1tch/zentools/actions/workflows/test.yml)
[![cross-build](https://github.com/ha1tch/zentools/actions/workflows/cross-build.yml/badge.svg)](https://github.com/ha1tch/zentools/actions/workflows/cross-build.yml)

A Go library and command-line toolkit for ZX Spectrum file formats: tapes
(TAP, TZX, PZX), snapshots (`.sna`, `.z80`, `.szx`), RZX recordings, BASIC,
`.scr` screens and `.cut` asset collections, the ZXMODE screen modes and
ZXPAL palettes, and the Spectrum Next's graphics and NEX files. No
third-party dependencies.

```sh
zx tap make game.bin --origin 0x8000 --loader --start 0x8000 -o game.tap
zx snap make game.bin --start 0x8000 --model 48k --z80 -o game
zx convert game.tap -o game.tzx
zx scr encode title.png -o title.scr
zx next nex build --bin 0x8000=game.bin --pc 0x8000 -o game.nex
zx info anything.tzx
```

## Tools

`maketap`, `totap`, `loadtap`, and `tap2tzx` are legacy,
[zxgotools](https://github.com/ha1tch/zxgotools)-compatible drop-in
replacements (same interfaces, corrected behaviour, no external
dependencies) kept for existing scripts; new work should use `zx` below.
Full flag reference for all four is in the [CLI tools manual](docs/CLI.md).

`zx` is the unified command, organised by format:

| Command      | Does                                                       |
| ------------ | ---------------------------------------------------------- |
| `zx tap`     | make, inspect, or append TAP images                        |
| `zx tzx`     | make and inspect TZX images                                |
| `zx pzx`     | make (from a TAP) and inspect PZX images                   |
| `zx basic`   | tokenise and detokenise ZX BASIC                           |
| `zx scr`     | convert, crop, and manage `.scr` screens and sprite/asset collections; ZXMODE pictures and ZXPAL palettes; recognise on-screen text |
| `zx next`    | ZX Spectrum Next graphics and programs: palettes, sprites and their attributes, Layer 2, LoRes and Timex screens, tiles and tilemaps from images (with colour reduction), drawn back to PNG; NEX files inspected, unpacked and built |
| `zx zx0`     | ZX0 compression and decompression, byte for byte as the reference compressor |
| `zx snap`    | build a runnable snapshot (`.sna`/`.z80`/`.szx`) from a binary, or inspect one |
| `zx rzx`     | inspect RZX input-recording files                          |
| `zx convert` | convert between tape/snapshot formats, and extract a flat `.bin` payload from either |
| `zx edit`    | list, extract, delete, import, or append blocks/tapes in a multi-block tape (`.tap`/`.tzx`/`.pzx`) |
| `zx build`   | build a multi-block tape file from a JSON specification    |
| `zx info`    | identify and summarise a file                              |

Three things go beyond conversion. `zx scr` is the whole screen and sprite
side: images to and from `.scr`, regions cut into `.cut` collections for
reuse, the ZXMODE linear screens and ZXPAL palettes (its `encode` is the
specification's reference encoder, pixel for pixel), and `ocr`, which reads
the text on a screen against the ROM font. `zx next` turns images into the
Next's palettes, sprites, Layer 2, LoRes and Timex screens and tiles with
their tilemaps, and builds NEX files; its output matches gfx2next, nextbmp,
nextraw and sjasmplus byte for byte. `zx zx0` compresses as the reference
compressor does.

## Documents

| Manual | What is in it |
|---|---|
| [CLI tools](docs/CLI.md) | Every command and flag, one of everything by example, the conversion table and what each conversion costs |
| [Tape handling](docs/TAPE-HANDLING.md) | Building, combining and editing multi-block tapes, with a worked tutorial |
| [Sprite handling](docs/SPRITE-HANDLING.md) | Screens, tiles and sprite collections, with a tutorial on real Spectrum artwork |
| [ZXMODE handling](docs/ZXMODE-HANDLING.md) | The ZXMODE screen modes and ZXPAL palettes: fitting, encoding and measuring pictures, palette design |
| [Next handling](docs/NEXT-HANDLING.md) | Spectrum Next palettes, sprites, screens, tiles and tilemaps, ZX0 and NEX files, worked through for a small game |
| [Library](docs/LIBRARY.md) | The packages, their types, examples, and what each codec does and does not model |
| [Architecture](docs/ARCHITECTURE.md) | How the code is organised |

Each has an HTML twin beside it in `docs/`.

## Install

```sh
go get github.com/ha1tch/zentools@latest   # library
go build ./...                             # command-line tools
```

Requires Go 1.25 or later. A `Makefile` covers the rest of the working
loop -- `make build`, `make test`, `make lint`, `make release
VERSION=x.y.z`, `make cross-build`; run `make help` for the full list.

## Library

Fifteen packages, each owning one format family, depending only on the
standard library. The [library manual](docs/LIBRARY.md) has the API.

Fifteen packages, each owning one format family, depending only on the
standard library:

| Package        | Does                                                  |
| -------------- | ---------------------------------------------------- |
| `pkg/tap`      | read and write TAP images                            |
| `pkg/tzx`      | read and write TZX images (v1.20)                    |
| `pkg/pzx`      | read and write PZX images (v1.0), the simpler TZX successor |
| `pkg/basic`    | tokenise and detokenise ZX BASIC (48K and 128K)      |
| `pkg/snapshot` | read and write `.sna` and `.z80` via a neutral state |
| `pkg/szx`      | read and write `.szx` (zx-state) via the same neutral state |
| `pkg/rzx`      | read and write `.rzx` input-recording files          |
| `pkg/build`    | overlay code onto a boot state, emit tapes/snapshots |
| `pkg/scr`      | `.scr` screens, ZXMODE pictures and ZXPAL palettes, image conversion and fitting, `.cut` asset collections, OCR |
| `pkg/zxn`      | Spectrum Next palettes, sprites, Layer 2, LoRes, Timex screens and tiles |
| `pkg/nex`      | read and write NEX files                             |
| `pkg/zx0`      | ZX0 compression and decompression                    |
| `pkg/bdf`      | read BDF bitmap fonts (the ROM font for `pkg/scr`'s OCR) |
| `pkg/symfile`  | read and write other tools' symbol files (CSpect maps, pasmo, sjasmplus, z88dk) |
| `pkg/version`  | the library version constant                         |

## License

Apache License 2.0. See LICENSE, or https://www.apache.org/licenses/LICENSE-2.0.

Copyright (c) 2026 haitch <h@ual.li>
