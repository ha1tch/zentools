package basic

import (
	"fmt"
	"sort"
	"strings"
)

// Option configures tokenisation. Options are applied left to right.
type Option func(*config)

type config struct {
	caseInsensitive bool
}

func defaults() config {
	return config{caseInsensitive: true}
}

// CaseSensitive makes keyword matching case-sensitive (keywords must be
// upper-case to tokenise). By default matching is case-insensitive, which suits
// hand-written source.
func CaseSensitive() Option {
	return func(c *config) { c.caseInsensitive = false }
}

// Tokenise converts BASIC source text into the on-disk tokenised byte form. It
// is the inverse of Detokenise.
//
// Input is one BASIC line per text line, each beginning with a line number, for
// example:
//
//	10 CLEAR 32767: LOAD "game"CODE: RANDOMIZE USR 32768
//	20 PRINT "DONE"
//
// Keywords are matched longest-first and only outside string literals and REM
// text. Numeric constants are emitted in the ROM integer form (visible digits,
// then 0x0E and a 5-byte value); only integer constants in 0..65535 are encoded.
//
// The result is the raw tokenised program with no PLUS3DOS header.
func Tokenise(src string, opts ...Option) ([]byte, error) {
	cfg := defaults()
	for _, o := range opts {
		o(&cfg)
	}

	var out []byte
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		encoded, num, err := tokeniseLine(line, &cfg)
		if err != nil {
			return nil, err
		}
		// Line header: [line number, big-endian][text length, little-endian].
		out = append(out,
			byte(num>>8), byte(num&0xFF),
			byte(len(encoded)&0xFF), byte(len(encoded)>>8),
		)
		out = append(out, encoded...)
	}
	return out, nil
}

// tokenValues is the inverse of basicTokens, sorted longest-keyword-first so
// that "GO SUB" matches before "GO" and "<=" before "<".
var tokenValues = buildTokenValues()

type tokenEntry struct {
	text  string
	token byte
}

func buildTokenValues() []tokenEntry {
	entries := make([]tokenEntry, 0, len(basicTokens))
	for tok, text := range basicTokens {
		entries = append(entries, tokenEntry{text: text, token: tok})
	}
	// Longest text first; stable tie-break by token value for determinism.
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].text) != len(entries[j].text) {
			return len(entries[i].text) > len(entries[j].text)
		}
		return entries[i].token < entries[j].token
	})
	return entries
}

// tokeniseLine encodes a single source line (after its line number) into the
// tokenised byte sequence terminated by 0x0D, and returns the line number.
func tokeniseLine(line string, cfg *config) ([]byte, int, error) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	start := i
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == start {
		return nil, 0, fmt.Errorf("line does not start with a line number: %q", line)
	}
	num := 0
	for _, c := range line[start:i] {
		num = num*10 + int(c-'0')
	}
	if num < 0 || num > 9999 {
		return nil, 0, fmt.Errorf("line number out of range (0-9999): %d", num)
	}

	body := line[i:]
	var out []byte
	j := 0
	for j < len(body) {
		c := body[j]

		// String literal: copy verbatim until the closing quote.
		if c == '"' {
			out = append(out, c)
			j++
			for j < len(body) {
				out = append(out, body[j])
				if body[j] == '"' {
					j++
					break
				}
				j++
			}
			continue
		}

		// Try to match a keyword token at this position (longest first).
		if tok, n, ok := matchToken(body[j:], cfg); ok {
			out = append(out, tok)
			j += n
			// REM: everything after the REM token is literal text.
			if tok == remToken {
				out = append(out, body[j:]...)
				j = len(body)
			}
			continue
		}

		// Numeric constant: ASCII digits, then the 0x0E + 5-byte value.
		if c >= '0' && c <= '9' {
			k := j
			for k < len(body) && body[k] >= '0' && body[k] <= '9' {
				k++
			}
			digits := body[j:k]
			val := 0
			for _, d := range digits {
				val = val*10 + int(d-'0')
			}
			if val > 0xFFFF {
				return nil, 0, fmt.Errorf("numeric constant out of range (0-65535): %s", digits)
			}
			out = append(out, digits...)
			// ROM integer form: 0x0E then 00 00 LL HH 00.
			out = append(out, numberMarker, 0x00, 0x00, byte(val&0xFF), byte(val>>8), 0x00)
			j = k
			continue
		}

		// Ordinary character (operators, punctuation, spaces, identifiers).
		out = append(out, c)
		j++
	}

	out = append(out, 0x0D)
	return out, num, nil
}

// matchToken returns the token byte and consumed length if the start of s is a
// keyword. Matching honours the case-sensitivity option and is longest-first.
func matchToken(s string, cfg *config) (byte, int, bool) {
	candidate := s
	if cfg.caseInsensitive {
		candidate = strings.ToUpper(s)
	}
	for _, e := range tokenValues {
		if strings.HasPrefix(candidate, e.text) {
			return e.token, len(e.text), true
		}
	}
	return 0, 0, false
}
