// Command tap2tzx converts one or more TAP files into a single TZX image.
//
// It is a drop-in replacement for the zxgotools tool of the same name, with one
// deliberate change: the configuration file (-c) is JSON rather than YAML, so
// that the implementation — like the rest of zentools — needs no third-party
// dependencies. The command-line flags are otherwise unchanged.
//
// Usage:
//
//	tap2tzx -o out.tzx [options] input1.tap [input2.tap ...]
//	tap2tzx -o out.tzx -c config.json
//
// Flags:
//
//	-o          output TZX file (required)
//	-c          JSON configuration file
//	-p          pause between blocks in ms (default 1000)
//	-m          add a metadata (archive-info) block
//	-title      program title (with -m)
//	-author     program author (with -m)
//	-year       publication year (with -m)
//	-128        program requires 128K
//	-ay         program uses the AY sound chip
//	-paging     program uses memory paging
//	-model      required model: +2, +2A, or +3
//	-multiload  program is multiload (adds 48K stop blocks between files)
//	-group      group name for the input files
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ha1tch/zentools/pkg/tzx"
)

type options struct {
	output     string
	configFile string
	pause      uint
	addArchive bool
	title      string
	author     string
	year       string
	k128Only   bool
	useAY      bool
	usePaging  bool
	modelType  string
	multiload  bool
	group      string
}

// jsonConfig mirrors the zxgotools YAML config schema, expressed as JSON.
type jsonConfig struct {
	Metadata struct {
		Title  string `json:"title"`
		Author string `json:"author"`
		Year   string `json:"year"`
	} `json:"metadata"`
	Hardware struct {
		K128Only bool   `json:"128k_only"`
		UseAY    bool   `json:"use_ay"`
		Model    string `json:"model"`
	} `json:"hardware"`
	Blocks []struct {
		Group string `json:"group"`
		File  string `json:"file"`
		Desc  string `json:"desc"`
	} `json:"blocks"`
}

