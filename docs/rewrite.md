# package rewrite

`import "github.com/arshnah/detsim/rewrite"`

`rewrite` source-rewrites an existing Go package's AST: goroutines, channels, `sync` locals,
`time`/`math/rand` calls, and a subset of `os`/`net` calls, so that ordinary, previously unaware
Go code runs on top of the `rt` package's deterministic scheduler instead of the real Go runtime.
It never touches the files on disk: it produces a `go build -overlay` JSON file pointing at
rewritten copies in a temp directory, and the real toolchain compiles those instead.

This is the second of detsim's two ways to get a system under `rt`. The first is writing directly
against `rt`'s API by hand (`rt.NewSched`, `rt.NewChan`, etc.), which is precise but requires the
system under test to already be written that way. `rewrite` exists for everything else: point it at
a package that uses plain `chan`, `go`, `sync.Mutex`, `time.Sleep`, and it gets the same
determinism without being rewritten by hand.

## `func Rewrite(dir, pattern string) (*Result, error)`

Loads the Go package at `pattern` (a standard package pattern, e.g. `.` or `./...`) rooted at `dir`
via `golang.org/x/tools/go/packages`, with full type information. Fails fast, before rewriting
anything, if:

- the package doesn't type-check (`go vet`-equivalent errors),
- the pattern doesn't resolve to exactly one package,
- any file in the package has a `struct` field whose type is `sync.Mutex`, `sync.RWMutex`,
  `sync.WaitGroup`, or `sync.Once` embedded directly (see **Struct-embedded sync fields** below).

Otherwise, for every file in the package, it applies the rewrite passes described below, formats
the result with `go/format`, and, for any file that actually changed, writes a rewritten copy
into a temp directory and records it in the overlay. Files that don't reference any of the
rewritten constructs at all are left out of the overlay entirely and never touched.

If at least one file was rewritten, `Rewrite` also generates one extra file,
`detsim_generated_sched.go`, in the package directory (see **The generated file** below).

## `type Result`

```go
type Result struct {
    OverlayPath    string
    PackageName    string
    PackageDir     string
    RewrittenFiles []string
    Warnings       []Warning
}
```

- `OverlayPath`: path to the `overlay.json` file. Pass it to the Go toolchain as
  `go build -overlay=<path>` / `go test -overlay=<path>` / `go vet -overlay=<path>` to build against
  the rewritten source instead of what's on disk.
- `PackageName`: the rewritten package's own name (for referencing the generated setter
  functions, see below).
- `PackageDir`: absolute directory the package's compiled files live in.
- `RewrittenFiles`: absolute paths (from the original source tree) of every file that was
  actually changed. Empty means nothing in the package used any construct this rewriter handles.
- `Warnings`: every `Warning` collected while rewriting, in the order encountered. Not fatal.
  The file each warning belongs to is either partially rewritten (the specific unsupported call is
  left as-is) or, for a whole-file skip, not rewritten at all. See below for which is which.

Call `(*Result).Close()` when done with it. It removes the temp directory the overlay and
rewritten copies live in. A `Result` with no temp directory (nothing was ever written) is safe to
`Close` as a no-op.

## `type Warning`

```go
type Warning struct {
    Pos     token.Position
    Message string
}
```

A `Warning`'s `Pos` points at the exact construct that triggered it. `Message` explains why.
`cmd/detsim-rewrite` and `cmd/detsim-test` both print these to stderr as
`warning: <pos>: <message>` before doing anything else.

## What gets rewritten

Applied to every file, in this order, before the file is re-rendered:

- **`select` statements**: each `case` becomes an `rt.RecvCase` / `rt.SendCase` / `rt.DefaultCase`
  argument to a single `sched.Select(...)` call, so the scheduler picks among them deterministically
  instead of the runtime picking at random. Each case's channel expression is first evaluated
  exactly once, in source order, into a temporary, matching real `select` semantics — a naive
  splice would evaluate `case <-time.After(d)`'s operand twice and read a different channel than
  the one checked for readiness.
