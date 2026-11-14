package ui

import (
	"image"
	"image/color"
	"math"
)

// sdf is a signed distance field in unit coordinates, negative inside.
type sdf func(x, y float64) float64

func circleSDF(cx, cy, r float64) sdf {
	return func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - r }
}

func capsuleSDF(ax, ay, bx, by, r float64) sdf {
	return func(x, y float64) float64 {
		dx, dy := bx-ax, by-ay
		t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
		return math.Hypot(x-ax-t*dx, y-ay-t*dy) - r
	}
}

func roundedBoxSDF(cx, cy, hw, hh, r float64) sdf {
	return func(x, y float64) float64 {
		qx, qy := math.Abs(x-cx)-hw+r, math.Abs(y-cy)-hh+r
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	}
}

func union(fields ...sdf) sdf {
	return func(x, y float64) float64 {
		out := math.Inf(1)
		for _, f := range fields {
			out = math.Min(out, f(x, y))
		}
		return out
	}
}

func subtract(a, b sdf) sdf {
	return func(x, y float64) float64 { return math.Max(a(x, y), -b(x, y)) }
}

func ringSDF(cx, cy, r, width float64) sdf {
	return func(x, y float64) float64 { return math.Abs(math.Hypot(x-cx, y-cy)-r) - width/2 }
}

// arcSDF is a round-capped arc running clockwise from the top of a circle.
func arcSDF(cx, cy, r, width, sweep float64) sdf {
	endSin, endCos := math.Sincos(sweep)
	ex, ey := cx+r*endSin, cy-r*endCos
	return func(x, y float64) float64 {
		angle := math.Atan2(x-cx, cy-y)
		if angle < 0 {
			angle += 2 * math.Pi
		}
		if angle <= sweep {
			return math.Abs(math.Hypot(x-cx, y-cy)-r) - width/2
		}
		return math.Min(math.Hypot(x-cx, y-(cy-r)), math.Hypot(x-ex, y-ey)) - width/2
	}
}

// paintField draws fill wherever the field is negative. Edge pixels are supersampled, so
// edges stay one pixel soft at any scale and for fields that are not exact distances.
func paintField(img *image.NRGBA, field sdf, fill func(x, y float64) color.NRGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	side := float64(min(w, h))
	ox, oy := (float64(w)-side)/2, (float64(h)-side)/2
	band := 2.5 / side
	const grid = 4
	for py := range h {
		for px := range w {
			x, y := (float64(px)+0.5-ox)/side, (float64(py)+0.5-oy)/side
			d := field(x, y)
			if d > band {
				continue
			}
			coverage := 1.0
			if d > -band {
				inside := 0
				for sy := range grid {
					for sx := range grid {
						ux := (float64(px) + (float64(sx)+0.5)/grid - ox) / side
						uy := (float64(py) + (float64(sy)+0.5)/grid - oy) / side
						if field(ux, uy) < 0 {
							inside++
						}
					}
				}
				if inside == 0 {
					continue
				}
				coverage = float64(inside) / (grid * grid)
			}
			blendOver(img, px, py, fill(x, y), coverage)
		}
	}
}

func blendOver(img *image.NRGBA, px, py int, c color.NRGBA, coverage float64) {
	existing := img.NRGBAAt(px, py)
	alpha := float64(c.A) / 255 * coverage
	back := float64(existing.A) / 255
	total := alpha + back*(1-alpha)
	if total <= 0 {
		return
	}
	mix := func(f, b uint8) uint8 {
		return uint8((float64(f)*alpha+float64(b)*back*(1-alpha))/total + 0.5)
	}
	img.SetNRGBA(px, py, color.NRGBA{mix(c.R, existing.R), mix(c.G, existing.G), mix(c.B, existing.B), uint8(total*255 + 0.5)})
}

func solid(c color.Color) func(x, y float64) color.NRGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return func(float64, float64) color.NRGBA { return n }
}

// ringImage is the battery gauge: a track with a clockwise arc from the top. A level
// below zero is unknown and draws as a dimmed broken ring.
func ringImage(w, h int, level int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	const radius, width = 0.45, 0.085
	if level < 0 {
		const dashes = 16.0
		broken := func(x, y float64) float64 {
			angle := math.Atan2(x-0.5, 0.5-y)
			if angle < 0 {
				angle += 2 * math.Pi
			}
			along := math.Mod(angle/(2*math.Pi)*dashes, 1)
			gap := (math.Abs(along-0.5) - 0.27) * 2 * math.Pi * radius / dashes
			return math.Max(math.Abs(math.Hypot(x-0.5, y-0.5)-radius)-width/2, gap)
		}
		paintField(img, broken, solid(look(colorRingTrack)))
		return img
	}
	level = min(level, 100)
	paintField(img, ringSDF(0.5, 0.5, radius, width), solid(look(colorRingTrack)))
	if level == 0 {
		return img
	}
	fill := look(colorBattery)
	if level <= 20 {
		fill = look(colorRed)
	}
	if level == 100 {
		paintField(img, ringSDF(0.5, 0.5, radius, width), solid(fill))
		return img
	}
	paintField(img, arcSDF(0.5, 0.5, radius, width, float64(level)/100*2*math.Pi), solid(fill))
	return img
}

func chevronImage(w, h int, open bool) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	top, bottom := 0.38, 0.62
	if open {
		top, bottom = bottom, top
	}
	field := union(capsuleSDF(0.24, top, 0.5, bottom, 0.038), capsuleSDF(0.5, bottom, 0.76, top, 0.038))
	paintField(img, field, solid(look(colorSecondary)))
	return img
}

