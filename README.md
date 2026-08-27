# detsim

Find distributed-system and concurrency bugs you can't reproduce, then reproduce them.

`detsim` runs your Go code, or a purpose-built scenario, through a deterministic virtual-time
scheduler that controls every goroutine interleaving, network fault, and storage fault. The same
seed always produces the exact same sequence of events. When something breaks, you get the seed
back and can replay the exact failure byte-for-byte, no "works on my machine," no flaky CI rerun.

This is the FoundationDB / TigerBeetle / Antithesis style of testing, exposed as a reusable Go
module instead of buried inside one company's database.

## Two ways to use it

**1. Point it at code you already have.** `detsim-test` source-rewrites a package via `go/ast`,
routing its goroutines, channels, `sync` primitives, `time`, `math/rand`, network, and file calls
through the deterministic scheduler, then compiles the rewrite as a Go build overlay, your files on
disk are never touched. It then sweeps seeds against your existing tests, looking for a failure:

```
$ go run ./cmd/detsim-test -dir=./myproject -pkg=./... -run=TestMyThing -trials=2000
1998/2000 seeds passed
failing seeds: [847291 1103982]
reproduce with: DETSIM_SEED=847291 go test -overlay=/tmp/detsim-rewrite-xxx/overlay.json -run=TestMyThing ./myproject
```

Add `-minimize` and a failing seed gets delta-debugged down to the smallest schedule that still
reproduces the bug, instead of handing you a 2000-decision trace to read through by hand. Add
`-novelty` and it stops sweeping by trial count and instead explores until distinct goroutine
schedule *shapes* stop appearing, which finds more real bugs per CPU-second than a flat seed sweep
on code with a lot of interleaving diversity and a little correctness risk.

When a failure does show up, `detsim-trace` turns the trace files into something you can actually
read. Spawn your goroutines with `rt.GoNamed` and every decision in the timeline gets a name and a
block reason instead of bare ids:

```
$ go run ./cmd/detsim-trace view detsim_minimized_trace.json
seed 847291 · 5 decisions · 3 goroutines · t=0 → 500ns

  #1 consumer blocked on chan recv  at t=0s
  #2 watchdog blocked on sleep  at t=0s
  #3 producer ran to completion  at t=0s
  #4 consumer blocked on chan send  at t=0s
  #5 watchdog ran to completion  at t=500ns
```

`detsim-trace diff original.json minimized.json` shows exactly which decisions the minimizer kept
(green) and dropped (dim), so you can see the skeleton it decided was load-bearing, and `stats`
gives per-goroutine pick counts, block-reason histograms, and the biggest virtual-time jumps.

**2. Write the scenario directly against the kernel.** For system-level testing (a Raft cluster, a
WAL-based store), write your test directly against `Sim`, `Network`, and `FaultyStorage`, the
lower-level, non-rewritten deterministic primitives. This is what `examples/raft` and
`examples/kv` do, and it's the harder but more precise path when you're testing a whole system
rather than a single package.

## What it actually catches

**Deadlocks, instantly, with a stack trace, not a CI timeout.** Two goroutines in a circular
channel wait get reported the moment the scheduler notices nothing is runnable:

```go
s.Go(func() { a.Send(1); b.Recv() })
s.Go(func() { b.Send(1); a.Recv() })

err := s.Run() // *rt.DeadlockError, both goroutines' blocked stacks attached, no wall-clock wait
```

**Rare interleavings, on purpose, not by luck.** A novelty search over a 4-worker job pool found
211 distinct goroutine schedule shapes across 1,005 trials in 0.09s of real time
(`examples/workerpool`, `TestNoveltySearchExploresDistinctSchedules`). A wall-clock-bound `go test
-race` loop would need to get unlucky 1,005 times to see the same diversity.

**Split-brain in a Raft cluster, across thousands of seeds, in seconds.**
`TestThousandsOfSeedsNoSplitBrain` runs 5,000 independently seeded 5-node clusters, each cycling
through normal operation, a network partition, and a heal, in about 7.5 seconds of real wall-clock
time, asserting no two leaders ever share a term. Any failing seed is exactly reproducible by
rerunning it (`TestSeedIsExactlyReproducible`).

**Why checksums matter, with an actual corrupted seed, not a claim.** `examples/kv` is a WAL-based
KV store with a checksum toggle. `TestNoChecksumStoreCanServeCorruptData` found the no-checksum
store serving corrupted data at seed 3 of 5,000; `TestChecksummedStoreNeverServesCorruptData`
confirms the checksummed version never does, under the identical fault profile, across 2,000 seeds.

## Try it

