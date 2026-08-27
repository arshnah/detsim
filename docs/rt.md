# package rt

`import "github.com/arshnah/detsim/rt"`

`rt` is a cooperative, single-stepping scheduler for real Go goroutines. Every "goroutine" spawned
through `Sched.Go` is still a genuine `go` statement under the hood, but a turnstile forces strict
single-stepping: exactly one logical goroutine is ever runnable at a time, chosen either randomly
(seeded) or by replaying a previously recorded decision sequence. No Go runtime patch, no
`GOMAXPROCS=1` requirement. Real parallelism simply never happens, because only one goroutine's
resume channel is ever unblocked at once.

This is the runtime the `rewrite` package's AST rewriter targets: rewritten code calls into `rt`
instead of the real `go`/`chan`/`sync`/`time`/`net`/`os` it was written against. `rt` is also usable
directly by hand. Every test in this package is written straight against it, no rewriting involved,
which is a reasonable way to get deterministic concurrency into new code without going through the
rewrite/overlay pipeline at all.

## Scheduler

### `type Sched struct { Seed int64; Rand *rand.Rand; ... }`

The scheduler itself. `Seed` and `Rand` are the same seed/RNG pattern as the root `detsim` package's
`Sim`. Every scheduling decision this `Sched` makes draws from `Rand`, so the same seed reproduces
the identical interleaving every run, unless the `Sched` was built from a trace (see Trace & replay
below), in which case decisions come from the trace instead of `Rand`.

### `func NewSched(seed int64) *Sched`

Builds a `Sched` that picks among ready goroutines uniformly at random, seeded.

### `func (s *Sched) Go(fn func())`

Spawns `fn` as a new scheduled goroutine. It does not start running immediately. It becomes eligible
once `Run` picks it. Calling any `rt` primitive (`Chan`, `Mutex`, `Sleep`, ...) from a goroutine that
was not started through `Go` on this same `Sched` panics. There is no ambient "current scheduler,"
which is why two `Sched`s can run fully independent, concurrent test scenarios in the same process
without interfering.

### `func (s *Sched) Run() error`

Drives the scheduler until every goroutine has finished, a deadlock is detected, a goroutine panics,
or `SetDecisionLimit` cuts it short. Returns `nil` on clean completion, a `*DeadlockError` if nothing
is runnable and something is still unfinished, a `*PanicError` if a goroutine panicked, or
`ErrDecisionLimit` if the decision limit was hit first. On each step, it collects every goroutine
that's either brand new or blocked with a now-true predicate. If none are ready, it advances virtual
time to the earliest pending `Sleep` wakeup and tries again. If nothing is ready and nothing is
sleeping, everything still unfinished is genuinely deadlocked.

### `func (s *Sched) Now() VirtualTime`

The scheduler's current virtual time. Only ever advances when `Run` fast-forwards past a gap with
nothing runnable but something asleep, never on a real wall-clock tick.

### `func (s *Sched) Shutdown()`