// Icons of the page. The first four are listening modes.
const (
	iconOff = iota
	iconTransparency
	iconAdaptive
	iconCancellation
	iconSpeaker
	iconDots
	iconStrengthLow
	iconStrengthHigh
)

func iconImage(kind, w, h int, tint color.Color) image.Image {
	if name, ok := iconFiles[kind]; ok {
		return assetIcon(name, w, h, tint)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	fill := solid(tint)
	switch kind {
	case iconOff:
		paintField(img, ringSDF(0.5, 0.5, 0.30, 0.058), fill)
		paintField(img, capsuleSDF(0.27, 0.73, 0.73, 0.27, 0.030), fill)
	case iconSpeaker:
		cone := func(x, y float64) float64 {
			box := roundedBoxSDF(0.20, 0.5, 0.075, 0.12, 0.02)(x, y)
			wedge := math.Max(math.Abs(y-0.5)-(0.12+(x-0.26)*1.05), math.Max(0.255-x, x-0.47))
			return math.Min(box, wedge)
		}
		paintField(img, subtract(cone, roundedBoxSDF(0.25, 0.5, 0.001, 0.2, 0.001)), fill)
		arc := func(r float64) sdf {
			return func(x, y float64) float64 {
				return math.Max(math.Abs(math.Hypot(x-0.46, y-0.5)-r)-0.024, 0.46+math.Abs(y-0.5)*1.05-x)
			}
		}
		paintField(img, arc(0.14), fill)
		paintField(img, arc(0.26), fill)
		paintField(img, arc(0.38), fill)
	case iconDots:
		for _, x := range []float64{0.26, 0.5, 0.74} {
			paintField(img, circleSDF(x, 0.5, 0.062), fill)
		}
	case iconStrengthLow:
		for i := range 8 {
			angle := float64(i) * math.Pi / 4
			paintField(img, circleSDF(0.5+0.36*math.Cos(angle), 0.5+0.36*math.Sin(angle), 0.06), fill)
		}
	case iconStrengthHigh:
		paintField(img, ringSDF(0.5, 0.5, 0.36, 0.085), fill)
	}
	return img
}

// thumbImage is the raised selection of the segmented control: a soft shadow below a fill
// with a fine rim that is brighter at the top. margin is the shadow room around the
// capsule, in the same units as width and height of the capsule (w and h include it).
func thumbImage(w, h int, margin float64, adaptive bool) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return img
	}
	fw, fh := float64(w), float64(h)
	m := margin
	radius := (fh - 2*m) / 2
	cap := func(x, y, inset float64) float64 {
		qx := math.Abs(x-fw/2) - (fw/2 - m - radius)
		qy := y - fh/2
		return math.Hypot(math.Max(qx, 0), qy) + inset - radius
	}
	light := isLight()
	shadowAlpha := 0.45
	if light {
		shadowAlpha = 0.20
	}
	fill := color.NRGBAModel.Convert(look(colorThumb)).(color.NRGBA)
	rim := color.NRGBAModel.Convert(look(colorThumbRim)).(color.NRGBA)
	if adaptive && !light {
		fill = hex(0x303233)
	}
	scale := fh / 64
	for py := range h {
		for px := range w {
			x, y := float64(px)+0.5, float64(py)+0.5
			if shadow := cap(x, y-3*scale, 0); shadow < m {
				t := math.Max(0, math.Min(1, 1-(shadow+m*0.2)/(m*1.2)))
				blendOver(img, px, py, color.NRGBA{0, 0, 0, 0xff}, shadowAlpha*t*t)
			}
			d := cap(x, y, 0)
			if d < 0.5 {
				coverage := math.Max(0, math.Min(1, 0.5-d))
				blendOver(img, px, py, fill, coverage)
				band := math.Max(1, 1.1*scale)
				if d > -band-0.5 {
					ring := math.Max(0, math.Min(1, d+band+0.5)) * coverage
					vertical := 1 - (y-m)/(fh-2*m)
					strength := 0.25 + 0.75*vertical*vertical
					blendOver(img, px, py, rim, ring*strength)
				}
			}
		}
	}
	return img
}

// regionImage is the wider lane behind the thumb in Adaptive: a soft horizontal gradient.
func regionImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return img
	}
	fw, fh := float64(w), float64(h)
	radius := fh / 2
	base := color.NRGBAModel.Convert(look(colorRegion)).(color.NRGBA)
	for py := range h {
		for px := range w {
			x, y := float64(px)+0.5, float64(py)+0.5
			qx := math.Abs(x-fw/2) - (fw/2 - radius)
			d := math.Hypot(math.Max(qx, 0), y-fh/2) - radius
			if d < 0.5 {
				c := base
				spread := math.Abs(x-fw/2) / (fw / 2)
				scale := 1 - 0.14*(1-spread*spread)
				if !isLightColor(base) {
					c.R, c.G, c.B = uint8(float64(c.R)*scale), uint8(float64(c.G)*scale), uint8(float64(c.B)*scale)
				}
				blendOver(img, px, py, c, math.Max(0, math.Min(1, 0.5-d)))
			}
		}
	}
	return img
}

func isLightColor(c color.NRGBA) bool { return int(c.R)+int(c.G)+int(c.B) > 3*160 }
