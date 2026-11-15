# zx0 test fixtures

The inputs are `a.bin` (the 2048-byte `pkg/zxn/testdata/s8.spr` sprite sheet), `b.bin` (6000
bytes of English text) and `c.bin` (4000 bytes drawn from {0, 0, 0, 1, 2, 255} with a fixed
seed). The other files are the reference compressor's output (github.com/einar-saukas/ZX0,
commit ecde3a2, built with `gcc -O2 src/zx0.c src/optimize.c src/compress.c src/memory.c`):

    zx0 -f    x.bin x.v2       ZX0 v2
    zx0 -f -c x.bin x.v1       classic (v1)
    zx0 -f -b x.bin x.back     backwards
    zx0 -f -q x.bin x.quick    quick

gfx2next v1.1.24 writes the classic format: `gfx2next -sprites -zx0-sprites -pal-none
-color-round s8.bmp` gives the same bytes as `a.v1`.
