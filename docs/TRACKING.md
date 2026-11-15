Version: 0.14.4 · Last reviewed: 2026-11-14

# zentools — open items

| ID | Summary | Theme | Priority | Status | Blocks |
|---|---|---|---|---|---|
| T-04 | Sprite attribute, Timex screen and 4-bit LoRes layouts are unverified on hardware or an emulator | next | P3 | ☐ | — |
| T-05 | ZX0 compression takes about twice as long as the reference C compressor | zx0 | P4 | ☐ | — |
| T-06 | Some gfx2next options have no zx next equivalent yet | next | P3 | ☐ | — |
| T-08 | JPEG decoding still differs from libjpeg-turbo by up to 3 levels in a few per cent of values | scr | P4 | ☐ | — |
| T-09 | zx next does not dither an image against RGB333 when its colours fit the palette | next | P3 | ☐ | — |

## next

### T-04. Sprite attribute, Timex screen and 4-bit LoRes layouts are unverified on hardware or an emulator

Theme: next · Priority: P3 · Status: ☐ · Blocks/after: —

**Trigger:** filed while implementing them for 0.12.0 (2026-11-14).

**Scope:** the sprite attribute bytes follow the Next developer guide's register descriptions (which contradict themselves on N6 for relative sprites: the anchor table says 1 picks bytes 128-255, the relative tables say 1 picks bytes 0-127; zxn treats N6 the same way for both, as the upper half). The Timex HiRes layout (even byte columns at 0x4000, odd at 0x6000) and the 4-bit LoRes mode (6144 bytes, high nibble left) come from the hardware documentation as remembered, not from a fixture. HiColour matches the zenzx renderer's layout (attributes at 0x6000 at the pixels' offsets). Check each by loading the output in an emulator with Next support (CSpect or ZEsarUX) or on a Next.

### T-06. Some gfx2next options have no zx next equivalent yet

Theme: next · Priority: P3 · Status: ☐ · Blocks/after: —

**Trigger:** found comparing zx next 0.12.0 with gfx2next v1.1.24 (2026-11-14).

**Scope:** tile cutting in column order (-tile-y) and in ldws order (-tile-ldws), planar 4-bit tiles (-tile-planar4), splitting output into 8, 16 or 48 KB bank files (-bank-8k and the rest), assembler and C header output (-asm-*), Tiled .tmx/.tsx files, ZX attribute maps (-pal-zx), and compressing output in one step (-zx0-*; zx zx0 does it as a second step). gfx2next's -tile-y also stores its map in a different order than it reads it, which needs checking before copying.

### T-09. zx next does not dither an image against RGB333 when its colours fit the palette

Theme: next · Priority: P3 · Status: ☐ · Blocks/after: —

**Trigger:** found writing the Next handling manual for 0.14.2 (2026-11-14): the Turner painting fitted to 320x256 reduces to 84 RGB333 colours, so `bitmap encode` keeps them all, `--dither fs` changes nothing, and the sky shows the steps of 3-bit channels.

**Scope:** `zxn.Reduce` dithers only when colours are chosen or matched; when the RGB333 colours of an image fit, each pixel is reduced to its own RGB333 colour without error diffusion. A photograph or painting would look smoother if `--dither fs` also diffused the RGB333 reduction error in that case (with the palette then built from the dithered pixel colours, which may no longer all fit). Without `--dither fs` the output must stay as it is, byte for byte with gfx2next and nextbmp.

## zx0

### T-05. ZX0 compression takes about twice as long as the reference C compressor

Theme: zx0 · Priority: P4 · Status: ☐ · Blocks/after: —

**Trigger:** measured while porting it for 0.12.0 (2026-11-14): 13 KB of text takes 1.5 s with the C zx0 (gcc -O2) and 2.9 s with `zx zx0`, producing the same bytes.

**Scope:** the parse visits every offset up to 32640 at every byte, as the reference does; most of the time is in allocating and reference-counting parse blocks in the arena, and in cache misses reading them. The time grows with the square of the length up to the 32640-byte offset limit, so an 80 KB Layer 2 screen takes about 45 seconds. Options: a cheaper block layout, or skipping offsets whose byte does not match through a per-value position list, keeping the output identical.

## scr

### T-08. JPEG decoding still differs from libjpeg-turbo by up to 3 levels in a few per cent of values

Theme: scr · Priority: P4 · Status: ☐ · Blocks/after: —

**Trigger:** found matching the ZXMODE specification's picture fit for 0.13.0 (2026-11-14).

**Scope:** `DecodeImage` now rebuilds JPEG colour with libjpeg-turbo's fancy chroma upsampling and colour tables, which removed differences of up to 68 levels against Pillow. What remains comes from the inverse DCT: Go's image/jpeg and libjpeg-turbo's islow IDCT round differently, leaving about 2-3% of samples 1 to 3 levels apart, so a fitted JPEG is not byte-identical to the specification's and its published errors reproduce to within a few hundredths rather than exactly: run entirely through `zx scr fit`, `encode` and `compare -metric blur`, 118 of the 220 values in `data/results_by_palette.csv` (44 pictures, colour and grey, ZXPAL0-4) come out equal to two decimals and none differs by more than 0.03. Closing it means decoding JPEG coefficients with libjpeg's jidctint arithmetic (a decoder of our own, or a port of the IDCT over coefficients read by a fork of image/jpeg).
