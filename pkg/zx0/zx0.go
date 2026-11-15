// Package zx0 compresses and decompresses data in Einar Saukas's ZX0 format,
// the optimal LZ77/LZSS format the Spectrum Next tools use (gfx2next's -zx0
// options, the dzx0 decompressors for the Z80).
//
// It is a port of the reference compressor (github.com/einar-saukas/ZX0,
// BSD-3-Clause, (c) 2021 Einar Saukas): the same optimal parse, so the same
// bytes. Two formats exist: ZX0 v2, the current one, and v1 ("classic",
// zx0 -c), which gfx2next writes; they differ only in that v2 inverts the
// data bits of the Elias gamma codes for offsets in forward mode.
package zx0

import "fmt"

const (
	initialOffset = 1
	// MaxOffset is the furthest back a match can reach; MaxOffsetQuick
	// is quick mode's limit (zx0 -q), faster but compressing less.
	MaxOffset      = 32640
	MaxOffsetQuick = 2176
)

// Options select the variant.
type Options struct {
	// Quick limits offsets to MaxOffsetQuick.
	Quick bool
	// Backwards compresses for decompression from the end of the data
	// backwards (zx0 -b), for in-place decompression.
	Backwards bool
	// Classic writes the v1 format (zx0 -c), as gfx2next does.
	Classic bool
}

// block is one step of a parse. Blocks live in an arena and refer to each
// other by index (-1 for none), with reference counts and a free list as the
// reference compressor keeps them, so memory stays bounded and the garbage
// collector has no pointers to follow.
type block struct {
	chain, ghost                      int32
	bits, index, offset, length, refs int32
}

type arena struct {
	b         []block
	ghostRoot int32
}

func (a *arena) allocate(bits, index, offset, length int, chain int32) int32 {
	var p int32
	if a.ghostRoot >= 0 {
		p = a.ghostRoot
		a.ghostRoot = a.b[p].ghost
		if c := a.b[p].chain; c >= 0 {
			a.b[c].refs--
			if a.b[c].refs == 0 {
				a.b[c].ghost = a.ghostRoot
				a.ghostRoot = c
			}
		}
	} else {
		a.b = append(a.b, block{})
		p = int32(len(a.b) - 1)
	}
	if chain >= 0 {
		a.b[chain].refs++
	}
	a.b[p] = block{chain: chain, ghost: -1, bits: int32(bits), index: int32(index), offset: int32(offset), length: int32(length)}
	return p
}

func (a *arena) assign(ptr *int32, chain int32) {
	a.b[chain].refs++
	if old := *ptr; old >= 0 {
		a.b[old].refs--
		if a.b[old].refs == 0 {
			a.b[old].ghost = a.ghostRoot
			a.ghostRoot = old
		}
	}
	*ptr = chain
}

func offsetCeiling(index, limit int) int {
	if index > limit {
		return limit
	}
	if index < initialOffset {
		return initialOffset
	}
	return index
}

func eliasGammaBits(value int) int {
	bits := 1
	for value > 1 {
		bits += 2
		value >>= 1
	}
	return bits
}

func fill(n int) []int32 {
	s := make([]int32, n)
	for i := range s {
		s[i] = -1
	}
	return s
}