Releases every goroutine the scheduler spawned that hasn't finished. When `Run` ends in a deadlock,
a panic, or a decision limit, the goroutines that were mid-scenario stay parked on scheduler
channels forever — in a test binary running thousands of seeded trials in-process, that's thousands
of leaked stacks. Call `Shutdown` after `Run` returns (never while it's running) and those
goroutines unwind via `runtime.Goexit`, so none of the code under test resumes. Idempotent, and
safe to call on a scheduler whose goroutines all finished.

### `func (s *Sched) Sleep(d VirtualTime)`

Blocks the calling goroutine until virtual time has advanced by at least `d`. A negative `d` is
treated as zero. This never actually waits in real time. `Run`'s clock-advance step jumps straight to
the wakeup instant.

### `func (s *Sched) After(d VirtualTime) *Chan[VirtualTime]`

The `rt` equivalent of `time.After`: spawns a goroutine that sleeps `d` then sends the wakeup time on
a fresh, capacity-1 channel, and returns that channel for use in a `Select`.

### `func (s *Sched) SetDecisionLimit(n int)`

Caps `Run` to at most `n` scheduling decisions before it returns `ErrDecisionLimit` instead of
running to completion. `n <= 0` means unlimited (the default). This exists for novelty search's cheap
peek pass (see below): fingerprinting a trial's schedule shape only needs its first few decisions,
not a full run to completion, so a decision-limited run can be meaningfully cheaper than a real one.

## Channels

### `type Chan[T any] struct { ... }`

A generic channel that behaves like a real Go channel (buffered or unbuffered, FIFO, panics on
send-to-closed and double-close) but blocks through the scheduler's park/resume mechanism instead of
the real runtime's channel implementation, so every send/receive rendezvous is a scheduling decision
`rt` can record and later replay.

### `func NewChan[T any](s *Sched, cap int) *Chan[T]`

Creates a channel with the given buffer capacity. `cap == 0` is an unbuffered (rendezvous) channel,
same as a real Go `chan`.

### `func (c *Chan[T]) Send(v T)`

Sends `v`. Blocks until there's buffer room, or (for an unbuffered channel) until a receiver is
already waiting. Panics if the channel is closed.

### `func (c *Chan[T]) Recv() T`

Receives a value, blocking until one is available. Returns the zero value if the channel is closed
and drained, equivalent to `v := <-ch` on a real channel, which silently loses the "was this
actually sent or just the zero value from a closed channel" distinction. Use `RecvOK` when that
distinction matters.

### `func (c *Chan[T]) RecvOK() (T, bool)`

Receives a value, blocking until one is available or the channel is closed. The bool is `false` only
when the channel is closed and empty, the `v, ok := <-ch` form.

### `func (c *Chan[T]) TryRecv() (T, bool)`

Non-blocking receive attempt: returns immediately with `(zero, false)` if nothing is buffered, never
parks the calling goroutine.

### `func (c *Chan[T]) Close()`

Closes the channel. Panics if already closed. Blocked receivers wake up and drain whatever remains
buffered before seeing "closed."

### `func (c *Chan[T]) Closed() bool`, `Len() int`, `Cap() int`

Current closed state, buffered element count, and configured capacity.

## Select

`Sched.Select` is the `rt` equivalent of a Go `select` statement. It's built from `SelectCase`
values rather than language syntax, since `rt` has no compiler support. This is exactly what the
`rewrite` package's AST rewriter generates when it encounters a real `select` statement.

### `func RecvCase[T any](ch *Chan[T], commit func()) SelectCase`

A case that's ready when `ch` has a buffered value or is closed. `commit` is called once this case is
chosen and is where the actual receive happens. `RecvCase` only handles readiness detection. It does
not receive on your behalf. A typical `commit` closure does something like `v := ch.Recv()` and
stashes `v` into an outer variable for the calling code to use afterward.

### `func SendCase[T any](ch *Chan[T], commit func()) SelectCase`

A case that's ready when `ch` has room (or, for an unbuffered channel, a receiver is already
waiting) and is not closed. As with `RecvCase`, `commit` performs the actual send.

### `func DefaultCase(commit func()) SelectCase`

Fires immediately if no other case is ready when `Select` is first evaluated, the `select { ...
default: }` case. Never causes `Select` to block.

### `func (s *Sched) Select(cases ...SelectCase)`

Evaluates every non-default case's readiness. If more than one is ready, it picks uniformly at random
(seeded) among them, matching real Go `select`'s documented random-among-ready-cases behavior. If
none are ready and a `DefaultCase` was given, that fires. Otherwise the calling goroutine parks until
some case becomes ready, then re-evaluates from scratch.

## Sync primitives

Deterministic counterparts to `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup`, `sync.Once`, and
`sync.Cond`, each bound to a `*Sched` at construction and blocking through the scheduler instead of
the real runtime.

### `type Mutex struct{ ... }` / `func NewMutex(s *Sched) *Mutex`

`Lock()` blocks while held. `TryLock() bool` acquires without blocking, returning whether it
succeeded. `Unlock()` panics if the mutex isn't currently locked.

### `type RWMutex struct{ ... }` / `func NewRWMutex(s *Sched) *RWMutex`

`Lock()`/`Unlock()` for exclusive access, `RLock()`/`RUnlock()` for shared read access. A writer
excludes all readers and vice versa. `Unlock()` panics if not write-locked. `RUnlock()` panics if
there is no outstanding reader.

