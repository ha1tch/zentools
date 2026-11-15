// file: banks.go
//
// The reference encoder over any colours, for trying palette designs: banks
// of colours that need not be a ZXPAL palette, cells other than 8x8, and a
// span penalty other than the reference one. This is the encoder of the
// specification's tools (zxcommon.render and zxfast.render_fast), which takes
// a list of banks and a cell size; EncodePicture is the same encoder fixed to
// the ZXPAL palettes and 8x8 cells. Its output is a picture to look at and
// measure, not a screen: no ZXMODE file holds such cells or colours.

package scr

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Banks are colour banks for EncodeBanks: each cell takes its two colours
// from one bank. A two-byte palette is one bank of all its colours; a
// one-byte palette is a bank per attribute bank (bright, or bits 7-6).
type Banks [][]color.RGBA

// PaletteBanks returns the banks the reference encoder uses for palette p
// with register colours regs (nil for the reset values), in its order:
// ZXPAL4's colours in ascending RGB order, the others' in register order.
// Repeated colours within a bank are dropped.
func PaletteBanks(p Palette, regs Registers) Banks {
	var out Banks
	for _, bank := range encoderBanks(p, regs.Or(p)) {
		var b []color.RGBA
		for _, c := range bank {
			b = append(b, color.RGBA{uint8(c.rgb[0]), uint8(c.rgb[1]), uint8(c.rgb[2]), 0xFF})
		}
		out = append(out, b)
	}
	return out
}

// Len is the number of colours in all the banks.
func (b Banks) Len() int {
	n := 0
	for _, bank := range b {
		n += len(bank)
	}
	return n
}

// BanksCSV writes banks as `zx scr palette -banks -o file.csv` does: a
// header, then bank,hex,r,g,b per colour.
func BanksCSV(banks Banks) []byte {
	var s strings.Builder
	s.WriteString("bank,hex,r,g,b\n")
	for i, bank := range banks {
		for _, c := range bank {
			fmt.Fprintf(&s, "%d,%02X%02X%02X,%d,%d,%d\n", i, c.R, c.G, c.B, c.R, c.G, c.B)
		}
	}
	return []byte(s.String())
}

// ParseBanksCSV reads banks from CSV with a header row: the colour from
// columns r, g and b (or hex as RRGGBB), the bank from a column named bank
// (numbered from 0, in any order; without one, every colour is in bank 0).
// It reads the files BanksCSV writes, and register tables as one bank.
func ParseBanksCSV(data []byte) (Banks, error) {
	var rows [][]string
	for _, l := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			rows = append(rows, strings.Split(l, ","))
		}
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("scr: a bank table needs a header row and one row per colour")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	ri, okR := col["r"]
	gi, okG := col["g"]
	bi, okB := col["b"]
	hi, okHex := col["hex"]
	if !(okR && okG && okB) && !okHex {
		return nil, fmt.Errorf("scr: a bank table needs r, g and b columns, or a hex column")
	}
	bankCol, hasBank := col["bank"]
	field := func(row []string, i int) string {
		if i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var banks Banks
	for k, row := range rows[1:] {
		bank := 0
		if hasBank {
			v, err := strconv.Atoi(field(row, bankCol))
			if err != nil || v < 0 || v > 255 {
				return nil, fmt.Errorf("scr: bank table row %d: bank %q is not 0-255", k+2, field(row, bankCol))
			}
			bank = v
		}
		var c [3]int
		if okR && okG && okB {
			for j, i := range []int{ri, gi, bi} {
				v, err := strconv.Atoi(field(row, i))
				if err != nil || v < 0 || v > 255 {
					return nil, fmt.Errorf("scr: bank table row %d: channel %q is not 0-255", k+2, field(row, i))
				}
				c[j] = v
			}
		} else {
			h := strings.TrimPrefix(field(row, hi), "#")
			v, err := strconv.ParseUint(h, 16, 32)
			if err != nil || len(h) != 6 {
				return nil, fmt.Errorf("scr: bank table row %d: hex %q is not RRGGBB", k+2, field(row, hi))
			}
			c = [3]int{int(v >> 16), int(v >> 8 & 0xFF), int(v & 0xFF)}
		}
		for len(banks) <= bank {
			banks = append(banks, nil)
		}
		banks[bank] = append(banks[bank], color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 0xFF})
	}
	for i, b := range banks {
		if len(b) == 0 {
			return nil, fmt.Errorf("scr: bank table has no colours in bank %d (banks are numbered from 0 without gaps)", i)
		}
	}
	return banks, nil
}

