package autopause

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix    = "org.mpris.MediaPlayer2."
	mprisPath      = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisPlayer    = "org.mpris.MediaPlayer2.Player"
	mprisCallLimit = time.Second
	mprisSettle    = 1500 * time.Millisecond
	mprisStep      = 50 * time.Millisecond
)

// mprisBus is the part of D-Bus MPRIS needs; tests fake it. Players are addressed by
// unique connection name, so a claim cannot follow a well known name to a new program.
type mprisBus interface {
	Players() ([]string, error)
	Status(player string) (string, error)
	CanPause(player string) bool
	Track(player string) string
	Call(player, method string) error
}

// SystemMedia is the desktop's media players: MPRIS on the session bus.
func SystemMedia() Media { return &mprisMedia{bus: &sessionBus{}, sleep: time.Sleep} }

type mprisMedia struct {
	bus   mprisBus
	sleep func(time.Duration)
}

type mprisClaim struct {
	player string
	track  string
}

type mprisPaused struct {
	bus    mprisBus
	claims []mprisClaim
}

// PauseActive pauses every playing player and keeps those that confirm it.
func (m *mprisMedia) PauseActive() (Paused, error) {
	players, err := m.bus.Players()
	if err != nil {
		return nil, err
	}
	var asked []mprisClaim
	for _, player := range players {
		status, err := m.bus.Status(player)
		if err != nil || status != "Playing" || !m.bus.CanPause(player) {
			continue
		}
		track := m.bus.Track(player)
		if m.bus.Call(player, "Pause") == nil {
			asked = append(asked, mprisClaim{player, track})
		}
	}
	if len(asked) == 0 {
		return nil, nil
	}
	var claims []mprisClaim
	for waited := time.Duration(0); ; waited += mprisStep {
		claims = claims[:0]
		for _, claim := range asked {
			if status, err := m.bus.Status(claim.player); err == nil && status == "Paused" {
				claims = append(claims, claim)
			}
		}
		if len(claims) == len(asked) || waited >= mprisSettle {
			break
		}
		m.sleep(mprisStep)
	}
	if len(claims) == 0 {
		return nil, errors.New("no player confirmed the pause")
	}
	return &mprisPaused{bus: m.bus, claims: claims}, nil
}

func (p *mprisPaused) ours(claim mprisClaim) bool {
	status, err := p.bus.Status(claim.player)
	return err == nil && status == "Paused" && p.bus.Track(claim.player) == claim.track
}

// StillOurs drops players that moved on and holds while any remains.
func (p *mprisPaused) StillOurs() bool {
	kept := p.claims[:0]
	for _, claim := range p.claims {
		if p.ours(claim) {
			kept = append(kept, claim)
		}
	}
	p.claims = kept
	return len(kept) > 0
}

// Resume plays each player that is still paused as we left it.
func (p *mprisPaused) Resume() error {
	var failed error
	for _, claim := range p.claims {
		if !p.ours(claim) {
			continue
		}
		if err := p.bus.Call(claim.player, "Play"); err != nil {
			failed = err
		}
	}
	return failed
}

// Release implements Paused; D-Bus names hold nothing open.
func (p *mprisPaused) Release() { p.claims = nil }

// sessionBus is the real session bus, the same connection the tray uses.
type sessionBus struct{}

func call(obj dbus.BusObject, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), mprisCallLimit)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}

func (s *sessionBus) Players() ([]string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	var names []string
	if err := call(conn.BusObject(), "org.freedesktop.DBus.ListNames").Store(&names); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var players []string
	for _, name := range names {
		if !strings.HasPrefix(name, mprisPrefix) {
			continue
		}
		var owner string
		if call(conn.BusObject(), "org.freedesktop.DBus.GetNameOwner", name).Store(&owner) != nil || seen[owner] {
			continue
		}
		seen[owner] = true
		players = append(players, owner)
	}
	return players, nil
}

func (s *sessionBus) property(player, name string) (dbus.Variant, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return dbus.Variant{}, err
	}
	var value dbus.Variant
	err = call(conn.Object(player, mprisPath), "org.freedesktop.DBus.Properties.Get", mprisPlayer, name).Store(&value)
	return value, err
}

func (s *sessionBus) Status(player string) (string, error) {
	value, err := s.property(player, "PlaybackStatus")
	if err != nil {
		return "", err
	}
	text, _ := value.Value().(string)
	return text, nil
}

func (s *sessionBus) CanPause(player string) bool {
	value, err := s.property(player, "CanPause")
	if err != nil {
		return true // A player that does not say is asked anyway; it can only refuse.
	}
	can, known := value.Value().(bool)
	return !known || can
}

// Track names the current track, "" when the player does not say.
func (s *sessionBus) Track(player string) string {
	value, err := s.property(player, "Metadata")
	if err != nil {
		return ""
	}
	metadata, _ := value.Value().(map[string]dbus.Variant)
	if id, ok := metadata["mpris:trackid"]; ok {
		return id.String()
	}
	return ""
}

func (s *sessionBus) Call(player, method string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	return call(conn.Object(player, mprisPath), mprisPlayer+"."+method).Err
}
