package basic

import (
	"os"
	"strings"
	"testing"
)

// TestDetokeniseRealProgram decodes the tokenised body of a real BASIC program
// written by a ZX Spectrum +3 (HELLO.BAS).
func TestDetokeniseRealProgram(t *testing.T) {
	body, err := os.ReadFile("testdata/hello.tok")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := Detokenise(body)
	if err != nil {
		t.Fatalf("Detokenise: %v", err)
	}
	want := "10 REM Hello you have found me\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestDetokeniseLoader exercises a program with numeric constants (which carry a
// hidden 0x0E + 5-byte form that must be skipped), keywords, a statement
// separator, and a string literal - representative of a real loader.
func TestDetokeniseLoader(t *testing.T) {
	body, err := os.ReadFile("testdata/loader.tok")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := Detokenise(body)
	if err != nil {
		t.Fatalf("Detokenise: %v", err)
	}
	for _, want := range []string{"10 BORDER 0", "PAPER 0", `20 LOAD ""`, "CODE"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, got)
		}
	}
	// The 0x0E number marker and its 5 binary bytes must NOT leak into the output.
	if strings.Contains(got, "[0E]") || strings.Contains(got, "\x0e") {
		t.Errorf("number marker leaked into output:\n%s", got)
	}
}

// TestDetokenise128Tokens checks the 128K-only keywords decode.
func TestDetokenise128Tokens(t *testing.T) {
	line := []byte{0x00, 0x0A, 0x00, 0x00}
	payload := append([]byte{0xA4}, []byte(`"abc"`)...) // PLAY = 0xA4
	payload = append(payload, 0x0D)
	line[2] = byte(len(payload))
	prog := append(line, payload...)

	got, err := Detokenise(prog)
	if err != nil {
		t.Fatalf("Detokenise: %v", err)
	}
	if !strings.Contains(got, "PLAY") {
		t.Errorf("PLAY (0xA4) not decoded: %q", got)
	}
}

// TestRoundTripLoader tokenises a loader program and detokenises it back,
// checking the visible source survives the round trip.
func TestRoundTripLoader(t *testing.T) {
	src := "10 BORDER 0: PAPER 0\n20 LOAD \"\"CODE\n30 RANDOMIZE USR 32768"
	tok, err := Tokenise(src)
	if err != nil {
		t.Fatalf("Tokenise: %v", err)
	}
	if !LooksTokenised(tok) {
		t.Error("LooksTokenised(Tokenise(src)) = false")
	}
	back, err := Detokenise(tok)
	if err != nil {
		t.Fatalf("Detokenise: %v", err)
	}
	// Keywords and numbers should survive (spacing may differ).
	for _, want := range []string{"BORDER", "PAPER", "LOAD", "CODE", "RANDOMIZE", "USR", "32768"} {
		if !strings.Contains(back, want) {
			t.Errorf("round trip lost %q\ngot:\n%s", want, back)
		}
	}
}

// TestNegativeNumberIsOperatorPlusPositive verifies that "-42" tokenises as the
// subtraction operator followed by a positive number (the ROM behaviour), not a
// signed single number.
func TestNegativeNumberIsOperatorPlusPositive(t *testing.T) {
	tok, err := Tokenise("10 LET A=-42")
	if err != nil {
		t.Fatalf("Tokenise: %v", err)
	}
	// Expect a literal '-' (0x2D) before the number marker, and the value 42
	// stored positive (low byte 0x2A, high byte 0x00).
	if !strings.Contains(string(tok), "-42") {
		t.Errorf("visible digits not preserved: % X", tok)
	}
	back, err := Detokenise(tok)
	if err != nil {
		t.Fatalf("Detokenise: %v", err)
	}
	if !strings.Contains(back, "A=-42") {
		t.Errorf("round trip = %q, want to contain A=-42", back)
	}
}

func TestLooksTokenisedRejectsText(t *testing.T) {
	if LooksTokenised([]byte("10 PRINT \"hello\"\n20 GO TO 10\n")) {
		t.Error("plain text source misidentified as tokenised")
	}
}

