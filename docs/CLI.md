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
  - [zx pzx](#zx-pzx)
  - [zx basic](#zx-basic)
  - [zx scr](#zx-scr)
  - [zx snap](#zx-snap)
  - [zx rzx](#zx-rzx)
  - [zx convert](#zx-convert)
  - [zx edit](#zx-edit)
  - [zx build](#zx-build)
  - [zx info](#zx-info)
- [Conversions in depth](#conversions-in-depth)

## Concepts: tapes and snapshots

Two kinds of file format run through everything below, and the difference
between them governs which conversions are possible.

A **tape** (TAP, TZX, or PZX) is an ordered sequence of named blocks. Each
block is either a header — carrying a name, a type, and a load address —
or the data that follows it. A tape describes *what to load and where*,
but it holds no record of a running machine: no register values, no
program counter, no notion of where execution should begin. TZX is the
richer of the two classic formats: in addition to the data blocks a TAP
would hold, it can carry archive metadata, hardware-requirement
information, grouping, and timing detail. PZX is a newer, simpler
successor to TZX with the same expressive range, built around raw pulse
sequences rather than TZX's many named block types.

A **snapshot** (SNA, Z80, or SZX) is the opposite: a frozen photograph of a
whole machine at one instant. It records the full contents of RAM, every
processor register, the interrupt state, and — on 128K machines — the
paging configuration. It has no block structure and no names; it is simply
the machine, exactly as it stood. SZX (zx-state) is the modern of the
three, and the only one that can also carry peripheral state (AY sound,
joysticks, disk controllers) — none of which these tools model, since
disassembly and format conversion only ever need CPU, paging, and memory.

An **RZX** file is neither: it's an input recording, a frame-by-frame log
of what every CPU `IN` instruction returned during an actual played
session, anchored by one embedded snapshot marking where the recording
starts. It converts to a snapshot or a flat binary (by way of that
embedded snapshot), and, one direction only, to a tape — but nothing
converts *to* an RZX, since there is no way to manufacture a recording of
a session that never happened.

A flat **binary** (`.bin`) is neither of the above: no header, no
structure, no metadata, just bytes. It's both a valid extraction target
from a tape or a snapshot (`zx convert ... -o code.bin`) and a valid
source for building one (`zx convert code.bin -o code.tap --origin ...`),
supplying via flags what the format itself can't carry.

Because a tape and a snapshot describe different things, converting
between the two kinds is never a clean copy. A tape lacks the entry point
a snapshot needs; a snapshot lacks the block structure and names a tape
uses. Conversions *within* a kind (tape to tape, snapshot to snapshot) are
clean; conversions *across* the divide are addressed carefully in
[Conversions in depth](#conversions-in-depth).

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
| `--autostart` | *(none)* | Auto-run line number (BASIC mode only).             |
| `-c`          | off     | Case-independent keyword matching (BASIC mode only). |

In binary mode, `totap` behaves like `maketap`. In BASIC mode it tokenises the
source into the Spectrum's internal byte format and writes a Program block.
Omitting `--autostart` entirely means no auto-run (encoded as the TAP
format's own sentinel, 32768) -- the tape loads and stops, ready for `RUN`.
Passing `--autostart 0` is a real, distinct choice: autostart at line 0,
since 0 is a genuinely valid BASIC line number, not itself a "no
autostart" marker.

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

Commands: `tap`, `tzx`, `pzx`, `basic`, `scr`, `snap`, `rzx`, `convert`,
`edit`, `build`, `info`, and `version`.
Running any command with no arguments prints its own help.

Throughout, addresses may be written in hexadecimal (`0x8000` or `$8000`) or in
decimal (`32768`).

### zx tap

```
zx tap make <input.bin> [--name N] [--origin <addr>] [--loader --start <addr>] -o out.tap
zx tap make --basic <input.bas> [--name N] [--autostart LINE] [--case-sensitive] -o out.tap
zx tap info <file.tap>
zx tap append <file1.tap> <file2.tap> [<file3.tap>...] -o out.tap
```

`make` wraps a binary as a CODE block by default. `--origin` sets the load
address; `--name` sets the block name (defaults to the input filename;
pass `--name ""` explicitly to force a genuinely empty name, e.g. to
reproduce another tool's tape byte-for-byte). Names are sanitised into
the ZX Spectrum's own character set: standard ASCII passes through
unchanged, £ and © map to their real Sinclair code points (0x60, 0x7F --
confirmed against the actual ZX Spectrum character set, not assumed --
replacing ASCII's grave accent and DEL at those same positions), and any
other non-ASCII character becomes a plain asterisk rather than being
silently corrupted by byte-level truncation. `zx tap info`/`zx edit list`
decode names symmetrically: a name encoded with £ or © reads back as the
actual character, not the ASCII byte (a grave accent, DEL) that happens
to share that code point. With `--loader`, a BASIC auto-run loader is
prepended so the tape loads and then jumps to
`--start`, giving a tape that runs on its own.

With `--basic`, `make` instead tokenises a BASIC source text file (the
same tokeniser `zx basic tokenise` uses) and wraps it as a Program block
rather than a CODE block -- the `zx`-native equivalent of a standalone
BASIC-to-tape tool. `--autostart LINE` sets the line the program jumps to
on load; the *absence* of `--autostart` means no auto-run at all (the
tape loads and simply stops, ready for `RUN`), encoded as the TAP
format's own real sentinel of 32768 -- not 0, which is itself a genuinely
valid BASIC line number, not a "no autostart" marker. `--autostart 0`
therefore means exactly what it says: autostart at line 0, a real
choice, distinct from omitting the flag entirely. Keyword matching is
case-*insensitive* by default (`print` tokenises the same as `PRINT`),
matching `zx basic tokenise`'s own convention; pass `--case-sensitive`
to require exact case. `--basic` and `--loader` are mutually exclusive --
a Program block does not take a CODE auto-run loader.

`info` lists the blocks in a tape, showing type, name, load address, and
checksum status.

`append` combines two or more TAP files into one. This is genuinely simple
raw byte concatenation, not a decode-and-re-encode round trip: TAP has no
container structure of its own, so a well-formed TAP file's bytes,
concatenated after another's, already form a valid multi-block TAP file.
Each input is still checked as real, parseable TAP first, so a mistaken
non-TAP file is caught with a clear error rather than silently producing
a corrupt result. For combining files that aren't all TAP — a mix of
TAP, TZX, and PZX inputs, or an output in a different format than the
inputs — use [`zx edit append`](#zx-edit) instead, which works across any
combination of the three tape formats.

### zx tzx

```
zx tzx make <input.tap> [--title T] [--author A] [--year Y] -o out.tzx
zx tzx info <file.tzx>
```

`make` converts a TAP image into TZX, optionally adding an archive-info block
from the title, author, and year. `info` lists the TZX blocks by identifier.

### zx pzx

```
zx pzx make <input.tap> [--title T] -o out.pzx
zx pzx info <file.pzx>
```

`make` encodes each of a TAP's blocks using the standard Spectrum ROM
pilot/sync/bit timings — the same timings PZX's own spec documents as the
canonical mapping for a TZX standard-speed block — optionally naming the
resulting archive with `--title`. There is no reverse direction built in
here: PZX has no block concept equivalent to a TAP header, only pulse
timing, so reconstructing named TAP blocks from an arbitrary PZX would
need pattern recognition this command doesn't attempt. (`zx convert`
*can* turn a PZX into a TAP regardless of what pulse timing the source
uses — a `DATA` block's payload is already fully-resolved bytes by the
time PZX's own decoder produces it, whatever timing represented them
physically on tape; see [Conversions in depth](#conversions-in-depth).
It's specifically `zx pzx make`, going the other way, that always writes
standard timing, since that's the only timing convention this command
itself defines.)

`info` lists a PZX's blocks by type — `PZXT`, `PULS`, `DATA`, `PAUS`,
`BRWS`, `STOP`, or a custom tag preserved verbatim — showing the header's
title and metadata, each `PULS` block's pulse-entry count, and each
`DATA` block's bit count and byte length.

### zx basic

```
zx basic tokenise   <input.bas> -o out.bin [--case-sensitive]
zx basic detokenise <input.bin> [-o out.bas]
```

`tokenise` converts BASIC source text into the Spectrum's internal byte format.
Matching is case-independent by default; `--case-sensitive` requires exact
keyword case. `detokenise` does the reverse, printing to standard output unless
`-o` is given.

### zx scr

```
zx scr encode [flags] <image>              PNG/JPEG/GIF -> .scr
zx scr decode [flags] <file.scr>           .scr -> PNG
zx scr crop   [flags] <image|.scr>         crop a region (or sprite) to PNG
zx scr cut    [flags] <file.scr>           extract a sub-region into a .cut asset collection
zx scr paste  [flags] <collection:name> <target.scr>   paste an asset into a screen
zx scr ls     <file.cut>                   list a .cut collection's assets
zx scr atlas  [flags] <file.cut>           render a collection as a labelled contact sheet
zx scr fromsnap [flags] <file.sna|file.z80>   extract a snapshot's display file as .scr
zx scr ocr    [flags] <image|.scr>         recognise on-screen text as plain text
```

A `.scr` file is the raw ZX Spectrum display: 6912 bytes, the pixel bitmap
followed by the per-8x8-cell colour attributes, exactly as they sit in
Spectrum RAM at `0x4000`-`0x5AFF`. `encode` and `decode` convert to and from
ordinary images, `crop` extracts a region (or, for a `.scr`, a sprite's
bounding box) as PNG, and `fromsnap` pulls the display straight out of a
snapshot without needing a running machine.

`cut`, `paste`, `ls`, and `atlas` manage `.cut` files: named collections of
extracted regions (sprites, tiles, UI panels) pulled from one or more `.scr`
screens, with sub-region reuse across a project in mind. `cut` adds a named
region to a collection (creating it if it doesn't exist); `paste` composites
a collection's asset back onto a target screen at a given position, with a
bit operation (`or`/`and`/`copy`/`xor`) and an optional mask asset for
non-rectangular shapes; `ls` lists a collection's contents; `atlas` renders
the whole collection as a labelled contact-sheet PNG for a visual overview.

`ocr` recognises the text on a screen — matching each 8x8 character cell
against the real Spectrum ROM font — and prints it as plain text. This
exists for driving emulators through captured screenshots: an automated
session can send input, capture the screen, and read back exactly what
printed, rather than a human reading each captured image by eye. A `.scr`
file needs no further flags (it is always exactly the native 256x192
pixels); an arbitrary screenshot — an emulator window, scaled and offset
within a larger capture — needs `--origin` (the display's top-left pixel
within the image) and `--scale` (pixels per emulated pixel) to say where the
256x192 display actually sits. Recognition matches each cell against the
standard 96-glyph ROM set (codes 32-127); content outside that set —
box-drawing borders, UDGs, decorative graphics — is matched to its nearest
visual approximation rather than skipped, so garbage characters in a
non-textual region of the screen are expected, not a bug.

Example — read the text off a captured CSpect window, rendered at `-w3`
(3x scale, with the classic 32-pixel border also scaled 3x before any
window chrome):

```
zx scr ocr --origin=256,112 --scale=3 screenshot.png
```

Example — read a `.scr` file directly, no geometry needed:

```
zx scr ocr screen.scr
```

Run `zx scr -h` for the full flag reference, including `crop`'s pixel- vs
cell-based region syntax and `--auto` sprite-extent detection, and `cut`'s
attribute and mask options.

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

`zx snap` also supports `.szx` (the modern zx-state format): add `--szx`
to `make`, and `info` recognises `.szx` alongside `.sna`/`.z80` (by
signature, even without the right extension). 128K-family snapshots of
any of the three formats accept `--bank N` (0-7) to select a specific RAM
bank at `0xC000`-`0xFFFF`, overriding whichever bank the snapshot's own
paging state has there.

### zx rzx

```
zx rzx info <file.rzx>
```

There is no `make`: an RZX file records the result of every CPU `IN`
instruction during a real emulated session, which is an emulator's job to
produce, not something built from a static file — see
[Concepts](#concepts-tapes-and-snapshots).

`info` reports the recording's own version, whether it's signed, and
lists each block: the creator identification, any embedded snapshot
(its format, whether it's embedded data or an external file reference,
and its size), and each recording block's frame count, how many frames
were flagged "repeated" (reusing the previous frame's port-read values
rather than storing their own), and the total number of port reads
across the whole recording.

To actually get code or a snapshot out of an RZX, use `zx convert` — see
the next section.

### zx convert

```
zx convert <input> -o <output> [flags]
zx convert <tape> --outdir <dir>
```

Converts a file between any supported tape, snapshot, flat-binary, or RZX
source and target. The source format is taken from the input's own
extension (or signature, where the format has one); the target format
from `-o`'s extension. Source and target must genuinely differ — `zx
convert` is for converting between formats, and rejects a same-format
"conversion" outright rather than silently copying the file, which could
mask an actual mistake such as a typo'd extension. Use `cp` to copy a
file.

| Flag           | Applies to                          | Meaning                                             |
| -------------- | ------------------------------------ | --------------------------------------------------- |
| `--start`      | tape/bin -> snap                     | Entry point (required; a tape/binary has none of its own). |
| `--sp`         | tape/bin -> snap                     | Stack pointer (default `0xFF00`).                    |
| `--model`      | tape/bin -> snap                     | Target machine (`48k`, `128k`, `plus2`, `plus2a`, `plus3`; default `48k`). |
| `--block`      | tape -> bin                          | Select a block by 0-based index (default: first Code-type block). |
| `--block-name` | tape -> bin                          | Select a block by exact header name, overriding `--block`. |
| `--org`        | snap/rzx -> bin                      | Start address (default: the snapshot's own PC).      |
| `--length`     | -> bin                               | Byte count to extract (default: everything available). |
| `--bank`       | 128K snap/rzx -> bin                 | Select a specific RAM bank (0-7), overriding the paging state. |
| `--origin`     | bin -> tape/snap                     | Load address (a raw binary carries none of its own). |
| `--name`       | bin -> tape                          | Block name (default: the input file's own base name). |
| `--outdir`     | tape (only)                          | Explode into a directory instead of converting to one file — see below. |

Examples:

```
zx convert game.tap -o game.tzx              # lossless
zx convert game.sna -o game.z80              # lossless
zx convert game.tap -o game.z80 --start 0x8000   # tape to snapshot needs --start
zx convert game.z80 -o code.bin --org 0x8000 --length 0x2000
zx convert code.bin -o code.tap --name GAME --origin 0x8000
zx convert session.rzx -o extracted.sna      # the recording's embedded snapshot
```

`--outdir` is a different shape of operation entirely: not format A to
format B, but a tape to *many files*. `zx convert game.tap --outdir
extracted/` writes one `.bin` per data block (named after its header when
it has one, index-only for a bare/headerless block) plus `manifest.json`
describing every block — header and data alike — in order, with checksum
status and header attribution. `-o` and `--outdir` are mutually
exclusive; `--outdir` only accepts a tape source (`tap`/`tzx`/`pzx`).

Full detail on what survives each conversion, and what's lost, is in
[Conversions in depth](#conversions-in-depth).

### zx edit

```
zx edit list <file> [--json]
zx edit extract <file> --block N [--raw] -o out.bin
zx edit delete <file> --block N[,M,...] -o out.tap|out.tzx|out.pzx
zx edit import <file> --data new.bin --name NAME [--kind code|program]
               [--org ADDR] [--autostart N] [--at N] [--raw]
               -o out.tap|out.tzx|out.pzx
zx edit append <file1> <file2> [<file3>...] -o out.tap|out.tzx|out.pzx
```

`zx edit` operates on a multi-block tape's flat *logical* block list — the
loadable header/data blocks themselves, the same list `zx convert`'s own
tape normalisation already works with. A TZX's structural blocks
(pauses, group markers, `STOP`) and a PZX's own `PAUS`/`BRWS`/`STOP`
blocks aren't part of that list and don't survive a `delete`/`import`/
`append` round trip — the same scope boundary `zx convert` already has
converting a tape to another tape format.

`list` prints every block: index, kind (header or data), and for a
header its type, name, and load address; for data, its length and
checksum status. `--json` emits the same manifest shape `zx convert
--outdir` writes to `manifest.json`.

`extract` pulls one block's payload out to a file — the same operation
`zx convert ... --block N` performs, plus `--raw`, which emits the block's
full raw bytes (flag, payload, and checksum together) instead of just the
payload, for re-importing elsewhere.

`delete` removes one or more blocks (comma-separated indices, all
against the tape's original numbering, not renumbered after each
deletion) and writes what remains. Deleting every block is rejected —
a tape edited down to nothing is almost certainly a mistake, not the
goal.

`import` inserts a new block, built the same way `zx tap make` builds
one (`--kind code`, needing `--org`) or `zx totap --basic` does
(`--kind program`, with `--autostart`), or, with `--raw`, an
already-encoded block (such as one extracted with `extract --raw`)
inserted verbatim. `--at N` inserts before that index; omitted, the new
block is appended at the end.

`append` concatenates two or more tape files' own block lists, in the
order given, regardless of what format each one is — the actual value
over shell-level concatenation, which only happens to work for bare TAP
(both TZX and PZX have their own container structure a raw
concatenation would corrupt).

`delete`/`import`/`append` all require `-o` (there is no editing in
place); the target format comes from `-o`'s own extension, falling back
to the source's format (or, for `append`, the first input's format) when
the extension isn't a recognised tape format.

Example — remove a header+data pair, then re-add a new block in its place:

```
zx edit delete game.tap --block 2,3 -o game2.tap
zx edit import game2.tap --data newlevel.bin --name LEVEL2 --org 0xC000 -o game3.tap
```

### zx build

```
zx build <spec.json> -o out.tap|out.tzx|out.pzx
```

Builds a multi-block tape file from a JSON specification — the
declarative counterpart to `zx edit`'s incremental list/import/delete,
for describing a whole tape's contents at once rather than one change at
a time.

```json
{
  "title": "optional, pzx target only",
  "blocks": [
    {"kind": "code", "name": "LOADER", "org": 32768, "file": "loader.bin"},
    {"kind": "program", "name": "BASIC", "autostart": 10, "file": "basic.bin"},
    {"kind": "raw", "file": "already_encoded_block.bin"}
  ]
}
```

Each block's `"file"` path resolves relative to the *spec file's own
directory*, not the current working directory, so a spec and its data
files can be moved together as a unit. `"org"` and `"autostart"` are
plain JSON numbers (decimal), not `"0x..."` strings — the spec format is
deliberately simpler than the CLI's own flag conventions. `"autostart"`,
for `kind=program`, defaults to `0x8000` (the conventional "no autostart
line" sentinel) if omitted.

This is a fresh design, not a revival of an older tool's YAML-based
configuration format from earlier in this project's history — see
[the tape-handling manual](TAPE-HANDLING.md) for a full worked example
assembling a complex multi-block tape from a spec like this one.

### zx info

```
zx info <file>
```

Identifies a file's format — by signature (TZX, SZX, PZX, and RZX all have
one) where possible, otherwise by extension — and prints the appropriate
summary, as `zx tap info`, `zx tzx info`, `zx pzx info`, `zx snap info`,
or `zx rzx info` would.

## Conversions in depth

`zx convert` spans the format matrix below. The two axes are the source
format (rows) and the target format (columns); a source or target of
`bin` (a flat binary, no structure at all) and `rzx` (an input recording)
are covered separately afterward, since neither fits the tape/snapshot
grid cleanly.

| from \ to | tap | tzx | pzx | sna | z80 | szx |
| --------- | --- | --- | --- | --- | --- | --- |
| **tap**   | —   | lossless | lossless | needs start | needs start | needs start |
| **tzx**   | lossless\* | —   | lossless\* | needs start | needs start | needs start |
| **pzx**   | lossless\* | lossless\* | —   | needs start | needs start | needs start |
| **sna**   | memory dump | memory dump | memory dump | —   | lossless | lossless |
| **z80**   | memory dump | memory dump | memory dump | lossless | —   | lossless |
| **szx**   | memory dump | memory dump | memory dump | lossless | lossless | —   |

The cells fall into four cases.

### Within a kind: lossless

**Any tape format to any other.** A TAP block, a TZX standard-speed
(`0x10`), Turbo Speed (`0x11`), or Pure Data (`0x14`) block, and a PZX
`DATA` block all carry the same thing underneath: fully-resolved
flag+payload+checksum bytes. Extraction and reconstruction between any
two of the three formats works from that shared shape, regardless of
what pulse timing represented the bytes physically on the *source* tape
— standard Spectrum ROM timing, a turbo loader, anything else. The
asterisks mark the one real caveat: what's *reconstructed* on the far
side (TAP or PZX output specifically) always uses standard ROM timing,
since that's the only timing TAP itself can express at all, and it's
what `zx pzx make` writes too — a turbo-loaded source still converts
losslessly (the bytes are identical), but the *loading protocol* a real
Spectrum would need to actually load the result back from tape is not
preserved, only its data.

**Any snapshot format to any other.** SNA, Z80, and SZX are three
containers for the same thing — a complete machine state (CPU
registers, paging, memory) — so transcoding between any pair is
lossless, with one narrow exception: **the `+3`'s secondary paging port
(`1FFD`) is lost converting to SNA specifically**, since the `.sna`
format has no field for it at all (confirmed by reading `pkg/snapshot`'s
own SNA encoder/decoder directly, not assumed). Port `7FFD` — the actual
RAM bank selection every 128K-family machine uses — is unaffected; this
is narrowly the `+3`'s own extra port. `zx convert` warns on stderr
whenever a real, non-zero `1FFD` value is about to be dropped this way,
rather than losing it silently. (One further, unrelated detail of the
48K SNA format specifically is discussed [below](#a-note-on-the-48k-sna-format);
it does not lose information.)

### Across the divide, downward: snapshot to tape

A snapshot holds a running machine; a tape holds blocks to load. Going
from snapshot to tape, the best a tape can do is carry the snapshot's
RAM as one or more CODE blocks. But this discards everything that made
it a snapshot: the processor registers, the interrupt state, and the
program counter that said where to resume. The resulting tape is a
**memory dump** — it will load the bytes back into memory, but it will
not resume execution where the snapshot left off.

For a 128K-family source, every RAM bank is emitted as its own named
block (`BANK0` through `BANK7`), not just whichever one happened to be
paged in at `0xC000` when the snapshot was taken — plus a small
dedicated `PAGING` block (2 bytes: port `7FFD`, then port `1FFD`) so
which bank was actually active survives the trip too. A tape format has
no field of its own for either of these; a tiny metadata block does the
job instead, the same convention TZX itself uses for its own non-audio
information.

`zx convert` performs this conversion but prints a warning making the
loss explicit. It is useful for recovering data from a snapshot, not for
producing a runnable program.

### Across the divide, upward: tape to snapshot

A tape carries code and a load address but no entry point, so it
cannot, by itself, say where a snapshot should begin executing.
Converting a tape to a snapshot therefore **requires a `--start`
address**; without one, `zx convert` reports the error rather than
guessing. Given a start address, the tool extracts the tape's CODE
block, overlays it onto a booted machine state — exactly as `zx snap
make` does with a binary — and produces a snapshot that runs. The
`--sp` and `--model` flags tune the stack pointer and machine type.

### Flat binaries: `bin` as source and target

A `.bin` file is the universal escape hatch: no structure, so it's
always available as both an extraction target (`--block`/`--block-name`
select which tape block, or `--org`/`--length`/`--bank` select which
slice of a snapshot) and a construction source (`--origin` supplies the
load address a raw binary has no way to carry itself; `--name` sets a
tape block's name).

`bin -> tape` and `bin -> snap` both need `--origin`; `bin -> snap`
needs `--start` too, for the same reason any tape does. `tape -> bin`
and `snap -> bin` need nothing beyond the source itself — `--length`
optionally limits how much is extracted, and `--block`/`--block-name`
(tape) or `--bank` (128K-family snapshot) select which part.

### RZX: a recording's embedded snapshot

An RZX converts to `bin`, a snapshot format, or a tape format — always
by way of its first embedded snapshot, decoded and then handled exactly
the way a standalone snapshot of that format would be (including the
128K-to-tape `PAGING`/`BANK0`-`BANK7` treatment above, and the same
`--org`/`--length`/`--bank` flags). Converting *to* the same format the
snapshot was already embedded as is a verbatim extraction, byte for
byte; converting to a different one decodes and re-encodes, with the
same costs and caveats as any other snapshot-format conversion above.

Nothing converts *to* an RZX. It's a recording of a real played
session — a frame-by-frame log of what every CPU `IN` instruction
returned — and there is no way to manufacture that from a static file;
see [Concepts](#concepts-tapes-and-snapshots).

### A note on the 48K SNA format

The 48K SNA format has no field for the program counter. Instead it stores the
program counter by pushing it onto the machine's stack, so that the loader
returns into the running program. A consequence is that transcoding *into* a 48K
SNA writes two bytes near the stack pointer and adjusts the stack pointer
accordingly; transcoding back reads them and restores it. The program counter
and stack pointer are preserved exactly across a round trip, and the snapshot
runs correctly; this is a property of the SNA format, not a loss of information.