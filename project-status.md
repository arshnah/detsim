# Market research: CI flaky-test deterministic replay

Question: is "capture a real flaky test failure in CI and deterministically replay
the exact goroutine schedule locally" (built on detsim's rt/rewrite work) an open
product gap, or already solved.

## Verdict

Open gap, confirmed via web research (6+ queries). Nobody currently offers
"reproduce this specific flaky failure that just happened in CI by replaying the
exact goroutine schedule."

## Closest competitors and how they differ

- **Antithesis** (antithesis.com) — different layer. Proprietary deterministic
  hypervisor that runs your whole system in simulated VMs to proactively fuzz for
  bugs (chaos/fault injection on clock, network, disk) — not replay of a failure
  that already happened in a real CI run. Pricing $0.80-$2.00/CPU-hour, $20K-$100K+/yr
  enterprise deals (Jane Street led their $105M Series A) — built for well-funded
  infra teams, not indie/OSS price point. Their Go SDK
  (github.com/antithesishq/antithesis-sdk-go) has an `-assert_only` standalone mode,
  but that's just assertion instrumentation — still needs their hypervisor for
  actual simulation/replay.
- **BuildPulse / Trunk Flaky Tests** (trunk.io, buildpulse.io) — detection,
  quarantine, historical-flakiness analytics only. Confirmed: no deterministic
  replay feature in either.
- **rr (Mozilla)** — real record/replay, but Linux-only; Go support is a bolt-on
  via Delve with "very basic level of support, no understanding of Go's channels
  or goroutines until recently." Not CI-integrated — a local gdb-replacement
  workflow, not a product.
- **Undo.io** — same category as rr, enterprise-licensed, C/C++/Java-first with Go
  as a secondary case.

## Technical red flag (honest)

Multiple sources state Go's scheduler is "effectively non-deterministic on a
multiprocessor" — this is the hard problem, not a trivial build. Matches what
detsim's rt/rewrite work is already attacking, so it's not starting from zero.

## Market signal

- Flaky tests cost a 50-person team $200-400K/yr, a 100-person team ~$2.6M/yr in
  lost productivity (Autonoma, FlakyGuard).
- 26% of teams hit flakiness in 2025, up from 10% in 2022 (Bitrise).
- Real, sizeable, validated pain point with money already flowing to inferior
  (detection-only) tooling.

## Bottom line

The niche — Go-specific, CI-native, deterministic goroutine-schedule replay of a
real flaky failure — is open. Antithesis is adjacent but heavier and enterprise-
priced; BuildPulse/Trunk don't replay; rr/Undo don't integrate with CI or
understand goroutines well. Legitimate wedge for a tool built on top of detsim.
