# package minimize

`import "github.com/arshnah/detsim/minimize"`

`minimize` implements delta-debugging (the `ddmin` algorithm) over a scheduler decision trace: given
a failing seed's full sequence of scheduling decisions and a way to check whether a given subsequence
still reproduces the failure, it shrinks that sequence down toward a minimal one that still fails,
so a real bug found by a 2,000-decision trace doesn't have to be debugged by reading 2,000 decisions.

## `func Ddmin(decisions []uint64, stillFails func([]uint64) bool) []uint64`

`decisions` is the full ordered trace (as produced by `rt.Sched.Trace()` / `rt.Trace.Decisions`).
`stillFails` is called with candidate subsequences and must report whether that subsequence, replayed,
still reproduces the original failure. In practice this means re-running the system under test with
`rt.NewSchedFromTraceLenient` seeded from the candidate and checking the same assertion failed.

`Ddmin` repeatedly tries removing contiguous chunks of the current candidate (starting by splitting
it into 2 chunks, then increasing the chunk count (shrinking chunk size) whenever a full pass
finds nothing removable), keeping any removal that `stillFails` still confirms, and stops once no
chunk size at all can remove anything further. `stillFails` is memoized internally on the exact
subsequence tried, so the identical candidate is never checked twice even across different chunk
sizes. It returns the smallest subsequence it found that still satisfies `stillFails`. On an
already-minimal input (`stillFails` is only ever true for the whole thing) it returns the input
unchanged.

This does not itself know anything about detsim, `rt`, or what "failure" means. `Ddmin` operates
purely on `[]uint64` and a boolean predicate, so it composes with any decision-trace-driven
scheduler in principle, not only `rt`.

## How `cmd/detsim-test` actually uses it

`cmd/detsim-test -minimize` re-runs the first failing seed to capture its full trace via
`rt.LoadTrace`, then calls `Ddmin` with a `stillFails` closure that: writes the candidate as a trace
file, re-invokes `go test` against the rewritten overlay with `rt.NewSchedFromTraceLenient` reading
that file (lenient because a truncated candidate trace legitimately runs out of recorded decisions
partway through and has to fall back to fresh randomness for anything past that point, rather than
erroring), and reports whether the same test still failed. The minimized result is saved back out via
`rt.SaveTrace` as `detsim_minimized_trace.json`, alongside a printed one-line command to replay it
directly.
