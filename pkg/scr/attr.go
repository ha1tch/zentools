// Attribute string parsing.
//
// An attribute can be written as a CSS-like specification - a list of
// semicolon-separated key:value fields - so that tools and config files can name
// a Spectrum colour cell in text. ParseAttribute is the shared parser; it is in
// the library rather than in any one command so every consumer agrees on the
// syntax.
//
// Syntax:
//
//	ink:<colour>; paper:<colour>; bright:<bool>; flash:<bool>
//
// Curly braces around the whole spec are optional. Spaces around the ':' and ';'
// separators are ignored. Colours are given by name (black, blue, red, magenta,
// green, cyan, yellow, white) or by ZX Spectrum palette index 0-7. Booleans
// accept 1/0, true/false, yes/no, on/off. Unspecified fields default to black
// ink, black paper, no bright, no flash. An empty spec yields that default.

package scr

import (
	"fmt"
	"strconv"
	"strings"
)

// colourNames maps the eight Spectrum colour names to their palette indices.
var colourNames = map[string]uint8{
	"black":   Black,
	"blue":    Blue,
	"red":     Red,
	"magenta": Magenta,
	"green":   Green,
	"cyan":    Cyan,
	"yellow":  Yellow,
	"white":   White,
}

// ParseColour resolves a colour token to a palette index (0-7). The token is
// either a colour name (case-insensitive) or a numeric index 0-7. Surrounding
// spaces are ignored.
func ParseColour(s string) (uint8, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 7 {
			return 0, fmt.Errorf("colour index %d out of range (0-7)", n)
		}
		return uint8(n), nil
	}
	if c, ok := colourNames[s]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("unknown colour %q (use a name or an index 0-7)", s)
}

// ParseBool resolves a boolean token. It accepts 1/0, true/false, yes/no,
// on/off, and any non-zero/zero integer. Surrounding spaces are ignored.
func ParseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n != 0, nil
	}
	return false, fmt.Errorf("want 0 or 1, got %q", s)
}

// ParseAttribute parses a CSS-like attribute spec into an Attribute. See the
// package-level syntax description above. An empty (or all-whitespace) spec
// yields the all-black default.
func ParseAttribute(s string) (Attribute, error) {
	a := Attribute{Ink: Black, Paper: Black}
	s = strings.TrimSpace(s)
	if s == "" {
		return a, nil
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}"))
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			return a, fmt.Errorf("attribute: %q is not key:value", part)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		switch k {
		case "ink":
			c, err := ParseColour(v)
			if err != nil {
				return a, fmt.Errorf("attribute ink: %w", err)
			}
			a.Ink = c
		case "paper":
			c, err := ParseColour(v)
			if err != nil {
				return a, fmt.Errorf("attribute paper: %w", err)
			}
			a.Paper = c
		case "bright":
			b, err := ParseBool(v)
			if err != nil {
				return a, fmt.Errorf("attribute bright: %w", err)
			}
			a.Bright = b
		case "flash":
			b, err := ParseBool(v)
			if err != nil {
				return a, fmt.Errorf("attribute flash: %w", err)
			}
			a.Flash = b
		default:
			return a, fmt.Errorf("attribute: unknown field %q (want ink, paper, bright, flash)", k)
		}
	}
	return a, nil
}

// String renders an Attribute back to its CSS-like form, the inverse of
// ParseAttribute. Colours are emitted as names. Useful for round-tripping and
// for echoing a parsed attribute back to a user.
func (a Attribute) String() string {
	name := func(i uint8) string {
		for n, idx := range colourNames {
			if idx == i&0x07 {
				return n
			}
		}
		return strconv.Itoa(int(i & 0x07))
	}
	b := 0
	if a.Bright {
		b = 1
	}
	f := 0
	if a.Flash {
		f = 1
	}
	return fmt.Sprintf("ink:%s; paper:%s; bright:%d; flash:%d",
		name(a.Ink), name(a.Paper), b, f)
}
