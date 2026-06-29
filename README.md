# zentools

A Go library and command-line toolkit for converting and manipulating ZX
Spectrum file formats: TAP, TZX, snapshots (`.sna`, `.z80`), and BASIC. No
third-party dependencies.

- **[CLI tools manual](docs/CLI.md)** — every command and flag, with the
  conversion table and what each conversion costs.
- **[Library manual](docs/LIBRARY.md)** — packages, types, and examples.

## Tools

Four focused tools:

| Tool      | Does                                              |
| --------- | ------------------------------------------------- |
| `maketap` | binary to TAP (a CODE block)                      |
| `totap`   | binary or BASIC text to TAP                       |
| `loadtap` | inspect a TAP, or extract its raw data            |
| `tap2tzx` | one or more TAPs to a single TZX, with metadata   |

These are drop-in replacements for the [zxgotools](https://github.com/ha1tch/zxgotools)
utilities of the same names: same interfaces, corrected behaviour, no external
dependencies. zxgotools is deprecated in favour of zentools.

And `zx`, a unified command organised by format:

| Command      | Does                                                       |
| ------------ | ---------------------------------------------------------- |
| `zx tap`     | make and inspect TAP images                                |
| `zx tzx`     | make and inspect TZX images                                |
| `zx basic`   | tokenise and detokenise ZX BASIC                           |
| `zx snap`    | build a runnable snapshot from a binary, or inspect one    |
| `zx convert` | convert between any tape and snapshot format               |
| `zx info`    | identify and summarise a file                              |

### Usage: the focused tools

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

# wrap a TAP as TZX with archive metadata
zx tzx make game.tap --title "My Game" --author "haitch" --year 2026 -o game.tzx

# tokenise BASIC source, then read it back
zx basic tokenise loader.bas -o loader.bin
zx basic detokenise loader.bin

# build a runnable snapshot from a binary, in both formats
zx snap make game.bin --start 0x8000 --model 48k --sna --z80 -o game
zx snap info game.z80

# convert between formats (extensions decide source and target)
zx convert game.tap -o game.tzx              # lossless
zx convert game.sna -o game.z80              # lossless
zx convert game.tap -o game.z80 --start 0x8000   # tape to snapshot needs --start

# identify any file
zx info game.tzx
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

Six packages, each owning one format family, depending only on the standard
library:

| Package        | Does                                                  |
| -------------- | ---------------------------------------------------- |
| `pkg/tap`      | read and write TAP images                            |
| `pkg/tzx`      | read and write TZX images (v1.20)                    |
| `pkg/basic`    | tokenise and detokenise ZX BASIC (48K and 128K)      |
| `pkg/snapshot` | read and write `.sna` and `.z80` via a neutral state |
| `pkg/build`    | overlay code onto a boot state, emit tapes/snapshots |
| `pkg/version`  | the library version constant                         |

See the **[library manual](docs/LIBRARY.md)** for the API and examples.

## Documentation

- **[CLI tools manual](docs/CLI.md)**
- **[Library manual](docs/LIBRARY.md)**
- **[Architecture](doc/ARCHITECTURE.md)**

## License

Apache License 2.0. See LICENSE, or https://www.apache.org/licenses/LICENSE-2.0.

Copyright (c) 2026 haitch <h@ual.li>