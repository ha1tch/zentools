# zentools library manual

The zentools library is a set of Go packages for reading, writing, and
converting ZX Spectrum file formats in memory. Every package depends only on the
Go standard library. This manual describes each package, its principal types and
functions, and how they compose.

For the command-line tools built on these packages, see the
[CLI tools manual](CLI.md).

## Contents

- [Overview](#overview)
- [pkg/tap](#pkgtap)
- [pkg/tzx](#pkgtzx)
- [pkg/basic](#pkgbasic)
- [pkg/snapshot](#pkgsnapshot)
- [pkg/build](#pkgbuild)
- [pkg/version](#pkgversion)
- [Composing the packages](#composing-the-packages)

## Overview

The import path root is `github.com/ha1tch/zentools`. Each package owns one
format family:

| Package         | Responsibility                                          |
| --------------- | ------------------------------------------------------- |
| `pkg/tap`       | TAP tape images.                                        |
| `pkg/tzx`       | TZX tape images.                                         |
| `pkg/basic`     | ZX BASIC tokenisation and detokenisation.               |
| `pkg/snapshot`  | `.sna` and `.z80` snapshots, via a neutral state type.  |
| `pkg/build`     | Overlaying code onto a boot state to emit artifacts.    |
| `pkg/version`   | The library version constant.                           |

The packages do not import one another except that `pkg/build` builds on `tap`,
`tzx`, `basic`, and `snapshot`. A typical flow runs from source material (a
binary or BASIC text), through `build`, out to a tape or snapshot format.

Install with:

```sh
go get github.com/ha1tch/zentools@latest
```

Requires Go 1.25 or later.

## pkg/tap

Reads and writes TAP tape images. A TAP image is a sequence of length-prefixed
blocks; each program is represented by a header block followed by a data block.

### Encoding

```go
func EncodeCode(name string, data []byte, loadAddress uint16) []byte
func EncodeProgram(name string, data []byte, autostart uint16) []byte
func WriteCodeFile(inputPath, outputPath, name string, loadAddress uint16) error
```

`EncodeCode` returns the complete TAP bytes for a CODE block — a header naming
the block and recording its load address, followed by the data. `EncodeProgram`
does the same for a BASIC Program block, recording an auto-run line in place of a
load address. `WriteCodeFile` is a file-to-file convenience wrapper around
`EncodeCode`.

The CODE-block encoding is verified byte-identical to pasmo's `--tap` output.

### Decoding

```go
func Decode(image []byte) ([]Block, error)
```

`Decode` parses a TAP image into its blocks:

```go
type Block struct {
    IsHeader   bool
    Flag       byte   // 0x00 for a standard header, 0xFF for standard data
    Data       []byte // payload between the flag and the checksum
    Checksum   byte
    ChecksumOK bool   // whether the stored checksum matches the computed one
    Type       byte   // TypeProgram / TypeCode / ...
    Name       string // ten-character name, trailing spaces trimmed
    DataLength uint16
    Param1     uint16 // load address (Code) or autostart line (Program)
    Param2     uint16 // 0x8000 (Code) or program length (Program)
}
```

The header fields (`Type`, `Name`, `DataLength`, `Param1`, `Param2`) are
populated only when `IsHeader` is true. The block type constants are
`TypeProgram` (0x00), `TypeNumArray` (0x01), `TypeCharArray` (0x02), and
`TypeCode` (0x03).

### Example

```go
import "github.com/ha1tch/zentools/pkg/tap"

image := tap.EncodeCode("mygame", code, 0x8000)
os.WriteFile("mygame.tap", image, 0o644)

blocks, err := tap.Decode(image)
if err != nil {
    return err
}
for _, b := range blocks {
    if b.IsHeader && b.Type == tap.TypeCode {
        fmt.Printf("%s loads at 0x%04X\n", b.Name, b.Param1)
    }
}
```

## pkg/tzx

Reads and writes TZX tape images, following the TZX v1.20 specification. TZX is a
richer container than TAP: a signature and version header, then typed blocks. A
TAP image converts to TZX by wrapping each block as a standard-speed (0x10)
block; metadata, hardware-type, and group blocks can be added around the data.

### Encoding from a TAP image

```go
func EncodeFromTAP(tapImage []byte, opts EncodeOptions) ([]byte, error)
```

`EncodeFromTAP` wraps a TAP image as TZX, with optional metadata:

```go
type EncodeOptions struct {
    Title       string         // archive title (0x32)
    Author      string         // archive author
    Year        string         // year of publication
    Description  string         // a text-description block (0x30)
    StopIn48K    bool           // emit a "stop in 48K mode" block (0x2A)
    Pause        uint16         // pause after each data block, ms
    Hardware     []HardwareInfo // hardware-type block (0x33)
    Group        string         // bracket the data in a named group (0x21/0x22)
}
```

A `HardwareInfo` entry names a machine or peripheral and the tape's relationship
to it:

```go
type HardwareInfo struct {
    Type byte // category, e.g. HWComputers
    ID   byte // item within the category, e.g. HWIDSpectrum128K
    Info byte // HWInfoRuns / HWInfoUses / HWInfoRunsUnused / HWInfoDoesntRun
}
```

The package defines the full set of hardware type, identifier, and information
constants from the specification's reference table.

### The incremental writer

For assembling several tape images into one file with structural blocks between
them, `Writer` builds a TZX block by block:

```go
type Writer struct { /* ... */ }

func NewWriter(pause uint16) *Writer
func (w *Writer) ArchiveInfo(opts EncodeOptions)
func (w *Writer) Description(desc string)
func (w *Writer) Hardware(entries []HardwareInfo)
func (w *Writer) GroupStart(name string)
func (w *Writer) GroupEnd()
func (w *Writer) StopIn48K()
func (w *Writer) AddTAP(tapImage []byte) error
func (w *Writer) Bytes() []byte
```

`EncodeFromTAP` is itself implemented on top of `Writer`, so the two share one
code path. Groups cannot nest; the caller pairs each `GroupStart` with a
`GroupEnd`.

### Decoding

```go
func Decode(image []byte) ([]Block, error)
```

`Decode` returns the blocks of a TZX image. The package round-trips every block
type it writes, so a TZX produced by `EncodeFromTAP` or `Writer` decodes
cleanly.

### Example

```go
import "github.com/ha1tch/zentools/pkg/tzx"

image, err := tzx.EncodeFromTAP(tapImage, tzx.EncodeOptions{
    Title:  "My Game",
    Author: "haitch",
    Year:   "2026",
    Hardware: []tzx.HardwareInfo{
        {Type: tzx.HWComputers, ID: tzx.HWIDSpectrum128K, Info: tzx.HWInfoUses},
    },
    Group: "Main Program",
})
```

## pkg/basic

Tokenises ZX BASIC source text into the Spectrum's internal byte representation,
and detokenises it back. Both the 48K and 128K keyword sets are supported; the
128K-only keywords `SPECTRUM` and `PLAY` are recognised.

```go
func Tokenise(src string, opts ...Option) ([]byte, error)
func Detokenise(prog []byte) (string, error)
func LooksTokenised(data []byte) bool
func CaseSensitive() Option
```

`Tokenise` converts source text to tokens. By default, keyword matching is
case-independent; passing `CaseSensitive()` requires exact keyword case, so that
lowercase words are kept as literal text rather than tokenised. `Detokenise`
reverses the process. `LooksTokenised` reports whether a byte slice appears to be
tokenised BASIC rather than plain text.

### Example

```go
import "github.com/ha1tch/zentools/pkg/basic"

tokens, err := basic.Tokenise("10 PRINT \"HELLO\"\n")
text, err := basic.Detokenise(tokens)
```

## pkg/snapshot

Reads and writes ZX Spectrum snapshots through a neutral machine-state type,
independent of any emulator. The `.sna` (48K and 128K) and `.z80` (versioned,
compressed) formats are supported.

### The machine state

Every encoder and decoder pivots on one type:

```go
type MachineState struct {
    Model  Model
    CPU    CPU
    Paging Paging
    IO     IO
    Memory Memory
}
```

`Memory` holds the eight 16-kilobyte RAM banks; `CPU` holds the registers,
interrupt mode, and flags; `Paging` holds the 128K paging ports. The model
constants are `Model48K`, `Model128K`, `ModelPlus2`, `ModelPlus2A`, and
`ModelPlus3`; the helper `Is128KFamily` reports whether a model is a 128K
variant.

### Encoding and decoding

```go
func EncodeSNA(s *MachineState) ([]byte, error)
func DecodeSNA(image []byte) (*MachineState, error)
func EncodeSNA128(s *MachineState) ([]byte, error)
func DecodeSNA128(image []byte) (*MachineState, error)
func EncodeZ80(s *MachineState) ([]byte, error)
func EncodeZ80v3(s *MachineState) ([]byte, error)
func DecodeZ80(image []byte) (*MachineState, error)
```

`EncodeSNA` and `EncodeSNA128` write 48K and 128K SNA images respectively;
`DecodeSNA` and `DecodeSNA128` read them. `EncodeZ80` and `EncodeZ80v3` write Z80
images, and `DecodeZ80` reads any supported Z80 version. Decoding is verified
against snapshots produced by z88dk and other tools.

A detail of the 48K SNA format is worth noting: it has no program-counter field
and stores the program counter on the stack. The encoders and decoders handle
this symmetrically, so a round trip preserves both the program counter and the
stack pointer.

### Example

```go
import "github.com/ha1tch/zentools/pkg/snapshot"

state, err := snapshot.DecodeZ80(data)
if err != nil {
    return err
}
sna, err := snapshot.EncodeSNA(state)
```

## pkg/build

Turns machine-code bytes into loadable artifacts. It overlays the code onto a
real booted machine state — one embedded boot snapshot per model — and emits
tapes and snapshots that load and run at a given entry point. This is the shared
procedure behind both `zenas build` and `zx snap`.

### The request

```go
type Request struct {
    Name   string // program name (<= 10 characters on tape)
    Code   []byte // assembled machine code
    Origin uint16 // load address of the first byte of Code
    Start  uint16 // entry point for snapshot output
    SP     uint16 // stack pointer for snapshot output
    Model  Model  // target model
}
```

The model is a string type with constants `Model48K` (`"48k"`), `Model128K`
(`"128k"`), `ModelPlus2` (`"plus2"`), `ModelPlus2A` (`"plus2a"`), and
`ModelPlus3` (`"plus3"`).

### Emitters

```go
func EncodeTAP(r Request) []byte
func EncodeTAPWithLoader(r Request) ([]byte, error)
func EncodeTZX(r Request) ([]byte, error)
func EncodeTZXFromTAP(tapImage []byte) ([]byte, error)
func EncodeSNA(r Request) ([]byte, error)
func EncodeZ80(r Request) ([]byte, error)
```

`EncodeTAP` produces a CODE tape; `EncodeTAPWithLoader` prepends a BASIC auto-run
loader that jumps to `Start`, giving a tape that runs on its own. `EncodeTZX` and
`EncodeTZXFromTAP` produce TZX output. `EncodeSNA` and `EncodeZ80` overlay the
code onto the model's boot state and produce a runnable snapshot with the program
counter at `Start` and the stack pointer at `SP`.

`Request` also offers `SPWarning`, which returns a non-empty message when the
stack pointer is positioned where it would collide with the loaded code.

### Example

```go
import "github.com/ha1tch/zentools/pkg/build"

req := build.Request{
    Name:   "mygame",
    Code:   code,
    Origin: 0x8000,
    Start:  0x8000,
    SP:     0xFF00,
    Model:  build.Model48K,
}
z80, err := build.EncodeZ80(req)   // a runnable snapshot
tape   := build.EncodeTAP(req)     // a CODE tape
```

## pkg/version

Exposes the library version as a string constant, `version.Version`, kept in
sync with the `VERSION` file at the repository root.

## Composing the packages

The packages are designed to flow into one another. A common pipeline takes
assembled code and produces several distribution formats from one request:

- `build.EncodeTAPWithLoader` for a tape that auto-runs on real hardware;
- `build.EncodeZ80` or `build.EncodeSNA` for a snapshot to test in an emulator;
- `tzx.EncodeFromTAP` over a `build.EncodeTAP` result to add archive metadata.

For inspection and conversion, `tap.Decode`, `tzx.Decode`, and the snapshot
decoders all return structured values you can read, transform, and re-encode.
Because every format the library writes it can also read, conversions compose
cleanly within a kind — tape to tape, snapshot to snapshot — while conversions
across the tape/snapshot divide carry the trade-offs described in the
[CLI tools manual](CLI.md).