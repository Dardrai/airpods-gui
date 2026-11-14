package ui

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	"github.com/Dardrai/airpods-gui/core/model"
)

//go:embed assets/*.png
var assetFiles embed.FS

const (
	artLeft = iota
	artRight
	artCase
	artHeadphones
)

// artFit is how much of the box each piece fills along its longer side, so buds and case keep
// the page's proportions at one size of box.
var artFit = map[int]float64{artLeft: 1, artRight: 1, artCase: 0.985, artHeadphones: 1}

// defaultArtSet is the artwork of a model nobody has identified.
const defaultArtSet = "pro_2"

// artSetFor picks a model's artwork: a set of left, right and case files, or for over-ear
// models a single image. Unknown models and those without artwork of their own use the default.
func artSetFor(number string) string {
	profile, ok := model.Find(number)
	if !ok {
		return defaultArtSet
	}
	switch profile.Family {
	case "airpods_1":
		return "1"
	case "airpods_2":
		return "2"
	case "airpods_3":
		return "3"
	case "airpods_4":
		return "4"
	case "airpods_5":
		return "5"
	case "airpods_pro_1":
		return "pro_1"
	case "airpods_pro_3":
		return "pro_3"
	case "airpods_max":
		if number == "A3454" {
			return "max_2"
		}
		return "max_1"
	}
	return defaultArtSet
}

func artFile(set string, kind int) string {
	switch kind {
	case artHeadphones:
		if set != "max_2" {
			set = "max_1"
		}
		return "airpods_" + set + ".png"
	case artLeft:
		return "airpods_" + set + "_left.png"
	case artRight:
		return "airpods_" + set + "_right.png"
	}
	return "airpods_" + set + "_case.png"
}

var iconFiles = map[int]string{
	iconTransparency: "ic_transparency.png",
	iconAdaptive:     "ic_adaptive.png",
	iconCancellation: "ic_noise_cancellation.png",
}

var (
	assetMu    sync.Mutex
	assetCache = map[string]*image.NRGBA{}
	assetSrc   = map[string]*image.NRGBA{}
)

// loadAsset decodes one embedded image, cropped to what it actually draws. The caller holds
// assetMu.
func loadAsset(name string) *image.NRGBA {
	if img, ok := assetSrc[name]; ok {
		return img
	}
	data, err := assetFiles.ReadFile("assets/" + name)
	if err != nil {
		panic(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	bounds := decoded.Bounds()
	full := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(full, full.Bounds(), decoded, bounds.Min, draw.Src)
	crop := image.Rectangle{}
	for y := range full.Rect.Dy() {
		for x := range full.Rect.Dx() {
			if full.NRGBAAt(x, y).A > 8 {
				crop = crop.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	out := full.SubImage(crop).(*image.NRGBA)
	assetSrc[name] = out
	return out
}

// artImage is a piece of the default artwork; see modelArtImage.
func artImage(kind int, w, h int, dim bool) image.Image {
	return modelArtImage(defaultArtSet, kind, w, h, dim)
}

// modelArtImage is the earbud, case or headphone image of an artwork set, scaled for the box
// and centred whatever its aspect ratio, dimmed for a stale or unavailable piece.
func modelArtImage(set string, kind int, w, h int, dim bool) image.Image {
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 0), max(h, 0)))
	}
	key := fmt.Sprintf("art/%s/%d/%d/%d/%v", set, kind, w, h, dim)
	assetMu.Lock()
	defer assetMu.Unlock()
	if img, ok := assetCache[key]; ok {
		return img
	}
	fit, ok := artFit[kind]
	if !ok {
		fit = artFit[artCase]
	}
	src := loadAsset(artFile(set, kind))
	sw, sh := float64(src.Rect.Dx()), float64(src.Rect.Dy())
	scale := math.Min(float64(w)/sw, float64(h)/sh) * fit
	dw, dh := max(1, int(sw*scale+0.5)), max(1, int(sh*scale+0.5))
	scaled := resample(src, dw, dh)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	at := image.Pt((w-dw)/2, (h-dh)/2)
	draw.Draw(img, scaled.Rect.Add(at), scaled, image.Point{}, draw.Src)
	if dim {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = uint8(float64(img.Pix[i]) * 0.34)
		}
	}
	keepAsset(key, img)
	return img
}

