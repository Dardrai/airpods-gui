package desktopnote

import "strings"

// quote is a PowerShell single-quoted string, which expands nothing; a quote inside, in any of
// the forms PowerShell accepts, is doubled.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}
