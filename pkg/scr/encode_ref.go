// The ZXPAL reference encoder (Part 3 of the ZXMODE specification, section 13).
//
// For each 8x8 cell it lists every pair of register colours the attribute can
// name (within one bank for the one-byte palettes, across all registers for the
// two-byte ones, a colour paired with itself included), scores each pair in
// sRGB as the sum over the cell's pixels of the distance to the segment between
// the two colours plus 0.204 x span x 64 / 1.7, keeps the lowest score (the
// first pair on a tie), and dithers between the two colours in linear light
// against an 8x8 Bayer matrix.
//
// The arithmetic follows the specification's Python reference (tools/zxfast.py)
// operation for operation, so the two produce the same picture: the sums of
// squares and dot products are exact on 8-bit inputs, and the remaining steps
// are evaluated in the same order.

package scr

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"sort"
	"sync"
)

const refPenalty = 0.204

// bayer8 is the 8x8 ordered-dither matrix, row by row, in 64ths.
var bayer8 = [8][8]float64{
	{0, 32, 8, 40, 2, 34, 10, 42}, {48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38}, {60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41}, {51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37}, {63, 31, 55, 23, 61, 29, 53, 21},
}

func init() {
	for y := range bayer8 {
		for x := range bayer8[y] {
			bayer8[y][x] /= 64.0
		}
	}
}

// toLinear converts an sRGB code (0-255) to linear light.
func toLinear(v float64) float64 { return linearTable[int(v)] }

