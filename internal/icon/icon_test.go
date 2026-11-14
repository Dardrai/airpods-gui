package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func TestPNGDecodesAtEverySize(t *testing.T) {
	for _, size := range Sizes {
		img, err := png.Decode(bytes.NewReader(PNG(Variant{Level: 80}, size)))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Errorf("size %d: %v", size, img.Bounds())
		}
	}
}

func TestICOStructure(t *testing.T) {
	data := ICO(Variant{Level: 42})
	le := binary.LittleEndian
	if le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 || int(le.Uint16(data[4:])) != len(Sizes) {
		t.Fatalf("header % x", data[:6])
	}
	end := 6 + 16*len(Sizes)
	for i, size := range Sizes {
		entry := data[6+16*i:]
		side := int(entry[0])
		if side == 0 {
			side = 256
		}
		if side != size {
			t.Errorf("entry %d is %d wide", i, side)
		}
		length, offset := int(le.Uint32(entry[8:])), int(le.Uint32(entry[12:]))
		if offset != end {
			t.Errorf("entry %d at %d, want %d", i, offset, end)
		}
		end = offset + length
		body := data[offset : offset+length]
		if size >= 256 {
			if _, err := png.Decode(bytes.NewReader(body)); err != nil {
				t.Errorf("large entry: %v", err)
			}
		} else if le.Uint32(body) != 40 || int(le.Uint32(body[8:])) != size*2 {
			t.Errorf("entry %d header % x", i, body[:12])
		}
	}
	if end != len(data) {
		t.Errorf("%d trailing bytes", len(data)-end)
	}
}

// downscale is what a tray host does to the 64 pixel PNG: an area average to its own size.
func downscale(src *image.NRGBA, to int) *image.NRGBA {
	from := src.Bounds().Dx()
	out := image.NewNRGBA(image.Rect(0, 0, to, to))
	ratio := float64(from) / float64(to)
	for y := range to {
		for x := range to {
			var r, g, b, a, total float64
			for sy := int(float64(y) * ratio); sy < from && float64(sy) < float64(y+1)*ratio; sy++ {
				wy := math.Min(float64(sy+1), float64(y+1)*ratio) - math.Max(float64(sy), float64(y)*ratio)
				for sx := int(float64(x) * ratio); sx < from && float64(sx) < float64(x+1)*ratio; sx++ {
					wx := math.Min(float64(sx+1), float64(x+1)*ratio) - math.Max(float64(sx), float64(x)*ratio)
					c := src.NRGBAAt(sx, sy)
					w := wx * wy
					alpha := float64(c.A) * w
					r += float64(c.R) * alpha
					g += float64(c.G) * alpha
					b += float64(c.B) * alpha
					a += alpha
					total += w
				}
			}
			if a > 0 {
				out.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / total)})
			}
		}
	}
	return out
}

func alphaSum(img *image.NRGBA) float64 {
	sum := 0.0
	for i := 3; i < len(img.Pix); i += 4 {
		sum += float64(img.Pix[i]) / 255
	}
	return sum
}

func on(panel color.NRGBA, img *image.NRGBA) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), &image.Uniform{panel}, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Over)
	return out
}

func TestVariantsDiffer(t *testing.T) {
	variants := []Variant{
		{Level: NoBadge}, {Level: NoBadge, Dim: true}, {Level: NoBadge, NoDongle: true}, {Level: NoBadge, LightPanel: true},
		{Level: 80}, {Level: 20}, {Level: 100}, {Level: NoBadge, Tile: true},
	}
	seen := map[string]int{}
	for i, v := range variants {
		data := string(PNG(v, 64))
		if j, dup := seen[data]; dup {
			t.Errorf("variants %d and %d look the same", i, j)
		}
		seen[data] = i
	}
}

func saturated(img *image.NRGBA) int {
	count := 0
	for i := 0; i < len(img.Pix); i += 4 {
		c := img.Pix[i : i+4]
		if c[3] > 64 && (int(c[0])-int(c[2]) > 40 || int(c[2])-int(c[0]) > 40) {
			count++
		}
	}
	return count
}

func TestColourIsOnlyForALowBattery(t *testing.T) {
	for _, v := range []Variant{{Level: NoBadge}, {Level: NoBadge, Dim: true}, {Level: NoBadge, NoDongle: true}, {Level: 21}, {Level: 50}, {Level: 100}} {
		if n := saturated(Render(v, 64)); n != 0 {
			t.Errorf("%+v has %d coloured pixels", v, n)
		}
	}
	if saturated(Render(Variant{Level: 20}, 64)) == 0 {
		t.Error("a low battery shows no colour")
	}
}

// Every state must stand out on both panels, even on the one it was not made for, at the
// size a host really shows.
func TestReadableOnBothPanelsAfterScaling(t *testing.T) {
	panels := []color.NRGBA{{0x20, 0x22, 0x26, 0xff}, {0xf2, 0xf2, 0xf2, 0xff}}
	for _, light := range []bool{false, true} {
		for _, v := range []Variant{{Level: NoBadge}, {Level: NoBadge, Dim: true}, {Level: NoBadge, NoDongle: true}, {Level: 75}} {
			v.LightPanel = light
			for _, panel := range panels {
				img := on(panel, downscale(Render(v, 64), 22))
				bg := float64(panel.R)
				differs := 0
				for y := range 22 {
					for x := range 22 {
						if math.Abs(float64(img.RGBAAt(x, y).R)-bg) > 30 {
							differs++
						}
					}
				}
				if differs < 25 {
					t.Errorf("%+v on %v: only %d pixels stand out", v, panel, differs)
				}
			}
		}
	}
}

