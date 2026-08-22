# detsim

A standalone deterministic simulation testing library for Go. It brings the FoundationDB/TigerBeetle style of testing distributed and stateful systems, but exposed as a reusable module instead of buried inside one company's database.

## What this is

- A virtual-time discrete-event kernel (`Sim`). No real sleeps, no wall-clock dependence. A scenario with a simulated hour of network traffic runs in milliseconds, and the exact same seed always produces the exact same sequence of events.
- A fault-injectable network (`Network`), with seeded message drops, delays, and partitions, all driven off the same deterministic RNG as everything else.
- A fault-injectable storage layer (`FaultyStorage`): torn writes, byte corruption, silently-dropped syncs, and reordered syncs, plus a `Crash()` that discards anything not yet durable. This is the part that actually catches crash-consistency bugs instead of just asserting the happy path.

## What this is NOT (yet)

This does not intercept the real Go scheduler or give you true goroutine-level determinism the way `gosim` does. That requires patching the runtime, which is a much bigger undertaking. Actors here are driven by callbacks registered with the event kernel (`Network.Register`), not free-running goroutines racing each other. That's a real scope limitation, not something to pretend away: it means you write your system-under-test against this event-driven style rather than dropping in arbitrary existing goroutine-heavy code unmodified.

## Why this exists

Every "build your own Raft/database" tutorial stops at the happy path. Real distributed systems teams (FoundationDB, TigerBeetle) get their correctness confidence from running thousands of seeded fault-injected scenarios and replaying any failure byte-for-byte. That methodology isn't available as an off-the-shelf Go library yet. This is an attempt at a minimal, honest version of one.

## Status

Core kernel, network fault injection, and storage fault injection are implemented and tested.

`examples/raft` is a from-scratch event-driven Raft (leader election + log replication, §5.2-§5.4 of the paper, including the fast-backtrack optimization) written directly against this kernel to dogfood it. `TestThousandsOfSeedsNoSplitBrain` runs 5,000 independently seeded 5-node clusters, each cycling through normal operation, a network partition, and a heal, in about 7 seconds of real wall-clock time, asserting the core safety invariant (no two leaders in the same term) never breaks. A wall-clock-bound test suite could never afford that many iterations. That gap is exactly the reason this library exists. Any seed that ever fails is exactly reproducible by rerunning with that seed (`TestSeedIsExactlyReproducible`).

`examples/kv` is a small WAL-based key-value store running against `FaultyStorage`, with a checksum toggle so it can directly prove why per-entry checksums matter under real fault injection instead of just claiming they do. `TestNoChecksumStoreCanServeCorruptData` found the no-checksum store serving corrupted data at seed 3 out of 5000, while `TestChecksummedStoreNeverServesCorruptData` confirms the checksummed version never does under the identical fault profile across 2000 seeds.

## Bugs this harness has already caught

- **A determinism bug in the example Raft test harness itself.** Node startup was driven by ranging over a Go map, whose iteration order is randomized by the runtime. That randomized call order fed into tie-break sequencing for simultaneous virtual-time events, so the same seed could produce different election outcomes across runs, directly breaking the library's core promise. Fixed by iterating an ordered slice instead of the map.
- **A byte-addressability bug in `FaultyStorage`.** The original version only supported whole-block reads keyed by the exact offset a write was issued at, not true byte-range access. The KV store's header/body reads within a single written entry silently returned nothing. Fixed by rebuilding `FaultyStorage` as a real byte-addressable virtual disk.

## Try it

`cmd/detsim` is a small CLI that runs the examples directly instead of through `go test`, so the fault-injection results are visible without reading test output.

```
go run ./cmd/detsim -mode=raft -seed=1 -trials=5 -drop=0.15
go run ./cmd/detsim -mode=kv -seed=1 -trials=10 -checksums=false
go run ./cmd/detsim -mode=kv -seed=1 -trials=10 -checksums=true
```

Sample output, no-checksum KV store hunting for a corrupted seed:

```
seed=1 ok, checksums=false, 2 keys recovered clean
seed=2 ok, checksums=false, 0 keys recovered clean
seed=3 CORRUPTED: key3 got "value-\xcc-3" want "value-3-3"
seed=4 ok, checksums=false, 0 keys recovered clean
seed=5 CORRUPTED: key3 got "val\x8ae-5-3" want "value-5-3"
```

Same seed range with `-checksums=true` never produces a `CORRUPTED` line.

Next: push fault rates further and see what else breaks.