// BankOptions set the cell size and span penalty of EncodeBanks.
type BankOptions struct {
	// CellW and CellH are the cell size in pixels; zero means 8.
	CellW, CellH int
	// Penalty weighs the span term of each pair's score; the reference
	// encoder's is 0.204 (RefPenalty).
	Penalty float64
}

// RefPenalty is the reference encoder's span penalty.
const RefPenalty = refPenalty

// bayer4 is the 4x4 ordered-dither matrix, in 16ths, used for 4x4 cells.
var bayer4 = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

func init() {
	for y := range bayer4 {
		for x := range bayer4[y] {
			bayer4[y][x] /= 16.0
		}
	}
}

// EncodeBanks encodes img with the reference encoder over banks: each cell
// of CellW x CellH pixels takes the pair of colours from one bank that
// scores best (the first on a tie, pairs listed bank by bank as the colours
// come), and dithers between them in linear light. A 4x4 cell dithers
// against the 4x4 Bayer matrix and an 8x8 one against the 8x8 matrix, as the
// specification's tools do; any other cell against the 8x8 matrix laid over
// the whole picture. The picture's size must be a multiple of the cell's.
// It returns the encoded picture and, per cell, the bank chosen.
func EncodeBanks(img image.Image, banks Banks, opt BankOptions) (*image.RGBA, [][]int, error) {
	cw, ch := opt.CellW, opt.CellH
	if cw == 0 {
		cw = 8
	}
	if ch == 0 {
		ch = 8
	}
	if cw < 1 || ch < 1 || cw > 64 || ch > 64 {
		return nil, nil, fmt.Errorf("scr: cell size %dx%d: each side must be 1 to 64", cw, ch)
	}
	if len(banks) == 0 {
		return nil, nil, fmt.Errorf("scr: no colour banks")
	}
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	if W%cw != 0 || H%ch != 0 {
		return nil, nil, fmt.Errorf("scr: a %dx%d picture does not divide into %dx%d cells", W, H, cw, ch)
	}
	var pairs []encPair
	var pairBank []int
	n := cw * ch
	for bi, bank := range banks {
		var cols []encColour
		for _, c := range bank {
			cols = append(cols, encColour{rgb: regColour(c)})
		}
		cols = dedupe(cols)
		for i := range cols {
			for j := i; j < len(cols); j++ {
				pairs = append(pairs, makePair(cols[i].rgb, cols[j].rgb, opt.Penalty, n))
				pairBank = append(pairBank, bi)
			}
		}
	}
	threshold := func(cx, cy, i int) float64 {
		x, y := i%cw, i/cw
		switch {
		case cw == 8 && ch == 8:
			return bayer8[y][x]
		case cw == 4 && ch == 4:
			return bayer4[y][x]
		}
		return bayer8[(cy*ch+y)%8][(cx*cw+x)%8]
	}

	out := image.NewRGBA(image.Rect(0, 0, W, H))
	rows, cols := H/ch, W/cw
	chosen := make([][]int, rows)
	for i := range chosen {
		chosen[i] = make([]int, cols)
	}
	work := make(chan int, rows)
	for cy := 0; cy < rows; cy++ {
		work <- cy
	}
	close(work)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x := make([][3]float64, n)
			lx := make([][3]float64, n)
			s := make([]float64, n)
			for cy := range work {
				for cx := 0; cx < cols; cx++ {
					for i := 0; i < n; i++ {
						r, g, bl, _ := img.At(b.Min.X+cx*cw+i%cw, b.Min.Y+cy*ch+i/cw).RGBA()
						v := [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
						for k := 0; k < 3; k++ {
							x[i][k] = float64(v[k])
							lx[i][k] = linearTable[v[k]]
						}
						s[i] = x[i][0]*x[i][0] + x[i][1]*x[i][1] + x[i][2]*x[i][2]
					}
					best, bestCost := 0, math.Inf(1)
					for k := range pairs {
						pr := &pairs[k]
						sum := 0.0
						for i := 0; i < n; i++ {
							xa := x[i][0]*pr.a[0] + x[i][1]*pr.a[1] + x[i][2]*pr.a[2]
							xab := x[i][0]*pr.ab[0] + x[i][1]*pr.ab[1] + x[i][2]*pr.ab[2] - pr.aab
							t := xab / pr.ab2s
							if t < 0 {
								t = 0
							} else if t > 1 {
								t = 1
							}
							d2 := s[i] - 2*xa + pr.a2 - float64(2*t*xab) + float64(t*t*pr.ab2)
							if d2 < 0 {
								d2 = 0
							}
							sum += math.Sqrt(d2)
						}
						if c := sum + pr.pen; c < bestCost {
							best, bestCost = k, c
						}
					}
					pr := &pairs[best]
					chosen[cy][cx] = pairBank[best]
					for i := 0; i < n; i++ {
						c := pr.a
						if pr.span != 0 {
							tl := (float64((lx[i][0]-pr.la[0])*pr.lab[0]) + float64((lx[i][1]-pr.la[1])*pr.lab[1]) +
								float64((lx[i][2]-pr.la[2])*pr.lab[2])) / pr.lab2s
							if tl < 0 {
								tl = 0
							} else if tl > 1 {
								tl = 1
							}
							if tl > threshold(cx, cy, i) {
								c = pr.b
							}
						}
						out.SetRGBA(cx*cw+i%cw, cy*ch+i/cw, color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 0xFF})
					}
				}
			}
		}()
	}
	wg.Wait()
	return out, chosen, nil
}

