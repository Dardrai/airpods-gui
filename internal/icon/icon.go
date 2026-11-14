// Package icon draws the tray and application icon: a solid pair of AirPods, from LibrePods'
// monochrome vector (GPL-3.0, https://github.com/Ripthulhu/librepods), with a small badge for
// the lowest earbud percentage. It is white on a dark panel and near black on a light one, and
// faint in the opposite colour around the edge, so it stays visible if the panel is guessed
// wrong. Away is a dimmed glyph and no dongle an outline. Red is kept for a low battery.
package icon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// NoBadge is a Variant.Level without a known percentage.
const NoBadge = -1

// Variant selects what the icon shows.
type Variant struct {
	Level      int  // 0..100, or NoBadge
	Dim        bool // dimmed glyph: the AirPods are not connected
	NoDongle   bool // outline glyph: no dongle
	LightPanel bool // for a light panel: dark glyph
	Tile       bool // the application icon: white glyph on a dark rounded tile
}

// Sizes are the images in the ICO file.
var Sizes = []int{16, 20, 24, 32, 48, 64, 256}

var (
	onDark   = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	onLight  = color.NRGBA{0x1f, 0x23, 0x28, 0xff}
	lowBadge = color.NRGBA{0xe0, 0x35, 0x35, 0xff}
	tileFill = color.NRGBA{0x1c, 0x1c, 0x1e, 0xff}
)

const (
	dimAlpha      = 0.45
	outlineAlpha  = 0.9
	haloAlpha     = 0.35
	detailMinSize = 128 // below this the speaker slots and tip holes only blur
	lowLevel      = 20
)

// The vector, in its own coordinates. Its parts are the stems, the heads and the tips; the
// detail paths are holes in them. All absolute M, C, L and Z.
const (
	viewWidth, viewHeight = 902.34, 607.69
	contentLeft           = 29.05
	contentRight          = 873.29
	contentTop            = 30.29
	contentBottom         = 577.41

	pathStems = "M315.92,550.31C315.92,567.88 304.69,577.41 286.13,577.41L261.48,577.41C242.92,577.41 231.45,567.88 231.45,550.31L231.45,358.73C267.25,355.98 293.73,349.94 315.92,341.64ZM670.9,358.73L670.9,550.31C670.9,567.88 659.42,577.41 640.87,577.41L616.21,577.41C597.66,577.41 586.43,567.88 586.43,550.31L586.43,341.64C608.62,349.94 635.09,355.98 670.9,358.73Z"
	pathHeads = "M429.2,153.09C429.2,221.45 388.18,270.28 335.2,299.57C312.69,312 288.17,321.38 249.56,326.11C255.64,311.65 259.03,294.95 259.03,276.14C259.03,213.57 207.98,151 137.53,139.93C143.33,128.37 149.48,118.78 153.81,113.29C192.14,56.65 252.44,29.55 306.64,30.29C375.24,31.02 429.2,76.43 429.2,153.09ZM748.53,113.29C752.86,118.78 759.01,128.37 764.82,139.93C694.36,151 643.31,213.57 643.31,276.14C643.31,294.95 646.71,311.65 652.78,326.11C614.17,321.38 589.66,312 567.14,299.57C514.16,270.28 473.14,221.45 473.14,153.09C473.14,76.43 527.1,31.02 595.7,30.29C649.9,29.55 710.21,56.65 748.53,113.29Z"
	pathSlots = "M346.19,100.11L301.51,137.71C295.41,142.84 294.68,151.62 299.56,157.48C304.69,163.83 313.72,164.32 319.34,159.44L364.75,121.84C370.85,116.71 371.09,107.92 365.97,102.06C361.33,95.72 352.3,94.98 346.19,100.11ZM536.38,102.06C531.25,107.92 531.49,116.71 537.6,121.84L583.01,159.44C588.62,164.32 597.66,163.83 602.78,157.48C607.67,151.62 606.93,142.84 600.83,137.71L556.15,100.11C550.05,94.98 541.02,95.72 536.38,102.06Z"
	pathTips  = "M140.87,364.76C189.45,364.76 229.98,334.48 229.98,276.14C229.98,222.43 180.18,167.25 114.99,167.25C61.28,167.25 29.05,209.24 29.05,254.65C29.05,320.08 84.72,364.76 140.87,364.76ZM761.47,364.76C817.63,364.76 873.29,320.08 873.29,254.65C873.29,209.24 841.06,167.25 787.35,167.25C722.17,167.25 672.36,222.43 672.36,276.14C672.36,334.48 712.89,364.76 761.47,364.76Z"
	pathHoles = "M125.98,319.59C117.19,327.16 102.29,319.35 84.23,297.38C66.41,275.89 61.52,260.51 70.07,252.94C79.1,245.37 93.75,252.94 111.82,274.92C129.4,297.13 134.52,312.27 125.98,319.59ZM776.37,319.59C767.82,312.27 772.95,297.13 790.53,274.92C808.59,252.94 823.24,245.37 832.28,252.94C840.82,260.51 835.94,275.89 818.11,297.38C800.29,319.35 785.16,327.16 776.37,319.59Z"
)

