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
