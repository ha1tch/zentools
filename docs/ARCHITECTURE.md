# zentools — architecture

zentools is the canonical Go library for ZX Spectrum binary formats: tape images,
snapshots, BASIC tokenisation, screens and graphics (classic, ZXMODE and
Spectrum Next), and NEX files. Disk images stay with plus3 (see Format
ownership). It exists so that the tools in
the ecosystem (zenas, zenzx, plus3, zendis, and future ones) share one verified
implementation of each format instead of maintaining several partial,
"sometimes badly" copies.

This document defines the package layout, the public API shape, and the rules
that keep the library reusable. It is the contract the migration builds to.

## Design principles

1. **Formats are pure data transforms.** A format package turns bytes into a
   neutral Go value and back. It never imports an emulator, a disk runtime, or
   any consumer. If a function needs live emulator state, it does not belong in
   zentools — it belongs in the consumer's adapter.

2. **In-memory first.** Every codec has a `[]byte`-in / `[]byte`-out (or
   value-out) core. File and stream helpers are thin wrappers over that core, so
   an assembler that already holds the bytes never has to round-trip through a
   file.

3. **No surprise dependencies.** The format packages depend only on the Go
   standard library. Anything needing a third-party dependency (YAML config,
   CLI parsing) lives in `cmd/`, never in `pkg/`.

4. **A neutral machine state is the snapshot pivot.** Snapshot formats encode and
   decode to a single `MachineState` struct that describes a Spectrum's
   registers, memory, and hardware config. Consumers adapt their own live state
   to/from `MachineState`; the codecs never see the consumer's types.

5. **Verified against real artifacts.** Each package ships tests that check its
   output against an independent oracle (another tool's bytes, a real program, an
   emulator load) — not just self-consistency. Correctness is the product.

## Package layout

```
github.com/ha1tch/zentools
├── pkg/
│   ├── tap/         TAP tape images. Encode/decode CODE and Program blocks.
│   ├── tzx/         TZX tape images. Encode (and decode) the common block set.
│   ├── pzx/         PZX tape images, the simpler TZX successor.
│   ├── snapshot/    .sna / .z80 via a neutral MachineState.
│   │   ├── state.go     MachineState and its sub-structs (the pivot type)
│   │   ├── sna.go       48K / 128K .sna
│   │   └── z80.go       .z80 (versioned, compressed)
│   ├── szx/         .szx (zx-state) via the same neutral MachineState.
│   ├── rzx/         .rzx input-recording files (embeds a snapshot block).
│   ├── basic/       BASIC tokenise + detokenise (48K and 128K keywords).
│   ├── scr/         .scr screens, image conversion (PNG, BMP, GIF, JPEG), .cut
│   │                asset collections, ZXMODE pictures and ZXPAL palettes.
│   ├── zxn/         ZX Spectrum Next graphics: palettes, sprites and attributes,
│   │                Layer 2, LoRes and Timex screens, tiles, colour reduction.
│   ├── nex/         NEX files, the Spectrum Next executable format.
│   ├── zx0/         ZX0 compression (a port of the reference compressor).
│   ├── bdf/         BDF bitmap font reader, used internally by scr's OCR.
│   ├── symfile/     Symbol files of other tools: CSpect maps, pasmo, sjasmplus, z88dk.
│   ├── build/       overlay machine code onto a boot state; emit tapes/snapshots.
│   └── version/     library version constant.
├── cmd/             CLI tools over the packages.
│   ├── maketap, totap, loadtap, tap2tzx   drop-in zxgotools replacements.
│   └── zx/          modern unified front-end (tap, tzx, pzx, basic, scr, next, zx0,
│                    snap, rzx, convert, edit, build, info). Flag parsing lives
│                    here, not in pkg/; zentools has no third-party dependencies.
└── (hygiene: VERSION, Makefile, .repoman.json, release.sh, syncver.sh,
    CHANGELOG.md, .gitignore, LICENSE)
```

## Format ownership

A format lives in zentools if and only if its purpose is interoperability
between tools. Formats whose value is tied to one project's identity stay native
to that project:

| Format | Owner | Rationale |
|--------|-------|-----------|
| `.tap`, `.tzx` | zentools | interchange tape formats every tool reads |
| `.sna`, `.z80`, `.szx`, `.rzx` | zentools | portable snapshot and recording standards |
| `.scr`, `.zm1`, `.zm2`, `.cut` | zentools | Spectrum screens, ZXMODE pictures and asset collections, shared by the graphics tools |
| `.nxp`, `.spr`, `.nxi`, `.nxt`, `.nxm`, `.nxb`, `.nex` (Spectrum Next) | zentools | the Next's interchange graphics and program formats, written by the community tools (gfx2next, nextbmp, sjasmplus); zentools absorbed zenpal for them |
| BASIC tokenisation | zentools | shared encode/decode |
| `.zxs` | **zenzx (native)** | zenzx's proprietary chunked format; its magic and metadata are emulator-specific |
| `.dsk` (+3DOS) | **plus3 (native)** | mature, tested, CI-backed; zentools may depend on plus3 for disk, never the reverse |

Consequence for consumers: zenzx keeps `.zxs` but swaps its `.sna`/`.z80` for
zentools; plus3 keeps `.dsk` but swaps its broken in-tree TAP for
`zentools/pkg/tap`. No project loses its signature format; all share the
interchange ones.

## Package coupling

