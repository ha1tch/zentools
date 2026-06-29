# zentools consolidation — status and work ahead

Updated 2026-06-29. This refreshes the reorganisation tables from the original
scavenging investigation to reflect what has actually been built, released, and
validated. The earlier tables described a plan; these describe progress against
it.

Legend: ✓ done · ◐ in progress · ☐ not started

## 1. Per-format consolidation

### Source projects

The consolidation scavenged format implementations from several existing
projects. These are *sources*, not consumers — they are not migrated onto
zentools; they are where zentools' code came from.

- **zxgotools** (`zxgotools`, a standalone Go module) — the prior-art ZX
  Spectrum toolset and the principal ancestor of zentools' interchange formats.
  It provided the original `pkg/tap`, `pkg/basic`, and the `tap2tzx` encoder, plus
  standalone CLI tools (`loadtap`, `maketap`, `tap2tzx`, `totap`). zentools took
  these implementations as the architectural starting point and then corrected
  and slimmed them: its `pkg/tap` (≈320 lines) is the fixed descendant of the
  zxgotools original (≈175 lines), and its `pkg/basic` (≈470 lines) is a distilled,
  number-handling-corrected rewrite of the zxgotools version (≈1700 lines).
  zxgotools is a frozen snapshot (single commit), is not under the `ha1tch`
  namespace, and does not depend on zentools. Its libraries are superseded by
  zentools, but its **CLI binaries have no zentools equivalent** — zentools is a
  library, whereas zxgotools shipped cross-compiled command-line tools. It is
  therefore retained as-is for those tools and as the provenance record, not
  retired.
- **plus3** — contributed the correct, both-directions BASIC implementation and
  is itself a consumer (see §2); its disk core stays native.
- **zenzx** — the sole prior implementation of the `.sna` and `.z80` snapshot
  codecs, and itself a consumer (see §2).

The "best source" column below records which of these each format's canonical
implementation came from. The status column tracks whether that implementation
now lives in zentools (or its rightful owner) and is validated.

| Format | Best source | Canonical home | Status | Validation |
|--------|-------------|----------------|--------|------------|
| TAP | zentools (built from zxgotools, fixed) | `zentools/pkg/tap` | ✓ released (0.1.0+) | pasmo byte-identical; independent decoder; zenzx load |
| TZX | zxgotools encoder (verified) + zenzx parser | `zentools/pkg/tzx` | ✓ released | byte-identical to zxgotools `tap2tzx` |
| BASIC | plus3 (correct, both directions) + zxgotools (architecture) | `zentools/pkg/basic` | ✓ released | real +3 programs; corrected number-handling |
| `.sna` 48K | zenzx (only impl) | `zentools/pkg/snapshot` | ✓ released (0.2.0) | z88dk `zx_48.sna`; zenzx load |
| `.sna` 128K | zenzx (only impl) | `zentools/pkg/snapshot` | ✓ released (0.2.0) | z88dk `zx_128.sna` |
| `.z80` v1 | zenzx (only impl) | `zentools/pkg/snapshot` | ✓ released (0.2.0) | Jet Set Willy (real game); byte-identical RLE vs independent decoder |
| `.z80` v2 | zenzx (only impl) | `zentools/pkg/snapshot` | ✓ released (0.2.0) | Manic Miner (spectrumcomputing) |
| `.z80` v3 128K | zenzx (only impl) | `zentools/pkg/snapshot` | ✓ released (0.2.0) | Z80Attack (real +2 snapshot); encode/decode cross-check |
| `.zxs` | zenzx | **zenzx (native)** | ✓ stays native | not moved — emulator-specific by decision |
| `.dsk` (+3DOS) | plus3 | **plus3 (native)** | ✓ stays native | not moved — plus3 is the disk authority |
| `.scr` | — | `zentools/pkg/snapshot` (future) | ☐ not started | portable screen format, if/when needed |
| `.nex` | not implemented anywhere | `zentools` (future) | ☐ not started | must be written fresh from spec |

**Library status:** every interchange format that existed in the ecosystem is now
built, released, and externally validated in zentools. The two native formats
(`.zxs`, `.dsk`) stay with their owners by the ownership rule. The only open
format items are `.scr` and `.nex`, both new work rather than consolidation, and
neither is currently needed.

## 2. Consumer migrations

All three consumer migrations are now complete and released. Each consumes the
zentools formats rather than carrying its own. plus3 and zenzx *replaced* broken
or duplicated in-tree code; zenas *added* a new packaging command on top of the
zentools formats.

| Consumer | Swap | Keeps (its own) | Status | Gate |
|----------|------|-----------------|--------|------|
| **plus3** | broken in-tree TAP → `pkg/tap`; in-tree BASIC → `pkg/basic` | `.dsk` disk-image core | ✓ done, released 0.9.8 | TAP round-trip + golden-identical BASIC output |
| **zenas** | new `build` command emits tapes (`pkg/tap`/`pkg/tzx`), snapshots (`pkg/snapshot`), and a BASIC loader (`pkg/basic`) | assembler core | ✓ done, released 0.7.0 | snapshots load-and-run on all five models; tapes byte-identical CODE blocks; loader encoding verified |
| **zenzx** | in-tree `.sna` / `.z80` codecs → `pkg/snapshot` + adapter | `.zxs`; tape playback (pulse gen, fast-inject, ROM trap) | ✓ done, released 0.4.0 | encode→load→memory round-trip + real-game render |