// linearTable holds the linear-light value of each sRGB code: c/12.92 for
// c = v/255 up to 0.04045, ((c+0.055)/1.055)^2.4 above, correctly rounded as the
// reference computes them. Go's math.Pow differs from that in the last bit for
// 88 of the 256 codes, enough to move a pixel across a dither threshold, so the
// values are tabulated rather than computed.
var linearTable = [256]float64{
	0.0, 0.0003035269835488375, 0.000607053967097675, 0.0009105809506465125,
	0.00121410793419535, 0.0015176349177441874, 0.001821161901293025, 0.0021246888848418626,
	0.0024282158683907, 0.0027317428519395373, 0.003035269835488375, 0.003346535763899161,
	0.003676507324047436, 0.004024717018496307, 0.004391442037410293, 0.004776953480693729,
	0.005181516702338385, 0.005605391624202723, 0.006048833022857054, 0.006512090792594475,
	0.006995410187265387, 0.007499032043226175, 0.008023192985384994, 0.008568125618069307,
	0.009134058702220787, 0.009721217320237847, 0.010329823029626936, 0.010960094006488246,
	0.011612245179743885, 0.012286488356915872, 0.012983032342173012, 0.013702083047289686,
	0.014443843596092545, 0.015208514422912708, 0.01599629336550963, 0.016807375752887384,
	0.017641954488384078, 0.018500220128379697, 0.019382360956935723, 0.020288563056652397,
	0.021219010376003555, 0.02217388479338738, 0.02315336617811041, 0.024157632448504756,
	0.02518685962736163, 0.026241221894849898, 0.027320891639074894, 0.028426039504420793,
	0.0295568344378088, 0.030713443732993635, 0.03189603307301153, 0.033104766570885055,
	0.03433980680868217, 0.03560131487502034, 0.03688945040110004, 0.0382043715953465,
	0.03954623527673283, 0.04091519690685319, 0.042311410620809675, 0.043735029256973465,
	0.04518620438567554, 0.046665086336880095, 0.04817182422688942, 0.04970656598412723,
	0.05126945837404324, 0.052860647023180246, 0.05448027644244237, 0.05612849004960009,
	0.05780543019106723, 0.0595112381629812, 0.06124605423161761, 0.06301001765316767,
	0.06480326669290577, 0.06662593864377289, 0.06847816984440017, 0.07036009569659588,
	0.07227185068231748, 0.07421356838014963, 0.07618538148130784, 0.07818742180518633,
	0.08021982031446832, 0.0822827071298148, 0.08437621154414882, 0.08650046203654976,
	0.08865558628577294, 0.09084171118340768, 0.09305896284668745, 0.09530746663096469,
	0.09758734714186246, 0.09989872824711389, 0.1022417330881013, 0.10461648409110419,
	0.10702310297826761, 0.10946171077829933, 0.1119324278369056, 0.11443537382697373,
	0.11697066775851084, 0.11953842798834562, 0.12213877222960187, 0.12477181756095049,
	0.12743768043564743, 0.1301364766903643, 0.13286832155381798, 0.13563332965520566,
	0.13843161503245183, 0.14126329114027164, 0.14412847085805777, 0.14702726649759498,
	0.14995978981060856, 0.15292615199615017, 0.1559264637078274, 0.1589608350608804,
	0.162029375639111, 0.1651321945016676, 0.16826940018969075, 0.1714411007328226,
	0.17464740365558504, 0.17788841598362912, 0.18116424424986022, 0.184474994500441,
	0.18782077230067787, 0.19120168274079138, 0.1946178304415758, 0.19806931955994886,
	0.20155625379439707, 0.20507873639031693, 0.20863687014525575, 0.21223075741405523,
	0.21586050011389926, 0.2195261997292692, 0.2232279573168085, 0.22696587351009836,
	0.23074004852434915, 0.23455058216100522, 0.238397573812271, 0.24228112246555486,
	0.24620132670783548, 0.25015828472995344, 0.25415209433082675, 0.25818285292159576,
	0.26225065752969623, 0.26635560480286247, 0.2704977910130658, 0.27467731206038465,
	0.2788942634768104, 0.2831487404299921, 0.2874408377269175, 0.29177064981753587,
	0.2961382707983211, 0.3005437944157765, 0.3049873140698863, 0.30946892281750854,
	0.31398871337571754, 0.31854677812509186, 0.32314320911295075, 0.3277780980565422,
	0.33245153634617935, 0.33716361504833037, 0.3419144249086609, 0.3467040563550296,
	0.35153259950043936, 0.3564001441459435, 0.3613067797835095, 0.3662525955988395,
	0.3712376804741491, 0.37626212299090644, 0.38132601143253014, 0.386429433787049,
	0.3915724777497232, 0.39675523072562685, 0.40197777983219574, 0.4072402119017367,
	0.41254261348390375, 0.4178850708481375, 0.4232676699860717, 0.4286904966139066,
	0.4341536361747489, 0.4396571738409188, 0.44520119451622786, 0.45078578283822346,
	0.45641102318040466, 0.4620769996544071, 0.467783796112159, 0.47353149614800955,
	0.4793201831008268, 0.4851499400560704, 0.4910208498478356, 0.4969329950608704,
	0.5028864580325687, 0.5088813208549338, 0.5149176653765214, 0.5209955732043543,
	0.5271151257058131, 0.5332764040105052, 0.5394794890121071, 0.5457244613701866,
	0.5520114015120001, 0.5583403896342679, 0.5647115057049292, 0.5711248294648731,
	0.5775804404296506, 0.5840784178911641, 0.5906188409193369, 0.5972017883637634,
	0.6038273388553378, 0.6104955708078648, 0.6172065624196511, 0.6239603916750761,
	0.6307571363461467, 0.6375968739940326, 0.6444796819705821, 0.6514056374198242,
	0.6583748172794485, 0.665387298282272, 0.6724431569576875, 0.6795424696330938,
	0.6866853124353135, 0.6938717612919899, 0.7011018919329731, 0.7083757798916868,
	0.7156935005064807, 0.7230551289219692, 0.7304607400903537, 0.7379104087727308,
	0.7454042095403874, 0.7529422167760779, 0.7605245046752924, 0.768151147247507,
	0.7758222183174236, 0.7835377915261935, 0.7912979403326302, 0.799102738014409,
	0.8069522576692516, 0.8148465722161012, 0.8227857543962835, 0.8307698767746546,
	0.83879901174074, 0.846873231509858, 0.8549926081242337, 0.8631572134541023,
	0.8713671191987972, 0.8796223968878317, 0.8879231178819663, 0.8962693533742664,
	0.9046611743911496, 0.9130986517934192, 0.9215818562772946, 0.9301108583754237,
	0.938685728457888, 0.9473065367331999, 0.9559733532492861, 0.9646862478944651,
	0.9734452903984125, 0.9822505503331171, 0.9911020971138298, 1.0,
}

