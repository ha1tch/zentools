# zentools

[![test](https://github.com/ha1tch/zentools/actions/workflows/test.yml/badge.svg)](https://github.com/ha1tch/zentools/actions/workflows/test.yml)
[![cross-build](https://github.com/ha1tch/zentools/actions/workflows/cross-build.yml/badge.svg)](https://github.com/ha1tch/zentools/actions/workflows/cross-build.yml)

A Go library and command-line toolkit for converting and manipulating ZX
Spectrum file formats: TAP, TZX, PZX, snapshots (`.sna`, `.z80`, `.szx`),
RZX input recordings, BASIC, and `.scr` screens. No third-party
dependencies.

- **[CLI tools manual](docs/CLI.md)** — every command and flag, with the
  conversion table and what each conversion costs.
- **[Tape handling manual](docs/TAPE-HANDLING.md)** — building, combining,
  and editing multi-block tapes, with a full worked tutorial.
- **[Sprite handling manual](docs/SPRITE-HANDLING.md)** — screens, tiles,
  and sprite collections, with a tutorial using real ZX Spectrum artwork.
- **[Library manual](docs/LIBRARY.md)** — packages, types, and examples.

Beyond tape and snapshot conversion, `zx scr` covers the whole screen and
sprite side: `encode`/`decode` convert between ordinary images and the
native 6912-byte `.scr` format, `crop` pulls out a region as PNG, and
`fromsnap` extracts a display straight out of a snapshot. `cut`, `paste`,
`ls`, and `atlas` manage `.cut` files — named collections of extracted
sprites and tiles, pulled from one or more screens, meant for reuse
across a project. `ocr` recognises the text on a screen against the real
Spectrum ROM font and prints it as plain text, for driving emulators
through captured screenshots without a human reading each one by eye.

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
| `zx scr`     | convert, crop, and manage `.scr` screens and sprite/asset collections; recognise on-screen text |
| `zx snap`    | build a runnable snapshot (`.sna`/`.z80`/`.szx`) from a binary, or inspect one |
| `zx rzx`     | inspect RZX input-recording files                          |
| `zx convert` | convert between tape/snapshot formats, and extract a flat `.bin` payload from either |
| `zx edit`    | list, extract, delete, import, or append blocks/tapes in a multi-block tape (`.tap`/`.tzx`/`.pzx`) |
| `zx build`   | build a multi-block tape file from a JSON specification    |
| `zx info`    | identify and summarise a file                              |

### Usage: the legacy tools

```
# binary to TAP, named, loading at 0x8000
maketap --name game --address 32768 game.bin game.tap

# binary to TAP (CODE block)
totap --binary --name game --address 32768 game.bin game.tap

# BASIC text to an auto-running TAP
totap --basic --name loader --autostart 10 loader.bas loader.tap

# list a tape's blocks; -d adds a hex dump
loadtap game.tap
loadtap -d game.tap

# extract a tape's raw data, skipping headers
loadtap -r game.tap > game.payload

# one TAP to TZX with metadata
tap2tzx -o game.tzx -m --title "My Game" --author "haitch" --year 2026 game.tap

# several TAPs to one 128K multiload TZX, grouped
tap2tzx -o game.tzx -128 --multiload --group "Game" part1.tap part2.tap part3.tap
```

### Usage: zx

```
# make a TAP from a binary, with a BASIC auto-run loader
zx tap make game.bin --origin 0x8000 --loader --start 0x8000 -o game.tap

# inspect a tape
zx tap info game.tap

# combine several TAP files into one -- raw byte concatenation is valid
# for pure TAP (no container structure of its own); for mixed formats
# (or TZX/PZX inputs), use zx edit append instead
zx tap append loader.tap main.tap level1.tap -o game.tap

# wrap a TAP as TZX with archive metadata
zx tzx make game.tap --title "My Game" --author "haitch" --year 2026 -o game.tzx

# tokenise BASIC source, then read it back
zx basic tokenise loader.bas -o loader.bin
zx basic detokenise loader.bin

# build a runnable snapshot from a binary, in any of three formats
zx snap make game.bin --start 0x8000 --model 48k --sna --z80 --szx -o game
zx snap info game.z80

# wrap a TAP as PZX, using the standard Spectrum ROM pilot/sync/bit timings
zx pzx make game.tap --title "My Game" -o game.pzx
zx pzx info game.pzx

# inspect an RZX input-recording file (frame counts, embedded snapshots)
zx rzx info session.rzx

# convert between formats (extensions decide source and target)
zx convert game.tap -o game.tzx              # lossless
zx convert game.sna -o game.z80              # lossless
zx convert game.tap -o game.z80 --start 0x8000   # tape to snapshot needs --start
zx convert game.pzx -o game.tap              # only PULS+DATA pairs using standard
                                              # ROM timing convert; anything else is dropped

# extract a flat binary -- no headers, no registers, just bytes
zx convert game.tap -o code.bin                          # first Code-type block
zx convert game.tap -o code.bin --block-name MAIN         # a specific block, by name
zx convert game.z80 -o code.bin                          # from the snapshot's own PC
zx convert game.z80 -o code.bin --org 0x8000 --length 0x2000
zx convert game128.sna -o bank.bin --bank 6              # a 128K bank not currently paged in

# wrap a flat binary as a tape or snapshot -- the reverse of extraction;
# --origin supplies what a raw binary has no way to carry itself
zx convert code.bin -o code.tap --name GAME --origin 0x8000
zx convert code.bin -o code.sna --origin 0x8000 --start 0x8000

# pull a snapshot back out of an RZX recording (the first embedded one;
# --org/--length/--bank all apply the same way as for a real snapshot source)
zx convert session.rzx -o extracted.z80      # same format as embedded, verbatim
zx convert session.rzx -o extracted.sna      # different format: decode, re-encode
zx convert session.rzx -o code.bin --length 0x100

# explode a multi-block tape into a directory: one .bin per data block
# (named after its header, if it has one -- some tapes have bare,
# headerless data blocks too, e.g. for a custom fast loader) plus a
# manifest.json describing every block, header and data alike, in order
zx convert game.tap --outdir extracted/

# list, extract, delete, import, or append blocks in an existing multi-block tape
zx edit list game.tap                                # plain text
zx edit list game.tap --json                         # same shape as --outdir's manifest.json
zx edit extract game.tap --block 3 -o code.bin        # one block's payload
zx edit extract game.tap --block 0 --raw -o hdr.bin   # the full raw block, for re-importing elsewhere
zx edit delete game.tap --block 2,3 -o game2.tap      # remove a header+data pair
zx edit import game.tap --data new.bin --name EXTRA --org 0xA000 -o game2.tzx
zx edit import game.tap --data hdr.bin --raw --at 0 -o game2.tap  # re-insert a --raw-extracted block
zx edit append loader.tap main.tzx level.pzx -o combined.tzx  # combine files of DIFFERENT tape
                                                                # formats -- unlike shell `cat`,
                                                                # which only works for bare TAP

# build a fresh multi-block tape from a JSON spec -- "file" paths resolve
# relative to the spec file's own directory, not the current directory
cat > game.json <<'JSON'
{
  "title": "My Game",
  "blocks": [
    {"kind": "code", "name": "LOADER", "org": 32768, "file": "loader.bin"},
    {"kind": "program", "name": "BASIC", "autostart": 10, "file": "basic.bin"}
  ]
}
JSON
zx build game.json -o game.tzx
```

`zx edit`/`zx build` operate on a tape's flat *logical* block list -- the loadable header/data blocks themselves, the same list `zx convert`'s own tape normalisation already works with. A TZX's structural blocks (pauses, group markers, `STOP`) and a PZX's own `PAUS`/`BRWS`/`STOP` blocks aren't part of that list and don't survive a `delete`/`import`/`build` round trip -- the same scope boundary `zx convert` already has converting a tape to another tape format.

```

# identify any file
zx info game.tzx

# encode an image to a screen and back; bestfit preserves aspect
zx scr encode logo.png --resize=bestfit --fillattr="paper:black" -o logo.scr
zx scr decode logo.scr -o logo.png

# extract the screen from a snapshot
zx scr fromsnap game.z80 -o title.scr

# cut named regions into a .cut collection, list it, render a contact sheet
zx scr cut --cells 13,3,8,11 --name gem title.scr -o parts.cut
zx scr cut --cells 2,14,8,5  --name logo title.scr -o parts.cut
zx scr ls parts.cut
zx scr atlas parts.cut --scale 4 -o parts-atlas.png

# composite an asset back onto a screen (AND a mask, then OR the data)
zx scr paste parts.cut:mask --at 96,80 --op and screen.scr -o tmp.scr
zx scr paste parts.cut:data --at 96,80 --op or  tmp.scr    -o out.scr
```

Flags may appear in any position. See the **[CLI tools manual](docs/CLI.md)** for
every flag and a full account of the conversions.

## Install

```sh
go get github.com/ha1tch/zentools@latest   # library
go build ./...                             # command-line tools
```

Requires Go 1.25 or later.

## Library

Ten packages, each owning one format family, depending only on the standard
library:

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
| `pkg/scr`      | read/write `.scr` screens and `.cut` asset collections |
| `pkg/version`  | the library version constant                         |

`pkg/szx` decodes into and encodes from the same `snapshot.MachineState`
the SNA/`.z80` codecs use -- a third codec for one neutral type, not a
separate representation. Its scope covers the CPU/paging/memory state
every snapshot needs (`ZXSTZ80REGS`/`ZXSTSPECREGS`/`ZXSTRAMPAGE`); the
zx-state spec's thirty-odd further peripheral block types (AY sound,
joysticks, disk controllers, and so on) are skipped on read rather than
modelled, per the spec's own rule for unrecognised blocks -- see the
package's own doc comment for the exact boundary. `pkg/rzx` fully
supports the Creator/Snapshot/Recording blocks that make up a working,
unprotected recording; the DSA security blocks used for tournament
submissions are read and written verbatim but not cryptographically
verified. `pkg/pzx` implements all four mandatory block types (`PZXT`/
`PULS`/`DATA`/`PAUS`) plus the two the spec calls out as
should-be-supported (`BRWS`/`STOP`); unrecognised block tags are
preserved as raw bytes rather than dropped, since the spec explicitly
reserves lowercase tags for custom extensions implementations are
expected to round-trip.

See the **[library manual](docs/LIBRARY.md)** for the API and examples.

## Documentation

- **[CLI tools manual](docs/CLI.md)**
- **[Tape handling manual](docs/TAPE-HANDLING.md)**
- **[Sprite handling manual](docs/SPRITE-HANDLING.md)**
- **[Library manual](docs/LIBRARY.md)**
- **[Architecture](docs/ARCHITECTURE.md)**

## License

Apache License 2.0. See LICENSE, or https://www.apache.org/licenses/LICENSE-2.0.

Copyright (c) 2026 haitch <h@ual.li>