// The two buds stay apart with a clear gap at the scaled size, with and without a badge.
func TestTwoBudsAfterScaling(t *testing.T) {
	for _, to := range []int{16, 22, 24, 32} {
		img := downscale(Render(Variant{Level: NoBadge}, 64), to)
		middle := 0.0
		for y := range to {
			middle += float64(img.NRGBAAt(to/2, y).R) * float64(img.NRGBAAt(to/2, y).A) / 65025
		}
		left := 0.0
		for y := range to {
			c := img.NRGBAAt(to/4, y)
			left += float64(c.R) * float64(c.A) / 65025
		}
		if left <= middle {
			t.Errorf("at %d the middle is as full as a bud: %v and %v", to, middle, left)
		}
	}
}

func TestDigitsSurviveTheHostScaling(t *testing.T) {
	// Two different percentages must still differ strongly once scaled.
	a := downscale(Render(Variant{Level: 11}, 64), 22)
	b := downscale(Render(Variant{Level: 88}, 64), 22)
	diff := 0
	for i := 0; i < len(a.Pix); i += 4 {
		if math.Abs(float64(a.Pix[i])-float64(b.Pix[i])) > 50 {
			diff++
		}
	}
	if diff < 8 {
		t.Errorf("only %d pixels tell 11 from 88 at 22", diff)
	}
}

func TestDetailOnlyWhereItHolds(t *testing.T) {
	small, large := alphaSum(Render(Variant{Level: NoBadge, Tile: true}, 64)), alphaSum(Render(Variant{Level: NoBadge, Tile: true}, 128))
	if small <= 0 || large <= 0 {
		t.Fatal("drew nothing")
	}
	if got := SVG(); !strings.Contains(got, pathHeads) || !strings.Contains(got, pathSlots) {
		t.Error("the scalable icon must come from the same path")
	}
}

// TestPreviewSheet saves a sheet of every state on dark and light panels, magnified, when
// AIRPODS_ICON_SHEET names a file: native sizes, then the 64 pixel PNG scaled to 22 and 24.
func TestPreviewSheet(t *testing.T) {
	path := os.Getenv("AIRPODS_ICON_SHEET")
	if path == "" {
		t.Skip("set AIRPODS_ICON_SHEET to write the sheet")
	}
	variants := []Variant{
		{Level: NoBadge}, {Level: 75}, {Level: 42}, {Level: 100}, {Level: 18}, {Level: 7},
		{Level: NoBadge, Dim: true}, {Level: NoBadge, NoDongle: true},
	}
	type cell struct {
		name string
		make func(Variant) *image.NRGBA
	}
	cells := []cell{}
	for _, size := range []int{16, 22, 24, 32} {
		cells = append(cells, cell{"native", func(v Variant) *image.NRGBA { return Render(v, size) }})
	}
	for _, size := range []int{22, 24} {
		cells = append(cells, cell{"scaled", func(v Variant) *image.NRGBA { return downscale(Render(v, 64), size) }})
	}
	const zoom = 4
	rowHeight := 32*zoom + 8
	width := 0
	columns := make([]int, len(cells))
	sizes := []int{16, 22, 24, 32, 22, 24}
	for i := range cells {
		columns[i] = width
		width += sizes[i]*zoom + 8
	}
	panels := []struct {
		fill  color.NRGBA
		light bool
	}{{color.NRGBA{0x20, 0x22, 0x26, 0xff}, false}, {color.NRGBA{0xee, 0xee, 0xf0, 0xff}, true}}
	sheet := image.NewRGBA(image.Rect(0, 0, width*2+24, rowHeight*len(variants)+256+16))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.NRGBA{0x80, 0x80, 0x80, 0xff}}, image.Point{}, draw.Src)
	for p, panel := range panels {
		left := p * (width + 24)
		draw.Draw(sheet, image.Rect(left, 0, left+width, rowHeight*len(variants)), &image.Uniform{panel.fill}, image.Point{}, draw.Src)
		for row, v := range variants {
			v.LightPanel = panel.light
			for c, entry := range cells {
				icon := on(panel.fill, entry.make(v))
				size := icon.Bounds().Dx()
				for y := range size * zoom {
					for x := range size * zoom {
						sheet.Set(left+columns[c]+4+x, row*rowHeight+4+y, icon.At(x/zoom, y/zoom))
					}
				}
			}
		}
	}
	draw.Draw(sheet, image.Rect(0, rowHeight*len(variants)+8, 256, rowHeight*len(variants)+8+256), Render(Variant{Level: NoBadge, Tile: true}, 256), image.Point{}, draw.Over)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, sheet); err != nil {
		t.Fatal(err)
	}
}