### `type WaitGroup struct{ ... }` / `func NewWaitGroup(s *Sched) *WaitGroup`

`Add(delta int)` adjusts the counter and panics if it goes negative. `Done()` is `Add(-1)`. `Wait()`
blocks until the counter reaches zero.

### `type Once struct{ ... }` / `func NewOnce(s *Sched) *Once`

`Do(fn func())` runs `fn` exactly once across however many goroutines call it. Concurrent callers
during the first run block until it completes, then return without rerunning `fn`.

### `type Cond struct{ L *Mutex; ... }` / `func NewCond(s *Sched, l *Mutex) *Cond`

`Wait()` unlocks `L`, blocks until signaled, then reacquires `L` before returning. The caller must
hold `L` when calling `Wait`, matching `sync.Cond`. `Signal()` wakes at most one waiter. `Broadcast()`
wakes all current waiters. Wakeups are token-based (a `Signal` while nobody is waiting is not "saved"
for a future waiter), matching real `sync.Cond` semantics.

## Network

A deterministic, in-process stand-in for `net.Dial`/`net.Listen`, addressed by plain strings rather
than real sockets. It lives on the same `*Sched` clock as everything else in this package. It is a
distinct implementation from the root `detsim` package's `Network` type, which runs on `Sim`'s clock
instead, since bridging the two kernels' notions of time was judged not worth the complexity.

### `func NewNetwork(s *Sched) *Network`

Creates a network with a default 1–5 virtual-time-unit delivery delay range and no faults configured.

### `func (n *Network) SetDropRate(rate float64)`, `SetDelayRange(min, max VirtualTime)`

Configures the probability a given write is silently dropped (never delivered, but `Write` still
reports success, mirroring a real fire-and-forget network write) and the random delay range applied
to delivered writes.

### `func (n *Network) PartitionAddr(addr string)`, `HealAddr(addr string)`

Fully isolates or restores a single address: while partitioned, nothing can dial in or out of that
address in either direction.

### `func (n *Network) Partition(groupA, groupB []string)`, `HealAll()`

Symmetric pairwise partitioning between two groups of addresses (every address in `groupA` is cut off
from every address in `groupB`, both directions, and addresses within the same group are unaffected).
`HealAll` clears every mesh partition set this way, but not `PartitionAddr`/`HealAddr` isolation.

### `func (n *Network) Listen(addr string) (*Listener, error)`

Registers a listener at `addr`. Fails if that address is already listening.

### `func (l *Listener) Accept() (*Conn, error)`

Blocks until an incoming connection arrives, or returns an error once the listener is closed.

### `func (l *Listener) Addr() *Addr`, `func (l *Listener) Close() error`

The listener's own address, and closing it (idempotent, further `Accept` calls see the listener as
closed).

### `func (n *Network) Dial(addr string) (*Conn, error)`

Dials `addr` with no particular local address, equivalent to `DialFrom("", addr)`. Fails if the pair
is partitioned or nothing is listening at `addr`.

### `func (n *Network) DialFrom(from, to string) (*Conn, error)`

Dials `to` from a specific local address `from`, so partition checks and delivery delay can be
address-pair-aware. This is the entry point to use when the code under test already knows its own
address. A plain `net.Dial(addr)` call site, which real Go gives no "my own address" to infer from,
gets rewritten to the address-less `Dial` form instead.

### `type Conn struct{ ... }`

A connected pair of endpoints, `Write` on one delivering to the other's `Read` after a randomized
delay (or being silently dropped, per `SetDropRate`). Delivery is re-checked against the current
partition state at delivery time, not just at write time, so a partition that opens up mid-transit
still drops an in-flight message. `Write` on a closed connection errors. `Read` on a closed/drained
connection returns `io.EOF`. `LocalAddr`/`RemoteAddr` return `*Addr`, and `Close` is idempotent.

## Filesystem

A deterministic stand-in for `os.Open`/`os.Create`, backed per-filename by the root `detsim`
package's `FaultyStorage`, so file I/O inherits the same torn-write/corruption/dropped-sync fault
injection as everything else built on `FaultyStorage`.