// assetIcon draws a monochrome icon in the tint, using the file's alpha as coverage.
func assetIcon(name string, w, h int, tint color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return img
	}
	key := fmt.Sprintf("icon/%s/%d/%d", name, w, h)
	assetMu.Lock()
	mask, ok := assetCache[key]
	if !ok {
		src := loadAsset(name)
		sw, sh := float64(src.Rect.Dx()), float64(src.Rect.Dy())
		scale := math.Min(float64(w)/sw, float64(h)/sh)
		dw, dh := max(1, int(sw*scale+0.5)), max(1, int(sh*scale+0.5))
		scaled := resample(src, dw, dh)
		mask = image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(mask, scaled.Rect.Add(image.Pt((w-dw)/2, (h-dh)/2)), scaled, image.Point{}, draw.Src)
		keepAsset(key, mask)
	}
	assetMu.Unlock()
	c := color.NRGBAModel.Convert(tint).(color.NRGBA)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = c.R, c.G, c.B
		img.Pix[i+3] = uint8(uint32(mask.Pix[i+3]) * uint32(c.A) / 255)
	}
	return img
}

func keepAsset(key string, img *image.NRGBA) {
	if len(assetCache) > 128 {
		clear(assetCache)
	}
	assetCache[key] = img
}

// resample scales with a Catmull-Rom filter, widened when shrinking, on premultiplied colour.
func resample(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	pixels := make([][4]float64, sw*sh)
	for y := range sh {
		for x := range sw {
			c := src.NRGBAAt(src.Rect.Min.X+x, src.Rect.Min.Y+y)
			a := float64(c.A) / 255
			pixels[y*sw+x] = [4]float64{float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, float64(c.A)}
		}
	}
	across := scaleLines(pixels, sw, sh, dw, 1, sw, false)
	down := scaleLines(across, sh, dw, dh, dw, 1, true)
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	clamp := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v)) + 0.5) }
	for y := range dh {
		for x := range dw {
			p := down[y*dw+x]
			a := math.Max(0, math.Min(255, p[3]))
			if a <= 0 {
				continue
			}
			f := 255 / a
			out.SetNRGBA(x, y, color.NRGBA{clamp(p[0] * f), clamp(p[1] * f), clamp(p[2] * f), clamp(a)})
		}
	}
	return out
}

// scaleLines resizes every line of a grid along one axis. srcStep and lineStep walk the source
// along and across the lines; the result is packed the same way when vertical is false, and
// with dst lines running down the columns when it is true.
func scaleLines(from [][4]float64, length, lines, to, srcStep, lineStep int, vertical bool) [][4]float64 {
	result := make([][4]float64, to*lines)
	ratio := float64(length) / float64(to)
	support := 2 * math.Max(1, ratio)
	for i := range to {
		center := (float64(i)+0.5)*ratio - 0.5
		first, last := max(0, int(math.Ceil(center-support))), min(length-1, int(math.Floor(center+support)))
		weights := make([]float64, last-first+1)
		total := 0.0
		for j := first; j <= last; j++ {
			weights[j-first] = catmullRom((float64(j) - center) / math.Max(1, ratio))
			total += weights[j-first]
		}
		for line := range lines {
			var sum [4]float64
			for j := first; j <= last; j++ {
				w := weights[j-first] / total
				p := from[line*lineStep+j*srcStep]
				for k := range sum {
					sum[k] += p[k] * w
				}
			}
			if vertical {
				result[i*lines+line] = sum
			} else {
				result[line*to+i] = sum
			}
		}
	}
	return result
}

func catmullRom(x float64) float64 {
	x = math.Abs(x)
	switch {
	case x < 1:
		return 1.5*x*x*x - 2.5*x*x + 1
	case x < 2:
		return -0.5*x*x*x + 2.5*x*x - 4*x + 2
	}
	return 0
}
