# zentools — architecture

zentools is the canonical Go library for ZX Spectrum binary formats: tape images,
snapshots, disk images, and BASIC tokenisation. It exists so that the tools in
the ecosystem (zenas, zenzx, plus3, and future ones) share one verified
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
│   ├── snapshot/    .zxs / .sna / .z80 via a neutral MachineState.
│   │   ├── state.go     MachineState and its sub-structs (the pivot type)
│   │   ├── zxs.go       chunked ZenZX format
│   │   ├── sna.go       48K / 128K .sna
│   │   └── z80.go       .z80 (versioned, compressed)
│   ├── basic/       BASIC tokenise + detokenise (48K and 128K keywords).
│   └── version/     library version constant.
├── cmd/             thin CLI tools over the packages (maketap, tap2tzx, ...).
│                    third-party deps (YAML, flags) live here, not in pkg/.
└── (hygiene: VERSION, syncver.sh, release.sh, CHANGELOG.md, .gitignore, LICENSE)
```

## Format ownership

A format lives in zentools if and only if its purpose is interoperability
between tools. Formats whose value is tied to one project's identity stay native
to that project:

| Format | Owner | Rationale |
|--------|-------|-----------|
| `.tap`, `.tzx` | zentools | interchange tape formats every tool reads |
| `.sna`, `.z80` | zentools | portable snapshot standards |
| BASIC tokenisation | zentools | shared encode/decode |
| `.zxs` | **zenzx (native)** | zenzx's proprietary chunked format; its magic and metadata are emulator-specific |
| `.dsk` (+3DOS) | **plus3 (native)** | mature, tested, CI-backed; zentools may depend on plus3 for disk, never the reverse |

Consequence for consumers: zenzx keeps `.zxs` but swaps its `.sna`/`.z80` for
zentools; plus3 keeps `.dsk` but swaps its broken in-tree TAP for
`zentools/pkg/tap`. No project loses its signature format; all share the
interchange ones.

## Package coupling

Packages do not import each other except that `cmd/` imports `pkg/*`. A format
package importing another format package would couple them; keep them flat.
(The one defensible exception, if it arises: a future `pkg/spectrum` holding
shared constants like the XOR checksum or memory-map addresses. Not needed yet —
TAP and TZX each own their small bit of shared knowledge until duplication
actually hurts.)

## Public API shape (per package)

The naming is uniform so a consumer learns one pattern. Each format provides an
in-memory core plus optional file/stream helpers.

### pkg/tap (built, verified)

```go
func EncodeCode(name string, data []byte, loadAddress uint16) []byte
func EncodeProgram(name string, data []byte, autostart uint16) []byte
func Decode(image []byte) ([]Block, error)   // to add: parse a TAP into blocks
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
`EncodeOptions` carries the metadata/hardware flags as a plain struct; YAML
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

func EncodeZXS(s *MachineState) ([]byte, error)
func DecodeZXS(image []byte) (*MachineState, error)
func EncodeSNA(s *MachineState) ([]byte, error)
func DecodeSNA(image []byte) (*MachineState, error)
func EncodeZ80(s *MachineState) ([]byte, error)
func DecodeZ80(image []byte) (*MachineState, error)
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
