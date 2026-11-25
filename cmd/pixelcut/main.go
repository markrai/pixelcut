package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/markrai/pixelcut/internal/media"
)

const usage = `PixelCut - source-aware video editing foundation

Usage:
  pixelcut inspect INPUT
  pixelcut remux INPUT OUTPUT.mp4
  pixelcut trim -start SECONDS -duration SECONDS INPUT OUTPUT.mp4
  pixelcut crop -x X -y Y -width W -height H INPUT OUTPUT.mp4
  pixelcut preview [-hdr] INPUT

Trim and crop currently support only explicitly tagged 10-bit HLG footage.
Preview requires mpv. Export requires FFmpeg with libx265.`

func run(ctx context.Context, args []string, stdout io.Writer) error {
	e := media.Default()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, usage)
		return nil
	}
	switch args[0] {
	case "inspect":
		if len(args) != 2 {
			return errors.New("usage: pixelcut inspect INPUT")
		}
		r, err := e.Inspect(ctx, args[1])
		if err != nil {
			return err
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "remux":
		if len(args) != 3 {
			return errors.New("usage: pixelcut remux INPUT OUTPUT.mp4")
		}
		return e.Remux(ctx, args[1], args[2])
	case "trim", "crop", "preview":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		fs.SetOutput(stdout)
		var edit media.Edit
		var hdr bool
		switch args[0] {
		case "trim":
			edit.Trim = true
			fs.Float64Var(&edit.Start, "start", 0, "start in seconds")
			fs.Float64Var(&edit.Duration, "duration", 0, "duration in seconds")
		case "crop":
			edit.Crop = &media.Crop{}
			fs.IntVar(&edit.Crop.X, "x", 0, "left edge")
			fs.IntVar(&edit.Crop.Y, "y", 0, "top edge")
			fs.IntVar(&edit.Crop.Width, "width", 0, "crop width")
			fs.IntVar(&edit.Crop.Height, "height", 0, "crop height")
		case "preview":
			fs.BoolVar(&hdr, "hdr", false, "request native HDR output on a compatible display")
		}
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if args[0] == "preview" {
			if fs.NArg() != 1 {
				return errors.New("preview requires one input")
			}
			return e.Preview(ctx, fs.Arg(0), hdr)
		}
		if fs.NArg() != 2 {
			return errors.New("expected INPUT OUTPUT.mp4 after flags")
		}
		return e.Render(ctx, fs.Arg(0), fs.Arg(1), edit)
	default:
		return fmt.Errorf("unknown command %q; use pixelcut help", args[0])
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pixelcut:", err)
		os.Exit(1)
	}
}
