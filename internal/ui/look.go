package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Colours of the page, in both variants, by name.
const (
	colorCard      fyne.ThemeColorName = "apCard"
	colorSecondary fyne.ThemeColorName = "apSecondary"
	colorDim       fyne.ThemeColorName = "apDim"
	colorDivider   fyne.ThemeColorName = "apDivider"
	colorGreen     fyne.ThemeColorName = "apGreen"
	colorBattery   fyne.ThemeColorName = "apBattery"
	colorRingTrack fyne.ThemeColorName = "apRingTrack"
	colorRed       fyne.ThemeColorName = "apRed"
	colorThumb     fyne.ThemeColorName = "apThumb"
	colorThumbRim  fyne.ThemeColorName = "apThumbRim"
	colorRegion    fyne.ThemeColorName = "apRegion"
	colorRail      fyne.ThemeColorName = "apRail"
	colorQuiet     fyne.ThemeColorName = "apQuiet"
	colorQuietRim  fyne.ThemeColorName = "apQuietRim"
	colorSwitchOff fyne.ThemeColorName = "apSwitchOff"
	colorKnobOff   fyne.ThemeColorName = "apKnobOff"
	colorKnob      fyne.ThemeColorName = "apKnob"
	colorArt       fyne.ThemeColorName = "apArt"
	colorTrack     fyne.ThemeColorName = "apSliderTrack"
)

// Text sizes of the page.
const (
	textTitle   float32 = 21
	textBody    float32 = 15
	textStatus  float32 = 12
	textLabel   float32 = 11
	textPercent float32 = 16.5
)

func hex(rgb uint32) color.NRGBA {
	return color.NRGBA{uint8(rgb >> 16), uint8(rgb >> 8), uint8(rgb), 0xff}
}

func hexa(rgb uint32, alpha uint8) color.NRGBA {
	c := hex(rgb)
	c.A = alpha
	return c
}

var darkColors = map[fyne.ThemeColorName]color.Color{
	theme.ColorNameBackground:          hex(0x000000),
	theme.ColorNameOverlayBackground:   hex(0x2c2c2e),
	theme.ColorNameMenuBackground:      hex(0x2c2c2e),
	theme.ColorNameHeaderBackground:    hex(0x1c1c1e),
	theme.ColorNameForeground:          hex(0xf5f5f7),
	theme.ColorNameForegroundOnPrimary: hex(0xffffff),
	theme.ColorNameForegroundOnError:   hex(0xffffff),
	theme.ColorNameForegroundOnSuccess: hex(0x000000),
	theme.ColorNameForegroundOnWarning: hex(0x000000),
	theme.ColorNameButton:              hex(0x2c2c2e),
	theme.ColorNameDisabledButton:      hex(0x232325),
	theme.ColorNameDisabled:            hex(0x98989d),
	theme.ColorNameInputBackground:     hex(0x2c2c2e),
	theme.ColorNameInputBorder:         hex(0x2c2c2e),
	theme.ColorNamePlaceHolder:         hex(0x8e8e93),
	theme.ColorNamePrimary:             hex(0x0a84ff),
	theme.ColorNameHyperlink:           hex(0x0a84ff),
	theme.ColorNameFocus:               hexa(0x0a84ff, 0x55),
	theme.ColorNameHover:               hex(0x3a3a3c),
	theme.ColorNamePressed:             hex(0x48484a),
	theme.ColorNameSelection:           hexa(0x0a84ff, 0x66),
	theme.ColorNameSeparator:           hex(0x38383a),
	theme.ColorNameScrollBar:           hexa(0x8e8e93, 0x99),
	theme.ColorNameScrollBarBackground: color.Transparent,
	theme.ColorNameShadow:              hexa(0x000000, 0x88),
	theme.ColorNameSuccess:             hex(0x30d158),
	theme.ColorNameWarning:             hex(0xffd60a),
	theme.ColorNameError:               hex(0xff453a),
	colorCard:                          hex(0x1c1c1e),
	colorSecondary:                     hex(0xa1a1a6),
	colorDim:                           hex(0x636366),
	colorDivider:                       hex(0x38383a),
	colorGreen:                         hex(0x30d158),
	colorBattery:                       hex(0x30d158),
	colorRingTrack:                     hex(0x48484a),
	colorRed:                           hex(0xff453a),
	colorThumb:                         hex(0x48484a),
	colorThumbRim:                      hex(0x7c7c80),
	colorRegion:                        hex(0x3a3a3c),
	colorRail:                          hex(0x1c1c1e),
	colorQuiet:                         hex(0x2c2c2e),
	colorQuietRim:                      hex(0x3a3a3c),
	colorSwitchOff:                     hex(0x98989d),
	colorKnobOff:                       hex(0xc7c7cc),
	colorKnob:                          hex(0xffffff),
	colorArt:                           hex(0xfafafc),
	colorTrack:                         hex(0x8e8e93),
}

