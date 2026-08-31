# Tape handling with zentools

This manual is focused on one thing: building, combining, and editing
multi-block ZX Spectrum tape files (`.tap`, `.tzx`, `.pzx`) with `zx`.
It doesn't repeat the full flag reference for every command — that's in
the [CLI manual](CLI.md) — but instead walks through the *workflows*:
which tool to reach for, why, and how the pieces fit together.

If you're looking for snapshot handling, RZX recordings, or screen/image
conversion, see the [CLI manual](CLI.md) instead. This document is tapes
only.

## Contents

- [The tools, and when to reach for each](#the-tools-and-when-to-reach-for-each)
- [Tutorial: assembling STARQUEST](#tutorial-assembling-starquest)
  - [The parts](#the-parts)
  - [Step 1 — bootstrapping the core with `zx build`](#step-1--bootstrapping-the-core-with-zx-build)
  - [Step 2 — adding pieces as they're ready with `zx edit import`](#step-2--adding-pieces-as-theyre-ready-with-zx-edit-import)
  - [Step 3 — finding and fixing a bad block](#step-3--finding-and-fixing-a-bad-block)
  - [Step 4 — the same tape, reproducibly, from one spec](#step-4--the-same-tape-reproducibly-from-one-spec)
  - [Step 5 — wrapping for distribution](#step-5--wrapping-for-distribution)
- [Reference notes](#reference-notes)
  - [The logical block list, and its limits](#the-logical-block-list-and-its-limits)
  - [`zx edit` vs `zx build`: which one, when](#zx-edit-vs-zx-build-which-one-when)
  - [Combining files: three different tools, three different jobs](#combining-files-three-different-tools-three-different-jobs)

## The tools, and when to reach for each

Six commands touch tape files, and each has a distinct job:

| Command         | Job                                                             |
| --------------- | ---------------------------------------------------------------- |
| `zx tap make`   | Wrap one binary as one CODE block. The simplest case.            |
| `zx tap append` | Concatenate whole TAP files. Simple, TAP-only, byte-level.       |
| `zx tzx make` / `zx pzx make` | Wrap a TAP as TZX/PZX, with archive metadata.      |
| `zx build`      | Describe an entire multi-block tape at once, from a JSON spec.   |
| `zx edit`       | Change one thing about an *existing* multi-block tape: list, extract, delete, import, or append. |
| `zx convert`    | Move between formats, or pull one block out as a flat binary.    |

The tutorial below uses all six in a realistic order: `zx basic` and
`zx scr` to prepare raw pieces, `zx build` to lay down the initial
structure, `zx edit import` to add pieces incrementally as they become
ready, `zx edit delete`/`import` to fix a mistake, and `zx tzx make` to
package the result for release.

## Tutorial: assembling STARQUEST

STARQUEST is a hypothetical Spectrum game: a BASIC loader, a title
screen, a game engine, and three levels — five genuinely different kinds
of content, assembled from five separately-produced files into one
tape. Every command below was run for real to write this tutorial; the
output shown is exactly what it produced.

### The parts

A BASIC loader that sets the screen colours, prints a loading message,
then loads and runs the machine code that follows:

```basic
10 BORDER 1: PAPER 1: INK 7: CLS
20 PRINT "Loading STARQUEST..."
30 LOAD "" CODE
40 RANDOMIZE USR 32768
```

Tokenise it into the Spectrum's internal byte format — this is raw
tokenised bytes, not yet a tape block of any kind:

```
$ zx basic tokenise loader.bas -o loader_tok.bin
```

A title screen, encoded from an ordinary image to the native `.scr`
format (6912 bytes: the pixel bitmap plus colour attributes, exactly as
they'd sit in Spectrum RAM at `0x4000`):

```
$ zx scr encode title.png -o title.scr
wrote title.scr (6912 bytes)
```

And three more pieces that a real project would produce from an
assembler and a level editor — `engine.bin`, `level1.bin`, `level2.bin`,
and `level3.bin`. For this tutorial they're just distinguishable
placeholder bytes; the tape-assembly steps that follow don't care what's
actually inside a CODE block, only its name, length, and load address.

Five pieces, three different "kinds" as far as a tape block is
concerned: one BASIC program, one screen (which a tape sees as an
ordinary CODE block — it's just 6912 bytes with nowhere special to sit
except `0x4000`), and three machine-code blocks.

### Step 1 — bootstrapping the core with `zx build`

The loader and title screen are the tape's fixed skeleton — every build
of this game starts with the same two blocks, in the same order. That's
exactly what `zx build`'s declarative spec is for: describe the known
structure once, in a file that can be re-run identically at any time.

```json
{
  "title": "STARQUEST",
  "blocks": [
    {"kind": "program", "name": "LOADER", "autostart": 10, "file": "loader_tok.bin"},
    {"kind": "code", "name": "TITLE", "org": 16384, "file": "title.scr"}
  ]
}
```

(`16384` is `0x4000` — the spec format uses plain decimal, not the
`0x...` hex strings the CLI flags accept elsewhere; see
[Reference notes](#zx-edit-vs-zx-build-which-one-when).)

```
$ zx build core.json -o starquest.tap
Wrote starquest.tap (7065 bytes)
$ zx edit list starquest.tap
starquest.tap: 4 block(s)
  [0] header  type=Program name="LOADER" autostart=0x000A
  [1] data    103 bytes  checksum_ok=true
  [2] header  type=Code name="TITLE" load=0x4000
  [3] data    6912 bytes  checksum_ok=true
```

### Step 2 — adding pieces as they're ready with `zx edit import`

Unlike the loader and title screen, the engine and levels don't all
exist at once in a real project — they land over time, as each is
finished. `zx edit import` is the incremental tool for exactly that: add
one block to an *existing* tape, appended at the end unless `--at` says
otherwise.

The engine arrives first:

```
$ zx edit import starquest.tap --data engine.bin --name ENGINE --org 0x8000 -o starquest.tap
Wrote starquest.tap (7158 bytes)
```

`-o` names the same file as the input — reading happens before writing,
so this is safe, and it's the natural way to build a tape up over
several sessions rather than always producing a new filename. Then the
three levels, one by one as each is finished:

```
$ zx edit import starquest.tap --data level1.bin --name LEVEL1 --org 0xC000 -o starquest.tap
$ zx edit import starquest.tap --data level2.bin --name LEVEL2 --org 0xC000 -o starquest.tap
$ zx edit import starquest.tap --data level3.bin --name LEVEL3 --org 0xC000 -o starquest.tap
$ zx edit list starquest.tap
starquest.tap: 12 block(s)
  [0] header  type=Program name="LOADER" autostart=0x000A
  [1] data    103 bytes  checksum_ok=true
  [2] header  type=Code name="TITLE" load=0x4000
  [3] data    6912 bytes  checksum_ok=true
  [4] header  type=Code name="ENGINE" load=0x8000
  [5] data    68 bytes  checksum_ok=true
  [6] header  type=Code name="LEVEL1" load=0xC000
  [7] data    128 bytes  checksum_ok=true
  [8] header  type=Code name="LEVEL2" load=0xC000
  [9] data    128 bytes  checksum_ok=true
  [10] header  type=Code name="LEVEL3" load=0xC000
  [11] data    128 bytes  checksum_ok=true
```

All three levels load at the same address, `0xC000` — a real game
typically loads one level at a time into a shared buffer, which is
exactly why they can share a load address on tape without conflict: only
one is ever resident in memory at once.

### Step 3 — finding and fixing a bad block

LEVEL2 turns out to have a bug. `zx edit list --json` gives a scriptable
way to find exactly which index it's at:

```
$ zx edit list starquest.tap --json | python3 -c "
import json, sys
m = json.load(sys.stdin)
for b in m['blocks']:
    if b.get('name') == 'LEVEL2':
        print('LEVEL2 header at index', b['index'])
"
LEVEL2 header at index 8
```

Its header is at index 8, so its data is at index 9 — `zx edit delete`
takes a comma-separated list against the tape's *original* numbering, so
both indices are named together in one call, not two sequential ones
where the second would have shifted:

```
$ zx edit delete starquest.tap --block 8,9 -o starquest.tap
Wrote starquest.tap (7464 bytes)
$ zx edit list starquest.tap
starquest.tap: 10 block(s)
  [0] header  type=Program name="LOADER" autostart=0x000A
  [1] data    103 bytes  checksum_ok=true
  [2] header  type=Code name="TITLE" load=0x4000
  [3] data    6912 bytes  checksum_ok=true
  [4] header  type=Code name="ENGINE" load=0x8000
  [5] data    68 bytes  checksum_ok=true
  [6] header  type=Code name="LEVEL1" load=0xC000
  [7] data    128 bytes  checksum_ok=true
  [8] header  type=Code name="LEVEL3" load=0xC000
  [9] data    128 bytes  checksum_ok=true
```

Then the fixed version goes back in at the same position, `--at 8`,
putting it back between LEVEL1 and LEVEL3 rather than tacked onto the
end:

```
$ zx edit import starquest.tap --data level2_fixed.bin --name LEVEL2 --org 0xC000 --at 8 -o starquest.tap
Wrote starquest.tap (7617 bytes)
$ zx edit list starquest.tap
starquest.tap: 12 block(s)
  [0] header  type=Program name="LOADER" autostart=0x000A
  [1] data    103 bytes  checksum_ok=true
  [2] header  type=Code name="TITLE" load=0x4000
  [3] data    6912 bytes  checksum_ok=true
  [4] header  type=Code name="ENGINE" load=0x8000
  [5] data    68 bytes  checksum_ok=true
  [6] header  type=Code name="LEVEL1" load=0xC000
  [7] data    128 bytes  checksum_ok=true
  [8] header  type=Code name="LEVEL2" load=0xC000
  [9] data    128 bytes  checksum_ok=true
  [10] header  type=Code name="LEVEL3" load=0xC000
  [11] data    128 bytes  checksum_ok=true
```

Extracting block 9 confirms it's genuinely the fixed content, not the
old one under a new name:

```
$ zx edit extract starquest.tap --block 9 -o check.bin
$ head -c 20 check.bin
LEVEL-TWO-FIXED-LEVE
```

### Step 4 — the same tape, reproducibly, from one spec

Once the final composition is known, it can be written down as a
single `zx build` spec — six blocks this time, the two fixed ones from
Step 1 plus the four added incrementally since:

```json
{
  "title": "STARQUEST",
  "blocks": [
    {"kind": "program", "name": "LOADER", "autostart": 10, "file": "loader_tok.bin"},
    {"kind": "code", "name": "TITLE", "org": 16384, "file": "title.scr"},
    {"kind": "code", "name": "ENGINE", "org": 32768, "file": "engine.bin"},
    {"kind": "code", "name": "LEVEL1", "org": 49152, "file": "level1.bin"},
    {"kind": "code", "name": "LEVEL2", "org": 49152, "file": "level2_fixed.bin"},
    {"kind": "code", "name": "LEVEL3", "org": 49152, "file": "level3.bin"}
  ]
}
```

```
$ zx build full.json -o starquest_rebuilt.tap
Wrote starquest_rebuilt.tap (7617 bytes)
$ cmp starquest.tap starquest_rebuilt.tap && echo "IDENTICAL"
IDENTICAL
```

Byte-for-byte identical to the tape assembled incrementally across
Steps 1-3. Neither approach is "more correct" than the other — they're
the same underlying block list, described two different ways. Building
up with `zx edit` suits a project where pieces arrive over time and you
want to see the tape grow; writing a `zx build` spec suits a release
process where you want one reproducible command that always produces the
exact same result from the exact same inputs.

### Step 5 — wrapping for distribution

The final step: wrap the finished TAP as a TZX with proper archive
metadata for release.

```
$ zx tzx make starquest.tap --title "STARQUEST" --author "haitch" --year 2026 -o starquest.tzx
Wrote starquest.tzx (7692 bytes)
$ zx tzx info starquest.tzx
starquest.tzx: 13 blocks
  [0] 0x32
  [1] 0x10
  [2] 0x10
  [3] 0x10
  [4] 0x10
  [5] 0x10
  [6] 0x10
  [7] 0x10
  [8] 0x10
  [9] 0x10
  [10] 0x10
  [11] 0x10
  [12] 0x10
```

Thirteen blocks, not twelve — block `0x32` at the start is the
archive-info metadata block `--title`/`--author`/`--year` added; the
other twelve are the same content blocks as before. Converting back to
TAP confirms exactly this: the metadata block, having no TAP
equivalent, is dropped with an explicit note, while every content block
survives untouched.

```
$ zx convert starquest.tzx -o roundtrip.tap
Note: dropped 1 block(s) with no TAP-representable data (pilot tones, pulse
sequences, direct recordings, or structural/metadata blocks)
$ cmp starquest.tap roundtrip.tap && echo "IDENTICAL"
IDENTICAL
```

STARQUEST is done: one BASIC loader, a title screen, an engine, three
levels (one of them replaced mid-project), assembled two different ways
that agree exactly, and packaged for release — with every step verified
against the real content at every stage, not just "the command didn't
error."

## Reference notes

### The logical block list, and its limits

`zx edit` and `zx build` both work at the level of a tape's flat
*logical* block list — the loadable header/data blocks themselves, the
same list `zx convert`'s own tape normalisation already works with. A
TZX's own structural blocks (pauses, group markers, `STOP`, archive
metadata — like the one in Step 5 above) and a PZX's own
`PAUS`/`BRWS`/`STOP` blocks aren't part of that list and don't survive a
`delete`/`import`/`append`/`build` round trip. This isn't a bug to work
around; it's the same scope `zx convert` already has going from TZX down
to TAP, and it's why Step 5's round trip dropped exactly one block and
named it explicitly rather than silently losing it.

### `zx edit` vs `zx build`: which one, when

|                          | `zx edit`                        | `zx build`                        |
| ------------------------ | --------------------------------- | ---------------------------------- |
| Shape                    | One change at a time              | The whole tape, described at once  |
| Input                    | An existing tape file             | A JSON spec + separate data files  |
| Best for                 | Growing a tape incrementally, fixing one block, inspecting an existing file | A known, reproducible final composition |
| Can start from nothing?  | No — always edits an existing file | Yes — this is how Step 1 above began |

If you don't yet have a tape to edit, `zx build` is where you start
(Step 1). If you have one and want to change one thing, `zx edit` is
usually less to write than updating and re-running a whole spec (Steps
2-3). Nothing stops using both across a project's lifetime, as this
tutorial did.

### Combining files: three different tools, three different jobs

Three commands can produce one tape from several inputs, and they're not
interchangeable:

- **`zx tap append`** — TAP files only, both in and out. Genuinely raw
  byte concatenation, since TAP has no container structure of its own
  to corrupt; the fastest and simplest option when every input already
  is TAP.
- **`zx edit append`** — any mix of TAP/TZX/PZX inputs, any one of the
  three as output. Decodes each input's own logical block list and
  re-encodes them together, so it works where raw concatenation
  wouldn't (TZX and PZX both have container structure a byte-level
  `cat` would break).
- **`zx build`** — not combining *existing tape files* at all, but
  assembling fresh blocks from raw data files (binaries, tokenised
  BASIC, `.scr` screens) named in a spec. This is what Steps 1 and 4
  used, since the individual pieces were never their own standalone
  tapes to begin with.
