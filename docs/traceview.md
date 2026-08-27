# package traceview

`import "github.com/arshnah/detsim/traceview"`

`traceview` renders `rt` traces for humans. A raw trace file is a seed and a list of goroutine ids,
which is exactly what a machine needs to replay a failure and exactly what a person needs to
understand exactly none of it. This package turns those ids back into a story: which goroutine ran,
when, and what it blocked on when its turn ended.

It's the engine behind `cmd/detsim-trace`, but it's plain functions over `rt.Trace` writing to an
`io.Writer`, so it's usable directly and fully testable without a terminal.

## Where the readable part comes from

A trace only carries names and block reasons if the run recorded them. `rt.GoNamed("producer", fn)`
attaches a label that survives into `Trace()` and `SaveTrace`; unnamed goroutines render as `g0`,
`g1`, and so on. The per-decision detail (virtual time, what the goroutine did next) is recorded
automatically for every run; traces saved before this existed simply render without it.

## `func View(w io.Writer, tr rt.Trace, opt Options) error`

The annotated decision timeline, one line per scheduling decision:

```
seed 847291 · 5 decisions · 3 goroutines · t=0 → 500ns

  #1 consumer blocked on chan recv  at t=0s
  #2 watchdog blocked on sleep  at t=0s
  #3 producer ran to completion  at t=0s
  #4 consumer blocked on chan send  at t=0s
  #5 watchdog ran to completion  at t=500ns
```

`Options.Limit` caps output at the last N decisions (with a note saying how many were skipped),
which is what you want on a 2,000-decision trace where the interesting part is usually the end.

## `func Diff(w io.Writer, orig, min rt.Trace, opt Options) error`

The minimizer's report card. Every decision in the original trace is marked `keep` or `drop`
depending on whether the minimized trace contains it, so you can see the skeleton ddmin decided was
actually load-bearing for the failure. A minimized trace should be a subsequence of the original;
if lenient replay made it drift out of alignment, `Diff` says so and prints both shapes instead of
drawing a misleading comparison.

## `func Stats(w io.Writer, tr rt.Trace, opt Options) error`

Per-goroutine pick counts ordered busiest-first, a block-reason histogram per goroutine
(`chan send×2, finished×1`), and the biggest virtual-time jumps in the run, which is usually where
the scenario's sleeps and timeouts live.

## Color

`Options.Color` gates ANSI escapes. `cmd/detsim-trace` defaults it to on for terminals and off
when stdout is piped, with `NO_COLOR` respected, but the package itself never guesses: it does
exactly what the option says.