### `func NewFileSystem(s *Sched, profile detsim.FaultProfile) *FileSystem`

Creates a filesystem where every file created through it shares the given fault profile.

### `func (fs *FileSystem) Open(name string) (*File, error)`

Opens an existing file. Returns an `*fs.PathError` wrapping `fs.ErrNotExist` if it was never created.
Unlike an earlier version of this type, `Open` does not implicitly create the file.

### `func (fs *FileSystem) Create(name string) (*File, error)`

Creates (or truncates, if it already exists) a file backed by a fresh `FaultyStorage` seeded from the
filesystem's own scheduler-bound RNG.

### `func (fs *FileSystem) Remove(name string) error`, `Rename(oldName, newName string) error`

Delete or rename a file. Both fail with `fs.ErrNotExist` if the source name doesn't exist.

### `func (fs *FileSystem) Stat(name string) (FileInfo, error)`

Returns size/name info for an existing file, `fs.ErrNotExist` otherwise.

### `func (fs *FileSystem) ReadFile(name string) ([]byte, error)`, `WriteFile(name string, data []byte) error`

Whole-file convenience helpers, equivalent to `os.ReadFile`/`os.WriteFile`. `WriteFile` syncs before
returning.

### `type File struct{ ... }`

Implements `Read`, `Write`, `Sync`, `Close` against the backing `FaultyStorage` at a tracked offset.
`Read` at end-of-file returns `io.EOF`, matching `os.File`. `Close` is a no-op. The backing storage
isn't tied to any real OS handle.

### `type FileInfo struct{ ... }`

A minimal `io/fs.FileInfo`-shaped value: `Name`, `Size`, and fixed stand-in values for `Mode` (0644),
`ModTime` (zero time), `IsDir` (always false), `Sys` (always nil), enough for code that only cares
about name and size.

## Errors

### `type DeadlockError struct{ Goroutines []DeadlockGoroutine }`

Returned by `Run` when nothing is runnable and something is still unfinished. `Error()` renders one
block per stuck goroutine: its ID, what it was blocked on (`"chan send"`, `"mutex lock"`,
`"waitgroup wait"`, ...), and a captured stack trace from the moment it parked, so a real deadlock
comes back as one readable error naming every stuck goroutine and where, instead of the test binary
just hanging until `go test`'s own timeout kills it.

### `type PanicError struct{ Value any; Stack string }`

Returned by `Run` when a scheduled goroutine panics. `Value` is the recovered panic value, `Stack` is
its stack trace at the point of the panic.

### `var ErrDecisionLimit error`

Returned by `Run` when `SetDecisionLimit` cuts a run short before it reaches completion.

## Trace & replay

A `Trace` is the exact sequence of scheduling decisions `Run` made. Together with the original seed,
it's enough to deterministically reproduce (or investigate) one specific interleaving without needing
the seed alone to happen to reselect it.

### `type Trace struct { Seed int64; Decisions []uint64; Labels []string; Steps []Step }`

### `func (s *Sched) Trace() Trace`

The trace of every decision this `Sched` has made so far. Meaningful mid-run or after `Run` returns.
`Decisions` alone replays a run; `Labels` (goroutine id → name from `GoNamed`) and `Steps` (one
entry per decision: the virtual time it ran at, and whether the goroutine finished or parked on
what afterward) are the human-readable enrichment that `detsim-trace` renders. Both are optional in
the JSON, so traces recorded before they existed still load.

### `func (s *Sched) GoNamed(name string, fn func())`

`Go` with a label. The name lands in `Trace.Labels` and shows up in `detsim-trace` output, which is
the difference between reading "g7 blocked on chan send" and "producer blocked on chan send" when
staring down a minimized failure.

### `func SaveTrace(path string, trace Trace) error`, `func LoadTrace(path string) (Trace, error)`

Persists/reads a `Trace` as indented JSON.

### `func NewSchedFromTrace(trace Trace) *Sched`

