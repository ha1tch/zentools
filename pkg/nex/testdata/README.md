# nex test fixtures

Each `<name>.nex.gz` is the NEX file sjasmplus v1.24.0 (github.com/z00m128/sjasmplus, built
with `make USE_LUA=0`) writes from `<name>.asm`, gzipped (`gzip -9 -n`) to keep the banks of
repeated test data small:

    sjasmplus --nologo <name>.asm

`common.i` fills each 16 KB bank with its bank number, the address and 0x5A, four bytes at a
time; `v13_320.asm` also appends `common.i` itself after the banks (`SAVENEX CLOSE "common.i"`).

| File | Version | Loading screen | Banks | Other |
|---|---|---|---|---|
| v12_l2 | V1.2 | Layer 2 256x192 with palette | 5, 2, 0, 7 | CORE, CFG, BAR |
| v13_320 | V1.3 | Layer 2 320x256 with palette | 5, 2, 4, 50 | CFG3 with checksum, copper, appended file |
| scr_ula | V1.2 | ULA | 1 | |
| shr | V1.3 (asked for) | Timex HiRes, colour 5 | 5 | |
| tile | V1.3 | tilemap (bank 5) | 5, 3 | |
| lores | V1.2 | LoRes | none | |

sjasmplus computes the V1.3 checksum over the header it wrote when the file was opened (the
`SAVENEX OPEN` fields only, and "NextV1.2"), not over the final header, so the checksum in
`v13_320` does not match its file; the tests check that it is exactly that stale value.
