package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// sentText is the note of a value that was sent and never reported back. There is none:
// the row shows the value as chosen, and the user could do nothing with the caveat.
func sentText() string { return "" }

func adaptiveOnlyText() string { return i18n.T("settings.adaptiveOnly") }

func notReportedHint() string { return i18n.T("value.notReported") }

// arrivingHint is the line of a row with no report while the reports are still arriving.
func arrivingHint() string { return i18n.T("value.arriving") }

// translated is the text for a key, or a fallback for a value nobody has named.
func translated(key, fallback string) string {
	if i18n.Has(key) {
		return i18n.T(key)
	}
	return fallback
}

// holdActionWords are the words the hold-actions setting encodes for each ear.
var holdActionWords = []string{"noise-control", "siri"}

func titleCase(word string) string {
	spaced := strings.ReplaceAll(word, "-", " ")
	if spaced == "" {
		return spaced
	}
	return strings.ToUpper(spaced[:1]) + spaced[1:]
}

func settingLabel(name string) string { return translated("setting."+name, titleCase(name)) }

// wordLabel names a value. The listening modes share the tray's names.
func wordLabel(word string) string {
	if i18n.Has("mode." + word) {
		return i18n.T("mode." + word)
	}
	return translated("word."+word, titleCase(word))
}

// chipLabel names a listening mode on a chip, where four share one row.
func chipLabel(word string) string { return translated("chip."+word, wordLabel(word)) }

// settingHelp explains the rows whose effect isn't obvious from the name; empty for the rest.
func settingHelp(name string) string { return translated("help."+name, "") }

// silentControl lists settings the AirPods can apply without echoing them, on the
// reported model. For these, no report is normal and does not cast doubt on the write.
func silentControl(control byte, number string) bool {
	switch control {
	case 0x28, 0x2e:
		return true
	case model.ControlAllowOff:
		return model.SilentAllowOff(number)
	}
	return false
}

// settingRow is one setting's widgets. Exactly one of the control fields is set, by kind.
type settingRow struct {
	setting *dongle.Setting
	label   *widget.Label
	// help and note share the hint slot under the title; a note replaces the help.
	help    *widget.Label
	note    *widget.Label
	body    fyne.CanvasObject
	refit   func(model.View) // applies the model's options and labels
	control fyne.CanvasObject
	show    func(decoded string, known bool) // updates the widget without writing
	enable  func(bool)
	// relabel puts the current language on the control's own values; nil when it has none.
	relabel func()

	rowValue

	toggle  *Switch
	dash    *dashPill
	choice  *widget.Select
	segment *Segmented
	slider  *Slider
	dimmed  bool
	number  *valueText
	chips   []*chip
	actions [2]*widget.Select
}

func isToggle(s *dongle.Setting) bool {
	if s.Kind != dongle.KindNamed || len(s.Values) != 2 {
		return false
	}
	words := s.Values[0].Word + s.Values[1].Word
	return words == "onoff" || words == "offon"
}

func (t *settingsTab) newRow(s *dongle.Setting) *settingRow {
	row := &settingRow{setting: s, note: smallLabel(""), label: widget.NewLabel(settingLabel(s.Name))}
	row.label.Wrapping = fyne.TextWrapWord
	row.note.Hide()
	if text := settingHelp(s.Name); text != "" {
		row.help = smallLabel(text)
	}
	switch {
	case s.Control == model.ControlMode:
		t.buildSegmented(row)
	case isToggle(s):
		t.buildToggle(row)
	case s.Kind == dongle.KindNamed:
		t.buildSelect(row)
	case s.Kind == dongle.KindPercent:
		t.buildSlider(row)
	case s.Kind == dongle.KindModes:
		t.buildModes(row)
	default:
		t.buildActions(row)
	}
	enable := row.enable
	row.enable = func(on bool) {
		enable(on)
		setDim(row.label, !on)
	}
	return row
}

// buildToggle stacks the switch and the dash shown in its place while the value is
// unknown. A tap on the dash chooses on; the row then shows a normal switch.
func (t *settingsTab) buildToggle(row *settingRow) {
	row.toggle = NewSwitch(func(on bool) {
		if t.applying {
			return
		}
		word := "off"
		if on {
			word = "on"
		}
		t.write(row, word)
	})
	row.dash = newDashPill(func() { t.write(row, "on") })
	row.control = container.NewStack(row.toggle, row.dash)
	row.show = func(decoded string, known bool) {
		row.toggle.SetChecked(known && decoded == "on")
		showIf(row.toggle, known)
		showIf(row.dash, !known)
	}
	row.enable = func(on bool) {
		enableWidget(row.toggle, on)
		enableWidget(row.dash, on)
	}
	row.show("", false)
}

