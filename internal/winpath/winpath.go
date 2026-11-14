// Package winpath adds a folder to the user's PATH on Windows and takes it out again, for the
// installer: it puts the folder of the airpods command there, so that a terminal finds it.
package winpath

import "strings"

// same reports whether two PATH entries name one folder: Windows ignores case, and a
// trailing backslash and surrounding quotes or blanks change nothing.
func same(a, b string) bool {
	clean := func(s string) string {
		s = strings.Trim(strings.TrimSpace(s), `"`)
		return strings.TrimRight(s, `\/`)
	}
	return strings.EqualFold(clean(a), clean(b))
}

// Add returns list with dir as its last entry. Every other entry stays as it is written,
// with its %VARIABLES% unexpanded. changed is false when dir is already there.
func Add(list, dir string) (result string, changed bool) {
	for _, entry := range strings.Split(list, ";") {
		if same(entry, dir) {
			return list, false
		}
	}
	if strings.TrimRight(list, "; ") == "" {
		return dir, true
	}
	return strings.TrimRight(list, ";") + ";" + dir, true
}

// Remove returns list without every entry that names dir. changed is false when there is
// none.
func Remove(list, dir string) (result string, changed bool) {
	var kept []string
	for _, entry := range strings.Split(list, ";") {
		if same(entry, dir) {
			changed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !changed {
		return list, false
	}
	return strings.Join(kept, ";"), true
}