- **`make(chan T)` / `make(chan T, n)`** → `rt.NewChan[T](sched, n)` (unbuffered `make(chan T)`
  becomes capacity 0).
- **channel types** (`chan T` used as a type, e.g. in a struct field or function signature) →
  `*rt.Chan[T]`.
- **channel sends** (`ch <- v`) → `ch.Send(v)`.
- **channel receives**: both forms. `v := <-ch` → `v := ch.Recv()`, and the two-value form
  `v, ok := <-ch` → `v, ok := ch.RecvOK()`.
- **`close(ch)`** → `ch.Close()`.
- **`go f(a, b)`** → captures `f`, `a`, and `b` into locals at the point of the `go` statement (via
  an immediately-invoked closure) exactly the way the real `go` statement evaluates its function and
  arguments eagerly, then spawns `sched.Go(func() { f(a, b) })`. This two-step capture-then-spawn
  exists specifically so a value mutated on the next line after the `go` statement, before the
  scheduler ever runs the new goroutine, is *not* visible inside it, matching real Go semantics
  instead of the naive (and wrong) `go func(){ f(a, b) }()`-via-closure translation, which would
  evaluate `f`, `a`, `b` lazily instead.
- **local `sync.Mutex` / `sync.RWMutex` / `sync.WaitGroup` / `sync.Once` variable declarations**
  (`var mu sync.Mutex`) → `mu := rt.NewMutex(sched)` (and the `rt` equivalents for the other three).
  Only plain local `var` declarations with no initializer are rewritten this way. Anything else
  involving these types is left alone (see limitations).
- **`time.Sleep`, `time.Now`, `time.After`** → `sched.Sleep`, `sched.Now`, `sched.After`. Any other
  `time.*` call is left unrewritten and generates a warning.
- **`math/rand` package-level calls** (`rand.Int`, `rand.Intn`, `rand.Int31`, `rand.Int31n`,
  `rand.Int63`, `rand.Int63n`, `rand.Float64`, `rand.Float32`, `rand.Perm`, `rand.Shuffle`) →
  the same call on `sched.Rand`, the scheduler's own seeded source, so every random decision in the
  rewritten package traces back to the scheduler's seed. `rand.Seed(...)` calls are replaced with a
  no-op (with a warning): the scheduler's own seed is what controls randomness now, an explicit
  reseed from the target code would break that. Any other `rand.*` call is left unrewritten and
  warned about.
- **`os.Open`, `os.Create`, `os.Remove`, `os.Rename`, `os.Stat`, `os.ReadFile`, `os.WriteFile`,
  and the `*os.File` / `os.FileInfo` types** → the equivalent method on a `*rt.FileSystem` and the
  `*rt.File` / `rt.FileInfo` types. `os.WriteFile`'s optional trailing permission-bits argument is
  dropped (`rt.FileSystem.WriteFile` only takes name and data). `os.IsNotExist`, `os.IsExist`,
  `os.IsPermission`, and `os.IsTimeout` are left alone on purpose. They only unwrap an error via
  `errors.Is` against a real `io/fs.PathError`, which `rt`'s filesystem already returns correctly,
  so there's nothing to rewrite. Any other `os.*` call is left unrewritten and warned about.
- **`net.Listen`, `net.Dial`, and the `net.Conn` / `net.Listener` / `net.Addr` types** → the
  `*rt.Network` equivalents (`Listen`/`Dial` on a package-level `detsimNet` variable, types become
  `*rt.Conn` / `*rt.Listener` / `*rt.Addr`). Only the plain `net.Dial(addr)` form is reachable this
  way, since an ordinary `net.Dial` call site has no notion of "my own address" the way
  `rt.Network.DialFrom(from, to)` does. Code that already knows its own address has to call
  `DialFrom` directly against `detsimNet` by hand. Any other `net.*` call is left unrewritten and
  warned about.

After all of the above, unused `sync`/`time`/`math/rand`/`os`/`net` imports are dropped and an
import of `github.com/arshnah/detsim/rt` is added if the file ends up referencing it.