func enableWidget(item fyne.Disableable, on bool) {
	if on {
		item.Enable()
	} else {
		item.Disable()
	}
}

func (t *settingsTab) buildSelect(row *settingRow) {
	words := map[string]string{}
	labelsNow := func() []string {
		labels := make([]string, len(row.setting.Values))
		clear(words)
		for i, value := range row.setting.Values {
			labels[i] = wordLabel(value.Word)
			words[labels[i]] = value.Word
		}
		return labels
	}
	row.choice = widget.NewSelect(labelsNow(), func(label string) {
		if !t.applying {
			t.write(row, words[label])
		}
	})
	row.choice.PlaceHolder = dashText
	row.control = compactSelect(row.choice)
	row.show = func(decoded string, known bool) {
		if known {
			row.choice.SetSelected(wordLabel(decoded))
		} else {
			row.choice.ClearSelected()
		}
	}
	row.enable = func(on bool) { enableWidget(row.choice, on) }
	row.relabel = func() {
		row.choice.SetOptions(labelsNow())
		row.choice.Selected = ""
	}
}

// modeOrder is the left to right order of the pill.
var modeOrder = []string{"off", "transparency", "adaptive", "anc"}

// wordIcons is the icon of each listening mode, by its word.
var wordIcons = map[string]int{
	"off": iconOff, "transparency": iconTransparency, "adaptive": iconAdaptive, "anc": iconCancellation,
}

// modeIcons maps the pill's labels, in the current language, to their icons.
func modeIcons() map[string]int {
	icons := make(map[string]int, len(wordIcons))
	for word, icon := range wordIcons {
		icons[wordLabel(word)] = icon
	}
	return icons
}

