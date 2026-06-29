package basic

import (
	"fmt"
	"strings"
)

// LooksTokenised reports whether data appears to be already-tokenised BASIC
// rather than plain-text source. It parses data as a sequence of tokenised
// lines ([line# big-endian, 0-9999][length little-endian][length bytes][0x0D])
// and returns true only if the whole buffer parses cleanly into one or more
// such lines with ascending line numbers. This is a structural check, not a
// guess on a single byte, so it is suitable for an advisory warning: it will
// not misfire on ordinary text source, which does not have this layout.
func LooksTokenised(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	i := 0
	prevLine := -1
	lines := 0
	for i < len(data) {
		if i+4 > len(data) {
			return false // dangling partial line header
		}
		lineNo := int(data[i])<<8 | int(data[i+1])
		length := int(data[i+2]) | int(data[i+3])<<8
		if lineNo > 9999 || lineNo <= prevLine {
			return false // line numbers must be valid and ascending
		}
		bodyStart := i + 4
		bodyEnd := bodyStart + length
		if length == 0 || bodyEnd > len(data) {
			return false
		}
		if data[bodyEnd-1] != 0x0D {
			return false // each line's body must end with 0x0D
		}
		prevLine = lineNo
		lines++
		i = bodyEnd
	}
	return lines > 0
}

// Detokenise converts a tokenised Sinclair BASIC program (the raw program
// bytes, with no PLUS3DOS header) into readable text. It is the inverse of
// Tokenise.
//
// Program structure: a sequence of lines, each
//
//	[line number: 2 bytes big-endian][length: 2 bytes little-endian][text...][0x0D]
//
// Within the text, keyword tokens (0xA3-0xFF) expand to keywords; a numeric
// constant appears as its visible ASCII digits followed by a 0x0E marker and a
// 5-byte binary form, which is skipped (the visible digits are what we print).
//
// This handles the cases a loader needs: keywords, numbers, strings, and the
// statement separator. It does not reproduce embedded colour/AT/TAB control-code
// arguments beyond passing printable bytes through; non-printable bytes are
// shown as a [XX] hex escape so output stays lossless.
func Detokenise(prog []byte) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(prog) {
		if i+4 > len(prog) {
			break // trailing bytes shorter than a line header; stop cleanly
		}
		lineNo := int(prog[i])<<8 | int(prog[i+1])
		length := int(prog[i+2]) | int(prog[i+3])<<8
		i += 4
		if i+length > len(prog) {
			return "", fmt.Errorf("line %d claims %d bytes but only %d remain",
				lineNo, length, len(prog)-i)
		}
		text := prog[i : i+length]
		i += length

		out.WriteString(fmt.Sprintf("%d ", lineNo))
		out.WriteString(detokeniseLine(text))
		out.WriteByte('\n')
	}
	return out.String(), nil
}

// detokeniseLine renders a single line's text (up to and including its 0x0D).
func detokeniseLine(text []byte) string {
	var b strings.Builder
	for j := 0; j < len(text); j++ {
		c := text[j]
		switch {
		case c == 0x0D:
			// End-of-line marker; nothing to emit.
		case c == numberMarker:
			// The 5 binary bytes that follow are the value already shown as ASCII
			// digits just before this marker. Skip them.
			j += 5
		case c >= 0xA3:
			b.WriteString(basicTokens[c])
			// A single trailing space after the keyword keeps tokens from running
			// together; the Spectrum's own listing spacing is contextual, and this
			// is the safe, readable choice.
			b.WriteByte(' ')
		case c >= 0x20 && c < 0x7F:
			// Printable ASCII (digits, letters, punctuation, quotes).
			b.WriteByte(c)
		default:
			// Non-printable / control byte we do not specifically handle: show it
			// as a hex escape so output stays lossless and obvious.
			b.WriteString(fmt.Sprintf("[%02X]", c))
		}
	}
	return strings.TrimRight(b.String(), " ")
}