type point struct{ x, y float64 }

// flatten turns path data into closed polygons.
func flatten(data string) [][]point {
	var polygons [][]point
	var current []point
	numbers := func(s string) []float64 {
		fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
		out := make([]float64, len(fields))
		for i, field := range fields {
			value, err := strconv.ParseFloat(field, 64)
			if err != nil {
				panic(err) // The path is a constant above.
			}
			out[i] = value
		}
		return out
	}
	for i := 0; i < len(data); {
		end := i + 1
		for end < len(data) && !strings.ContainsRune("MCLZ", rune(data[end])) {
			end++
		}
		args := numbers(data[i+1 : end])
		switch data[i] {
		case 'M':
			current = []point{{args[0], args[1]}}
		case 'L':
			for j := 0; j+1 < len(args); j += 2 {
				current = append(current, point{args[j], args[j+1]})
			}
		case 'C':
			for j := 0; j+5 < len(args); j += 6 {
				p0 := current[len(current)-1]
				for step := 1; step <= 16; step++ {
					t := float64(step) / 16
					u := 1 - t
					a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
					current = append(current, point{
						a*p0.x + b*args[j] + c*args[j+2] + d*args[j+4],
						a*p0.y + b*args[j+1] + c*args[j+3] + d*args[j+5],
					})
				}
			}
		case 'Z':
			polygons = append(polygons, current)
			current = nil
		}
		i = end
	}
	return polygons
}

// fill returns the even-odd coverage of polygons, placed by scale and offset, on a square.
func fill(polygons [][]point, size int, scale, dx, dy float64) []float64 {
	const lines = 8
	cover := make([]float64, size*size)
	for line := range size * lines {
		y := (float64(line) + 0.5) / lines
		var crossings []float64
		for _, polygon := range polygons {
			for i, a := range polygon {
				b := polygon[(i+1)%len(polygon)]
				ay, by := a.y*scale+dy, b.y*scale+dy
				if (ay <= y) != (by <= y) {
					crossings = append(crossings, a.x*scale+dx+(y-ay)/(by-ay)*(b.x-a.x)*scale)
				}
			}
		}
		slices.Sort(crossings)
		row := line / lines
		for i := 0; i+1 < len(crossings); i += 2 {
			x0, x1 := max(crossings[i], 0), min(crossings[i+1], float64(size))
			for x := int(x0); x < size && float64(x) < x1; x++ {
				overlap := min(x1, float64(x+1)) - max(x0, float64(x))
				if overlap > 0 {
					cover[row*size+x] += overlap / lines
				}
			}
		}
	}
	return cover
}

var (
	polygonsOnce     sync.Once
	simplePolygons   [][]point
	detailedPolygons [][]point
	maskMu           sync.Mutex
	masks            = map[maskKey][]float64{}
)

type maskKey struct {
	size   int
	badged bool
	tile   bool
}

// glyphArea is the box the glyph gets. A badge takes the bottom right, so the glyph rises to
// the top and keeps its full width; on a tile it keeps a margin.
func glyphArea(key maskKey) (scale, dx, dy float64) {
	size := float64(key.size)
	width := contentRight - contentLeft
	height := contentBottom - contentTop
	inner := size
	if key.tile {
		inner = size * 0.78
	}
	scale = inner / width
	dx = (size - width*scale) / 2
	dy = (size - height*scale) / 2
	if key.badged {
		dy = 0
	}
	return scale, dx - contentLeft*scale, dy - contentTop*scale
}

// glyphMask is the glyph's coverage, without the detail at sizes that would only blur it.
func glyphMask(key maskKey) []float64 {
	polygonsOnce.Do(func() {
		simplePolygons = flatten(pathStems + pathHeads + pathTips)
		detailedPolygons = flatten(pathStems + pathHeads + pathSlots + pathTips + pathHoles)
	})
	maskMu.Lock()
	defer maskMu.Unlock()
	if mask, ok := masks[key]; ok {
		return mask
	}
	polygons := simplePolygons
	if key.size >= detailMinSize {
		polygons = detailedPolygons
	}
	scale, dx, dy := glyphArea(key)
	mask := fill(polygons, key.size, scale, dx, dy)
	masks[key] = mask
	return mask
}