var lightColors = map[fyne.ThemeColorName]color.Color{
	theme.ColorNameBackground:          hex(0xf2f2f7),
	theme.ColorNameOverlayBackground:   hex(0xffffff),
	theme.ColorNameMenuBackground:      hex(0xffffff),
	theme.ColorNameHeaderBackground:    hex(0xffffff),
	theme.ColorNameForeground:          hex(0x1c1c1e),
	theme.ColorNameForegroundOnPrimary: hex(0xffffff),
	theme.ColorNameForegroundOnError:   hex(0xffffff),
	theme.ColorNameForegroundOnSuccess: hex(0xffffff),
	theme.ColorNameForegroundOnWarning: hex(0x000000),
	theme.ColorNameButton:              hex(0xeeeef2),
	theme.ColorNameDisabledButton:      hex(0xf2f2f7),
	theme.ColorNameDisabled:            hex(0x636366),
	theme.ColorNameInputBackground:     hex(0xf2f2f7),
	theme.ColorNameInputBorder:         hex(0xf2f2f7),
	theme.ColorNamePlaceHolder:         hex(0x8e8e93),
	theme.ColorNamePrimary:             hex(0x0066d6),
	theme.ColorNameHyperlink:           hex(0x0066d6),
	theme.ColorNameFocus:               hexa(0x0066d6, 0x44),
	theme.ColorNameHover:               hex(0xe7e7eb),
	theme.ColorNamePressed:             hex(0xd1d1d6),
	theme.ColorNameSelection:           hexa(0x0066d6, 0x44),
	theme.ColorNameSeparator:           hex(0xd1d1d6),
	theme.ColorNameScrollBar:           hexa(0x636366, 0x88),
	theme.ColorNameScrollBarBackground: color.Transparent,
	theme.ColorNameShadow:              hexa(0x000000, 0x33),
	theme.ColorNameSuccess:             hex(0x248a3d),
	theme.ColorNameWarning:             hex(0xb25000),
	theme.ColorNameError:               hex(0xd70015),
	colorCard:                          hex(0xffffff),
	colorSecondary:                     hex(0x636366),
	colorDim:                           hex(0xaeaeb2),
	colorDivider:                       hex(0xd1d1d6),
	colorGreen:                         hex(0x34c759),
	colorBattery:                       hex(0x248a3d),
	colorRingTrack:                     hex(0xd1d1d6),
	colorRed:                           hex(0xd70015),
	colorThumb:                         hex(0xffffff),
	colorThumbRim:                      hex(0xffffff),
	colorRegion:                        hex(0xeeeef2),
	colorRail:                          hex(0xe3e3e8),
	colorQuiet:                         hex(0xffffff),
	colorQuietRim:                      hex(0xd1d1d6),
	colorSwitchOff:                     hex(0x8e8e93),
	colorKnobOff:                       hex(0x636366),
	colorKnob:                          hex(0xffffff),
	colorArt:                           hex(0xffffff),
	colorTrack:                         hex(0x8e8e93),
}

// pageTheme is the app's look: black or light grey page, rounded cards, roomy rows.
type pageTheme struct {
	fyne.Theme
	forced  bool
	variant fyne.ThemeVariant
}

// Color answers from the page palette, then the default theme.
func (t pageTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if t.forced {
		variant = t.variant
	}
	colors := darkColors
	if variant == theme.VariantLight {
		colors = lightColors
	}
	if c, ok := colors[name]; ok {
		return c
	}
	return t.Theme.Color(name, variant)
}

// Size removes the default form feel: larger radii, thin separators and scrollbars.
func (t pageTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return textBody
	case theme.SizeNameCaptionText:
		return 12.5
	case theme.SizeNameHeadingText:
		return textTitle
	case theme.SizeNameSubHeadingText:
		return textPercent
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius:
		return 10
	case theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameMenuRadius, theme.SizeNamePopupRadius, theme.SizeNameCardRadius, theme.SizeNameDialogRadius:
		return 14
	case theme.SizeNameInputBorder, theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNameScrollBar:
		return 6
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameScrollBarRadius:
		return 3
	}
	return t.Theme.Size(name)
}

