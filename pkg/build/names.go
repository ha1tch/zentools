// file: names.go
//
// Three different filename namespaces meet in a build, and conflating them
// produces subtly wrong output:
//
//   - Host filenames: UTF-8 long filenames, whatever the operating system
//     allows. This is the name of the .tap/.z80/etc. file on disk.
//   - Spectrum tape header names: up to 10 characters, any byte, space-padded
//     to exactly 10. This is what appears in a tape block header and what the
//     user sees during LOAD.
//   - +3DOS (+3 disk) filenames: 8.3 form, uppercase alphanumeric, as a CP/M
//     derived filesystem requires.
//
// Each has its own normaliser below. The host name is never used directly as a
// Spectrum name, and vice versa.

package build

import (
	"path/filepath"
	"strings"
)

// tapeName converts an arbitrary base name into a Spectrum tape header name:
// at most 10 characters, taken from the leading characters of the input. The
// tap encoder space-pads to 10, so this only needs to supply the significant
// characters. Non-printable bytes are replaced with spaces so the LOAD display
// is legible; the Spectrum itself permits any byte, but a legible name is
// friendlier and avoids control codes in the header.
func tapeName(base string) string {
	var b strings.Builder
	for _, r := range base {
		if b.Len() >= 10 {
			break
		}
		if r < 0x20 || r > 0x7E {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// plus3Name converts an arbitrary base name into a +3DOS 8.3 filename:
// uppercase, alphanumeric, name truncated to 8 characters and extension to 3.
// An optional extension may be supplied (without the dot); if empty, the input
// is split on its final dot. Characters outside [A-Z0-9] are dropped.
func plus3Name(base, ext string) string {
	name := base
	if ext == "" {
		if dot := strings.LastIndex(base, "."); dot >= 0 {
			name = base[:dot]
			ext = base[dot+1:]
		}
	}
	clean := func(s string, max int) string {
		var b strings.Builder
		for _, r := range strings.ToUpper(s) {
			if b.Len() >= max {
				break
			}
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	n := clean(name, 8)
	e := clean(ext, 3)
	if n == "" {
		n = "PROGRAM"
	}
	if e == "" {
		return n
	}
	return n + "." + e
}

// hostBase strips a host path down to its base name without extension, for use
// as the default output basename and as the source for the Spectrum names.
func hostBase(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}