// optimize is the reference compressor's optimal parse; it returns the
// arena and the last block of the best parse.
func optimize(in []byte, skip, offsetLimit int) (*arena, int32) {
	n := len(in)
	maxOffset := offsetCeiling(n-1, offsetLimit)
	a := &arena{ghostRoot: -1}
	lastLiteral := fill(maxOffset + 1)
	lastMatch := fill(maxOffset + 1)
	optimal := fill(n + 1)
	matchLength := make([]int, maxOffset+1)
	bestLength := make([]int, n+1)
	if n >= 3 {
		bestLength[2] = 2
	}
	bitsOf := func(p int32) int { return int(a.b[p].bits) }
	a.assign(&lastMatch[initialOffset], a.allocate(-1, skip-1, initialOffset, 0, -1))
	for index := skip; index < n; index++ {
		bestLengthSize := 2
		maxOffset = offsetCeiling(index, offsetLimit)
		for offset := 1; offset <= maxOffset; offset++ {
			if index != skip && index >= offset && in[index] == in[index-offset] {
				// copy from last offset
				if ll := lastLiteral[offset]; ll >= 0 {
					length := index - int(a.b[ll].index)
					bits := bitsOf(ll) + 1 + eliasGammaBits(length)
					a.assign(&lastMatch[offset], a.allocate(bits, index, offset, length, ll))
					if optimal[index] < 0 || bitsOf(optimal[index]) > bits {
						a.assign(&optimal[index], lastMatch[offset])
					}
				}
				// copy from new offset
				matchLength[offset]++
				if matchLength[offset] > 1 {
					if bestLengthSize < matchLength[offset] {
						bits := bitsOf(optimal[index-bestLength[bestLengthSize]]) + eliasGammaBits(bestLength[bestLengthSize]-1)
						for {
							bestLengthSize++
							bits2 := bitsOf(optimal[index-bestLengthSize]) + eliasGammaBits(bestLengthSize-1)
							if bits2 <= bits {
								bestLength[bestLengthSize] = bestLengthSize
								bits = bits2
							} else {
								bestLength[bestLengthSize] = bestLength[bestLengthSize-1]
							}
							if bestLengthSize >= matchLength[offset] {
								break
							}
						}
					}
					length := bestLength[matchLength[offset]]
					bits := bitsOf(optimal[index-length]) + 8 + eliasGammaBits((offset-1)/128+1) + eliasGammaBits(length-1)
					if lm := lastMatch[offset]; lm < 0 || int(a.b[lm].index) != index || bitsOf(lm) > bits {
						a.assign(&lastMatch[offset], a.allocate(bits, index, offset, length, optimal[index-length]))
						if optimal[index] < 0 || bitsOf(optimal[index]) > bits {
							a.assign(&optimal[index], lastMatch[offset])
						}
					}
				}
			} else {
				// copy literals
				matchLength[offset] = 0
				if lm := lastMatch[offset]; lm >= 0 {
					length := index - int(a.b[lm].index)
					bits := bitsOf(lm) + 1 + eliasGammaBits(length) + length*8
					a.assign(&lastLiteral[offset], a.allocate(bits, index, 0, length, lm))
					if optimal[index] < 0 || bitsOf(optimal[index]) > bits {
						a.assign(&optimal[index], lastLiteral[offset])
					}
				}
			}
		}
	}
	return a, optimal[n-1]
}

type writer struct {
	out       []byte
	bitMask   byte
	bitIndex  int
	backtrack bool
}

func (w *writer) byte(v byte) { w.out = append(w.out, v) }

func (w *writer) bit(v bool) {
	if w.backtrack {
		if v {
			w.out[len(w.out)-1] |= 1
		}
		w.backtrack = false
		return
	}
	if w.bitMask == 0 {
		w.bitMask = 128
		w.bitIndex = len(w.out)
		w.byte(0)
	}
	if v {
		w.out[w.bitIndex] |= w.bitMask
	}
	w.bitMask >>= 1
}

func (w *writer) gamma(value int, backwards, invert bool) {
	i := 2
	for i <= value {
		i <<= 1
	}
	i >>= 1
	for i >>= 1; i > 0; i >>= 1 {
		w.bit(backwards)
		bit := value&i != 0
		if invert {
			bit = !bit
		}
		w.bit(bit)
	}
	w.bit(!backwards)
}

func reversed(b []byte) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[len(b)-1-i] = v
	}
	return out
}

// Compress returns data compressed. The input is not changed.
func Compress(data []byte, opt Options) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("zx0: nothing to compress")
	}
	in := data
	if opt.Backwards {
		in = reversed(data)
	}
	limit := MaxOffset
	if opt.Quick {
		limit = MaxOffsetQuick
	}
	invert := !opt.Classic && !opt.Backwards
	a, best := optimize(in, 0, limit)
	size := (int(a.b[best].bits) + 18 + 7) / 8

	// Un-reverse the chain.
	next := int32(-1)
	for p := best; p >= 0; {
		prev := a.b[p].chain
		a.b[p].chain = next
		next = p
		p = prev
	}
	w := &writer{out: make([]byte, 0, size)}
	pos, lastOffset, first := 0, initialOffset, true
	for p := a.b[next].chain; p >= 0; p = a.b[p].chain {
		b := a.b[p]
		length, offset := int(b.length), int(b.offset)
		switch {
		case offset == 0: // literals
			if first {
				first = false
			} else {
				w.bit(false)
			}
			w.gamma(length, opt.Backwards, false)
			w.out = append(w.out, in[pos:pos+length]...)
			pos += length
		case offset == lastOffset: // copy from the last offset
			w.bit(false)
			w.gamma(length, opt.Backwards, false)
			pos += length
		default: // copy from a new offset
			w.bit(true)
			w.gamma((offset-1)/128+1, opt.Backwards, invert)
			if opt.Backwards {
				w.byte(byte(((offset - 1) % 128) << 1))
			} else {
				w.byte(byte((255 - (offset-1)%128) << 1))
			}
			w.backtrack = true
			w.gamma(length-1, opt.Backwards, false)
			pos += length
			lastOffset = offset
		}
	}
	w.bit(true)
	w.gamma(256, opt.Backwards, invert)
	if len(w.out) != size {
		return nil, fmt.Errorf("zx0: internal error: wrote %d bytes, expected %d", len(w.out), size)
	}
	if opt.Backwards {
		return reversed(w.out), nil
	}
	return w.out, nil
}