func main() {
	opts := parseFlags()

	w := tzx.NewWriter(uint16(opts.pause))

	if opts.configFile != "" {
		if err := buildFromConfig(w, opts.configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing config file: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := buildFromFlags(w, opts, flag.Args()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(opts.output, w.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully created %s\n", opts.output)
}

func parseFlags() *options {
	opts := &options{}
	flag.StringVar(&opts.output, "o", "", "output TZX file (required)")
	flag.StringVar(&opts.configFile, "c", "", "JSON configuration file")
	flag.UintVar(&opts.pause, "p", 1000, "pause duration between blocks in ms")
	flag.BoolVar(&opts.addArchive, "m", false, "add metadata block")
	flag.StringVar(&opts.title, "title", "", "program title (requires -m)")
	flag.StringVar(&opts.author, "author", "", "program author (requires -m)")
	flag.StringVar(&opts.year, "year", "", "publication year (requires -m)")
	flag.BoolVar(&opts.k128Only, "128", false, "program requires 128K")
	flag.BoolVar(&opts.useAY, "ay", false, "program uses AY sound chip")
	flag.BoolVar(&opts.usePaging, "paging", false, "program uses memory paging")
	flag.StringVar(&opts.modelType, "model", "", "required model: +2, +2A, or +3")
	flag.BoolVar(&opts.multiload, "multiload", false, "program is multiload (adds 48K stop blocks)")
	flag.StringVar(&opts.group, "group", "", "group name for following files")
	flag.Parse()

	if opts.output == "" {
		fmt.Fprintf(os.Stderr, "Error: output file (-o) is required\n\n")
		flag.Usage()
		os.Exit(1)
	}
	if flag.NArg() == 0 && opts.configFile == "" {
		fmt.Fprintf(os.Stderr, "Error: no input files or config file specified\n\n")
		flag.Usage()
		os.Exit(1)
	}
	return opts
}

// buildFromFlags assembles the TZX from command-line inputs and flags.
func buildFromFlags(w *tzx.Writer, opts *options, files []string) error {
	if opts.addArchive {
		w.ArchiveInfo(tzx.EncodeOptions{
			Title:  opts.title,
			Author: opts.author,
			Year:   opts.year,
		})
	}
	if hw := hardwareFromFlags(opts); len(hw) > 0 {
		w.Hardware(hw)
	}

	grouped := opts.group != ""
	if grouped {
		w.GroupStart(opts.group)
	}
	for i, file := range files {
		tapImage, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}
		if err := w.AddTAP(tapImage); err != nil {
			return fmt.Errorf("processing %s: %w", file, err)
		}
		// A multiload tape stops between parts in 48K mode, but not after the
		// last part.
		if opts.multiload && i < len(files)-1 {
			w.StopIn48K()
		}
	}
	if grouped {
		w.GroupEnd()
	}
	return nil
}

// buildFromConfig assembles the TZX from a JSON config file.
func buildFromConfig(w *tzx.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	var cfg jsonConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parsing JSON config: %w", err)
	}

	if cfg.Metadata.Title != "" || cfg.Metadata.Author != "" || cfg.Metadata.Year != "" {
		w.ArchiveInfo(tzx.EncodeOptions{
			Title:  cfg.Metadata.Title,
			Author: cfg.Metadata.Author,
			Year:   cfg.Metadata.Year,
		})
	}
	if hw := hardwareFromConfig(&cfg); len(hw) > 0 {
		w.Hardware(hw)
	}

	// Each block may open a new named group; an open group is closed when the
	// next group opens or at the end. Groups cannot nest, matching the spec.
	openGroup := false
	for _, block := range cfg.Blocks {
		if block.Group != "" {
			if openGroup {
				w.GroupEnd()
			}
			w.GroupStart(block.Group)
			openGroup = true
		}
		if block.Desc != "" {
			w.Description(block.Desc)
		}
		if block.File != "" {
			tapImage, err := os.ReadFile(block.File)
			if err != nil {
				return fmt.Errorf("reading %s: %w", block.File, err)
			}
			if err := w.AddTAP(tapImage); err != nil {
				return fmt.Errorf("processing %s: %w", block.File, err)
			}
		}
	}
	if openGroup {
		w.GroupEnd()
	}
	return nil
}

// hardwareFromFlags maps the 128K/AY/model flags to hardware-type entries.
func hardwareFromFlags(opts *options) []tzx.HardwareInfo {
	return hardwareEntries(opts.k128Only, opts.useAY, opts.modelType)
}

func hardwareFromConfig(cfg *jsonConfig) []tzx.HardwareInfo {
	return hardwareEntries(cfg.Hardware.K128Only, cfg.Hardware.UseAY, cfg.Hardware.Model)
}

// hardwareEntries builds the 0x33 entries for the given hardware requirements.
// A 128K-only program is recorded as using the 128K and not running on 48K. A
// model selects the specific 128K variant ID.
func hardwareEntries(k128Only, useAY bool, model string) []tzx.HardwareInfo {
	var entries []tzx.HardwareInfo
	if k128Only {
		id := byte(tzx.HWIDSpectrum128K)
		switch model {
		case "+2":
			id = tzx.HWIDSpectrum128KPlus2
		case "+2A", "+3":
			id = tzx.HWIDSpectrumPlus2APlus3
		}
		entries = append(entries,
			tzx.HardwareInfo{Type: tzx.HWComputers, ID: id, Info: tzx.HWInfoUses},
			tzx.HardwareInfo{Type: tzx.HWComputers, ID: tzx.HWIDSpectrum48K, Info: tzx.HWInfoDoesntRun},
		)
	} else if model != "" {
		id := byte(tzx.HWIDSpectrum128K)
		switch model {
		case "+2":
			id = tzx.HWIDSpectrum128KPlus2
		case "+2A", "+3":
			id = tzx.HWIDSpectrumPlus2APlus3
		}
		entries = append(entries,
			tzx.HardwareInfo{Type: tzx.HWComputers, ID: id, Info: tzx.HWInfoUses},
		)
	}
	if useAY {
		entries = append(entries,
			tzx.HardwareInfo{Type: tzx.HWSoundDevices, ID: tzx.HWIDClassicAY, Info: tzx.HWInfoUses},
		)
	}
	return entries
}
