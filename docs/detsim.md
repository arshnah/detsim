# package detsim

The root package: a deterministic virtual-time event kernel (`Sim`), a fault-injectable network
(`Network`), and a fault-injectable storage layer (`FaultyStorage`). This is the low-level API for
writing a system under test directly against the kernel, callback-driven rather than goroutine-driven,
the way `examples/raft` and `examples/kv` do. If you'd rather point the deterministic scheduler at
ordinary Go code that already uses real goroutines and channels, see the `rt` and `rewrite` packages
instead.

## Virtual time

```go
type VirtualTime = time.Duration
```

An alias for `time.Duration`, used everywhere a duration or a point in simulated time is expected.
Virtual time never advances on its own. Only `Sim.Run`/`RunUntil`/`RunFor` move it forward, one
scheduled event at a time. There is no relationship between virtual time and the wall clock: a
scenario spanning a simulated hour can run in milliseconds of real time.

## Sim

```go
type Sim struct {
    Seed int64
    Rand *rand.Rand
    // unexported fields
}

func New(seed int64) *Sim
```

The deterministic event kernel: a min-heap of events ordered by `(virtual_time, insertion_sequence)`.
`New` seeds `Rand` (`*rand.Rand`) from `Seed`. Every fault decision made by `Network` and
`FaultyStorage` draws from this same `Rand`, so an entire scenario's outcome, network faults, storage
faults, and event ordering together, traces back to one seed. Two `Sim`s built with the same seed and
driven through the same call sequence produce the identical event order on every run.

**`func (s *Sim) Now() VirtualTime`**
Returns the current virtual time, the `at` of whichever event most recently ran.

**`func (s *Sim) After(d VirtualTime, fn func(*Sim))`**
Schedules `fn` to run `d` after the current virtual time. Negative `d` is clamped to `0`. This is the
usual way to schedule work: a timer, a delayed message delivery, a retry.

**`func (s *Sim) At(t VirtualTime, fn func(*Sim))`**
Schedules `fn` to run at absolute virtual time `t`, regardless of `Now()`. Useful for scheduling
relative to a fixed point rather than "from now."

**`func (s *Sim) Immediately(fn func(*Sim))`**
Shorthand for `After(0, fn)`: runs `fn` as the next event, after anything already queued at the
current time ahead of it in insertion order.

**`func (s *Sim) Run() (steps int, ranToCompletion bool)`**
Drains the event queue, running events strictly in `(time, sequence)` order, until either the queue
is empty (`ranToCompletion == true`) or the step budget set by `SetMaxSteps` is exhausted
(`ranToCompletion == false`, a runaway-scheduling guard, default cap 5,000,000 steps). Returns how
many events actually ran.

**`func (s *Sim) SetMaxSteps(n int)`**
Overrides the step budget `Run` enforces. Mainly useful for tests that specifically want to assert a
scenario terminates within a tight bound, or that need a larger budget than the default.

**`func (s *Sim) RunUntil(t VirtualTime) (steps int)`**
Runs events up to and including virtual time `t`, then stops even if more events remain queued beyond
`t`. Useful for a scenario that reschedules itself forever (a heartbeat, a retry loop) and would
otherwise never let `Run` drain the queue on its own.

**`func (s *Sim) RunFor(d VirtualTime) (steps int)`**
Runs for `d` more virtual time, advancing from the highest horizon ever requested across any prior
`RunUntil`/`RunFor` call, not from `Now()`. There's a real footgun this avoids. Calling
`RunUntil(sim.Now() + delta)` on every iteration can silently stall: if a call doesn't reach any new
event, `Now()` doesn't advance, so the next iteration recomputes the exact same horizon, forever, with
no error. `RunFor` can't stall this way.

## Network

```go
type NodeID string

type Network struct {
    // unexported fields
}

func NewNetwork(s *Sim) *Network
```

A fault-injectable network built on top of a `Sim`: seeded message drops, seeded delivery delay, and
partitions between named node groups, all driven off the `Sim`'s own `Rand` so the fault pattern for a
given seed is reproducible.

**`func (n *Network) Register(id NodeID, handler func(from NodeID, msg any))`**
Registers the callback that receives messages addressed to `id`. Each node needs exactly one handler.

**`func (n *Network) SetDropRate(rate float64)`**
Sets the probability, checked independently for every send and every delivery attempt, that a message
is silently dropped. `0` disables random drops entirely (partitions still apply).

**`func (n *Network) SetDelayRange(min, max VirtualTime)`**
Sets the range delivery delay is drawn from, uniformly at random, per message. Default is `[1, 5)`.