type reader struct {
	in        []byte
	pos       int
	bitMask   byte
	bitValue  byte
	lastByte  byte
	backtrack bool
	err       error
}

func (r *reader) byte() byte {
	if r.pos >= len(r.in) {
		if r.err == nil {
			r.err = fmt.Errorf("zx0: the compressed data ends early")
		}
		return 0
	}
	r.lastByte = r.in[r.pos]
	r.pos++
	return r.lastByte
}

func (r *reader) bit() bool {
	if r.backtrack {
		r.backtrack = false
		return r.lastByte&1 != 0
	}
	r.bitMask >>= 1
	if r.bitMask == 0 {
		r.bitMask = 128
		r.bitValue = r.byte()
	}
	return r.bitValue&r.bitMask != 0
}

// gamma reads an interlaced Elias gamma code; a backwards stream continues
// on 1 bits and stops on a 0, a forward one the other way round.
func (r *reader) gamma(invert, backwards bool) int {
	value := 1
	for r.bit() == backwards {
		if r.err != nil || value > 1<<17 {
			if r.err == nil {
				r.err = fmt.Errorf("zx0: an Elias gamma code is too long")
			}
			return 0
		}
		b := r.bit()
		if invert {
			b = !b
		}
		value <<= 1
		if b {
			value |= 1
		}
	}
	return value
}

// Decompress returns data decompressed, read with the same options it was
// compressed with (Quick does not matter). limit, if above 0, refuses output
// longer than that.
func Decompress(data []byte, opt Options, limit int) ([]byte, error) {
	in := data
	if opt.Backwards {
		in = reversed(data)
	}
	invert := !opt.Classic && !opt.Backwards
	r := &reader{in: in}
	var out []byte
	copyFrom := func(offset, length int) error {
		if offset > len(out) || offset <= 0 {
			return fmt.Errorf("zx0: an offset of %d reaches before the start of the data", offset)
		}
		for ; length > 0; length-- {
			out = append(out, out[len(out)-offset])
		}
		return nil
	}
	lastOffset := initialOffset
	state := 0 // 0 literals, 1 last offset, 2 new offset
	for {
		if r.err != nil {
			return nil, r.err
		}
		if limit > 0 && len(out) > limit {
			return nil, fmt.Errorf("zx0: the data decompresses to more than %d bytes", limit)
		}
		switch state {
		case 0:
			n := r.gamma(false, opt.Backwards)
			for i := 0; i < n && r.err == nil; i++ {
				out = append(out, r.byte())
			}
			if r.bit() {
				state = 2
			} else {
				state = 1
			}
		case 1:
			if err := copyFrom(lastOffset, r.gamma(false, opt.Backwards)); err != nil {
				return nil, err
			}
			if r.bit() {
				state = 2
			} else {
				state = 0
			}
		case 2:
			msb := r.gamma(invert, opt.Backwards)
			if r.err != nil {
				return nil, r.err
			}
			if msb == 256 {
				if opt.Backwards {
					return reversed(out), nil
				}
				return out, nil
			}
			lsb := r.byte()
			if opt.Backwards {
				lastOffset = (msb-1)<<7 + 1 + int(lsb>>1)
			} else {
				lastOffset = (msb-1)<<7 + 128 - int(lsb>>1)
			}
			r.backtrack = true
			if err := copyFrom(lastOffset, r.gamma(false, opt.Backwards)+1); err != nil {
				return nil, err
			}
			if r.bit() {
				state = 2
			} else {
				state = 0
			}
		}
	}
}