```
go run ./cmd/detsim -mode=raft -seed=1 -trials=5 -drop=0.15
go run ./cmd/detsim -mode=kv -seed=1 -trials=10 -checksums=false
go run ./cmd/detsim -mode=kv -seed=1 -trials=10 -checksums=true
go run ./cmd/detsim -mode=read -seed=1 -trials=10
go run ./cmd/detsim -mode=membership -seed=1 -trials=10

go run ./cmd/detsim-rewrite -dir=./yourpackage -pkg=. -print-overlay
go run ./cmd/detsim-test -dir=./yourpackage -pkg=. -run=TestX -trials=2000 -minimize

go run ./cmd/detsim-trace view detsim_minimized_trace.json
go run ./cmd/detsim-trace diff detsim_failure_trace.json detsim_minimized_trace.json
go run ./cmd/detsim-trace stats detsim_failure_trace.json
```

Sample output, no-checksum KV store hunting for a corrupted seed:

```
seed=1 ok, checksums=false, 2 keys recovered clean
seed=2 ok, checksums=false, 0 keys recovered clean
seed=3 CORRUPTED: key3 got "value-\xcc-3" want "value-3-3"
seed=4 ok, checksums=false, 0 keys recovered clean
seed=5 CORRUPTED: key3 got "val\x8ae-5-3" want "value-5-3"
```

## Benchmarks

All numbers are wall-clock time to simulate the described scenario in full, `go test -bench=.`:

| Scenario | Simulated | Real wall-clock |
| --- | --- | --- |
| Raft election + partition + heal cycle | 8s of simulated time | ~1.3ms |
| Raft: commit 100 log entries | — | ~1.1ms |
| Raft: snapshot compaction cycle | — | ~1.2ms |
| KV: put + sync + recover cycle | — | ~37µs |
| Raft: 5,000-seed no-split-brain sweep | 5,000 independent clusters | ~7.5s |
| Workerpool: novelty search, 1,005 trials, 211 distinct schedules | — | ~0.09s |

Simulated time is nearly free. That's the entire point, thousands of trials that would take hours
of real wall-clock time run in seconds, so you can actually afford to run them in CI on every push.

## Verification

- `go build ./...`, `go vet ./...`, and `gofmt -l .` are clean.
- The full suite passes under `-race`. That means something here specifically: the architecture is
  single-threaded per simulation by construction, and the race detector confirms that claim rather
  than just asserting it.
- `FuzzRecoverNeverPanics` feeds arbitrary byte sequences into `kv.Store.Recover`, checksums on and
  off. 340,000+ executions, zero panics.
- CI (`.github/workflows/ci.yml`) runs build, vet, gofmt check, and the full `-race` suite on every
  push, with the Go version pinned via `go-version-file` so it can't silently drift from `go.mod`.

## Bugs this harness has already caught in itself

- **A determinism bug in the example Raft test harness.** Node startup ranged over a Go map, whose
  iteration order is runtime-randomized, feeding into tie-break sequencing for simultaneous
  virtual-time events. The same seed could produce different election outcomes across runs,
  directly breaking the library's core promise. Fixed by iterating an ordered slice instead.
- **A byte-addressability bug in `FaultyStorage`.** The original version only supported
  whole-block reads keyed by the exact write offset, not true byte-range access, so the KV store's
  header/body reads within a single entry silently returned nothing. Fixed by rebuilding
  `FaultyStorage` as a real byte-addressable virtual disk.
- **A footgun in the `Sim` timing API itself.** A loop calling `sim.RunUntil(sim.Now() + delta)` on
  every iteration silently stalled: if a call doesn't reach a new event, `Now()` doesn't advance, so
  the next iteration recomputes the same horizon forever, with no error. Added `RunFor(delta)`,
  which advances from the highest horizon ever requested instead of from `Now()`, so it can't stall
  this way.

## What's not here

- `FaultyStorage` grows its committed buffer but never shrinks or compacts. It's a fault-injection
  test double standing in for a disk, not a production storage engine; compaction isn't the property
  under test.
- The AST rewrite path (`rewrite`/`cmd/detsim-test`) covers goroutines, channels, `sync`, `time`,
  `math/rand`, and basic network/file I/O, including aliased stdlib imports. Code with unsafe
  struct-embedded sync fields is detected and rejected rather than silently rewritten wrong; range
  over a channel is flagged with a precise warning since it can't compile once rewritten; anything
  else outside that construct set is not yet intercepted.

## License

MPL-2.0, see `LICENSE`. Anyone can use this as a dependency without their own project having to
open up, the copyleft only reaches the files actually covered by it, but a fork of the covered
files themselves has to stay open and can't be silently relicensed.