**`func (n *Network) Partition(groupA, groupB []NodeID)`**
Blocks all delivery between every node in `groupA` and every node in `groupB`, in both directions.
Nodes within the same group can still reach each other.

**`func (n *Network) HealAll()`**
Clears every partition set by `Partition`. Random drops from `SetDropRate` are unaffected.

**`func (n *Network) Send(from, to NodeID, msg any)`**
Sends `msg` from `from` to `to`. If the pair is partitioned, or the drop-rate roll fails, the message
is silently lost, matching real network behavior (the sender gets no synchronous error). Otherwise
delivery is scheduled after a random delay in the configured range, and the partition/drop check is
re-evaluated again at actual delivery time, not just at send time, so a message already in flight when
a partition forms can still be dropped mid-transit.

## FaultyStorage

```go
var ErrDiskFull = errors.New("detsim: disk full")

type FaultProfile struct {
    TornWriteRate   float64
    CorruptByteRate float64
    SkipSyncRate    float64
    ReorderRate     float64
    MaxSize         int64
}

type FaultyStorage struct {
    // unexported fields
}

func NewFaultyStorage(seed int64, profile FaultProfile) *FaultyStorage
```

A byte-addressable virtual disk that stands in for a real one in tests, injecting the failure modes a
real disk or filesystem can exhibit under crash or hardware fault, each independently controlled by
`FaultProfile`:

- `TornWriteRate`: probability a write is truncated to a random shorter length before being staged,
  simulating a write that was interrupted mid-flight.
- `CorruptByteRate`: probability a single random byte in a staged write is flipped, simulating bit
  rot or a corrupted sector.
- `SkipSyncRate`: probability a staged write is silently dropped at `Sync` time. `Sync` still returns
  `nil`. This is the most dangerous fault, and it's not hypothetical. PostgreSQL's 2018
  fsync-error-handling incident traced back to exactly this: on some kernels, `fsync` can report a
  write error once, then silently discard the failed page on the next call, so a caller that doesn't
  check again believes the write landed. Any code that advances its own bookkeeping unconditionally
  after a successful `Sync` call, instead of re-deriving its trusted length from something `Sync` (or
  `Size`) actually confirms, drifts out of sync with what's really durable that same way.
- `ReorderRate`: probability the order staged writes are committed in, within one `Sync` call, is
  shuffled instead of preserved.
- `MaxSize`: if positive, caps the virtual disk. A `WriteAt` that would exceed it returns
  `ErrDiskFull` and writes nothing.

**`func (f *FaultyStorage) WriteAt(data []byte, offset int64) (n int, err error)`**
Stages a write at `offset`, subject to `TornWriteRate`/`CorruptByteRate`. Returns `len(data), nil` on
success, even if the write was torn or corrupted internally. The return value can't be trusted alone.
It matches the shape of the real `io.WriterAt` contract but not its guarantee. Only `Sync` and a later
`Read`/replay can reveal a torn or corrupted write. Returns
`ErrDiskFull` and writes nothing if `MaxSize` would be exceeded.

**`func (f *FaultyStorage) ReadAt(p []byte, offset int64) (n int, err error)`**
Reads from the current materialized view (committed data with any still-pending, not-yet-synced
writes layered on top). Reading past the end returns `0, nil`, matching `FaultyStorage`'s permissive
short-read convention rather than `io.EOF`.

**`func (f *FaultyStorage) Sync() error`**
Commits all currently staged writes to the durable `committed` buffer, subject to `SkipSyncRate`
(silently dropped) and `ReorderRate` (commit order shuffled within this batch). Always returns `nil`.
A dropped write is not reported as an error, by design, since that's the fault being modeled.

**`func (f *FaultyStorage) Size() int64`**
Returns the length of the current materialized view. This is the one way to detect that a write your
code believes succeeded didn't actually make it into `committed`, comparing `Size()` before and after
a `Sync()` call, or against an offset counter your own code advanced optimistically, is how to catch
the `SkipSyncRate`/`TornWriteRate` faults in a caller.

**`func (f *FaultyStorage) Crash()`**
Discards anything currently staged but not yet synced, simulating a crash between a `WriteAt` and its
`Sync`.

**`func (f *FaultyStorage) SeedRaw(data []byte)`**
Replaces the committed buffer wholesale and clears anything staged, for tests that want to start from
specific pre-existing bytes (fuzzing a recovery path against arbitrary input, for example) rather than
building up state through `WriteAt`/`Sync` calls.