// encPair is one candidate pair of colours, A (paper) and B (ink).
type encPair struct {
	a, b, ab  [3]float64
	ab2, ab2s float64
	aab, a2   float64
	span, pen float64
	la, lab   [3]float64
	lab2s     float64
	attr      CellAttr
}

// encColour is a register colour available to the encoder.
type encColour struct {
	rgb  [3]float64
	reg  int // register number
	slot int // position within its bank (one-byte palettes)
}

// encoderBanks lists the colours each attribute can pair, bank by bank, in
// the reference order, for the given register colours.
func encoderBanks(pal Palette, regs Registers) [][]encColour {
	var banks [][]encColour
	switch pal {
	case ZXPAL0, ZXPAL1, ZXPAL2:
		for b := 0; b < len(regs)/8; b++ {
			var bank []encColour
			for i := 0; i < 8; i++ {
				bank = append(bank, encColour{regColour(regs[8*b+i]), 8*b + i, i})
			}
			banks = append(banks, dedupe(bank))
		}
	case ZXPAL3:
		var bank []encColour
		for r := range regs {
			bank = append(bank, encColour{rgb: regColour(regs[r]), reg: r})
		}
		banks = [][]encColour{dedupe(bank)}
	case ZXPAL4:
		// Distinct colours in ascending RGB order, each naming its lowest register.
		var bank []encColour
		for r := range regs {
			bank = append(bank, encColour{rgb: regColour(regs[r]), reg: r})
		}
		bank = dedupe(bank)
		sort.SliceStable(bank, func(i, j int) bool {
			a, b := bank[i].rgb, bank[j].rgb
			if a[0] != b[0] {
				return a[0] < b[0]
			}
			if a[1] != b[1] {
				return a[1] < b[1]
			}
			return a[2] < b[2]
		})
		banks = [][]encColour{bank}
	}
	return banks
}

// makePair prepares the pair of colours a (paper) and b (ink) for a cell of
// n pixels, with the given span penalty.
func makePair(a, b [3]float64, penalty float64, n int) encPair {
	p := encPair{a: a, b: b}
	for k := 0; k < 3; k++ {
		p.ab[k] = p.b[k] - p.a[k]
	}
	p.ab2 = p.ab[0]*p.ab[0] + p.ab[1]*p.ab[1] + p.ab[2]*p.ab[2]
	p.ab2s = p.ab2
	if p.ab2 == 0 {
		p.ab2s = 1
	}
	p.aab = p.a[0]*p.ab[0] + p.a[1]*p.ab[1] + p.a[2]*p.ab[2]
	p.a2 = p.a[0]*p.a[0] + p.a[1]*p.a[1] + p.a[2]*p.a[2]
	p.span = math.Sqrt(p.ab2)
	p.pen = penalty * p.span * float64(n) / 1.7
	var lb [3]float64
	for k := 0; k < 3; k++ {
		p.la[k] = toLinear(p.a[k])
		lb[k] = toLinear(p.b[k])
		p.lab[k] = lb[k] - p.la[k]
	}
	lab2 := float64(p.lab[0]*p.lab[0]) + float64(p.lab[1]*p.lab[1]) + float64(p.lab[2]*p.lab[2])
	p.lab2s = lab2
	if lab2 == 0 {
		p.lab2s = 1
	}
	return p
}