### plus3 — ✓ complete (0.9.8)

Both swaps done and released. TAP (`convert.go`) now delegates to `pkg/tap`,
fixing the malformed-TAP bug (stray checksum byte, missing flag, wrong checksum
range) that shipped with no tests. BASIC (`basictok.go` + `basic.go`) delegates
to `pkg/basic`, verified byte-identical to the previous output against a golden
baseline. ~600 lines of duplication removed; dead code swept; CI green. This was
the highest-immediate-payoff migration because the TAP code was an active bug.

### zenas — ✓ complete (0.7.0)

The planned scope was tape output (Feature 3). What shipped is broader: a new
`zenas build` command that assembles the source and packages it into loadable
artifacts in four formats, consuming all the relevant zentools packages rather
than just `pkg/tap`.

- **Tapes** (`.tap`, `.tzx`) via `pkg/tap` / `pkg/tzx` — the originally planned
  output. `--loader` additionally prepends a BASIC auto-run loader, generated as
  ASCII BASIC and tokenised through `pkg/basic` (no hand-assembled bytes).
- **Snapshots** (`.sna`, `.z80` v3) via `pkg/snapshot` — beyond the original
  plan. The assembled code is overlaid onto a real booted-machine state captured
  per model (48k, 128k, plus2, plus2a, plus3), with the entry point and stack
  pointer set so the snapshot loads and runs immediately.

Because this is additive (a new command, not a replacement of broken code), the
gate was that the output loads correctly. Snapshots were verified executing
across all five models (each runs its program to a filled screen). Tapes were
verified as structurally correct CODE blocks (type 3, correct load address) and,
for the loader variant, the BASIC was checked three ways: detokenise round-trip,
tape-header autostart line, and byte-level token/number encoding.

The recommended workflow documented for users: `.z80`/`.sna` snapshots for
development testing, tapes for wider distribution. Three filename namespaces
(Spectrum tape ≤10 chars, +3DOS 8.3, host UTF-8) are kept distinct.

### zenzx — ✓ complete (0.4.0)

zenzx's in-tree `.sna` / `.z80` codecs were lifted onto the neutral
`MachineState` via a thin adapter (`toMachineState` / `fromMachineState` in
`snapshot_adapter.go`), and `SaveSNA` / `LoadSNA` / `SaveZ80` / `LoadZ80` are now
thin delegations into `pkg/snapshot` — roughly 460 lines of in-tree codec
removed. zenzx keeps `.zxs` and all emulator-coupled playback (pulse generation,
fast-inject, ROM trap). The adapter also restores derived state the raw load
bypasses: 128K paging re-applied from the loaded port, and the screen buffers
re-synced from the displayed bank.

Two improvements landed with the swap:

- **`.sna` 48K PC bug fixed.** The old `SaveSNA` never pushed PC onto the stack
  for 48K snapshots (the header has no PC field), silently losing it. The
  zentools codec does this correctly; a regression test pins PC survival.
- **`.z80` save upgraded to version 3** (extended header, 128K-capable), where it
  previously wrote only v1.

Validation went past the round-trip gate to real artifacts: 48K/128K `.sna` and
`.z80` round-trips; sentinel loads of genuine games (Jet Set Willy v1, Manic
Miner v2, Z80 Attack v3 128K); a load → re-save → independent-decode check
proving byte-identical memory across a v1-to-v3 conversion; and headless runs of
all five sentinels to their title/menu screens, confirming the restored state
actually executes (the z88dk `JR -2` templates correctly idle while the ROM
interrupt clock keeps ticking).

This was the last consolidation step — the one that *removes* format code from a
consumer. With it done, no project carries a duplicate interchange-format
implementation.

## 3. Summary of work ahead

All consolidation is complete, and the one planned additive consumer feature
(zenas tape output) shipped in 0.7.0 — in fact broadened to full `build`
packaging across tapes and snapshots. What remains is genuinely new capability
that no consumer currently needs:

1. **`.scr` / `.nex`** — new portable formats, only if/when a consumer needs
   them. `.scr` is a trivial 6912-byte format that would belong in zentools;
   `.nex` must be written fresh against the spec, not reconstructed.
2. **zenzx Phase 2 export formats** (deferred from the 0.4.0 scope) — `.scr`,
   `.tap`, and `.bas` save. `.bas` refuses cleanly when no valid BASIC program is
   in memory; `.scr` depends on the zentools `.scr` work above.
3. **zenzx fast-loader Program-block support** — surfaced during zenas 0.7.0
   work: zenzx's fast-load path injects CODE blocks only and skips Program
   blocks, so a loader tape does not auto-run in zenzx fast mode (it does on real
   hardware / accurate emulation). This is a zenzx enhancement, not a zenas or
   zentools concern, and is noted here only so it is not lost.

Everything from the original consolidation plan — building zentools, validating
every format against real artifacts, and migrating plus3, zenzx, and zenas — is
done and released.
