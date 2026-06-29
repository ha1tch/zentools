# zentools command-line tools

This manual documents every zentools command in detail: the focused drop-in
tools (`maketap`, `totap`, `loadtap`, `tap2tzx`) and the unified `zx` front-end.
It also explains, at length, what each conversion does and what it costs, since
the formats involved carry different kinds of information and not every
conversion is lossless.

For the programmatic interface behind these tools, see the
[library manual](LIBRARY.md).

## Contents

- [Concepts: tapes and snapshots](#concepts-tapes-and-snapshots)
- [maketap](#maketap)
- [totap](#totap)
- [loadtap](#loadtap)
- [tap2tzx](#tap2tzx)
- [zx](#zx)
  - [zx tap](#zx-tap)
  - [zx tzx](#zx-tzx)
  - [zx basic](#zx-basic)
  - [zx snap](#zx-snap)
  - [zx convert](#zx-convert)
  - [zx info](#zx-info)
- [Conversions in depth](#conversions-in-depth)

## Concepts: tapes and snapshots

Two kinds of file format run through everything below, and the difference
between them governs which conversions are possible.

A **tape** (TAP or TZX) is an ordered sequence of named blocks. Each block is
either a header — carrying a name, a type, and a load address — or the data that
follows it. A tape describes *what to load and where*, but it holds no record of
a running machine: no register values, no program counter, no notion of where
execution should begin. TZX is the richer of the two: in addition to the data
blocks a TAP would hold, it can carry archive metadata, hardware-requirement
information, grouping, and timing detail.

A **snapshot** (SNA or Z80) is the opposite: a frozen photograph of a whole
machine at one instant. It records the full contents of RAM, every processor
register, the interrupt state, and — on 128K machines — the paging
configuration. It has no block structure and no names; it is simply the machine,
exactly as it stood.

Because a tape and a snapshot describe different things, converting between the
two kinds is never a clean copy. A tape lacks the entry point a snapshot needs;
a snapshot lacks the block structure and names a tape uses. Conversions *within*
a kind (tape to tape, snapshot to snapshot) are clean; conversions *across* the
divide are addressed carefully in [Conversions in depth](#conversions-in-depth).

## maketap

Creates a TAP file from a binary, stored as a single CODE block.

```
maketap [--name NAME] [--address ADDR] input.bin output.tap
```

| Flag        | Default | Meaning                                          |
| ----------- | ------- | ------------------------------------------------ |
| `--name`    | input filename | Block name, up to ten characters.         |
| `--address` | 32768   | Load address for the CODE block.                 |

Both positional arguments are required: the input binary and the output tape.

Example — wrap a binary that loads at 0x8000 and name the block `game`:

```
maketap --name game --address 32768 game.bin game.tap
```

The resulting tape holds one header block and one data block. The CODE header
records the standard parameter value of 0x8000 in its second parameter field;
this is a correction over the older zxgotools tool, which wrote zero there.

## totap

Converts either a binary or a BASIC text file into a TAP file.

```
totap --binary [--name NAME] [--address ADDR] input.bin output.tap
totap --basic  [--name NAME] [--autostart LINE] [-c] input.bas output.tap
```

Exactly one of `--binary` or `--basic` must be given; supplying both, or
neither, is an error.

| Flag          | Default | Meaning                                              |
| ------------- | ------- | ---------------------------------------------------- |
| `--binary`    | off     | Treat the input as a raw binary (a CODE block).      |
| `--basic`     | off     | Treat the input as BASIC source text.                |
| `--name`      | input filename | Block name, up to ten characters.             |
| `--address`   | 32768   | Load address (binary mode only).                     |
| `--autostart` | 0       | Auto-run line number (BASIC mode only).              |
| `-c`          | off     | Case-independent keyword matching (BASIC mode only). |

In binary mode, `totap` behaves like `maketap`. In BASIC mode it tokenises the
source into the Spectrum's internal byte format and writes a Program block. The
`--autostart` line, if non-zero, makes the program run automatically on load.

By default, keyword matching in BASIC mode is case-*sensitive*: `PRINT`
tokenises but `print` is kept as literal text. Passing `-c` makes matching
case-independent, so `print` also tokenises. This preserves the observable
behaviour of the older tool's `-c` flag.

If the tokenised program uses a 128K-only keyword (`SPECTRUM` or `PLAY`), a note
is printed to remind you the program will not run on a 48K machine.

Example — tokenise an auto-running loader:

```
totap --basic --name loader --autostart 10 loader.bas loader.tap
```

## loadtap

Reads a TAP file and reports its structure.

```
loadtap [-d] [-r] input.tap
```

| Flag | Meaning                                                       |
| ---- | ------------------------------------------------------------ |
| `-d` | Also dump each block's data as hexadecimal.                  |
| `-r` | Write the raw data-block bytes to standard output (no headers). |

With no flags, `loadtap` prints a summary: the number of blocks, then for each
block its length, flag byte, header fields (type, filename, data length, and the
two parameters) where present, checksum, and data length.

`-d` appends a hex dump of every block's data, sixteen bytes per line.

`-r` is different in kind: instead of a report it writes the concatenated
payloads of the data blocks to standard output, skipping headers. This lets you
extract a tape's raw contents for piping elsewhere:

```
loadtap -r game.tap > game.payload
```

The analysis output is byte-identical to the older zxgotools tool's, so existing
scripts that parse it continue to work unchanged.

## tap2tzx

Converts one or more TAP files into a single TZX, optionally adding metadata,
hardware information, and grouping.

```
tap2tzx -o out.tzx [options] input1.tap [input2.tap ...]
tap2tzx -o out.tzx -c config.json
```

| Flag          | Default | Meaning                                                   |
| ------------- | ------- | --------------------------------------------------------- |
| `-o`          | —       | Output TZX file. Required.                                |
| `-c`          | —       | JSON configuration file (see below).                      |
| `-p`          | 1000    | Pause between blocks, in milliseconds.                    |
| `-m`          | off     | Add a metadata (archive-info) block.                      |
| `--title`     | —       | Program title (used with `-m`).                           |
| `--author`    | —       | Program author (used with `-m`).                          |
| `--year`      | —       | Year of publication (used with `-m`).                     |
| `-128`        | off     | The program requires a 128K machine.                      |
| `-ay`         | off     | The program uses the AY sound chip.                       |
| `-paging`     | off     | The program uses memory paging.                           |
| `--model`     | —       | Required model: `+2`, `+2A`, or `+3`.                     |
| `--multiload` | off     | Insert a "stop in 48K mode" block between input files.    |
| `--group`     | —       | Bracket the input files under a named group.              |

The hardware flags are recorded in a TZX hardware-type block: `-128` marks the
tape as using a 128K machine and not running on a 48K one; `--model` selects the
specific 128K variant; `-ay` marks use of the AY sound chip.

When several input files are given, their blocks are concatenated in order. With
`--multiload`, a "stop in 48K mode" block is placed between each pair of files,
so that a 48K machine pauses between loading stages while a 128K machine loads
straight through.

Example — assemble a titled, 128K, multiload tape from three parts:

```
tap2tzx -o game.tzx -m --title "My Game" --author "haitch" --year 2026 \
        -128 --multiload --group "Game" part1.tap part2.tap part3.tap
```

### Configuration file

Instead of flags, `-c` reads a JSON file describing the whole tape. The schema
has three sections: `metadata` (title, author, year), `hardware`
(`128k_only`, `use_ay`, `model`), and `blocks` — an ordered list, each entry
naming a `file` to include, with optional `group` and `desc` fields. A new
`group` value opens a group that runs until the next group or the end.

The configuration is JSON, not the YAML used by the older tool, so that
`tap2tzx` — like the rest of zentools — needs no third-party dependencies. The
schema is otherwise equivalent.

## zx

`zx` is a single command with subcommands organised by format. Unlike the
focused tools, its flags may appear in any position, before or after the
positional arguments.

```
zx <command> [arguments]
```

Commands: `tap`, `tzx`, `basic`, `snap`, `convert`, `info`, and `version`.
Running any command with no arguments prints its own help.

Throughout, addresses may be written in hexadecimal (`0x8000` or `$8000`) or in
decimal (`32768`).

### zx tap

```
zx tap make <input.bin> [--name N] [--origin <addr>] [--loader --start <addr>] -o out.tap
zx tap info <file.tap>
```

`make` wraps a binary as a CODE block. `--origin` sets the load address; `--name`
sets the block name. With `--loader`, a BASIC auto-run loader is prepended so the
tape loads and then jumps to `--start`, giving a tape that runs on its own.

`info` lists the blocks in a tape, showing type, name, load address, and
checksum status.

### zx tzx

```
zx tzx make <input.tap> [--title T] [--author A] [--year Y] -o out.tzx
zx tzx info <file.tzx>
```

`make` converts a TAP image into TZX, optionally adding an archive-info block
from the title, author, and year. `info` lists the TZX blocks by identifier.

### zx basic

```
zx basic tokenise   <input.bas> -o out.bin [--case-sensitive]
zx basic detokenise <input.bin> [-o out.bas]
```

`tokenise` converts BASIC source text into the Spectrum's internal byte format.
Matching is case-independent by default; `--case-sensitive` requires exact
keyword case. `detokenise` does the reverse, printing to standard output unless
`-o` is given.

### zx snap

```
zx snap make <input.bin> --start <addr> [--origin <addr>] [--sp <addr>] [--model <name>] [--sna] [--z80] -o <basename>
zx snap info <file.sna|file.z80>
```

`make` builds a snapshot from a binary. The binary is placed in memory at
`--origin` (default 0x8000), the program counter is set to `--start` (required),
the stack pointer to `--sp` (default 0xFF00), and the machine type to `--model`
(`48k`, `128k`, `plus2`, `plus2a`, or `plus3`; default `48k`). At least one of
`--sna` or `--z80` selects the output format; both may be given. The code is
overlaid onto a genuine booted machine state, so the resulting snapshot loads
and runs at the entry point.

`info` decodes a snapshot and reports its model, program counter, stack pointer,
interrupt mode and flag, and — on 128K machines — the paging value.

Example — build a runnable 48K snapshot in both formats:

```
zx snap make demo.bin --start 0x8000 --sna --z80 -o demo
```

### zx convert

```
zx convert <input> -o <output> [--start <addr>] [--sp <addr>] [--model <name>]
```

Converts a file between any supported tape or snapshot format. The source and
target formats are taken from the file extensions. The `--start`, `--sp`, and
`--model` flags apply only when converting a tape into a snapshot, where an entry
point is needed.

The behaviour of each conversion is set out in full in the next section.

### zx info

```
zx info <file>
```

Identifies a file's format — by signature where possible, otherwise by
extension — and prints the appropriate summary, as `zx tap info`, `zx tzx info`,
or `zx snap info` would.

## Conversions in depth

`zx convert` spans the format matrix below. The two axes are the source format
(rows) and the target format (columns).

| from \ to | tap         | tzx         | sna         | z80         |
| --------- | ----------- | ----------- | ----------- | ----------- |
| **tap**   | —           | lossless    | needs start | needs start |
| **tzx**   | lossless\*  | —           | needs start | needs start |
| **sna**   | memory dump | memory dump | —           | lossless    |
| **z80**   | memory dump | memory dump | lossless    | —           |

The cells fall into four cases.

### Within a kind: lossless

**tap to tzx, and tzx to tap.** A TAP block wraps directly into a TZX
standard-speed data block, and a TZX standard-speed block unwraps directly back
into a TAP block. Round-tripping a tape through TZX and back yields the original
bytes.

The asterisk on **tzx to tap** marks the one caveat: only standard-speed data
survives the trip down to TAP, because TAP has no way to express anything else.
A TZX file produced by zentools contains only standard-speed blocks, so its
conversion is lossless. But a TZX from elsewhere may carry turbo or custom-loader
blocks, or structural blocks (groups, hardware information, archive metadata);
these have no TAP equivalent and are dropped. When that happens, `zx convert`
prints a note saying how many blocks were dropped.

**sna to z80, and z80 to sna.** Both formats are containers for the same thing —
a complete machine state — so transcoding between them is lossless. (One small
detail of the 48K SNA format is discussed below; it does not lose information.)

### Across the divide, downward: snapshot to tape

A snapshot holds a running machine; a tape holds blocks to load. Going from
snapshot to tape, the best a tape can do is carry the snapshot's RAM as a CODE
block. But this discards everything that made it a snapshot: the processor
registers, the interrupt state, and the program counter that said where to
resume. The resulting tape is a **memory dump** — it will load the bytes back
into memory, but it will not resume execution where the snapshot left off, and
on a 128K machine it cannot express the paging of the banks that are not
currently visible.

`zx convert` performs this conversion but prints a warning making the loss
explicit. It is useful for recovering data from a snapshot, not for producing a
runnable program.

### Across the divide, upward: tape to snapshot

A tape carries code and a load address but no entry point, so it cannot, by
itself, say where a snapshot should begin executing. Converting a tape to a
snapshot therefore **requires a `--start` address**; without one, `zx convert`
reports the error rather than guessing. Given a start address, the tool extracts
the tape's CODE block, overlays it onto a booted machine state — exactly as
`zx snap make` does with a binary — and produces a snapshot that runs. The
`--sp` and `--model` flags tune the stack pointer and machine type.

### A note on the 48K SNA format

The 48K SNA format has no field for the program counter. Instead it stores the
program counter by pushing it onto the machine's stack, so that the loader
returns into the running program. A consequence is that transcoding *into* a 48K
SNA writes two bytes near the stack pointer and adjusts the stack pointer
accordingly; transcoding back reads them and restores it. The program counter
and stack pointer are preserved exactly across a round trip, and the snapshot
runs correctly; this is a property of the SNA format, not a loss of information.