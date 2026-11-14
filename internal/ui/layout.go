package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	pageMaxWidth float32 = 640
	pageMargin   float32 = 24
	cardRadius   float32 = 24
	cardPadH     float32 = 20
	cardPadV     float32 = 0
	rowHeight    float32 = 64
	rowGap       float32 = 16
	dividerInset float32 = 16
	sectionGap   float32 = 26 // between the sections of the page
)

// columnLayout centres one child at a bounded width, so wide windows keep the page shape.
type columnLayout struct{}

func (columnLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	width := min(pageMaxWidth, size.Width-2*pageMargin)
	for _, object := range objects {
		object.Move(fyne.NewPos((size.Width-width)/2, 0))
		object.Resize(fyne.NewSize(width, size.Height))
	}
}

func (columnLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for _, object := range objects {
		out = out.Max(object.MinSize())
	}
	return out.Add(fyne.NewSize(2*pageMargin, 0))
}

// rowLayout puts main on the left and control on the right, centred vertically.
type rowLayout struct{}

func (rowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	main, control := objects[0], objects[1]
	cw := control.MinSize()
	control.Resize(cw)
	control.Move(fyne.NewPos(size.Width-cw.Width, (size.Height-cw.Height)/2))
	mw := max(0, size.Width-cw.Width-rowGap)
	mh := main.MinSize().Height
	main.Resize(fyne.NewSize(mw, mh))
	main.Move(fyne.NewPos(0, (size.Height-mh)/2))
}

func (rowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	main, control := objects[0].MinSize(), objects[1].MinSize()
	return fyne.NewSize(main.Width+control.Width+rowGap, max(rowHeight, max(main.Height, control.Height)+rowPadV*2))
}

// rowPadV keeps a tall row, such as one with a hint, off its dividers.
const rowPadV float32 = 12

// fixedWidth gives a control a stable width in a row.
type fixedWidth struct{ width float32 }

func (l fixedWidth) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Resize(size)
		object.Move(fyne.NewPos(0, 0))
	}
}

func (l fixedWidth) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for _, object := range objects {
		out = out.Max(object.MinSize())
	}
	out.Width = max(out.Width, l.width)
	return out
}

func sized(width float32, object fyne.CanvasObject) fyne.CanvasObject {
	return container.New(fixedWidth{width}, object)
}

// insetLayout pads its children; negative values let them reach past the box.
type insetLayout struct{ left, top, right, bottom float32 }

func (l insetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(l.left, l.top))
		object.Resize(fyne.NewSize(size.Width-l.left-l.right, size.Height-l.top-l.bottom))
	}
}

func (l insetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for _, object := range objects {
		if object.Visible() {
			out = out.Max(object.MinSize())
		}
	}
	return out.Add(fyne.NewSize(l.left+l.right, l.top+l.bottom))
}

// caption is the small grey title above a card, inset to line up with the card's rows.
func caption(value string) fyne.CanvasObject {
	return container.New(insetLayout{left: dividerInset, top: 4, bottom: 9},
		newText(value, 13, true, colorSecondary, fyne.TextAlignLeading))
}

// card is a rounded group on the page.
func card(content fyne.CanvasObject) *surface {
	return newSurface(content, colorCard, cardRadius, cardPadH, cardPadV)
}

// divider is a thin line that reaches slightly past the row text, inside a card.
func divider() fyne.CanvasObject {
	reach := cardPadH - dividerInset
	return container.New(insetLayout{left: -reach, right: -reach, bottom: 0}, widget.NewSeparator())
}

// holder is a container that is a widget, so a struct built on it stays a real canvas
// object: drivers do not look inside a plain struct that merely embeds *fyne.Container.
type holder struct {
	widget.BaseWidget
	Container *fyne.Container
}

// CreateRenderer shows the container.
func (h *holder) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(h.Container) }

// Refresh passes through to the container and what it holds.
func (h *holder) Refresh() {
	h.Container.Refresh()
	h.BaseWidget.Refresh()
}

// stack lists rows with dividers between the visible ones.
type stack struct {
	holder
	rows     []fyne.CanvasObject
	dividers []fyne.CanvasObject
}

func newStack(rows ...fyne.CanvasObject) *stack {
	s := &stack{holder: holder{Container: container.New(tightLayout{})}}
	s.ExtendBaseWidget(s)
	for _, row := range rows {
		s.add(row)
	}
	return s
}

func (s *stack) add(row fyne.CanvasObject) {
	line := divider()
	s.rows = append(s.rows, row)
	s.dividers = append(s.dividers, line)
	s.Container.Add(line)
	s.Container.Add(row)
	s.tidy()
}