// encoderPairs lists the candidate pairs in the reference order, for the
// given register colours and span penalty.
func encoderPairs(pal Palette, regs Registers, penalty float64) []encPair {
	var pairs []encPair
	for bi, bank := range encoderBanks(pal, regs) {
		for i := range bank {
			for j := i; j < len(bank); j++ {
				pa, pb := bank[i], bank[j]
				p := makePair(pa.rgb, pb.rgb, penalty, 64)
				if pal.TwoByte() {
					p.attr = CellAttr{A: byte(pb.reg), B: byte(pa.reg)}
				} else {
					p.attr = CellAttr{A: byte(bi<<6 | pa.slot<<3 | pb.slot)}
				}
				pairs = append(pairs, p)
			}
		}
	}
	return pairs
}

// dedupe keeps the first occurrence of each colour, in order.
func dedupe(in []encColour) []encColour {
	seen := map[[3]float64]bool{}
	var out []encColour
	for _, c := range in {
		if !seen[c.rgb] {
			seen[c.rgb] = true
			out = append(out, c)
		}
	}
	return out
}

// EncodePicture encodes img, which must be exactly the mode's size, with the
// reference encoder. The encoder never sets flash.
func EncodePicture(img image.Image, mode Mode, pal Palette) (*Picture, error) {
	return EncodePictureWith(img, mode, pal, EncodeOptions{})
}

// EncodeOptions refine EncodePictureWith.
type EncodeOptions struct {
	// Registers are the register colours to encode for, nil for the
	// palette's reset values. The pairs are listed as for the reset values:
	// within banks for the one-byte palettes, across all registers for the
	// two-byte ones, ZXPAL4's in ascending RGB order.
	Registers Registers

	// Penalty, when set, replaces the reference weight (0.204) of the span
	// term in each pair's score: a larger one favours closer pairs.
	Penalty *float64

	// The options below are experiments from Part 4, section 5 of the
	// specification; with all of them zero the encoder is the reference one.

	// LinearPairs chooses each cell's pair by distance in linear light (the
	// light the dither mixes) instead of sRGB.
	LinearPairs bool
	// Diffuse spreads each cell's colour error, times this factor (1 for
	// all of it), into the cells not yet encoded, as Floyd-Steinberg
	// spreads a pixel's: right 7/16, below-left 3/16, below 5/16,
	// below-right 1/16. It makes the encoding sequential.
	Diffuse float64
	// FlatSpread, when not zero, fills a cell whose pixels spread less than
	// this (the RMS distance from their mean, on a 0-255 scale) with one
	// register colour instead of dithering a pair.
	FlatSpread float64
}