// spread is a mask grown (or, with shrink, cut back) by a radius in pixels.
func spread(mask []float64, size int, radius float64, shrink bool) []float64 {
	out := make([]float64, len(mask))
	r := int(math.Ceil(radius))
	for y := range size {
		for x := range size {
			best := mask[y*size+x]
			for oy := -r; oy <= r; oy++ {
				for ox := -r; ox <= r; ox++ {
					distance := math.Hypot(float64(ox), float64(oy))
					if distance > radius+0.5 {
						continue
					}
					weight := min(1, radius+0.5-distance)
					nx, ny := x+ox, y+oy
					value := 0.0
					if nx >= 0 && ny >= 0 && nx < size && ny < size {
						value = mask[ny*size+nx]
					}
					if shrink {
						best = min(best, 1-(1-value)*weight)
					} else {
						best = max(best, value*weight)
					}
				}
			}
			out[y*size+x] = best
		}
	}
	return out
}

func over(dst *image.NRGBA, x, y int, c color.NRGBA, coverage float64) {
	if coverage <= 0 {
		return
	}
	bg := dst.NRGBAAt(x, y)
	a := float64(c.A) / 255 * min(coverage, 1)
	ba := float64(bg.A) / 255
	out := a + ba*(1-a)
	mix := func(fg, back uint8) uint8 {
		return uint8((float64(fg)*a + float64(back)*ba*(1-a)) / out)
	}
	dst.SetNRGBA(x, y, color.NRGBA{mix(c.R, bg.R), mix(c.G, bg.G), mix(c.B, bg.B), uint8(out*255 + 0.5)})
}

// paintMask draws a coverage mask in a colour.
func paintMask(dst *image.NRGBA, mask []float64, c color.NRGBA, alpha float64) {
	size := dst.Bounds().Dx()
	for y := range size {
		for x := range size {
			over(dst, x, y, c, mask[y*size+x]*alpha)
		}
	}
}

// clearRect erases, for the gap between the glyph and the badge.
func clearRect(dst *image.NRGBA, x0, y0, x1, y1 int) {
	for y := max(y0, 0); y < min(y1, dst.Bounds().Dy()); y++ {
		for x := max(x0, 0); x < min(x1, dst.Bounds().Dx()); x++ {
			dst.SetNRGBA(x, y, color.NRGBA{})
		}
	}
}

// digits is a 3x5 pixel font; each row is three bits.
var digits = [10][5]byte{
	{7, 5, 5, 5, 7}, {2, 6, 2, 2, 7}, {7, 1, 7, 4, 7}, {7, 1, 3, 1, 7}, {5, 5, 7, 1, 1},
	{7, 4, 7, 1, 7}, {7, 4, 7, 5, 7}, {7, 1, 1, 2, 2}, {7, 5, 7, 5, 7}, {7, 5, 7, 1, 7},
}

// checkMark is the full battery's badge, five bits wide; three digits would take more of a
// small icon than the glyph can spare.
var checkMark = [5]byte{1, 3, 22, 28, 8}

func rect(dst *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			over(dst, x, y, c, 1)
		}
	}
}

// drawBadge draws whole-pixel characters on a pill with cut corners, so they stay sharp at
// tray sizes and survive a host's scaling. The pill is the panel's foreground, red when low.
func drawBadge(dst *image.NRGBA, level int, foreground, background color.NRGBA) {
	size := dst.Bounds().Dx()
	var chars [][5]byte
	columns := 3
	if level >= 100 {
		chars, columns = [][5]byte{checkMark}, 5
	} else {
		for _, r := range strconv.Itoa(level) {
			chars = append(chars, digits[r-'0'])
		}
	}
	scale := max(1, size/16)
	width := (len(chars)*(columns+1) - 1) * scale
	height := 5 * scale
	pad := scale
	x1, y1 := size, size
	x0, y0 := x1-width-2*pad, y1-height-2*pad
	clearRect(dst, x0-scale, y0-scale, x1, y1)
	fillColor, ink := foreground, background
	if level <= lowLevel {
		fillColor, ink = lowBadge, onDark
	}
	rect(dst, x0, y0, x1, y1, fillColor)
	for _, corner := range [][2]int{{x0, y0}, {x1 - scale, y0}, {x0, y1 - scale}, {x1 - scale, y1 - scale}} {
		clearRect(dst, corner[0], corner[1], corner[0]+scale, corner[1]+scale)
	}
	for i, font := range chars {
		for row, bits := range font {
			for col := range columns {
				if bits&(1<<(columns-1-col)) != 0 {
					px := x0 + pad + (i*(columns+1)+col)*scale
					py := y0 + pad + row*scale
					rect(dst, px, py, px+scale, py+scale, ink)
				}
			}
		}
	}
}

