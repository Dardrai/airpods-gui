// Package choices remembers, per AirPods pair, values the AirPods accept but do not
// report, so the window can show what was last chosen. Pairs are known by a salted hash of
// their Bluetooth address, so that neither file names nor settings give the address away.
package choices

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
)

const (
	maxFileBytes = 1024
	// saltFile holds this install's random salt; it is what makes a hash useless elsewhere.
	saltFile  = "pair-salt"
	saltBytes = 32
	// pairKeyBytes is how much of the hash names a pair, which is plenty among a few pairs.
	pairKeyBytes = 16
)

// Store keeps its files in one directory. Use it through a pointer: it holds the salt once read.
type Store struct {
	Dir string
	// Logf is told, once each, what keeps the store from reading; nil keeps it quiet.
	Logf func(format string, args ...any)

	mu     sync.Mutex
	salted []byte // the salt, after the first successful read
	// migrated is set once every file of an earlier version has its new name.
	migrated bool
	logged   string
}

// DefaultDir is the per-user location.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "airpods-tray"), nil
}

// salt is this install's salt, read from disk once and made the first time, when there is
// no file. Anything else in the way is an error and nothing is written: a salt that is only
// unreadable for now, or damaged, still names everything saved under it, and a new one would
// orphan all of that for good.
func (s *Store) salt() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.salted != nil {
		return s.salted, nil
	}
	path := filepath.Join(s.Dir, saltFile)
	salt, err := os.ReadFile(path)
	switch {
	case err == nil && len(salt) != saltBytes:
		return nil, fmt.Errorf("%s holds %d bytes instead of %d; it is left as it is", path, len(salt), saltBytes)
	case err == nil:
		if info, err := os.Stat(path); err == nil && info.Mode().Perm() != 0o600 {
			if err := os.Chmod(path, 0o600); err != nil {
				return nil, err
			}
		}
	case errors.Is(err, fs.ErrNotExist):
		salt = make([]byte, saltBytes)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
		if err := s.write(path, salt); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	s.salted = salt
	return salt, nil
}

// unavailable logs why nothing can be read or moved, once per reason.
func (s *Store) unavailable(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if text := err.Error(); s.Logf != nil && text != s.logged {
		s.logged = text
		s.Logf("nothing is read from what was remembered per pair: %v", err)
	}
}

// PairKey is the name a pair goes by in anything stored. Without a salt there is no name: the
// address is never stored in the clear instead.
func (s *Store) PairKey(peer [6]byte) (string, error) {
	salt, err := s.salt()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write(peer[:])
	return hex.EncodeToString(mac.Sum(nil)[:pairKeyBytes]), nil
}

// LegacyPeer reads the address out of the name versions before the hash gave a pair.
func LegacyPeer(name string) ([6]byte, bool) {
	var peer [6]byte
	if !legacyName.MatchString(name) {
		return peer, false
	}
	_, err := hex.Decode(peer[:], []byte(name))
	return peer, err == nil
}

// LegacyName is the name versions before the hash gave a pair.
func LegacyName(peer [6]byte) string { return hex.EncodeToString(peer[:]) }

var (
	legacyName = regexp.MustCompile(`^[0-9a-f]{12}$`)
	legacyFile = regexp.MustCompile(`^(device-choices-|allow-off-)([0-9a-f]{12})(\.json)?$`)
)

// Migrate renames the files of earlier versions, every pair at once, since their names are
// the addresses to protect. A name that is taken means the newer value wins. The app runs it
// at startup, so that no address waits on disk for its pair to come by; a pass that could not
// finish is made again by the next read, and one that did is not repeated.
func (s *Store) Migrate() {
	s.mu.Lock()
	done := s.migrated
	s.mu.Unlock()
	if done {
		return
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.unavailable(err)
		return
	}
	complete := true
	for _, entry := range entries {
		match := legacyFile.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		peer, ok := LegacyPeer(match[2])
		if !ok {
			continue
		}
		key, err := s.PairKey(peer)
		if err != nil {
			s.unavailable(err)
			return
		}
		old, next := filepath.Join(s.Dir, entry.Name()), filepath.Join(s.Dir, match[1]+key+match[3])
		if _, err := os.Stat(next); err == nil {
			err = os.Remove(old)
			complete = complete && err == nil
			continue
		}
		complete = os.Rename(old, next) == nil && complete
	}
	s.mu.Lock()
	s.migrated = complete
	s.mu.Unlock()
}

func (s *Store) path(peer [6]byte) (string, error) {
	key, err := s.PairKey(peer)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, "device-choices-"+key+".json"), nil
}

func (s *Store) allowOffPath(peer [6]byte) (string, error) {
	key, err := s.PairKey(peer)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, "allow-off-"+key), nil
}

func valid(control byte, value uint32) bool {
	if !model.ExplicitControl(control) {
		return false
	}
	for _, table := range [][]dongle.Setting{dongle.Settings, dongle.ExtraSettings} {
		for i := range table {
			if table[i].Control == control {
				decoded := table[i].Decode(value)
				return len(decoded) < 7 || decoded[:7] != "unknown"
			}
		}
	}
	return false
}

// Load returns the saved choices. Anything unexpected in the file voids all of them.
func (s *Store) Load(peer [6]byte) map[byte]uint32 {
	s.Migrate()
	path, err := s.path(peer)
	if err != nil {
		s.unavailable(err)
		return map[byte]uint32{}
	}
	file, err := os.Open(path)
	if err != nil {
		return map[byte]uint32{}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil || len(data) > maxFileBytes {
		return map[byte]uint32{}
	}
	var raw map[string]uint32
	if json.Unmarshal(data, &raw) != nil {
		return map[byte]uint32{}
	}
	out := make(map[byte]uint32, len(raw))
	for key, value := range raw {
		id, err := strconv.ParseUint(key, 10, 8)
		if err != nil || strconv.FormatUint(id, 10) != key || !valid(byte(id), value) {
			return map[byte]uint32{}
		}
		out[byte(id)] = value
	}
	return out
}

func (s *Store) write(path string, data []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.Dir, ".choice-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	// On disk before it takes the name: a crash must not leave an empty file under it.
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// Save records one choice for a pair.
func (s *Store) Save(peer [6]byte, control byte, value uint32) error {
	if peer == [6]byte{} || !valid(control, value) {
		return fmt.Errorf("invalid device choice")
	}
	saved := s.Load(peer)
	saved[control] = value
	path, err := s.path(peer)
	if err != nil {
		return err
	}
	raw := make(map[string]uint32, len(saved))
	for id, v := range saved {
		raw[strconv.Itoa(int(id))] = v
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return s.write(path, data)
}

// LoadAllowOff returns the saved Allow Off value, nil when there is none.
func (s *Store) LoadAllowOff(peer [6]byte) *bool {
	s.Migrate()
	path, err := s.allowOffPath(peer)
	if err != nil {
		s.unavailable(err)
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var on bool
	switch string(data) {
	case "true":
		on = true
	case "false":
	default:
		return nil
	}
	return &on
}

// SaveAllowOff records the Allow Off value sent to a pair.
func (s *Store) SaveAllowOff(peer [6]byte, on bool) error {
	if peer == [6]byte{} {
		return fmt.Errorf("invalid device choice")
	}
	s.Migrate()
	path, err := s.allowOffPath(peer)
	if err != nil {
		return err
	}
	return s.write(path, []byte(strconv.FormatBool(on)))
}