// ParseCellSize reads a cell size as WxH, or a single number for a square.
func ParseCellSize(s string) (w, h int, err error) {
	a, bs, ok := strings.Cut(strings.ToLower(strings.TrimSpace(s)), "x")
	if !ok {
		bs = a
	}
	w, e1 := strconv.Atoi(a)
	h, e2 := strconv.Atoi(bs)
	if e1 != nil || e2 != nil || w < 1 || h < 1 || w > 64 || h > 64 {
		return 0, 0, fmt.Errorf("scr: cell size %q: want WxH (or N for NxN), each 1 to 64", s)
	}
	return w, h, nil
}

// FitRegistersOptions control FitRegisters. Zero values take the defaults
// of the specification's tools (tools/z4_palette_search.py), given in
// brackets.
type FitRegistersOptions struct {
	Add        int     // colours to add (required)
	PerPicture int     // pixels sampled from each picture [20000]
	Candidates int     // candidate colours drawn from the sample [1500]
	Subsample  int     // pixels the greedy choice is scored on [12000]
	Iterations int     // refinement passes [25]
	MinMembers int     // pixels a colour needs to move in a pass [30]
	Blur       float64 // Gaussian blur before sampling [1.5]
	Seed       uint64  // seed of the random draws [1]
}

// FitStats describe how far the training pixels lie from their nearest
// colour.
type FitStats struct {
	MeanDistance float64 // mean sRGB distance, 0-441
	FarShare     float64 // share farther than 50
}