Package and builtin identification is type-driven, not spelling-driven: `mysync "sync"`,
`mytime "time"`, and other aliased stdlib imports are rewritten exactly like plain ones (and
pruned when their last use goes away), while a local variable shadowing `make` or `close` is left
strictly alone. Type conversions such as `time.Duration(x)` aren't flagged as unsupported calls.

## What refuses to rewrite

Two situations stop rewriting before it can produce something subtly wrong, rather than producing
it:

- **Struct-embedded sync fields.** A struct field typed as `sync.Mutex` (or `RWMutex`/`WaitGroup`/
  `Once`) directly, `type Foo struct { mu sync.Mutex }`, can't be safely rewritten. `rt`'s
  scheduler-backed replacements need a `*rt.Sched` at construction time, but a struct field's zero
  value has no way to carry one. Leaving the field as a real `sync.Mutex` is actively dangerous, not
  neutral: a real mutex blocks the actual OS thread, while `rt`'s cooperative scheduler expects every
  blocking operation to yield back to it, so a real lock contested from scheduled code can hang the
  whole test binary with no deadlock report. `Rewrite` detects this ahead of time and refuses to
  rewrite the *entire package*, returning an error naming every offending field. The fix is to give
  the type an explicit `*rt.Sched`-backed constructor by hand instead of relying on the zero value.
- **A `select` case whose body contains `return`, `break`, `continue`, or `goto`.** Rewriting a
  `select` moves each case's body into a closure. A `return` inside that body would then only exit
  the closure, not the enclosing function. Same for a `break`/`continue` targeting an enclosing
  loop, or a `goto` targeting a label outside the closure. That's a silent behavior change, not a
  compile error, which is worse than refusing outright. `Rewrite` scans every case in a `select`
  first. If any one of them has this shape, the *entire file* containing that select is left
  completely unrewritten, and a warning explains why, at the file level, before any other pass runs
  on it. (This means an otherwise-rewritable `select` elsewhere in the same file, or other
  goroutine/channel code in that file, doesn't get rewritten either. The check is file-wide, not
  statement-by-statement, since other rewritten code earlier in the file could otherwise reference
  channel types that this now-unrewritten select still expects in their original form.)

Everything else unsupported (an uncommon `time`/`rand`/`os`/`net` function, for instance) is a
per-call warning with the call left exactly as it was, not a refusal. The rest of the file still
gets rewritten normally.

## The generated file

If anything in the package was rewritten, `Rewrite` also emits `detsim_generated_sched.go` into the
package directory (as part of the overlay, so it exists only for the build, never written to real
disk):

```go
package <pkgname>

import "github.com/arshnah/detsim/rt"

var detsimSched *rt.Sched
var detsimFS *rt.FileSystem
var detsimNet *rt.Network

func DetsimSetSched(s *rt.Sched)       { detsimSched = s }
func DetsimSetFileSystem(fs *rt.FileSystem) { detsimFS = fs }
func DetsimSetNetwork(n *rt.Network)   { detsimNet = n }
```

Rewritten code inside the package references the package-level `detsimSched` / `detsimFS` /
`detsimNet` variables (that's what `schedIdent()` etc. resolve to in the rewritten output), so a
driver (a `_test.go` file, a `main` package, whatever is actually running the rewritten package)
has to call the relevant `DetsimSet*` function(s) *before* running anything that touches the
rewritten code. `rewrite/testdata/*_driver` has small end-to-end examples of this:

```go
sched := rt.NewSched(seed)
pipeline.DetsimSetSched(sched)

sched.Go(func() {
    out = pipeline.Run(ids)
})
if err := sched.Run(); err != nil {
    // ...
}
```

`cmd/detsim-test` does the equivalent wiring inside the test binary itself, driven from environment
variables (`rt.SchedFromEnv`), which is what lets it sweep seeds against a package it rewrote without
that package's own tests needing to know anything about `detsim` at all.
