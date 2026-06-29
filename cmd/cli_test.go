package cmd_test

// Integration smoke tests for the command-line tools. Each test builds the
// tool, runs it on a small input, and checks the output is well-formed. These
// guard the CLI wiring; the underlying format correctness is covered by the
// package-level tests in pkg/tap, pkg/tzx, and pkg/basic.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildTool compiles a command into a temp binary and returns its path.
func buildTool(t *testing.T, name string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	// The test runs in the cmd/ directory; each tool is a subpackage of it.
	cmd := exec.Command("go", "build", "-o", bin, "./"+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", name, err, out)
	}
	return bin
}

func TestMaketapProducesTAP(t *testing.T) {
	bin := buildTool(t, "maketap")
	dir := t.TempDir()
	in := filepath.Join(dir, "code.bin")
	out := filepath.Join(dir, "code.tap")
	if err := os.WriteFile(in, []byte{0xF3, 0x76}, 0o644); err != nil {
		t.Fatal(err)
	}
	if o, err := exec.Command(bin, "--name", "test", "--address", "32768", in, out).CombinedOutput(); err != nil {
		t.Fatalf("maketap: %v\n%s", err, o)
	}
	data, err := os.ReadFile(out)
	if err != nil || len(data) == 0 {
		t.Fatalf("no tap output: %v", err)
	}
	// Header block: length 0x13, flag 0x00.
	if data[0] != 0x13 || data[2] != 0x00 {
		t.Errorf("unexpected tap header: % X", data[:3])
	}
}

func TestTotapBasicWorks(t *testing.T) {
	bin := buildTool(t, "totap")
	dir := t.TempDir()
	in := filepath.Join(dir, "prog.bas")
	out := filepath.Join(dir, "prog.tap")
	if err := os.WriteFile(in, []byte("10 PRINT 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// This is the path that hung in zxgotools; assert it completes and produces output.
	if o, err := exec.Command(bin, "--basic", "--name", "p", in, out).CombinedOutput(); err != nil {
		t.Fatalf("totap --basic: %v\n%s", err, o)
	}
	if data, err := os.ReadFile(out); err != nil || len(data) == 0 {
		t.Fatalf("no basic tap output: %v", err)
	}
}

func TestLoadtapRoundTrips(t *testing.T) {
	maketap := buildTool(t, "maketap")
	loadtap := buildTool(t, "loadtap")
	dir := t.TempDir()
	in := filepath.Join(dir, "c.bin")
	tapf := filepath.Join(dir, "c.tap")
	os.WriteFile(in, []byte{0x01, 0x02, 0x03}, 0o644)
	exec.Command(maketap, in, tapf).Run()

	o, err := exec.Command(loadtap, tapf).CombinedOutput()
	if err != nil {
		t.Fatalf("loadtap: %v\n%s", err, o)
	}
	if len(o) == 0 {
		t.Fatal("loadtap produced no analysis output")
	}
}

func TestTap2tzxProducesTZX(t *testing.T) {
	maketap := buildTool(t, "maketap")
	tap2tzx := buildTool(t, "tap2tzx")
	dir := t.TempDir()
	in := filepath.Join(dir, "c.bin")
	tapf := filepath.Join(dir, "c.tap")
	tzxf := filepath.Join(dir, "c.tzx")
	os.WriteFile(in, []byte{0x01, 0x02}, 0o644)
	exec.Command(maketap, in, tapf).Run()

	if o, err := exec.Command(tap2tzx, "-o", tzxf, "-m", "-title", "T", "-128", "-ay", tapf).CombinedOutput(); err != nil {
		t.Fatalf("tap2tzx: %v\n%s", err, o)
	}
	data, err := os.ReadFile(tzxf)
	if err != nil || string(data[:7]) != "ZXTape!" {
		t.Fatalf("not a TZX file: %v", err)
	}
}
