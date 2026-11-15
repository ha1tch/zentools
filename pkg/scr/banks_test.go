package scr

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func loadBanks(t *testing.T, name string) Banks {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "banks", name))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseBanksCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEncodeBanksMatchesSpecTools checks EncodeBanks against pictures the
// specification's tools encoded from the synthetic scene: 4x4 cells
// (zxfast.render_fast), cells 4 wide and 8 tall (zxcommon.render with
// cs=(8, 4), rows first) and a 64-colour bank
// with a penalty of 0.5.
func TestEncodeBanksMatchesSpecTools(t *testing.T) {
	scene := loadTestImage(t, filepath.Join("testdata", "zxmode", "scene.png"))
	for _, c := range []struct {
		banks, want string
		w, h        int
		penalty     float64
	}{
		{"zxpal2_banks.csv", "c4_zxpal2.png", 4, 4, RefPenalty},
		{"zxpal1_banks.csv", "c4x8_zxpal1.png", 4, 8, RefPenalty},
		{"z64_bank.csv", "c8_z64_p05.png", 8, 8, 0.5},
	} {
		got, chosen, err := EncodeBanks(scene, loadBanks(t, c.banks), BankOptions{CellW: c.w, CellH: c.h, Penalty: c.penalty})
		if err != nil {
			t.Fatal(err)
		}
		if len(chosen) != 384/c.h || len(chosen[0]) != 512/c.w {
			t.Errorf("%s: chosen banks are %dx%d", c.want, len(chosen[0]), len(chosen))
		}
		want := loadTestImage(t, filepath.Join("testdata", "banks", c.want))
		if n, max := channelDiffs(got, want); n != 0 {
			t.Errorf("%s: %d channel values differ (largest %d)", c.want, n, max)
		}
	}
}

// TestEncodeBanksIsTheReferenceEncoder checks that EncodeBanks over a
// palette's banks, with 8x8 cells and the reference penalty, draws what
// EncodePicture encodes.
func TestEncodeBanksIsTheReferenceEncoder(t *testing.T) {
	scene := loadTestImage(t, filepath.Join("testdata", "zxmode", "scene.png"))
	for p := ZXPAL0; p < NumPalettes; p++ {
		pic, err := EncodePicture(scene, ZXMODE2, p)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := EncodeBanks(scene, PaletteBanks(p, nil), BankOptions{Penalty: RefPenalty})
		if err != nil {
			t.Fatal(err)
		}
		if n, max := channelDiffs(got, pic.Render(false)); n != 0 {
			t.Errorf("%s: %d channel values differ (largest %d)", p, n, max)
		}
		// The written bank table reads back as the same banks.
		again, err := ParseBanksCSV(BanksCSV(PaletteBanks(p, nil)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(BanksCSV(again), BanksCSV(PaletteBanks(p, nil))) {
			t.Errorf("%s: bank table does not read back", p)
		}
	}
	if _, _, err := EncodeBanks(scene, PaletteBanks(ZXPAL0, nil), BankOptions{CellW: 6, CellH: 6}); err == nil {
		t.Error("6x6 cells on 512x384: no error")
	}
}

// TestFitRegisters checks that the fitted colours bring the training pixels
// nearer, and that a fit repeats exactly.
func TestFitRegisters(t *testing.T) {
	scene := loadTestImage(t, filepath.Join("testdata", "zxmode", "scene.png"))
	base := ZXPAL3.Registers()
	opt := FitRegistersOptions{Add: 8, PerPicture: 5000, Candidates: 300, Subsample: 3000, Iterations: 5}
	add, before, after, err := FitRegisters([]image.Image{scene}, base, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(add) != 8 {
		t.Fatalf("%d colours added", len(add))
	}
	if after.MeanDistance >= before.MeanDistance {
		t.Errorf("mean distance %.2f is not below %.2f", after.MeanDistance, before.MeanDistance)
	}
	again, _, _, err := FitRegisters([]image.Image{scene}, base, opt)
	if err != nil {
		t.Fatal(err)
	}
	for i := range add {
		if add[i] != again[i] {
			t.Errorf("second fit differs at colour %d: %v, %v", i, add[i], again[i])
		}
	}
}