A format package imports another only to share a model, never for convenience:
`szx` and `build` use `snapshot`'s machine state, `build` composes `tap`, `tzx`,
`pzx`, `basic` and `nex`, `scr`'s OCR uses `bdf`, and `zxn` takes the Spectrum ULA colours
from `scr` so they are defined once. `cmd/` imports `pkg/*`. Otherwise keep the
packages flat.
(The one defensible exception, if it arises: a future `pkg/spectrum` holding
shared constants like the XOR checksum or memory-map addresses. Not needed yet —
TAP and TZX each own their small bit of shared knowledge until duplication
actually hurts.)

## Public API shape (per package)

The naming is uniform so a consumer learns one pattern. Each format provides an
in-memory core plus optional file/stream helpers.

### pkg/tap

```go
func EncodeCode(name string, data []byte, loadAddress uint16) []byte
func EncodeProgram(name string, data []byte, autostart uint16) []byte
func Decode(image []byte) ([]Block, error)
func DecodeBlock(raw []byte) (Block, error)
func WriteCodeFile(inputPath, outputPath, name string, loadAddress uint16) error
```

`Decode` returns typed blocks (header/data, with the header fields parsed) so a
consumer like zenzx replaces its in-tree `loadTAP` with `tap.Decode` and then
does its emulator-specific thing with the blocks.

### pkg/tzx

```go
func EncodeFromTAP(tapImage []byte, opts EncodeOptions) ([]byte, error)
func Encode(blocks []Block, opts EncodeOptions) ([]byte, error)
func Decode(image []byte) ([]Block, error)
```

The writer is lifted out of zxgotools' `cmd/tap2tzx` and made dependency-free.
`EncodeOptions` carries the metadata/hardware flags as a plain struct; JSON
parsing into that struct stays in `cmd/tap2tzx`.

### pkg/snapshot

```go
type MachineState struct {
    Model   Model            // 48K, 128K, +2, +3
    CPU     CPUState         // registers, including shadows, I, R, IFF, IM
    Memory  Memory           // RAM pages (and which is paged where)
    Paging  PagingState      // 128K paging port state
    IO      IOState          // border, etc.
    // Audio/FDC optional, format-permitting.
}

func EncodeSNA(s *MachineState) ([]byte, error)    // 48K
func DecodeSNA(image []byte) (*MachineState, error)
func EncodeSNA128(s *MachineState) ([]byte, error) // 128K
func DecodeSNA128(image []byte) (*MachineState, error)
func EncodeZ80(s *MachineState) ([]byte, error)
func EncodeZ80v3(s *MachineState) ([]byte, error)
func DecodeZ80(image []byte) (*MachineState, error) // versions 1 to 3
```

zenzx keeps a small adapter: `func (zx *ZenZX) toMachineState() *MachineState`
and `func (zx *ZenZX) fromMachineState(s *MachineState)`. Its in-tree
`SaveSNA`/`LoadSNA`/etc. become one-liners over the zentools codecs.

### pkg/basic

```go
func Tokenise(src string, opts ...Option) ([]byte, error)
func Detokenise(prog []byte) (string, error)
func LooksTokenised(data []byte) bool
```

Both directions (zxgotools has only tokenise; plus3 has both). Options carry
case-independence and the like (borrowed from zxgotools' parser).

## Source provenance and per-package plan

This was the plan when zentools began. The packages it lists are built;
the [library manual](LIBRARY.md) describes them as they are.

| Package | Best source | Plan |
|---------|-------------|------|
| `tap` | zentools (already built from zxgotools, fixed + verified) | Add `Decode`; done otherwise. |
| `tzx` | zxgotools `cmd/tap2tzx` (writer verified this session) | Factor writer out, drop YAML to cmd, add `Decode`. |
| `snapshot` | zenzx (only implementation) | Lift `.sna`/`.z80` codecs, decouple to `MachineState`, audit/fix. `.zxs` stays native to zenzx (see Format ownership). |
| `basic` | plus3 (correct, both directions, tested) as base; zxgotools for architecture | Reconcile. plus3 is the correctness base; borrow zxgotools' options. Fix the float-number case zxgotools' test exposes. |
| disk (`.dsk`) | plus3 `pkg/diskimg` (strong, tested, CI) | NOT moved into zentools. plus3 stays the disk authority; zentools may depend on plus3 for disk, not the reverse. |

Note on disk: plus3 is mature and CI-backed at what it does. Rather than absorb
it, the ecosystem treats plus3 as the disk-image library and zentools as the
tape/snapshot/basic library. plus3's one weak spot — its broken in-tree TAP
converter — is fixed by having plus3 import `zentools/pkg/tap`, which also
removes a duplicate.

## Consumer migration (after packages are verified)

1. **plus3** replaces its broken `convert.go` TAP code with `zentools/pkg/tap`
   calls. This is a correctness fix, the highest immediate payoff.
2. **zenas** uses `pkg/tap` / `pkg/tzx` to emit tape output (Feature 3).
3. **zenzx** swaps its in-tree tape parsing and snapshot codecs for zentools +
   thin adapters, keeping only emulator-coupled playback (pulse generation,
   fast-inject, ROM trap). The encode→load→memory round-trip is the regression
   gate.

## Verification standard

Each package's tests must include at least one independent oracle:

- `tap`: byte-comparison vs pasmo; structural decode; emulator load (zenzx).
- `tzx`: structural decode; vs sjasmplus/zxgotools output; emulator load.
- `snapshot`: round-trip (encode then decode equals input); load in a real
  emulator; compare against a snapshot a known tool produced.
- `basic`: round-trip (tokenise then detokenise); decode of real programs
  (plus3's existing test corpus); the number-encoding cases.

Self-consistency (round-trip) is necessary but not sufficient; an external
reference is required wherever one exists.