// drawTile draws the application icon's rounded square.
func drawTile(dst *image.NRGBA) {
	size := dst.Bounds().Dx()
	radius := float64(size) * 0.22
	for y := range size {
		for x := range size {
			hits := 0
			for sy := range 4 {
				for sx := range 4 {
					px, py := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4
					qx := math.Abs(px-float64(size)/2) - float64(size)/2 + radius
					qy := math.Abs(py-float64(size)/2) - float64(size)/2 + radius
					if math.Hypot(max(qx, 0), max(qy, 0))+min(max(qx, qy), 0)-radius <= 0 {
						hits++
					}
				}
			}
			over(dst, x, y, tileFill, float64(hits)/16)
		}
	}
}

// Render draws a variant at a square size.
func Render(v Variant, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	badged := v.Level != NoBadge && !v.NoDongle && !v.Tile
	mask := glyphMask(maskKey{size: size, badged: badged, tile: v.Tile})
	foreground, background := onDark, onLight
	if v.LightPanel {
		foreground, background = onLight, onDark
	}
	if v.Tile {
		drawTile(dst)
		paintMask(dst, mask, onDark, 1)
		return dst
	}
	unit := float64(size) / 22 // a pixel on the panel, after a host scales the icon to fit
	paintMask(dst, spread(mask, size, unit, false), background, haloAlpha)
	switch {
	case v.NoDongle:
		inner := spread(mask, size, max(1.3*unit, 1.2), true)
		ring := make([]float64, len(mask))
		for i := range mask {
			ring[i] = max(mask[i]-inner[i], 0)
		}
		clearInner(dst, inner)
		paintMask(dst, ring, foreground, outlineAlpha)
	case v.Dim:
		paintMask(dst, mask, foreground, dimAlpha)
	default:
		paintMask(dst, mask, foreground, 1)
	}
	if badged {
		drawBadge(dst, max(0, min(100, v.Level)), foreground, background)
	}
	return dst
}

// clearInner takes the halo back out from inside an outline.
func clearInner(dst *image.NRGBA, inner []float64) {
	size := dst.Bounds().Dx()
	for y := range size {
		for x := range size {
			if inner[y*size+x] > 0.5 {
				dst.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
}

// SVG is the application icon as vectors, from the same path as the bitmaps.
func SVG() string {
	const size = 256
	scale, dx, dy := glyphArea(maskKey{size: size, tile: true})
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]d %[1]d" width="%[1]d" height="%[1]d">
  <rect width="%[1]d" height="%[1]d" rx="%.2f" fill="#1c1c1e"/>
  <path transform="translate(%.3f %.3f) scale(%.5f)" fill="#ffffff" fill-rule="evenodd" d="%s"/>
</svg>
`, size, size*0.22, dx, dy, scale, pathStems+pathHeads+pathSlots+pathTips+pathHoles)
}

// PNG encodes a variant at one size.
func PNG(v Variant, size int) []byte {
	var out bytes.Buffer
	if err := png.Encode(&out, Render(v, size)); err != nil {
		panic(err) // Writing to a bytes.Buffer does not fail.
	}
	return out.Bytes()
}

// dib is one ICO image as 32-bit BGRA with its AND mask, bottom row first.
func dib(img *image.NRGBA) []byte {
	size := img.Bounds().Dx()
	var out bytes.Buffer
	header := struct {
		Size                  uint32
		Width, Height         int32
		Planes, BitCount      uint16
		Compression, ImageLen uint32
		XPels, YPels          int32
		Used, Important       uint32
	}{Size: 40, Width: int32(size), Height: int32(size * 2), Planes: 1, BitCount: 32}
	binary.Write(&out, binary.LittleEndian, header)
	for y := size - 1; y >= 0; y-- {
		for x := range size {
			c := img.NRGBAAt(x, y)
			out.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	rowBytes := ((size + 31) / 32) * 4
	mask := make([]byte, rowBytes*size)
	for row := range size {
		y := size - 1 - row
		for x := range size {
			if img.NRGBAAt(x, y).A == 0 {
				mask[row*rowBytes+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	out.Write(mask)
	return out.Bytes()
}

// ICO encodes a variant at every size in Sizes. The largest is stored as PNG, the
// others as plain bitmaps, which every Windows icon loader reads.
func ICO(v Variant) []byte {
	images := make([][]byte, len(Sizes))
	for i, size := range Sizes {
		if size >= 256 {
			images[i] = PNG(v, size)
		} else {
			images[i] = dib(Render(v, size))
		}
	}
	var out bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&out, le, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, data := range images {
		side := byte(Sizes[i])
		if Sizes[i] >= 256 {
			side = 0
		}
		out.Write([]byte{side, side, 0, 0})
		binary.Write(&out, le, [2]uint16{1, 32})
		binary.Write(&out, le, [2]uint32{uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range images {
		out.Write(data)
	}
	return out.Bytes()
}
