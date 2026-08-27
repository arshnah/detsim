# detsim documentation

Standalone API reference, and runnable example code alongside it.

- [`detsim`](detsim.md): the root package. `Sim`, `Network`, `FaultyStorage`. Write a system under
  test directly against the kernel. See `example_test.go` at the repo root for runnable usage.
- [`rt`](rt.md): the goroutine-level deterministic scheduler. Real goroutines, channels, and sync
  primitives, single-stepped through a turnstile instead of the real Go runtime. See
  `rt/example_test.go` for runnable usage.
- [`rewrite`](rewrite.md): the AST rewriter that points existing, unmodified Go source at `rt`
  through a `go build` overlay.
- [`minimize`](minimize.md): delta-debugging over a scheduler decision trace, shrinking a failing
  seed's schedule down to a minimal reproducer.

Start with the root README at the repo root for the overall pitch and the real bugs this project's
own testing process has found in itself. These four pages are the reference material for once you're
actually calling into the API.