// EncodePictureWith is EncodePicture with options.
func EncodePictureWith(img image.Image, mode Mode, pal Palette, opt EncodeOptions) (*Picture, error) {
	if err := CheckRegisters(pal, opt.Registers); err != nil {
		return nil, err
	}
	p, err := NewPicture(mode, pal)
	if err != nil {
		return nil, err
	}
	p.Registers = opt.Registers
	b := img.Bounds()
	if b.Dx() != p.Width || b.Dy() != p.Height {
		return nil, fmt.Errorf("scr: image is %dx%d, %s needs %dx%d (resize before converting)",
			b.Dx(), b.Dy(), mode, p.Width, p.Height)
	}
	penalty := refPenalty
	if opt.Penalty != nil {
		penalty = *opt.Penalty
	}
	pairs := encoderPairs(pal, opt.Registers.Or(pal), penalty)

	var lin [256]float64
	for v := range lin {
		lin[v] = toLinear(float64(v))
	}

	rows, cols := p.Height/CellSize, p.Width/CellSize
	// With error diffusion each cell depends on the ones before it, so the
	// cells are encoded one at a time in raster order; otherwise rows of
	// cells are shared among the CPUs.
	workers := runtime.NumCPU()
	var carry [][][3]float64
	if opt.Diffuse != 0 {
		workers = 1
		carry = make([][][3]float64, rows+1)
		for i := range carry {
			carry[i] = make([][3]float64, cols+2)
		}
	}
	work := make(chan int, rows)
	for cy := 0; cy < rows; cy++ {
		work <- cy
	}
	close(work)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var x [64][3]float64
			var lx [64][3]float64
			var s [64]float64
			for cy := range work {
				for cx := 0; cx < cols; cx++ {
					var mean [3]float64
					for i := 0; i < 64; i++ {
						r, g, bl, _ := img.At(b.Min.X+cx*8+i%8, b.Min.Y+cy*8+i/8).RGBA()
						v := [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
						if carry != nil {
							for k := 0; k < 3; k++ {
								v[k] = int(math.Round(math.Max(0, math.Min(255, float64(v[k])+carry[cy][cx+1][k]))))
							}
						}
						for k := 0; k < 3; k++ {
							x[i][k] = float64(v[k])
							lx[i][k] = lin[v[k]]
							mean[k] += x[i][k] / 64
						}
						s[i] = x[i][0]*x[i][0] + x[i][1]*x[i][1] + x[i][2]*x[i][2]
					}
					flat := false
					if opt.FlatSpread != 0 {
						var v float64
						for i := 0; i < 64; i++ {
							for k := 0; k < 3; k++ {
								d := x[i][k] - mean[k]
								v += d * d
							}
						}
						flat = math.Sqrt(v/192) < opt.FlatSpread
					}
					best, bestCost := 0, math.Inf(1)
					for k := range pairs {
						pr := &pairs[k]
						if flat && pr.span != 0 {
							continue
						}
						sum := 0.0
						if opt.LinearPairs {
							for i := 0; i < 64; i++ {
								var d [3]float64
								t := 0.0
								for c := 0; c < 3; c++ {
									d[c] = lx[i][c] - pr.la[c]
									t += d[c] * pr.lab[c]
								}
								t /= pr.lab2s
								if t < 0 {
									t = 0
								} else if t > 1 {
									t = 1
								}
								e := 0.0
								for c := 0; c < 3; c++ {
									q := d[c] - t*pr.lab[c]
									e += q * q
								}
								sum += 255 * math.Sqrt(e)
							}
						} else {
							for i := 0; i < 64; i++ {
								xa := x[i][0]*pr.a[0] + x[i][1]*pr.a[1] + x[i][2]*pr.a[2]
								xab := x[i][0]*pr.ab[0] + x[i][1]*pr.ab[1] + x[i][2]*pr.ab[2] - pr.aab
								t := xab / pr.ab2s
								if t < 0 {
									t = 0
								} else if t > 1 {
									t = 1
								}
								// float64() around a product stops the compiler fusing it into a
								// multiply-add (as Go does on arm64), which would change the rounding.
								d2 := s[i] - 2*xa + pr.a2 - float64(2*t*xab) + float64(t*t*pr.ab2)
								if d2 < 0 {
									d2 = 0
								}
								sum += math.Sqrt(d2)
							}
						}
						if c := sum + pr.pen; c < bestCost {
							best, bestCost = k, c
						}
					}
					pr := &pairs[best]
					p.Attr[cy][cx] = pr.attr
					inks := 0
					if pr.span != 0 {
						for i := 0; i < 64; i++ {
							tl := (float64((lx[i][0]-pr.la[0])*pr.lab[0]) + float64((lx[i][1]-pr.la[1])*pr.lab[1]) +
								float64((lx[i][2]-pr.la[2])*pr.lab[2])) / pr.lab2s
							if tl < 0 {
								tl = 0
							} else if tl > 1 {
								tl = 1
							}
							if tl > bayer8[i/8][i%8] {
								p.Ink[cy*8+i/8][cx*8+i%8] = true
								inks++
							}
						}
					}
					if carry != nil {
						// The cell's colour error, spread to the cells not yet
						// encoded as Floyd and Steinberg spread a pixel's.
						f := float64(inks) / 64
						for k := 0; k < 3; k++ {
							e := opt.Diffuse * (mean[k] - ((1-f)*pr.a[k] + f*pr.b[k]))
							carry[cy][cx+2][k] += e * 7 / 16
							carry[cy+1][cx][k] += e * 3 / 16
							carry[cy+1][cx+1][k] += e * 5 / 16
							carry[cy+1][cx+2][k] += e * 1 / 16
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	return p, nil
}
