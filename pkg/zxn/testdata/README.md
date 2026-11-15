# zxn test fixtures

`s8.png` (64x32, 256 random colours, random indices) and `s4.png` (48x16, 16 colours) are
indexed images drawn with a fixed random seed. The other files were written from the same images
saved as 8-bit BMP by gfx2next v1.1.24 (github.com/headkaze/Gfx2Next, MIT) and by nextbmp and
nextraw (github.com/stefanbylund/zxnext_bmp_tools, MIT):

    gfx2next -sprites -color-round s8.bmp             -> s8_round.nxp, s8.spr
    gfx2next -sprites -colors-4bit s4.bmp             -> s4.nxp, s4.spr
    nextbmp -floor s8.bmp n.bmp; nextraw -sep-palette n.bmp   -> n.nxp = s8_floor.nxp
    nextbmp -ceil  s8.bmp n.bmp; nextraw -sep-palette n.bmp   -> n.nxp = s8_ceil.nxp

For rounding to nearest the two tools agree. For floor and ceil they do not: gfx2next reduces
each palette colour twice (to RGB333, back to 8 bits, then to RGB333 again), which moves a
colour a further step down or up, while nextbmp reduces once, as the formula says. zxn follows
nextbmp.

The tests require zxn to produce the same bytes from the PNGs.

Palette layouts, the standard palette and minimising (gfx2next v1.1.24, default `-color-distance`
rounding unless named):

    gfx2next -sprites -pal-rgb332 s8.bmp                    -> s8_rgb332.nxp
    gfx2next -sprites -pal-gbr444 [-color-floor|-color-ceil] s8.bmp
                                                            -> s8_gbr444[_floor|_ceil].nxp
    gfx2next -sprites -colors-4bit -pal-bgr222 s4.bmp       -> s4_bgr222.nxp
    gfx2next -sprites -pal-std s8.bmp                       -> s8_std.nxp, s8_std.spr
    gfx2next -sprites -pal-std -color-floor s8.bmp          -> s8_std_floor.spr
    gfx2next -sprites -pal-min s8.bmp                       -> s8_min.nxp, s8_min.spr
    gfx2next -sprites -colors-4bit -pal-min s4.bmp          -> s4_min.nxp, s4_min.spr

(gfx2next's help names the option `-pal-gbr4444`; it accepts only `-pal-gbr444`.) With
`-pal-std -color-floor` gfx2next reduces the standard palette's own colours again, a step down,
so only its pixel indices are kept as a fixture. With `-pal-min -colors-4bit` the minimised
palette includes the black of the unused entries; s4 then has 17 colours and the last is dropped.

Layer 2: `l2_256.png` (256x192, 256 colours), `l2_320.png` (320x256, 256 colours) and
`l2_640.png` (640x256, 16 colours) are indexed images drawn with fixed seeds. gfx2next and nextraw
write the same bytes for each:

    gfx2next -bitmap -pal-embed -color-round l2_256.bmp      = nextraw l2_256.bmp
                                                            -> l2_256_embed.nxi
    gfx2next -bitmap -bitmap-y -color-round l2_320.bmp       = nextraw -columns -sep-palette
                                                            -> l2_320.nxi, l2_320.nxp
    gfx2next -bitmap -bitmap-y -colors-4bit -color-round l2_640.bmp
                                                            = nextraw -columns -4bit -sep-palette
                                                            -> l2_640.nxi, l2_640.nxp

Tiles: `t8.png` (64x32, 256 colours), `t4.png` (64x32, 16 colours in four banks of 16) and
`t1.png` (64x16, 2 colours) are built from a few random 8x8 tiles, each placed mirrored or turned
at random, plus an X-symmetric tile and a flat one. The `tiles_<name>` files are what gfx2next
wrote with `-pal-none` and:

    plain   t8.bmp                                        .nxt .nxm
    rep8    t8.bmp -tile-norepeat                         .nxt .nxm
    mir16   t8.bmp -tile-nomirror -map-16bit              .nxt .nxm
    rot16   t8.bmp -tile-norotate -map-16bit              .nxt .nxm
    rot16y  t8.bmp -tile-norotate -map-16bit -map-y -tile-offset=3 -tile-pal=1
    rot4    t4.bmp -colors-4bit -tile-norotate -map-16bit
    rep4p   t4.bmp -colors-4bit -tile-norepeat -map-16bit -tile-pal=1
    one     t1.bmp -colors-1bit -tile-norepeat
    blk     t8.bmp -tile-norepeat -block-size=2x2 -block-norepeat     .nxt .nxm .nxb
    blk16   t8.bmp -tile-norepeat -block-size=2x2 -block-16bit       .nxt .nxm .nxb
