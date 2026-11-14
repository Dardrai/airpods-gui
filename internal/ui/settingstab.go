package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

const (
	dashText                        = unknownValue
	adaptiveStrengthControl         = 0x2e
	sliderWidth             float32 = 230
)

const (
	holdGroup       = "Press and Hold AirPods"
	listeningGroup  = "Listening Mode"
	generalGroup    = "" // the card with no caption
	experimentGroup = "Experimental"
	refreshCooldown = 5 * time.Second
)

// settingGroups is the page's order. The first group is the listening block; the rest
// are plain cards. The last one holds what only shows with the experimental switch on,
// and sits further down the page than the others.
var settingGroups = []struct {
	title string
	names []string
}{
	{listeningGroup, []string{"mode", "adaptive-strength"}},
	{holdGroup, []string{"hold-actions", "hold-modes"}},
	{"Call Controls", []string{"call-controls"}},
	{"Audio", []string{"personalized-volume", "conversation-awareness", "en352-protection"}},
	{generalGroup, []string{"ear-detection", "connect-automatically", "microphone", "pause-when-asleep", "optimized-charging"}},
	{"Accessibility", []string{"tone-volume", "press-speed", "hold-duration", "one-bud-anc", "volume-swipe", "swipe-duration", "gain-swipe"}},
	{"Off Listening Mode", []string{"allow-off"}},
	{experimentGroup, []string{"crown-direction", "case-sounds", "case-volume", "allow-auto-connect"}},
}

// wideRows put their control under the title; it does not fit beside it.
var wideRows = map[string]bool{"hold-modes": true, "hold-actions": true}

// settingCard is one titled group of rows, hidden when the model has none of them.
type settingCard struct {
	title *text
	group fyne.CanvasObject
	rows  []*settingRow
	list  *stack
	base  string
}

type settingsTab struct {
	w       *Window
	content fyne.CanvasObject
	// experiments is the experimental card; the page places it below the main cards.
	experiments  fyne.CanvasObject
	rows         []*settingRow
	cards        []*settingCard
	byName       map[string]*settingRow
	strength     *fyne.Container
	strengthDrag func(int)
	rereadBt     *widget.Button
	rereadItem   *menuItem // the same action in the ... panel
	reading      bool
	applying     bool // set while the UI, not the user, changes a widget
	reported     map[byte]uint32
	everRead     bool // on this link; before that a missing report says nothing
	coolingDown  bool
	again        bool
	failures     int
	nextRetry    time.Time
	cooldown     time.Duration
	// overlay is reported plus the status's listening mode, for the model gates.
	overlay map[byte]uint32
	away    bool
	// sinceLink counts the ticks of this link; eagerUntil is the last of them to read on.
	sinceLink, eagerUntil int
	settled               bool // what the header was last told by settling
	wasArriving           bool // what the rows were last told by arriving
	// loading is set while the dongle refuses the snapshot because the AirPods controls are
	// not up yet, which is part of connecting rather than a problem.
	loading bool
}

func newSettingsTab(w *Window) *settingsTab {
	t := &settingsTab{w: w, byName: map[string]*settingRow{}}
	for _, table := range [][]dongle.Setting{dongle.Settings, dongle.ExtraSettings} {
		for i := range table {
			row := t.newRow(&table[i])
			t.rows = append(t.rows, row)
			t.byName[row.setting.Name] = row
		}
	}
	t.rereadBt = w.button("settings.readAgain", t.refreshClicked)
	t.rereadItem = w.menuItem("device.readAgain", func() {
		w.menu.close()
		t.refreshClicked()
	})
	t.cooldown = refreshCooldown
	var groups []fyne.CanvasObject
	placed := map[string]bool{}
	for _, group := range settingGroups {
		var rows []*settingRow
		for _, name := range group.names {
			if row, ok := t.byName[name]; ok {
				rows = append(rows, row)
				placed[name] = true
			}
		}
		box := t.newCard(group.title, rows)
		if group.title == experimentGroup {
			t.experiments = box
			continue
		}
		groups = append(groups, box)
	}
	var rest []*settingRow
	for _, row := range t.rows {
		if !placed[row.setting.Name] {
			rest = append(rest, row)
		}
	}
	if len(rest) > 0 {
		groups = append(groups, t.newCard("Other", rest))
	}
	t.content = container.New(tightLayout{}, groups...)
	t.linkStrength()
	t.updateEnabled()
	return t
}

// newCard builds a caption and a card of rows. The listening block has its own shape:
// the pill sits directly on the page and the strength slider appears in a card below it.
func (t *settingsTab) newCard(title string, rows []*settingRow) fyne.CanvasObject {
	label, head := captionText(cardTitle(title))
	list := newStack()
	for _, row := range rows {
		row.body = t.rowBody(row)
		if row.segment == nil {
			list.add(row.body)
		}
	}
	group := &settingCard{title: label, rows: rows, list: list, base: title}
	var body fyne.CanvasObject = card(list)
	box := container.New(tightLayout{})
	if title != generalGroup {
		box.Add(head)
	}
	top := float32(4)
	if title == listeningGroup {
		box.Add(t.byName["mode"].body)
		t.strength = container.New(insetLayout{top: 4}, body)
		box.Add(t.strength)
	} else {
		box.Add(body)
		top = 0
	}
	box.Add(gap(sectionGap))
	if top > 0 {
		box = container.New(insetLayout{top: top}, box)
	}
	group.group = box
	t.cards = append(t.cards, group)
	return box
}

// cardTitle is a card's caption; the general card has none.
func cardTitle(base string) string {
	if base == generalGroup {
		return ""
	}
	return i18n.T("group." + base)
}

// rowBody lays one row out: title and hint on the left, control on the right.
func (t *settingsTab) rowBody(row *settingRow) fyne.CanvasObject {
	if row.segment != nil {
		return row.control
	}
	objects := []fyne.CanvasObject{row.label}
	if row.help != nil {
		objects = append(objects, row.help)
	}
	objects = append(objects, row.note)
	main := tight(objects...)
	if wideRows[row.setting.Name] {
		return container.New(insetLayout{top: rowPadV, bottom: rowPadV}, container.New(tightLayout{gap: 10}, main, row.control))
	}
	return container.New(rowLayout{}, main, row.control)
}
