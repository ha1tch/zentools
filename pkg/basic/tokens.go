// Package basic tokenises and detokenises Sinclair BASIC programs.
//
// It converts between human-readable BASIC source (one line per text line,
// each beginning with a line number) and the on-disk tokenised form used by the
// Spectrum: a sequence of lines, each
//
//	[line number: 2 bytes big-endian][length: 2 bytes little-endian][text...][0x0D]
//
// Keyword tokens occupy 0xA3-0xFF; 0xA3 (SPECTRUM) and 0xA4 (PLAY) are 128K-only.
// Numeric constants appear as their visible ASCII digits followed by a 0x0E
// marker and a 5-byte binary value.
//
// Scope: this covers what loaders and ordinary hand-written BASIC need -
// keywords, integer numeric constants in 0..65535, string literals, and REM
// comments. A leading minus on a number is the subtraction operator (stored as a
// literal '-' followed by a positive number), matching the ROM; there is no
// signed single-number form. Floating-point literals, DEF FN calculator slots,
// and embedded colour-control argument bytes are not produced.
//
// This package depends only on the standard library.
package basic

// basicTokens maps Spectrum BASIC keyword token bytes (0xA3-0xFF) to their text.
// 0xA3 (SPECTRUM) and 0xA4 (PLAY) are 128K-only; the rest are common to 48K/128K.
// Verified against the ROM token order (final code = main code + 0xA5) and the
// tokenised-file format reference.
var basicTokens = map[byte]string{
	0xA3: "SPECTRUM", 0xA4: "PLAY", 0xA5: "RND", 0xA6: "INKEY$", 0xA7: "PI",
	0xA8: "FN", 0xA9: "POINT", 0xAA: "SCREEN$", 0xAB: "ATTR", 0xAC: "AT",
	0xAD: "TAB", 0xAE: "VAL$", 0xAF: "CODE", 0xB0: "VAL", 0xB1: "LEN",
	0xB2: "SIN", 0xB3: "COS", 0xB4: "TAN", 0xB5: "ASN", 0xB6: "ACS",
	0xB7: "ATN", 0xB8: "LN", 0xB9: "EXP", 0xBA: "INT", 0xBB: "SQR",
	0xBC: "SGN", 0xBD: "ABS", 0xBE: "PEEK", 0xBF: "IN", 0xC0: "USR",
	0xC1: "STR$", 0xC2: "CHR$", 0xC3: "NOT", 0xC4: "BIN", 0xC5: "OR",
	0xC6: "AND", 0xC7: "<=", 0xC8: ">=", 0xC9: "<>", 0xCA: "LINE",
	0xCB: "THEN", 0xCC: "TO", 0xCD: "STEP", 0xCE: "DEF FN", 0xCF: "CAT",
	0xD0: "FORMAT", 0xD1: "MOVE", 0xD2: "ERASE", 0xD3: "OPEN #", 0xD4: "CLOSE #",
	0xD5: "MERGE", 0xD6: "VERIFY", 0xD7: "BEEP", 0xD8: "CIRCLE", 0xD9: "INK",
	0xDA: "PAPER", 0xDB: "FLASH", 0xDC: "BRIGHT", 0xDD: "INVERSE", 0xDE: "OVER",
	0xDF: "OUT", 0xE0: "LPRINT", 0xE1: "LLIST", 0xE2: "STOP", 0xE3: "READ",
	0xE4: "DATA", 0xE5: "RESTORE", 0xE6: "NEW", 0xE7: "BORDER", 0xE8: "CONTINUE",
	0xE9: "DIM", 0xEA: "REM", 0xEB: "FOR", 0xEC: "GO TO", 0xED: "GO SUB",
	0xEE: "INPUT", 0xEF: "LOAD", 0xF0: "LIST", 0xF1: "LET", 0xF2: "PAUSE",
	0xF3: "NEXT", 0xF4: "POKE", 0xF5: "PRINT", 0xF6: "PLOT", 0xF7: "RUN",
	0xF8: "SAVE", 0xF9: "RANDOMIZE", 0xFA: "IF", 0xFB: "CLS", 0xFC: "DRAW",
	0xFD: "CLEAR", 0xFE: "RETURN", 0xFF: "COPY",
}

// remToken is the token byte for REM; text after it on a line is literal.
const remToken = 0xEA

// numberMarker precedes the 5-byte binary value of a numeric constant.
const numberMarker = 0x0E