Builds a `Sched` that replays `trace.Decisions` exactly: at each step it requires the recorded
goroutine ID to actually be ready, and errors with `ErrTraceMismatch` if it isn't, or
`ErrTraceExhausted` if the trace runs out before the scheduler quiesces. Either error means the code
under test has changed since the trace was recorded.

### `func NewSchedFromTraceLenient(trace Trace) *Sched`

Same as `NewSchedFromTrace`, but falls back to a random pick (like a fresh `Sched`) instead of erroring
on a mismatch or an exhausted trace, continuing to record from that point on. Used for the cheap
"peek" pass in novelty search, where the recorded prefix is trusted but the trace deliberately doesn't
cover a full run.

### `type TestingT interface { Failed() bool; Cleanup(func()); Logf(format string, args ...any) }`

The minimal subset of `*testing.T` the two helpers below need, so they don't have to import
`testing` directly.

### `func DumpTraceOnFailure(t TestingT, sched *Sched, path string)`

Registers a cleanup that saves `sched`'s trace to `path` only if `t` ends up failed, logging the seed
and path so a failure's exact reproduction is visible right in the test output.

### `func DumpTraceToEnvPath(t TestingT, sched *Sched, envVar string)`

Registers a cleanup that always saves `sched`'s trace, to whatever path `envVar` names, regardless of
pass/fail. If `envVar` isn't set, this is a no-op. This is what novelty search's full-run phase uses
to recover the trace for fingerprinting, independent of whether the trial passed.

## Novelty search

An alternative to sweeping a fixed number of seeds: instead of a trial-count budget, keep trying
seeds until a run of trials in a row all produce a schedule shape that's already been seen, on the
theory that this finds more genuinely distinct interleavings per CPU-second than a flat sweep does on
code with a lot of scheduling diversity and comparatively little correctness risk per trial.

### `type NoveltySearchConfig struct { StartSeed, MaxTrials int64/int; DryLimit, PrefixLen int }`

`StartSeed`: first seed to try, subsequent trials increment it by one. `MaxTrials`: hard cap
(default 10000). `DryLimit`: how many consecutive trials with no new schedule shape before stopping
(default 20). `PrefixLen`: how many leading decisions of a trace are hashed to fingerprint it as
novel or not (default 8). This is also the natural `SetDecisionLimit` value for a cheap peek pass,
since novelty only needs the prefix to be genuine, not the full trace.

### `type NoveltySearchResult struct { TrialsRun, DistinctTraces int; StoppedDry bool; FailedSeed int64; FailedErr error }`

`TrialsRun`/`DistinctTraces`: how many trials actually ran and how many produced a new prefix
fingerprint. `StoppedDry`: true if the search stopped because of `DryLimit`, false if it hit
`MaxTrials` first. `FailedSeed`/`FailedErr`: set if `trial` ever returned a non-nil error, at which
point the search stops immediately and returns.

### `func NoveltySearch(cfg NoveltySearchConfig, trial func(seed int64) (Trace, error)) NoveltySearchResult`

Runs the search. `trial` is called once per seed and must return that seed's resulting `Trace` (and
an error if the scenario itself failed, not a novelty-related error, since novelty search never fails
a trial itself, it only stops early).

## Env helpers

Small helpers for wiring a test up to be driven externally, by `cmd/detsim-test`'s seed sweeper or a
CI matrix, without importing `flag`/`os` boilerplate into every test file.

### `func SeedFromEnv(name string, fallback int64) int64`

Reads an int64 seed from the named environment variable, or returns `fallback` if unset or
unparseable.

### `func SchedFromEnv(seedEnv, traceEnv string, fallbackSeed int64) (*Sched, error)`

If `traceEnv` names a set environment variable, loads and replays that trace file leniently
(`NewSchedFromTraceLenient`). Otherwise builds a fresh `Sched` from `SeedFromEnv(seedEnv,
fallbackSeed)`. This is the single call a rewritten test's entry point needs to be drivable either by
a plain seed sweep or by a specific failing trace, depending on which environment variable is set.

### `func DecisionLimitFromEnv(envVar string) int`

Reads a decision limit from the named environment variable. `0` (unset, unparseable, or `<= 0`) means
no limit. Paired with `Sched.SetDecisionLimit` for novelty search's peek-mode env wiring.