func (t *settingsTab) buildSegmented(row *settingRow) {
	words := map[string]string{}
	values := map[string]byte{}
	fillWords := func() {
		clear(words)
		for _, value := range row.setting.Values {
			words[wordLabel(value.Word)] = value.Word
			values[value.Word] = byte(value.Value)
		}
	}
	fillWords()
	options := func(view model.View) []string {
		var out []string
		for _, word := range modeOrder {
			value := values[word]
			if (value == 1 && view.Identified() && !view.OffAvailable()) || (value == 4 && view.Identified() && !view.Adaptive()) {
				continue
			}
			out = append(out, wordLabel(word))
		}
		return out
	}
	row.segment = NewSegmented(options(model.View{}), modeIcons(), func(label string) {
		if !t.applying {
			t.write(row, words[label])
		}
	})
	row.control = row.segment
	row.refit = func(view model.View) {
		next := options(view)
		if !sameStrings(next, row.segment.Options) {
			row.segment.SetOptions(next)
		}
	}
	row.show = func(decoded string, known bool) {
		if known {
			row.segment.SetSelected(wordLabel(decoded))
		} else {
			row.segment.ClearSelected()
		}
	}
	row.enable = func(on bool) { enableWidget(row.segment, on) }
	row.relabel = func() {
		fillWords()
		row.segment.Icons = modeIcons()
		row.segment.Selected = ""
		row.segment.SetOptions(options(t.w.view()))
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (t *settingsTab) buildSlider(row *settingRow) {
	row.number = newValueText("")
	row.slider = NewSlider(0, 100)
	row.slider.Step = 1
	row.slider.OnChanged = func(value float64) { row.number.SetValue(strconv.Itoa(int(value)) + "%") }
	row.slider.OnChangeEnded = func(value float64) {
		if !t.applying {
			t.write(row, strconv.Itoa(int(value)))
		}
	}
	noTile := gap(0)
	noTile.Hide()
	row.dimmed = true
	row.slider.SetUnknown(true)
	dimThumb := func(unknown bool) {
		row.dimmed = unknown
		row.slider.SetUnknown(unknown)
	}
	// The kit's line is tall for a row of its own; here the row sets the height.
	line := container.New(sliderLineLayout{}, noTile, styledSlider(row.slider), row.number)
	row.control = sized(sliderWidth, container.New(insetLayout{top: -rowPadV, bottom: -rowPadV}, line))
	row.show = func(decoded string, known bool) {
		// The drag owns the row until release; a write or onDragIdle then brings it up to date.
		if row.slider.Dragging() {
			return
		}
		if value, err := strconv.Atoi(decoded); known && err == nil {
			dimThumb(false)
			row.slider.ShowValue(float64(value))
			row.number.SetValue(decoded + "%")
			return
		}
		// Nothing was reported: no bright thumb may claim the value is 0.
		dimThumb(true)
		row.slider.ShowValue(0)
		row.number.SetValue("")
	}
	showCurrent := func() {
		value, known := row.display()
		row.show(value, known)
	}
	row.slider.onDragIdle = showCurrent
	row.enable = func(on bool) {
		abandoned := !on && row.slider.Dragging()
		enableWidget(row.slider, on)
		if abandoned {
			showCurrent()
		}
	}
}

// buildModes uses the mode setting's words; the setting needs at least two of them.
func (t *settingsTab) buildModes(row *settingRow) {
	mode, _ := dongle.LookupSetting("mode")
	row.chips = make([]*chip, len(mode.Values))
	changed := func(bool) {
		if t.applying {
			return
		}
		var words []string
		for i, value := range mode.Values {
			if row.chips[i].Checked && row.chips[i].Visible() {
				words = append(words, value.Word)
			}
		}
		if len(words) < 2 {
			t.w.say(settingLabel(row.setting.Name) + ": " + i18n.T("settings.pickTwo"))
			if row.known {
				t.restore(row)
			}
			return
		}
		t.write(row, strings.Join(words, ","))
	}
	objects := make([]fyne.CanvasObject, len(mode.Values))
	for i, value := range mode.Values {
		row.chips[i] = newChip(chipLabel(value.Word), changed)
		if icon, ok := wordIcons[value.Word]; ok {
			row.chips[i].Icon = icon
		}
		objects[i] = row.chips[i]
	}
	row.control = container.New(chipRowLayout{}, objects...)
	row.refit = func(view model.View) {
		for i, value := range mode.Values {
			hidden := view.Identified() && ((value.Value == 1 && !view.OffAvailable()) || (value.Value == 4 && !view.Adaptive()))
			showIf(row.chips[i], !hidden)
		}
	}
	row.show = func(decoded string, known bool) {
		active := map[string]bool{}
		if known {
			for _, word := range strings.Split(decoded, ",") {
				active[word] = true
			}
		}
		for i, value := range mode.Values {
			row.chips[i].SetChecked(active[value.Word])
		}
	}
	row.enable = func(on bool) {
		for _, item := range row.chips {
			enableWidget(item, on)
		}
	}
	row.relabel = func() {
		for i, value := range mode.Values {
			row.chips[i].Label = chipLabel(value.Word)
			row.chips[i].Refresh()
		}
	}
}

// buildActions keeps ear order to Encode and Decode, which define the wire layout.
func (t *settingsTab) buildActions(row *settingRow) {
	labels := make([]string, len(holdActionWords))
	words := map[string]string{}
	fill := func() {
		clear(words)
		for i, word := range holdActionWords {
			labels[i] = wordLabel(word)
			words[labels[i]] = word
		}
	}
	fill()
	var left, right *widget.Select
	changed := func(string) {
		if t.applying || left.Selected == "" || right.Selected == "" {
			return
		}
		t.write(row, words[left.Selected]+","+words[right.Selected])
	}
	left = widget.NewSelect(append([]string(nil), labels...), changed)
	right = widget.NewSelect(append([]string(nil), labels...), changed)
	left.PlaceHolder, right.PlaceHolder = dashText, dashText
	row.actions = [2]*widget.Select{left, right}
	row.refit = func(view model.View) {
		options := append([]string(nil), labels...)
		if view.Identified() && !view.Listening() {
			options = []string{wordLabel("siri")}
		}
		if !sameStrings(options, left.Options) {
			t.applying = true
			left.SetOptions(options)
			right.SetOptions(options)
			t.applying = false
		}
	}
	side := func(key string, choice *widget.Select) fyne.CanvasObject {
		name := widget.NewLabel(i18n.T(key))
		t.w.bind(func() { name.SetText(i18n.T(key)) })
		return container.NewBorder(nil, nil, name, nil, compactSelect(choice))
	}
	row.control = container.NewGridWithColumns(2, side("battery.left", left), side("battery.right", right))
	row.relabel = func() {
		fill()
		left.Selected, right.Selected = "", ""
		row.refit(t.w.view())
	}
	row.show = func(decoded string, known bool) {
		parts := strings.SplitN(decoded, ",", 2)
		if !known || len(parts) != 2 {
			left.ClearSelected()
			right.ClearSelected()
			return
		}
		left.SetSelected(wordLabel(parts[0]))
		right.SetSelected(wordLabel(parts[1]))
	}
	row.enable = func(on bool) {
		enableWidget(left, on)
		enableWidget(right, on)
	}
}

func (t *settingsTab) restore(row *settingRow) {
	t.applying = true
	value, known := row.display()
	row.show(value, known)
	t.applying = false
}

// setNote puts a note in the hint slot, or gives the slot back to the help text. It does
// nothing when the text is unchanged, so repeated reads move nothing on the page.
func setNote(row *settingRow, text string) {
	setLabel(row.note, text)
	showIf(row.note, text != "")
	if row.help != nil {
		showIf(row.help, text == "")
	}
}
