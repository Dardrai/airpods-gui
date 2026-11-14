// Package updates finds out whether a newer release of the app is out on GitHub, and on
// Windows installs it with the release's own installer. It only talks to the network when
// the user has turned the check on.
package updates

import (
	"strconv"
	"strings"
)

// Version is a release tag taken apart: v0.2.1-beta has the numbers 0.2.1 and the label
// "beta". A build between tags, as git describe names it (v0.2.1-beta-3-gabc1234-dirty),
// has the tag's numbers and label and is marked After.
type Version struct {
	Major, Minor, Patch int
	// Label is the pre-release label, such as "alpha", "beta" or "rc1"; empty for a release.
	Label string
	// After marks a build made from commits past its tag.
	After bool
}

// Parse reads a tag such as v0.2.1, v0.2.1-beta or v0.2.1-beta-3-gabc1234-dirty.
func Parse(tag string) (Version, bool) {
	s := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	numbers, rest, _ := strings.Cut(s, "-")
	parts := strings.Split(numbers, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var v Version
	for i, target := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 || parts[i] != strconv.Itoa(n) {
			return Version{}, false
		}
		*target = n
	}
	if rest == "" {
		return v, true
	}
	fields := strings.Split(rest, "-")
	// git describe adds -<count>-g<hash>, and -dirty for uncommitted changes.
	if fields[len(fields)-1] == "dirty" {
		fields, v.After = fields[:len(fields)-1], true
	}
	if n := len(fields); n >= 2 && isCount(fields[n-2]) && isHash(fields[n-1]) {
		fields, v.After = fields[:n-2], true
	}
	v.Label = strings.Join(fields, "-")
	return v, true
}

func isCount(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

func isHash(s string) bool {
	if len(s) < 5 || s[0] != 'g' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// String is the version as a tag, without the v and without what describe adds.
func (v Version) String() string {
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Label != "" {
		s += "-" + v.Label
	}
	return s
}

// Numbers is the version without its label, such as 0.1.66 for the tag v0.1.66-26c9206e6934.
func (v Version) Numbers() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// Newer reports whether latest is a later release than running. Equal numbers go by the
// label: a release beats any pre-release, and labels compare as alpha < beta < rc. A build
// past a tag is never behind that same tag.
func Newer(latest, running Version) bool {
	if c := compareNumbers(latest, running); c != 0 {
		return c > 0
	}
	if latest.Label == running.Label {
		return false
	}
	if latest.Label == "" {
		return true
	}
	if running.Label == "" {
		return false
	}
	return labelRank(latest.Label) > labelRank(running.Label) ||
		(labelRank(latest.Label) == labelRank(running.Label) && latest.Label > running.Label)
}

func compareNumbers(a, b Version) int {
	for _, d := range []int{a.Major - b.Major, a.Minor - b.Minor, a.Patch - b.Patch} {
		if d != 0 {
			return d
		}
	}
	return 0
}

// labelRank orders the usual pre-release labels; anything else counts as alpha.
func labelRank(label string) int {
	switch l := strings.ToLower(label); {
	case strings.HasPrefix(l, "rc"):
		return 3
	case strings.HasPrefix(l, "beta"):
		return 2
	}
	return 1
}