// tidy shows a divider only between two visible rows.
func (s *stack) tidy() {
	seen := false
	for i, row := range s.rows {
		if row.Visible() {
			if seen {
				s.dividers[i].Show()
			} else {
				s.dividers[i].Hide()
			}
			seen = true
		} else {
			s.dividers[i].Hide()
		}
	}
	s.Container.Refresh()
}

// anyVisible reports whether a row is showing.
func (s *stack) anyVisible() bool {
	for _, row := range s.rows {
		if row.Visible() {
			return true
		}
	}
	return false
}

// labelled is a row title with an optional hint below it.
func labelled(title string, hint string) (*widget.Label, *widget.Label, fyne.CanvasObject) {
	label := widget.NewLabel(title)
	label.Wrapping = fyne.TextWrapWord
	objects := []fyne.CanvasObject{label}
	var small *widget.Label
	if hint != "" {
		small = smallLabel(hint)
		objects = append(objects, small)
	}
	return label, small, tight(objects...)
}

// smallLabel is grey secondary text. Low importance reads as the theme's disabled colour,
// which the page keeps readable for this purpose.
func smallLabel(value string) *widget.Label {
	label := widget.NewLabel(value)
	label.Wrapping = fyne.TextWrapWord
	label.SizeName = theme.SizeNameCaptionText
	label.Importance = widget.LowImportance
	return label
}

// tightLayout stacks children flush, with an optional gap. Labels carry an inner padding
// on every side; with labels set they are pulled out by it, so their text lines up with
// plain text and a title and its hint read as one block.
type tightLayout struct {
	gap    float32
	labels bool
}

func (l tightLayout) pull() float32 {
	if l.labels {
		return theme.InnerPadding()
	}
	return 0
}

func (l tightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	pull := l.pull()
	y := -pull
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		h := object.MinSize().Height
		object.Resize(fyne.NewSize(size.Width+2*pull, h))
		object.Move(fyne.NewPos(-pull, y))
		y += h + l.gap - 2*pull
	}
}

func (l tightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	pull := l.pull()
	var out fyne.Size
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		size := object.MinSize()
		out.Width = max(out.Width, size.Width-2*pull)
		out.Height += size.Height - 2*pull + l.gap
	}
	if out.Height > 0 {
		out.Height -= l.gap
	}
	return out
}

func tight(objects ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(tightLayout{gap: 3, labels: true}, objects...)
}

// section is a card that opens and closes from its header row.
type section struct {
	widget.BaseWidget
	Title    *text
	Subtitle *text
	body     fyne.CanvasObject
	chevron  *canvas.Raster
	open     bool
	OnToggle func(bool)
	header   *tappable
	content  *fyne.Container
	shell    *surface
	well     fyne.CanvasObject
}

func newSection(title, subtitle string, body fyne.CanvasObject) *section {
	s := &section{body: body}
	s.Title = newText(title, textBody, false, theme.ColorNameForeground, fyne.TextAlignLeading)
	s.Subtitle = newText(subtitle, textStatus, false, colorSecondary, fyne.TextAlignLeading)
	s.chevron = canvas.NewRaster(func(w, h int) imageNRGBA { return chevronImage(w, h, s.open) })
	s.chevron.SetMinSize(fyne.NewSquareSize(18))
	titles := container.New(tightLayout{gap: 3}, s.Title)
	if subtitle != "" {
		titles.Add(s.Subtitle)
	}
	row := container.New(rowLayout{}, titles, s.chevron)
	s.header = newTappable(container.New(insetLayout{left: cardPadH, right: cardPadH}, row), s.Toggle)
	s.well = container.New(insetLayout{left: cardPadH, right: cardPadH, top: 0, bottom: 8}, body)
	s.well.Hide()
	s.content = container.New(tightLayout{}, s.header, s.well)
	s.shell = newSurface(s.content, colorCard, cardRadius, 0, 0)
	s.ExtendBaseWidget(s)
	return s
}

// Open reports whether the section is showing its body.
func (s *section) Open() bool { return s.open }

// Toggle opens or closes the section.
func (s *section) Toggle() {
	s.open = !s.open
	if s.open {
		s.well.Show()
	} else {
		s.well.Hide()
	}
	s.chevron.Refresh()
	s.content.Refresh()
	s.Refresh()
	if s.OnToggle != nil {
		s.OnToggle(s.open)
	}
}

// CreateRenderer shows the card.
func (s *section) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(s.shell) }

// tappable makes any content clickable.
type tappable struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
}

func newTappable(content fyne.CanvasObject, onTap func()) *tappable {
	t := &tappable{content: content, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

// Tapped runs the action.
func (t *tappable) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// Cursor marks it clickable.
func (t *tappable) Cursor() desktop.Cursor { return desktop.PointerCursor }

// CreateRenderer shows the content.
func (t *tappable) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(t.content) }
