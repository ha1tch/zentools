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
  - [zx next](#zx-next)
  - [zx zx0](#zx-zx0)
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

Commands: `tap`, `tzx`, `pzx`, `basic`, `scr`, `next`, `zx0`, `snap`, `rzx`,
`convert`, `edit`, `build`, `info`, and `version`.
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
zx scr encode   [flags] <image>               image -> .scr, .zm1, .zm2 or a .cut asset
zx scr decode   [flags] <screen>              .scr, .zm1 or .zm2 -> PNG
zx scr fit      [flags] <image>               fit an image as encode would, to PNG
zx scr crop     [flags] <image|screen>        crop a region (or sprite) to PNG
zx scr cut      [flags] <screen>              extract a sub-region into a .cut asset collection
zx scr paste    [flags] <collection:name> <screen>   paste an asset into a screen
zx scr ls       <file.cut>                    list a .cut collection's assets
zx scr atlas    [flags] <file.cut>            render a collection as a labelled contact sheet
zx scr flip     -h|-v [flags] <screen|file.cut|image>   mirror a screen, assets or an image
zx scr convert  -to <zxpalN> [flags] <file.cut|screen>  rewrite attributes for another palette
zx scr fromsnap [flags] <file.sna|file.z80>   extract a snapshot's display file as .scr
zx scr ocr      [flags] <image|screen>        recognise on-screen text as plain text
zx scr palette  [-registers f] [-banks] [-o file.csv|file.png] <zxpalN>   list a palette's registers
zx scr fitregs  -add <n> -o <file.csv> [flags] <picture>...   fit colours to add to a palette
zx scr compare  [-metric m] <a> <b>           compare two images or screens
```

A `.scr` file is the raw ZX Spectrum display: 6912 bytes, the pixel bitmap
followed by the per-8x8-cell colour attributes, exactly as they sit in
Spectrum RAM at `0x4000`-`0x5AFF`. `encode` and `decode` convert to and from
ordinary images (PNG, JPEG, GIF or BMP), `crop` extracts a region (or, for a
screen, a sprite's bounding box) as PNG, and `fromsnap` pulls the display
straight out of a snapshot without needing a running machine.

`cut`, `paste`, `ls`, `atlas`, `flip` and `convert` manage `.cut` files:
named collections of extracted regions (sprites, tiles, UI panels) pulled
from one or more screens, with sub-region reuse across a project in mind.
`cut` adds a named region to a collection (creating it if it doesn't
exist); `paste` composites a collection's asset back onto a target screen
at a given position, with a bit operation (`or`/`and`/`copy`/`xor`) and an
optional mask asset for non-rectangular shapes; `ls` lists a collection's
contents; `atlas` renders the whole collection as a labelled contact-sheet
PNG for a visual overview; `flip` mirrors assets, and `convert` rewrites
their attributes for another palette.

#### ZXMODE modes and ZXPAL palettes

The ZXMODE video specification for the Spectrum family adds two linear
screen modes to the classic one and five palettes. The
[ZXMODE handling manual](ZXMODE-HANDLING.md) walks through them with a
worked example; this section is the reference.

| `--mode` | Picture | Stored form | File |
| --- | --- | --- | --- |
| `zxmode0` | 256x192, classic interleaved layout | 6912 bytes, as a `.scr` | `.scr` |
| `zxmode1` | 256x192, linear | bitmap, then the attribute plane(s): 6912 or 7680 bytes | `.zm1` |
| `zxmode2` | 512x384, linear | two half-screen pages, top then bottom: 27648 or 30720 bytes | `.zm2` |

| `--palette` | Registers | Attribute |
| --- | --- | --- |
| `zxpal0` | 16: the Spectrum colours | one byte, classic (flash, bright, paper, ink) |
| `zxpal1` | 32: the Spectrum colours in two banks, 16 greys in two more | one byte; bits 7-6 select the bank |
| `zxpal2` | 32: the eight colours at four levels | one byte; bits 7-6 select the level |
| `zxpal3` | 64: twelve hues and four greys at four levels | two bytes, ink register then paper register |
| `zxpal4` | 112: ZXPAL3 plus tones, tints and shades | two bytes, as ZXPAL3 |

`zxmode0` takes the one-byte palettes only. In the linear modes a two-byte
palette stores two attribute planes after the bitmap, ink then paper.

A screen file does not record its palette, so every command that reads one
(`decode`, `crop`, `ocr`, `cut`, `paste`, `flip`, `convert`, `compare`)
takes `--mode` (default from the extension: `.zm1`, `.zm2`, otherwise
`zxmode0`), `--palette` (default `zxpal0`) and `--registers`. A two-byte
file read without `--palette` is refused with a hint, and `zx info` on a
`.zm1` or `.zm2` says which palettes its size fits.

**Loaded registers.** The palette registers are loadable (Part 5 of the
specification), so a program may show a screen in colours other than the
palette's reset values. `--registers file.csv` names the register table a
screen is meant for, for encoding, decoding and every command above. The
CSV has a header and one row per register; the colour comes from columns
`r`, `g` and `b` (or `hex`), and the register number from `register`,
`slot` or `index` (or the row's position). `zx scr palette` writes one
(`-o file.csv`, from the reset values or from `--registers`), and the
specification's `data/zxpalN_registers.csv` files read as they are.

**Encoding.** `--mode`, `--palette` or `--registers` selects the
specification's reference encoder: for each 8x8 cell it chooses the pair of
register colours that best covers the cell and dithers between them in
linear light. It produces the same picture, pixel for pixel, as the
specification's own reference implementation. The classic converter used
without these flags posterises first, which turns normal colours into
bright ones; the reference encoder with `--palette zxpal0` keeps both.

An output ending in `.cut` appends the picture to a ZCUT collection as one
asset (`--name`, default the input's base name).

```
zx scr encode photo.jpg --resize fill --filter lanczos --mode zxmode2 --palette zxpal4 -o photo.zm2
zx scr decode photo.zm2 --palette zxpal4 -o check.png
zx scr encode photo.jpg --resize bestfit --mode zxmode1 --palette zxpal2 -o pics.cut --name title
zx scr encode photo.jpg --resize fill --mode zxmode0 --palette zxpal1 -o classic.scr
zx scr encode photo.jpg --resize fill --mode zxmode2 --palette zxpal4 --registers mine.csv -o photo.zm2
```

**Fitting.** An image that is not the screen's size needs `--resize`:

| `--resize` | Effect |
| --- | --- |
| `stretch` | scale to fill, ignoring aspect ratio |
| `bestfit` | scale to fit, preserving aspect, padding the border with `--fillattr`'s paper |
| `centre` | no scaling; centre, cropping or padding |
| `fill` (`cover`) | scale to cover, preserving aspect, cropping the overflow |

`--filter lanczos` scales with Pillow's Lanczos resampler instead of the
default (bilinear, or an area filter when `fill` shrinks), and
`--centering x,y` places `fill`'s crop (each 0 to 1; 0.5,0.5 centres it).
`--trim n` removes a scan margin of n pixels from every edge first, and
`--grey` converts the fitted picture to luminance as the specification does
(ITU-R 601, Pillow's `convert("L")`). Together they are the specification's
picture fit: `--resize fill --filter lanczos` gives, byte for byte, the
512x384 picture its tools produce from the same pixels (the picture table
in `pictures/README.md` gives each picture's trim and centring). `zx scr
fit` writes the fitted picture as PNG (`--size`, default 512x384), to look
at or to compare an encoding against.

JPEG colour is rebuilt as libjpeg-turbo (behind Pillow) rebuilds it, with
its chroma interpolation, rather than from the nearest chroma sample, so a
JPEG decodes to within a level or so of what the specification's tools
see; the remaining differences come from the two decoders' inverse DCTs.

**Experimental encoder options.** Part 4 of the specification lists
untested ways an encoder could improve on the reference one; three are
here, each leaving the reference encoder when used:

| Flag | Effect |
| --- | --- |
| `--diffuse f` | spreads each cell's colour error, times f (0 to 1), into the cells not yet encoded, as Floyd-Steinberg spreads a pixel's; the encoding then runs on one CPU |
| `--flat n` | fills cells whose pixels spread less than n (RMS, 0-255) with one colour instead of dithering |
| `--linear-pairs` | chooses each cell's pair by distance in linear light rather than sRGB |

On the specification's 22 colour pictures, fitted as above, the mean error
(blurred RMS, below) came out as follows:

| Palette | Reference | `--diffuse 0.5` | `--diffuse 1` | `--flat 4` | `--linear-pairs` |
| --- | --- | --- | --- | --- | --- |
| ZXPAL0 | 31.20 | 22.57 | 22.23 | 34.56 | 45.39 |
| ZXPAL2 | 20.79 | 17.35 | 18.39 | 21.90 | 31.04 |
| ZXPAL4 | 8.67 | 7.55 | 7.63 | 10.82 | 11.96 |

Diffusing half the error lowers the error by 13% (ZXPAL4) to 28% (ZXPAL0),
and also the lighter-blur error that keeps more fine detail. Filling flat
cells and choosing pairs in linear light both made it worse, as measured,
and linear-light pairs with half diffusion was worse than half diffusion
alone (28.62, 23.58 and 10.17).

`--penalty p` sets the weight of the span term in each pair's score
(the reference encoder's is 0.204; a larger one favours pairs of closer
colours, a smaller one wider pairs and more dithering). The output is
still a screen.

**Trying palette designs.** Two encode flags take the encoder beyond what
a screen file can hold, for weighing palette ideas as the specification
did; the output is then a PNG to look at and measure (`-o file.png`), not a
screen:

| Flag | Effect |
| --- | --- |
| `--banks file.csv` | encodes over these colours instead of a palette's: one row per colour, columns `bank`, `r`, `g`, `b` (or `hex`); each cell takes its two colours from one bank. Without a `bank` column every colour is in one bank, as in ZXPAL3 and ZXPAL4 |
| `--cell WxH` | cells of another size (`4` for 4x4), which must divide the picture. 4x4 cells dither with the 4x4 Bayer matrix, other sizes with the 8x8 one laid over the whole picture, as the specification's tools do |

`zx scr palette zxpalN --banks -o file.csv` writes a palette's banks in
the encoder's order, and encoding over that file gives the palette's own
encoding pixel for pixel. 4x4 cells, cells 4 wide and 8 tall, and other
penalties give the pictures the specification's `zxcommon.render` and
`zxfast.render_fast` give.

`zx scr fitregs` is the specification's register search
(`tools/z4_palette_search.py`): it adds `--add n` colours to ZXPAL3 or
ZXPAL4 (`--palette`, `--registers`) or to a one-bank table (`--banks`),
chosen so that the pixels of the given pictures lie nearer to some colour.
It blurs each picture by 1.5, samples its pixels, picks the colours
greedily from candidates drawn from the sample, each time the one that
most reduces the squared distance to the nearest colour, and refines them
by k-means with the base colours fixed. It writes the base colours and the
new ones as one bank, for `encode --banks`, and prints how far the sample
lies from its nearest colour before and after. Its random draws are its own
and seeded (`--seed`), so a fit repeats exactly but its colours are not the
Python script's. The pictures are used as they are: fit them first.

Run as the specification ran it, on its ten development pictures in two
folds of five (each fold's pictures encoded with colours fitted to the
other fold), the mean error came out as follows:

| Colours | ZXPAL3 | +8 | +16 | +32 | +48 |
| --- | --- | --- | --- | --- | --- |
| `zx scr fitregs` | 13.16 | 10.14 | 9.23 | 8.58 | 7.95 |
| the specification (section 17) | 13.16 | 10.28 | 9.42 | 8.47 | 7.91 |

```
zx scr palette zxpal4 --banks -o zxpal4_banks.csv
zx scr encode fitted.png --mode zxmode2 --banks zxpal4_banks.csv --cell 4 -o c4.png
zx scr fitregs --palette zxpal3 --add 16 -o z3_16.csv dev/*.png
zx scr encode test.png --mode zxmode2 --banks z3_16.csv -o test_16.png
zx scr compare --metric blur test.png test_16.png
```

**Comparing.** `compare` counts the pixels where two images differ and
exits non-zero if any do; a 256x192 image is doubled to compare it with a
512x384 one, as ZXMODE1 appears on the ZXMODE2 raster. Either side may be a
screen file. `--metric` gives the specification's measures instead:

| `--metric` | Measure |
| --- | --- |
| `blur` | the specification's error: RMS difference (0-255) after a Gaussian blur of 1.5 pixels per channel, computed as its tools compute it (Pillow's blur), so the numbers match theirs |
| `light` | the same with a blur of 0.75 |
| `blur=<sigma>` | any other blur |
| `rms` | no blur |
| `ssim` | SSIM, as scikit-image computes it with its defaults |
| `all` | all of the above |

```
zx scr fit photo.jpg --resize fill --filter lanczos -o fitted.png
zx scr encode fitted.png --mode zxmode2 --palette zxpal4 -o photo.zm2
zx scr compare --metric blur fitted.png photo.zm2 --palette zxpal4
```

**Assets and palettes.** A ZCUT collection records, for every attributed
asset, its palette (which fixes the attribute format: ZXPAL0's classic
byte, ZXPAL1's and ZXPAL2's banked byte, or ZXPAL3's and ZXPAL4's ink and
paper register planes) and, when it was cut from a screen with loaded
registers, its own register table, so it renders and converts as it was
drawn. Version 1 files are ZXPAL0 throughout, version 2 adds palettes and
two-byte attributes, version 3 adds register tables; each collection is
written at the lowest version that holds it.

`cut` reads any screen (`--mode`, `--palette`, `--registers`) and keeps a
cell-aligned region's attributes in its palette. `paste` writes an
attributed asset's attributes at a cell-aligned position and, when the
asset's palette or registers differ from the target's, converts them first
and says so. `convert --to <zxpalN>` (with `--to-registers`) does the same
conversion on its own, for one asset (`--name`) or a whole collection, or
for a screen (`--to-mode` may switch it between `zxmode0` and `zxmode1`,
which share a size). The bitmap is kept; each cell takes the target colours
nearest its own ink and paper, weighted by how many of its pixels each
covers, and within one bank for a one-byte palette, whose ink and paper
share a bank. A colour the target holds exactly is kept exactly; flash is
kept only from ZXPAL0 to ZXPAL0.

`flip -h` mirrors left to right and `flip -v` top to bottom: a screen
(written back in its own format), a collection's assets (all, or
`--name`), or an image (written as PNG). Attribute cells move with their
pixels.

```
zx scr cut title.zm2 --palette zxpal4 --cells 2,2,8,4 --name logo -o ui.cut
zx scr paste ui.cut:logo game.scr --at 64,16 --op copy
zx scr convert ui.cut --to zxpal1 -o ui_zxpal1.cut
zx scr flip ui.cut --name logo -h -o ui_mirrored.cut
```

`palette` prints a palette's registers (number, hex, red, green, blue), or
writes them as CSV or as a PNG of swatches, one row per bank or level;
`--registers` shows a loaded table instead of the reset values.

```
zx scr palette zxpal4 -o zxpal4.png
zx scr compare expected.png rebuilt-from-simulation.png
```

### zx next

```
zx next palette info    [-layout l] <file>
zx next palette decode  [-layout l] [-o file.png] <file>
zx next palette encode  [colour flags] -o file.nxp <image>
zx next palette convert -to l [-from l] [-round m] -o out <file>
zx next palette std     [-layout l] -o file.nxp
zx next sprite  info    [-depth 8|4] <file.spr>
zx next sprite  decode  [-depth 8|4] [-palette f] [-transparent n] [-per-row n] [-range a-b] [-split dir] -o file.png <file.spr>
zx next sprite  encode  [-depth 8|4] [-dedup] [-pad n] [colour flags] -o file.spr <image>
zx next attrs   encode  [-fixed5] -o file.bin <file.json>
zx next attrs   decode  [-fixed5] [-o file.json] <file.bin>
zx next bitmap  info    <file.nxi>
zx next bitmap  decode  [-mode m] [-palette f] [-transparent n] -o file.png <file.nxi>
zx next bitmap  encode  -mode m [-embed] [-resize r] [colour flags] -o file.nxi <image>
zx next timex   decode  -mode hires|hicolour [-ink n] -o file.png <file>
zx next timex   encode  -mode hires|hicolour [-ink n] [-resize r] -o file <image>
zx next tiles   encode  [tile flags] [colour flags] -o base <image>
zx next tiles   decode  [-size WxH] [-depth n] [-map f -map-size WxH ...] -o file.png <file.nxt>
zx next nex     info    <file.nex>
zx next nex     extract -o dir <file.nex>
zx next nex     build   [flags] -o file.nex
```

ZX Spectrum Next graphics and program files. Every Next colour is 9-bit RGB333,
three bits each of red, green and blue. The [Next handling manual](NEXT-HANDLING.md)
works through them for a small game; this section is the reference.

**Palettes.** A palette file is a list of colours with no header. Four
layouts are read and written (`-layout`, `-from`, `-to`):

| Layout | Bytes per colour | Holds | Written by |
|---|---|---|---|
| `rgb333` (default) | 2 | RGB332, then the low blue bit in bit 0 (bit 7, the Layer 2 priority bit, is kept as read) | gfx2next, nextraw: the usual `.nxp` |
| `rgb332` | 1 | RRRGGGBB; the hardware shows the missing blue bit as the OR of the other two | gfx2next `-pal-rgb332` |
| `gbr444` | 2 | GGGGBBBB then xxxxRRRR | gfx2next `-pal-gbr444` |
| `bgr222` | 1 | xxBBGGRR, the Master System layout | gfx2next `-pal-bgr222` |

`palette std` writes the Next's standard palette, in which index i is the
RGB332 colour i. `palette convert` rewrites a palette in another layout.

**Colour flags.** `palette`, `sprite`, `bitmap` and `tiles encode` share the
flags that turn an image into palette indices:

| Flag | Meaning |
|---|---|
| `-colors n` | palette size (default 256, or 16 at 4 bits per pixel) |
| `-round m` | how an 8-bit channel becomes 3 bits: `nearest` (the default, and nextbmp's), `floor`, `ceil`, or `distance` (gfx2next's default: the nearest of the 512 colours, the lower on a tie; it differs from `nearest` only on exact ties) |
| `-dither d` | `none` (default) or `fs` (Floyd-Steinberg), used when colours are chosen or matched |
| `-transparent n` | pixels less than half opaque take index n; that entry holds the RGB332 colour of n. Layer 2 compares that colour with NextReg 0x14, and sprites compare the index with NextReg 0x4B (4-bit sprites its low four bits), so 0xE3 suits both at their reset values (3 for 4-bit sprites); the tilemap compares its index with NextReg 0x4C |
| `-match f` | map the image's colours to the nearest of this palette's (`-match-layout` gives its layout) |
| `-std` | map onto the standard palette (8 bits only; gfx2next `-pal-std`) |
| `-min` | sort the palette, drop duplicates and renumber the pixels (gfx2next `-pal-min`) |
| `-palette f` | write the palette here, in `-layout`'s layout |

An indexed image (an indexed PNG or BMP) keeps its own palette in its own
order, and its pixel indices are used as they are, as gfx2next does; at 4 bits
per pixel the palette is its first 16 colours and an index its low four bits.
Any other image is matched (`-match`), mapped to the standard palette (`-std`)
or reduced: when it has no more distinct RGB333 colours than fit, they are
kept exactly, in the order they first appear; otherwise colours are chosen by
median cut, splitting first the group of colours that lies furthest from its
mean. Images may be PNG, BMP (uncompressed, 1 to 32 bits), GIF or JPEG.

**Sprites.** A `.spr` file is 16x16 patterns one after another: 256 bytes each
at 8 bits per pixel, 128 at 4 (the left pixel in the high nibble). `-dedup`
drops patterns identical to an earlier one and says which; `-pad n` fills the
sheet to n patterns with the `-transparent` index (or 0). `decode -range a-b`
draws some patterns only, and `-split dir` writes each as its own PNG
(`dir/NNN.png`, numbered from the file).

**Sprite attributes.** `attrs` converts between a JSON list and the table of
four or five bytes per sprite that port 0x57 takes. Each JSON object has `x`,
`y`, `visible`, `pattern` (0-63) and optionally `palette_offset`, `mirror_x`,
`mirror_y`, `rotate`, `extended` (send the fifth byte), `relative`, `four_bit`,
`n6` (the upper half of a 4-bit pattern slot), `unified`, `scale_x`, `scale_y`
(0-3 for 1x to 8x), `relative_palette` and `relative_pattern`. A relative
sprite's `x` and `y` are its offset from the anchor, -128 to 127. `-fixed5`
writes five bytes for every sprite (setting each E bit, so a port 0x57 upload
stays in step). The layout follows the Next's documentation; it has not been
checked against hardware here.

**Bitmaps.** `bitmap` reads and writes Layer 2 and LoRes screens (`.nxi`,
`.sl2`, `.slr`): raw pixels with no header, optionally preceded by their
palette (`-embed`; `decode` finds an embedded one from the file's size).

| Mode | Size | Bits | Stored | Bytes |
|---|---|---|---|---|
| `256x192` | 256x192 | 8 | row by row | 49152 |
| `320x256` | 320x256 | 8 | column by column | 81920 |
| `640x256` | 640x256 | 4 | column by column, two pixels per byte | 81920 |
| `lores` | 128x96 | 8 | row by row | 12288 |
| `lores4` | 128x96 | 4 | row by row | 6144 |

`320x256` and `640x256` files are the same size, so `decode` takes a 81920-byte
file as `320x256` unless `-mode 640x256` says otherwise. An image of another
size needs `-resize` (`stretch`, `bestfit`, `centre` or `fill`, as for `zx scr
encode`).

**Timex screens.** `timex` handles the two Timex modes the Next shares: HiRes,
512x192 in one ink colour (`-ink`, 0-7) and its complement as paper, and
HiColour, 256x192 with an attribute for every 8x1 pixels. Both files are 12288
bytes, the screens at 0x4000 and 0x6000 one after the other (for HiRes, the
even byte columns and then the odd), so `-mode` is always needed. HiColour
encoding chooses each 8x1 span's ink, paper and brightness for the least
error.

**Tiles.** `tiles encode` cuts an image into tiles (`-size`, default 8x8) and
writes `base.nxt` (the tiles: 8, 4 or 1 bits per pixel, `-depth`), `base.nxm`
(the map) and, with `-block WxH`, `base.nxb` (blocks of tile indices, the map
then holding block indices):

| Flag | Meaning (gfx2next's equivalent) |
|---|---|
| `-dedup d` | `none` keeps every tile; `repeat` drops repeats, `mirror` also mirrored ones, `rotate` also turned ones; a mirrored or turned match is recorded in the map entry's attribute bits (`-tile-norepeat`, `-tile-nomirror`, `-tile-norotate`) |
| `-map16` | two bytes per map entry: tile index, then the attribute byte (palette offset in bits 7-4, X mirror, Y mirror, rotate, index bit 8); needed for attributes and for more than 256 tiles (`-map-16bit`) |
| `-map-columns` | write the map column by column (`-map-y`) |
| `-offset n`, `-pal-offset n` | add n to every tile index; set every entry's palette offset (`-tile-offset`, `-tile-pal`) |
| `-block WxH`, `-block-dedup`, `-block16` | blocks of tiles (`-block-size`, `-block-norepeat`, `-block-16bit`) |
| `-no-map` | write no map |

At 4 bits a tile stores its pixels' low four bits and its map entry's palette
offset is the high four bits of its last pixel, as gfx2next does, so an indexed
image drawn in banks of 16 colours keeps its banks. `tiles decode` draws a tile
set, or with `-map` (and `-map-size`, `-map16`, `-map-columns`, `-offset` as
written) the map itself, attributes applied.

**NEX files.** `nex info` summarises a NEX file. `nex extract` writes its parts
to a directory: `header.json`, `palette.nxp`, `screen_<kind>.<ext>`,
`copper.bin`, `bank_NNN.bin` and `after.bin`. `nex build` puts a file
together:

| Flag | Meaning |
|---|---|
| `-header f` | start from a `header.json` written by `extract` |
| `-pc a`, `-sp a`, `-entry-bank n` | start address, stack pointer, bank mapped at 0xC000 |
| `-bank N=file` | a 16 KB bank (a shorter file is padded with zeros); repeatable |
| `-bin ADDR=file` | a file loaded at a 64 KB address, banks 5, 2 and 0 at 0x4000, 0x8000 and 0xC000; repeatable |
| `-screen kind=file` | loading screen: `l2`, `l2_320`, `l2_640`, `lores`, `ula`, `hires` or `hicolour`; repeatable |
| `-palette f` | the 512-byte palette for a Layer 2 or LoRes screen |
| `-border n`, `-core a.b.c`, `-2mb`, `-hires-colour n` | header fields |
| `-checksum`, `-copper f`, `-version 3` | V1.3 features; the version rises to V1.3 by itself when one is used |
| `-append f` | data after the banks |

The layout follows sjasmplus's `SAVENEX`, and the tests rebuild six files
sjasmplus wrote, byte for byte. One exception: sjasmplus (1.24.0 and earlier)
stores the V1.3 checksum of the header as it stood when the file was opened,
not as written at the end, so its checksums do not match their files; `nex
info` says so, and `nex build` writes the checksum of the file it writes.

The graphics output matches the community tools byte for byte: gfx2next for
palettes, sprites, Layer 2 bitmaps and tiles, and nextbmp/nextraw for Layer 2
and for `-round floor` and `ceil`. (In those two modes gfx2next reduces a
palette colour twice, moving it a further step; `zx next` reduces once, as
nextbmp does.)

```
zx next sprite encode hero.png -o hero.spr -palette hero.nxp
zx next sprite decode hero.spr -palette hero.nxp -transparent 0xE3 -o check.png
zx next bitmap encode title.jpg -mode 320x256 -resize fill -dither fs -o title.nxi -palette title.nxp
zx next tiles encode level.png -dedup rotate -map16 -o level -palette level.nxp
zx next nex build -bin 0x8000=game.bin -pc 0x8000 -sp 0xFF00 -screen l2_320=title.nxi -palette title.nxp -o game.nex
```

### zx zx0

```
zx zx0 [-d] [-classic] [-back] [-quick] [-o out] <file>
```

Compresses (or with `-d` decompresses) a file in Einar Saukas's ZX0 format,
which the Next tools and many Spectrum programs use. The compressor is a port
of the reference one, with the same optimal parse, so its bytes are the same as
`zx0`'s. `-classic` writes the v1 format (`zx0 -c`), which gfx2next writes;
`-back` compresses backwards for in-place decompression from the end (`zx0
-b`); `-quick` searches offsets up to 2176 only (`zx0 -q`), faster but
compressing less. The output defaults to the input with `.zx0` added (or
removed, with `-d`). Compression takes time in proportion to the file's length
times the offsets searched (up to 32640): about 3 seconds for 13 KB on an
ordinary machine, 45 seconds for an 80 KB Layer 2 screen, and much less with
`-quick`.

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

Identifies a file's format — by signature (TZX, SZX, PZX, RZX and NEX all
have one) where possible, otherwise by extension — and prints the appropriate
summary, as `zx tap info`, `zx tzx info`, `zx pzx info`, `zx snap info`,
`zx rzx info`, `zx next nex info`, `zx next palette info` (`.nxp`),
`zx next sprite info` (`.spr`) or `zx next bitmap info` (`.nxi`, `.sl2`,
`.slr`) would.

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