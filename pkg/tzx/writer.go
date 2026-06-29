// file: writer.go
//
// Writer assembles a TZX image block by block. EncodeFromTAP covers the common
// single-image case in one call; Writer is for callers that need to interleave
// several tape images with structural blocks — for example a tool that
// concatenates multiple TAP files into one TZX, bracketing them in a group and
// inserting "stop in 48K mode" blocks between multiload parts.
//
// A Writer starts with the TZX header already in place. Metadata blocks, if
// any, should be added first (the spec puts archive/hardware info ahead of the
// tape data), then the data and structural blocks in playback order.

package tzx

import "fmt"

// Writer builds a TZX image incrementally.
type Writer struct {
	buf   []byte
	pause uint16
}

// NewWriter returns a Writer with the TZX signature and version header written.
// pause is the inter-block pause in milliseconds applied to each data block; 0
// uses the standard default.
func NewWriter(pause uint16) *Writer {
	if pause == 0 {
		pause = defaultPause
	}
	return &Writer{buf: header(), pause: pause}
}

// ArchiveInfo appends a 0x32 archive-info block from the title/author/year in
// opts. It is a no-op if none are set.
func (w *Writer) ArchiveInfo(opts EncodeOptions) {
	if b := archiveInfoBlock(opts); b != nil {
		w.buf = append(w.buf, b...)
	}
}

// Description appends a 0x30 text-description block. No-op if desc is empty.
func (w *Writer) Description(desc string) {
	if b := textDescriptionBlock(desc); b != nil {
		w.buf = append(w.buf, b...)
	}
}

// Hardware appends a 0x33 hardware-type block. No-op if entries is empty.
func (w *Writer) Hardware(entries []HardwareInfo) {
	if b := hardwareTypeBlock(entries); b != nil {
		w.buf = append(w.buf, b...)
	}
}

// GroupStart appends a 0x21 group-start block. Groups cannot nest; the caller
// is responsible for pairing each GroupStart with a GroupEnd.
func (w *Writer) GroupStart(name string) {
	w.buf = append(w.buf, groupStartBlock(name)...)
}

// GroupEnd appends a 0x22 group-end block.
func (w *Writer) GroupEnd() {
	w.buf = append(w.buf, groupEndBlock()...)
}

// StopIn48K appends a 0x2A "stop the tape if in 48K mode" block.
func (w *Writer) StopIn48K() {
	w.buf = append(w.buf, idStopThe48K, 0, 0, 0, 0)
}

// AddTAP wraps every block of a TAP image as standard-speed (0x10) data blocks
// and appends them in order.
func (w *Writer) AddTAP(tapImage []byte) error {
	blocks, err := splitTAP(tapImage)
	if err != nil {
		return fmt.Errorf("splitting TAP image: %w", err)
	}
	for _, blk := range blocks {
		w.buf = append(w.buf, standardSpeedBlock(blk, w.pause)...)
	}
	return nil
}

// Bytes returns the assembled TZX image. The Writer may continue to be used
// after this call; the returned slice is a copy.
func (w *Writer) Bytes() []byte {
	out := make([]byte, len(w.buf))
	copy(out, w.buf)
	return out
}
