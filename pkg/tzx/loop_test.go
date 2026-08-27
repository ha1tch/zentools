package tzx

import "testing"

// TestDecode_LoopExpandsBodyExactly is the regression/verification test
// for 0x24/0x25 (Loop Start/Loop End) -- the one remaining block pair
// with genuine control-flow semantics SpecIde itself implements (unlike
// 0x23/0x26/0x27, which are stubs even there; see skipblocks_test.go).
//
// Traced precisely through SpecIde's own logic (source/src/TZXFile.cc)
// rather than assumed from the format spec: Loop Start reads a u16 count
// and remembers the position right after its own header as loopStart.
// Loop End decrements the counter and, if it is still nonzero, jumps
// back to loopStart; only once the counter reaches zero does it fall
// through past its own header. Working through what that means for a
// count of 3: the loop body runs once before Loop End is ever reached,
// then Loop End's decrement-and-jump-back fires twice more (3 -> 2,
// jump; 2 -> 1, jump), and only on the third arrival (1 -> 0) does it
// stop jumping -- so the body executes exactly 3 times total, not 3
// extra times beyond an initial pass. This package expands the loop at
// decode time (Decode returns a flat slice; there is no explicit
// loop-start/loop-end marker in the output, just the body's own blocks,
// repeated) rather than emitting Block entries for the loop markers
// themselves, which carry no data of their own once expanded.
func TestDecode_LoopExpandsBodyExactly(t *testing.T) {
	data := tzxHeader10()

	data = append(data, idLoopStart)
	data = append(data, le16(3)...) // count = 3

	// Loop body: a single, distinguishable block (Group Start with a
	// 1-byte name "X", chosen because it is simple, has a variable but
	// easily-constructed length, and is unlikely to be confused with any
	// other marker in this test).
	data = append(data, idGroupStart, 1, 'X')

	data = append(data, idLoopEnd)

	// A block after the loop, to confirm pos correctly resumes ordinary,
	// non-looping processing once the loop is done -- not just that the
	// body repeated, but that decoding did not get stuck or miscounted.
	data = append(data, idGroupEnd)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Expect: 3 repetitions of the Group Start body block, then the
	// trailing Group End marker. 4 blocks total.
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks %+v, want 4 (3 repeated body blocks + 1 trailing marker)", len(blocks), blocks)
	}
	for i := 0; i < 3; i++ {
		if blocks[i].ID != idGroupStart {
			t.Errorf("block %d: ID = 0x%02X, want 0x%02X (idGroupStart, repeated loop body)", i, blocks[i].ID, idGroupStart)
		}
	}
	if blocks[3].ID != idGroupEnd {
		t.Errorf("block 3 (after the loop): ID = 0x%02X, want 0x%02X (idGroupEnd, the trailing marker)", blocks[3].ID, idGroupEnd)
	}
}

// TestDecode_LoopCountOne confirms the boundary case: a loop with count=1
// runs its body exactly once, not zero or two times -- an off-by-one in
// the counter direction would most likely surface here first.
func TestDecode_LoopCountOne(t *testing.T) {
	data := tzxHeader10()
	data = append(data, idLoopStart)
	data = append(data, le16(1)...)
	data = append(data, idGroupStart, 1, 'X')
	data = append(data, idLoopEnd)
	data = append(data, idGroupEnd)

	blocks, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks %+v, want 2 (1 body block + 1 trailing marker)", len(blocks), blocks)
	}
	if blocks[0].ID != idGroupStart || blocks[1].ID != idGroupEnd {
		t.Errorf("got %+v", blocks)
	}
}
