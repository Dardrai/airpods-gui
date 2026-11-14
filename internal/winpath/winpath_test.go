package winpath

import "testing"

func TestAdd(t *testing.T) {
	const dir = `C:\Users\me\AppData\Local\Programs\AirPods Tray`
	for _, tc := range []struct {
		name, list, want string
		changed          bool
	}{
		{"empty", "", dir, true},
		{"only separators", ";", dir, true},
		{"appended", `%USERPROFILE%\bin;C:\tools`, `%USERPROFILE%\bin;C:\tools;` + dir, true},
		{"trailing separator", `C:\tools;`, `C:\tools;` + dir, true},
		{"already there", `C:\tools;` + dir, `C:\tools;` + dir, false},
		{"other case and a backslash", `c:\users\ME\appdata\local\programs\airpods tray\;C:\tools`, `c:\users\ME\appdata\local\programs\airpods tray\;C:\tools`, false},
		{"quoted", `"` + dir + `";C:\tools`, `"` + dir + `";C:\tools`, false},
		{"a longer folder is another one", dir + `\bin`, dir + `\bin;` + dir, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := Add(tc.list, dir)
			if got != tc.want || changed != tc.changed {
				t.Errorf("Add(%q) = %q, %v, want %q, %v", tc.list, got, changed, tc.want, tc.changed)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	const dir = `C:\Apps\AirPods`
	for _, tc := range []struct {
		name, list, want string
		changed          bool
	}{
		{"empty", "", "", false},
		{"not there", `%USERPROFILE%\bin;C:\tools`, `%USERPROFILE%\bin;C:\tools`, false},
		{"last", `%USERPROFILE%\bin;` + dir, `%USERPROFILE%\bin`, true},
		{"first", dir + `;C:\tools`, `C:\tools`, true},
		{"only", dir, "", true},
		{"twice, in another case", `c:\apps\airpods\;C:\tools;` + dir, `C:\tools`, true},
		{"a longer folder stays", dir + `\bin;` + dir, dir + `\bin`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := Remove(tc.list, dir)
			if got != tc.want || changed != tc.changed {
				t.Errorf("Remove(%q) = %q, %v, want %q, %v", tc.list, got, changed, tc.want, tc.changed)
			}
		})
	}
}

// What Add puts in, Remove takes out, and the rest is untouched.
func TestAddThenRemove(t *testing.T) {
	const list, dir = `%USERPROFILE%\bin;C:\Program Files\x`, `C:\Apps\AirPods`
	added, _ := Add(list, dir)
	if got, changed := Remove(added, dir); got != list || !changed {
		t.Errorf("got %q, %v", got, changed)
	}
}
