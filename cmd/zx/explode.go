// file: explode.go
//
// `zx convert <tape> --outdir <dir>` is a different mode from the rest of
// convert.go: not one format converted to another, but one multi-block tape
// (tap/tzx/pzx) exploded into a directory of individual files -- one .bin
// per data block, plus a manifest.json describing every block, header and
// data alike, in file order.
//
// A header's own raw bytes are never written out as a .bin: tap.Block's own
// doc comment is explicit that for a header, Data holds the same content
// already parsed into Type/Name/DataLength/Param1/Param2 -- writing it out
// again as a file would just be redundant with the manifest entry. Only a
// data block's Data is real payload worth its own file.
//
// A data block is attributed to the header immediately before it (index
// i-1) -- the same adjacency convention convert.go's own dataFollowing/
// selectBlockPayload already use -- and left unattributed (no header_index/
// header_name in its manifest entry) when there isn't one: real tapes have
// bare, headerless data blocks (confirmed against an actual commercial
// release's structure, not assumed -- see pkg/load's own history in the
// sibling zendis project, which hit the same real-world shape), and this
// explode mode has to represent that faithfully rather than guess at a
// header that isn't there.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/zentools/pkg/tap"
)

// blockManifestEntry is one block's entry in manifest.json. Kind is
// "header" or "data"; the header-only and data-only fields below are
// omitted (via omitempty) on whichever entries they don't apply to,
// rather than the manifest carrying two entirely separate array types.
type blockManifestEntry struct {
	Index      int    `json:"index"`
	Kind       string `json:"kind"`
	Flag       byte   `json:"flag"`
	ChecksumOK bool   `json:"checksum_ok"`

	// Header fields (kind == "header"): tap.Block's own parsed content,
	// not re-derived here.
	Type       *byte   `json:"type,omitempty"`
	TypeName   string  `json:"type_name,omitempty"`
	Name       string  `json:"name,omitempty"`
	DataLength *uint16 `json:"data_length,omitempty"`
	Param1     *uint16 `json:"param1,omitempty"`
	Param2     *uint16 `json:"param2,omitempty"`

	// Data fields (kind == "data").
	Length      int    `json:"length,omitempty"`
	File        string `json:"file,omitempty"`
	HeaderIndex *int   `json:"header_index,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
}

type tapeManifest struct {
	Source       string               `json:"source"`
	SourceFormat string               `json:"source_format"`
	Blocks       []blockManifestEntry `json:"blocks"`
}

// explodeTape decodes a tap/tzx/pzx source into its full block list (via
// tapeBlocksFor, the same normalisation convertTapeToBin already uses) and
// writes it out as outdir/manifest.json plus one outdir/NNN[-name].bin per
// data block.
func explodeTape(data []byte, src, inputPath, outdir string) error {
	blocks, err := tapeBlocksFor(data, src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", outdir, err)
	}

	m := tapeManifest{Source: filepath.Base(inputPath), SourceFormat: src}
	pendingHeaderIdx := -1
	pendingHeaderName := ""

	for i, b := range blocks {
		if b.IsHeader {
			typ, dataLen, p1, p2 := b.Type, b.DataLength, b.Param1, b.Param2
			m.Blocks = append(m.Blocks, blockManifestEntry{
				Index: i, Kind: "header", Flag: b.Flag, ChecksumOK: b.ChecksumOK,
				Type: &typ, TypeName: tapTypeName(b.Type), Name: b.Name,
				DataLength: &dataLen, Param1: &p1, Param2: &p2,
			})
			pendingHeaderIdx = i
			pendingHeaderName = b.Name
			continue
		}

		fname := fmt.Sprintf("%03d.bin", i)
		entry := blockManifestEntry{Index: i, Kind: "data", Flag: b.Flag, ChecksumOK: b.ChecksumOK, Length: len(b.Data)}
		if pendingHeaderIdx == i-1 && pendingHeaderIdx >= 0 {
			if safe := sanitizeFilename(pendingHeaderName); safe != "" {
				fname = fmt.Sprintf("%03d-%s.bin", i, safe)
			}
			idx := pendingHeaderIdx
			entry.HeaderIndex = &idx
			entry.HeaderName = pendingHeaderName
		}
		entry.File = fname
		if err := os.WriteFile(filepath.Join(outdir, fname), b.Data, 0o644); err != nil {
			return err
		}
		m.Blocks = append(m.Blocks, entry)
		pendingHeaderIdx = -1 // consumed (or never set) -- either way, not pending for the next block
	}

	manifestPath := filepath.Join(outdir, "manifest.json")
	f, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	encErr := enc.Encode(m)
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	if closeErr != nil {
		return closeErr
	}

	dataBlocks := 0
	for _, b := range blocks {
		if !b.IsHeader {
			dataBlocks++
		}
	}
	fmt.Printf("Wrote %d data block(s) and manifest.json to %s\n", dataBlocks, strings.TrimRight(outdir, "/")+"/")
	return nil
}

func tapTypeName(t byte) string {
	switch t {
	case tap.TypeProgram:
		return "Program"
	case tap.TypeNumArray:
		return "NumberArray"
	case tap.TypeCharArray:
		return "CharArray"
	case tap.TypeCode:
		return "Code"
	}
	return fmt.Sprintf("Unknown(%d)", t)
}

// sanitizeFilename keeps a header name usable as a filename component:
// alphanumerics and -/_ pass through, spaces become underscores (a
// standard Spectrum tape name is 10 characters, space-padded, and often
// contains internal spaces too), anything else is dropped rather than
// risking a path-unsafe or misleading character.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}