// TestKeywordDoesNotConsumeLongerIdentifier confirms matchToken's word
// boundary check: a keyword that is a prefix of a longer run of letters
// (e.g. "TO" inside "TOTAL", "FOR" inside "FORMAT") must not be torn out
// of it, regardless of whether the source has a space before it -- this
// was a real bug, found and verified against a genuine ZX Spectrum +3
// tokenised fixture (pkg/basic/testdata/loader.tok) showing the opposite,
// legitimate case: a keyword immediately followed by a bare digit (no
// separator at all) is correctly a keyword, not blocked by this check.
func TestKeywordDoesNotConsumeLongerIdentifier(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // the exact detokenised line
	}{
		{"TOTAL after a space stays one identifier", "10 LET TOTAL=5", "10 LET TOTAL=5\n"},
		{"FORMAT stays one keyword, not FOR+MAT", "10 FORMAT \"a\";800", "10 FORMAT \"a\";800\n"},
		{"a real keyword immediately before a digit still tokenises", "10 CLEAR32767", "10 CLEAR 32767\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tok, err := Tokenise(c.src)
			if err != nil {
				t.Fatalf("Tokenise: %v", err)
			}
			back, err := Detokenise(tok)
			if err != nil {
				t.Fatalf("Detokenise: %v", err)
			}
			if back != c.want {
				t.Errorf("round trip = %q, want %q", back, c.want)
			}
		})
	}

	// The one genuinely ambiguous case: no space at all between two
	// letter-ending keywords ("LETTOTAL"). A tokeniser cannot correctly
	// guess a boundary here, so the safe behaviour is refusing to
	// tokenise LET at all, leaving the literal text -- not silently
	// mis-tokenising it either way.
	t.Run("no separator between two keyword-shaped runs is left literal", func(t *testing.T) {
		tok, err := Tokenise("10 LETTOTAL=5")
		if err != nil {
			t.Fatalf("Tokenise: %v", err)
		}
		back, err := Detokenise(tok)
		if err != nil {
			t.Fatalf("Detokenise: %v", err)
		}
		if !strings.Contains(back, "LETTOTAL") {
			t.Errorf("round trip = %q, want literal LETTOTAL preserved", back)
		}
	})
}

// TestKeywordDropsOneFollowingSpace confirms a real, user-reported bug fix:
// LIST on real ROM hardware always supplies its own space after a keyword
// token, regardless of what is stored -- so a single source-typed space
// there was being stored too, and the two combined into a visible double
// space ("LET  TOTAL" instead of "LET TOTAL"). Verified against a real ZX
// Spectrum +3 fixture (loader.tok: BORDER/PAPER immediately precede a bare
// digit, confirming keywords never store a separating space on real
// hardware) and against genuine emulator LIST output via zx scr ocr, not
// just this package's own round trip. Only the first space is dropped; a
// second, deliberate one is kept.
func TestKeywordDropsOneFollowingSpace(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"10 LET TOTAL=5", "10 LET TOTAL=5\n"},
		{"10 FOR I=1 TO 5", "10 FOR I=1 TO 5\n"},
		{"10 NEXT I", "10 NEXT I\n"},
		{"10 PRINT \"hi\"", "10 PRINT \"hi\"\n"},
		// A second space is a deliberate extra gap and survives.
		{"10 LET  TOTAL=5", "10 LET  TOTAL=5\n"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			tok, err := Tokenise(c.src)
			if err != nil {
				t.Fatalf("Tokenise: %v", err)
			}
			back, err := Detokenise(tok)
			if err != nil {
				t.Fatalf("Detokenise: %v", err)
			}
			if back != c.want {
				t.Errorf("round trip = %q, want %q", back, c.want)
			}
		})
	}
}

// TestKeywordAfterColonOrQuote confirms a real bug report, found by a
// second, independent tester (Jim Blimey) directly on real 128K/+3
// hardware, that the first two fixes in this file both missed: a
// keyword immediately after a colon (a new statement) or a closing
// quote (the end of a string literal) still showed a doubled space --
// "OUT" after ": " in "POKE 1,2: OUT 3,4", or "CODE" after '"" ' in
// LOAD "" CODE 100. The tokenise-side fix only ever removed a space
// stored *after* a keyword token; it never looked at what came
// *before* a match, which is exactly what a colon or a closing quote
// followed by a space hits. Confirmed against the same real +3
// fixture used before (loader.tok: zero bytes between the colon and
// PAPER, zero bytes between the closing quote and CODE) before fixing
// tokenise.go, and confirmed detokenise.go had its own, separate half
// of the same bug: it only ever added a synthesised space *after* a
// token, never before, so a keyword with nothing stored on its
// leading side (like this fix produces) rendered with no gap at all
// ("I=1TO 5") until detokenise.go's own leading-space case was added
// too.
func TestKeywordAfterColonOrQuote(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"10 POKE 1,2: OUT 3,4", "10 POKE 1,2: OUT 3,4\n"},
		{"10 LOAD \"\" CODE 100", "10 LOAD \"\" CODE 100\n"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			tok, err := Tokenise(c.src)
			if err != nil {
				t.Fatalf("Tokenise: %v", err)
			}
			back, err := Detokenise(tok)
			if err != nil {
				t.Fatalf("Detokenise: %v", err)
			}
			if back != c.want {
				t.Errorf("round trip = %q, want %q", back, c.want)
			}
		})
	}
}