// chevronDownSVG is a filled outline: Fyne recolours fills of themed resources, not strokes.
const chevronDownSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="M6.7 9.3a1 1 0 0 1 1.4 0l3.9 3.9 3.9-3.9a1 1 0 1 1 1.4 1.4l-4.6 4.6a1 1 0 0 1-1.4 0l-4.6-4.6a1 1 0 0 1 0-1.4z"/></svg>`

var chevronDownIcon = func() fyne.Resource {
	icon := theme.NewThemedResource(fyne.NewStaticResource("chevron-down.svg", []byte(chevronDownSVG)))
	icon.ColorName = colorSecondary
	return icon
}()

// Icon swaps the triangle of dropdowns for a quiet chevron.
func (t pageTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	if name == theme.IconNameArrowDropDown {
		return chevronDownIcon
	}
	return t.Theme.Icon(name)
}

func look(name fyne.ThemeColorName) color.Color {
	settings := fyne.CurrentApp().Settings()
	if c := settings.Theme().Color(name, settings.ThemeVariant()); c != nil {
		return c
	}
	// A theme that does not know the page's names, such as the test theme.
	colors := darkColors
	if settings.ThemeVariant() == theme.VariantLight {
		colors = lightColors
	}
	if c, ok := colors[name]; ok {
		return c
	}
	return color.Transparent
}

func isLight() bool {
	return look(colorArt) == lightColors[colorArt] && look(theme.ColorNameBackground) == lightColors[theme.ColorNameBackground]
}

// tunedTheme changes a few values under one control and follows the app theme for the rest.
type tunedTheme struct {
	colors map[fyne.ThemeColorName]fyne.ThemeColorName
	sizes  map[fyne.ThemeSizeName]float32
}

func (t tunedTheme) base() fyne.Theme { return fyne.CurrentApp().Settings().Theme() }

func (t tunedTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if mapped, ok := t.colors[name]; ok {
		name = mapped
	}
	return t.base().Color(name, variant)
}

func (t tunedTheme) Font(style fyne.TextStyle) fyne.Resource    { return t.base().Font(style) }
func (t tunedTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return t.base().Icon(name) }

func (t tunedTheme) Size(name fyne.ThemeSizeName) float32 {
	if size, ok := t.sizes[name]; ok {
		return size
	}
	return t.base().Size(name)
}

func fade(c color.Color, alpha float64) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = uint8(float64(n.A) * alpha)
	return n
}

// surface is a rounded panel behind some content.
type surface struct {
	widget.BaseWidget
	content fyne.CanvasObject
	fill    fyne.ThemeColorName
	radius  float32
	padH    float32
	padV    float32
}

func newSurface(content fyne.CanvasObject, fill fyne.ThemeColorName, radius, padH, padV float32) *surface {
	s := &surface{content: content, fill: fill, radius: radius, padH: padH, padV: padV}
	s.ExtendBaseWidget(s)
	return s
}

func (s *surface) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(look(s.fill))
	rect.CornerRadius = s.radius
	return &surfaceRenderer{s: s, rect: rect}
}

type surfaceRenderer struct {
	s    *surface
	rect *canvas.Rectangle
}

func (r *surfaceRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
	r.s.content.Move(fyne.NewPos(r.s.padH, r.s.padV))
	r.s.content.Resize(fyne.NewSize(size.Width-2*r.s.padH, size.Height-2*r.s.padV))
}

func (r *surfaceRenderer) MinSize() fyne.Size {
	return r.s.content.MinSize().Add(fyne.NewSize(2*r.s.padH, 2*r.s.padV))
}

func (r *surfaceRenderer) Refresh() {
	r.rect.FillColor = look(r.s.fill)
	r.rect.Refresh()
	r.s.content.Refresh()
}

func (r *surfaceRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect, r.s.content}
}
func (r *surfaceRenderer) Destroy() {}

// text is one line of themed text at a chosen size.
type text struct {
	widget.BaseWidget
	Text     string
	TextSize float32
	Bold     bool
	Fill     fyne.ThemeColorName
	Align    fyne.TextAlign
}

func newText(value string, size float32, bold bool, fill fyne.ThemeColorName, align fyne.TextAlign) *text {
	t := &text{Text: value, TextSize: size, Bold: bold, Fill: fill, Align: align}
	t.ExtendBaseWidget(t)
	return t
}

// SetText changes the line.
func (t *text) SetText(value string) {
	if t.Text != value {
		t.Text = value
		t.Refresh()
	}
}

func (t *text) CreateRenderer() fyne.WidgetRenderer {
	line := canvas.NewText(t.Text, look(t.Fill))
	r := &textRenderer{t: t, line: line}
	r.Refresh()
	return r
}

type textRenderer struct {
	t    *text
	line *canvas.Text
}

func (r *textRenderer) Layout(size fyne.Size) { r.line.Resize(size); r.line.Move(fyne.NewPos(0, 0)) }
func (r *textRenderer) MinSize() fyne.Size    { return r.line.MinSize() }
func (r *textRenderer) Refresh() {
	r.line.Text = r.t.Text
	r.line.TextSize = r.t.TextSize
	r.line.TextStyle = fyne.TextStyle{Bold: r.t.Bold}
	r.line.Alignment = r.t.Align
	r.line.Color = look(r.t.Fill)
	r.line.Refresh()
}
func (r *textRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.line} }
func (r *textRenderer) Destroy()                     {}
