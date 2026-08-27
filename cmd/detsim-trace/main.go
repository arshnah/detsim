// Command detsim-trace reads the trace files detsim-test and rt.DumpTraceOnFailure
// write, and renders them for humans: an annotated timeline of every scheduling
// decision, per-goroutine stats, and a keep/drop diff between an original and a
// minimized trace.
//
//	view    detsim-trace view detsim_minimized_trace.json
//	stats   detsim-trace stats detsim_failure_trace.json
//	diff    detsim-trace diff detsim_failure_trace.json detsim_minimized_trace.json
//
// Traces are most readable when the code under test spawns goroutines through
// rt.GoNamed; unnamed goroutines show up as g0, g1, and so on.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/arshnah/detsim/rt"
	"github.com/arshnah/detsim/traceview"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	limit := fs.Int("limit", 0, "max decisions to print in view (0 = all)")
	color := fs.Bool("color", colorAuto(), "force ANSI color on or off")
	fs.Parse(os.Args[2:])

	opts := traceview.Options{Color: *color, Limit: *limit}

	switch os.Args[1] {
	case "view", "stats":
		if fs.NArg() != 1 {
			fmt.Fprintf(os.Stderr, "usage: detsim-trace %s <trace.json>\n", os.Args[1])
			return 2
		}
		tr, err := rt.LoadTrace(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if os.Args[1] == "view" {
			return emit(traceview.View(os.Stdout, tr, opts))
		}
		return emit(traceview.Stats(os.Stdout, tr, opts))

	case "diff":
		if fs.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "usage: detsim-trace diff <original.json> <minimized.json>")
			return 2
		}
		orig, err := rt.LoadTrace(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "original: %v\n", err)
			return 1
		}
		min, err := rt.LoadTrace(fs.Arg(1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "minimized: %v\n", err)
			return 1
		}
		return emit(traceview.Diff(os.Stdout, orig, min, opts))

	default:
		usage()
		return 2
	}
}

func emit(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// colorAuto turns color on only when stdout is a terminal and NO_COLOR isn't set, so
// piping into a file or pager stays clean.
func colorAuto() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: detsim-trace <command> [flags] <trace.json> [minimized.json]

commands:
  view   <trace.json>                     annotated decision timeline (-limit caps output)
  stats  <trace.json>                     per-goroutine picks, block reasons, time jumps
  diff   <original.json> <minimized.json> keep/drop view of what the minimizer kept

flags:
  -color      force ANSI color (default: on for terminals, off when piped)
  -limit N    cap view output at the last N decisions (0 = all)
`)
}