// FitRegisters chooses opt.Add colours to add to base so that the pixels of
// pictures lie nearer to some colour, as the specification's register
// search does: it blurs each picture (opt.Blur, Pillow's blur), samples its
// pixels, picks the new colours greedily from candidates drawn from the
// sample, each time the one that most reduces the total squared distance to
// the nearest colour, and then refines them by k-means (the base colours
// stay fixed; a new colour moves to the mean of its pixels when it has more
// than opt.MinMembers). It returns the new colours, and the distance
// statistics of the sample before and after.
func FitRegisters(pictures []image.Image, base []color.RGBA, opt FitRegistersOptions) ([]color.RGBA, FitStats, FitStats, error) {
	def := func(v *int, d int) {
		if *v == 0 {
			*v = d
		}
	}
	def(&opt.PerPicture, 20000)
	def(&opt.Candidates, 1500)
	def(&opt.Subsample, 12000)
	def(&opt.Iterations, 25)
	def(&opt.MinMembers, 30)
	if opt.Blur == 0 {
		opt.Blur = 1.5
	}
	if opt.Seed == 0 {
		opt.Seed = 1
	}
	if opt.Add < 1 {
		return nil, FitStats{}, FitStats{}, fmt.Errorf("scr: FitRegisters needs at least one colour to add")
	}
	if len(pictures) == 0 {
		return nil, FitStats{}, FitStats{}, fmt.Errorf("scr: FitRegisters needs at least one picture")
	}
	rng := splitmix{opt.Seed}

	// The training sample: blurred pixels, drawn without replacement.
	var X [][3]float64
	for _, img := range pictures {
		ch, w, h := channels(img)
		var px [][3]float64
		rb, gb, bb := BlurPillow(ch[0], w, h, opt.Blur), BlurPillow(ch[1], w, h, opt.Blur), BlurPillow(ch[2], w, h, opt.Blur)
		for i := range rb {
			px = append(px, [3]float64{float64(rb[i]), float64(gb[i]), float64(bb[i])})
		}
		X = append(X, rng.choose(px, opt.PerPicture)...)
	}
	var baseC [][3]float64
	seen := map[[3]float64]bool{}
	for _, c := range base {
		v := regColour(c)
		if !seen[v] {
			seen[v] = true
			baseC = append(baseC, v)
		}
	}
	d2 := func(a, b [3]float64) float64 {
		x, y, z := a[0]-b[0], a[1]-b[1], a[2]-b[2]
		return x*x + y*y + z*z
	}
	nearest := func(p [3]float64, cs ...[][3]float64) (int, float64) {
		bi, bd, k := -1, math.Inf(1), 0
		for _, set := range cs {
			for _, c := range set {
				if d := d2(p, c); d < bd {
					bi, bd = k, d
				}
				k++
			}
		}
		return bi, bd
	}
	stats := func(cs ...[][3]float64) FitStats {
		var st FitStats
		for _, p := range X {
			_, d := nearest(p, cs...)
			d = math.Sqrt(d)
			st.MeanDistance += d
			if d > 50 {
				st.FarShare++
			}
		}
		st.MeanDistance /= float64(len(X))
		st.FarShare /= float64(len(X))
		return st
	}
	before := stats(baseC)

	// Greedy choice on a subsample, from candidates drawn from the sample.
	Xs := rng.choose(X, opt.Subsample)
	C := rng.choose(X, opt.Candidates)
	cur := make([]float64, len(Xs))
	for i, p := range Xs {
		_, cur[i] = nearest(p, baseC)
	}
	var added [][3]float64
	for len(added) < opt.Add {
		gain := make([]float64, len(C))
		parallel(len(C), func(j int) {
			g := 0.0
			for i, p := range Xs {
				if d := cur[i] - d2(p, C[j]); d > 0 {
					g += d
				}
			}
			gain[j] = g
		})
		bj := 0
		for j := range gain {
			if gain[j] > gain[bj] {
				bj = j
			}
		}
		c := C[bj]
		added = append(added, c)
		for i, p := range Xs {
			if d := d2(p, c); d < cur[i] {
				cur[i] = d
			}
		}
	}

	// k-means refinement of the added colours over the whole sample.
	for it := 0; it < opt.Iterations; it++ {
		asg := make([]int, len(X))
		parallel(len(X), func(i int) { asg[i], _ = nearest(X[i], baseC, added) })
		sum := make([][3]float64, len(added))
		cnt := make([]int, len(added))
		for i, a := range asg {
			if j := a - len(baseC); j >= 0 {
				for k := 0; k < 3; k++ {
					sum[j][k] += X[i][k]
				}
				cnt[j]++
			}
		}
		for j := range added {
			if cnt[j] > opt.MinMembers {
				for k := 0; k < 3; k++ {
					added[j][k] = sum[j][k] / float64(cnt[j])
				}
			}
		}
	}
	out := make([]color.RGBA, len(added))
	for j, c := range added {
		var v [3]uint8
		for k := 0; k < 3; k++ {
			v[k] = uint8(math.Max(0, math.Min(255, math.RoundToEven(c[k]))))
			added[j][k] = float64(v[k])
		}
		out[j] = color.RGBA{v[0], v[1], v[2], 0xFF}
	}
	return out, before, stats(baseC, added), nil
}

// parallel runs f(0) .. f(n-1) across the CPUs.
func parallel(n int, f func(int)) {
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < n; i += workers {
				f(i)
			}
		}(w)
	}
	wg.Wait()
}

// splitmix is a small seeded generator (SplitMix64), so a fit is the same
// on every machine and Go version.
type splitmix struct{ s uint64 }

func (r *splitmix) next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	return z ^ z>>31
}

// choose draws k of the items without replacement (all of them when there
// are no more than k), keeping the drawn ones in their original order.
func (r *splitmix) choose(items [][3]float64, k int) [][3]float64 {
	if k > len(items) {
		k = len(items)
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < k; i++ {
		j := i + int(r.next()%uint64(len(idx)-i))
		idx[i], idx[j] = idx[j], idx[i]
	}
	sel := idx[:k]
	sort.Ints(sel)
	out := make([][3]float64, k)
	for i, j := range sel {
		out[i] = items[j]
	}
	return out
}
