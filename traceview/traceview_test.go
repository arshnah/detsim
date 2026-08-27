package traceview

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arshnah/detsim/rt"
)

func sampleTrace() rt.Trace {
	return rt.Trace{
		Seed:      847291,
		Decisions: []uint64{0, 1, 0, 0, 1},
		Labels:    []string{"sender", "receiver"},
		Steps: []rt.Step{
			{At: 0, Goroutine: 0, After: "chan send"},
			{At: 0, Goroutine: 1, After: "chan recv"},
			{At: 2000, Goroutine: 0, After: "chan send"},
			{At: 4000, Goroutine: 0, After: "finished"},
			{At: 4000, Goroutine: 1, After: "finished"},
		},
	}
}

func render(t *testing.T, fn func(w *bytes.Buffer) error) string {
	t.Helper()
	var b bytes.Buffer
	if err := fn(&b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestViewShowsNamesTimesAndBlockReasons(t *testing.T) {
	out := render(t, func(w *bytes.Buffer) error { return View(w, sampleTrace(), Options{}) })

	for _, want := range []string{
		"seed 847291",
		"5 decisions",
		"2 goroutines",
		"sender",
		"receiver",
		"blocked on chan send",
		"blocked on chan recv",
		"ran to completion",
		"at t=0s",
		"at t=2µs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("view output missing %q:\n%s", want, out)
		}
	}
}

func TestViewFallsBackToBareIdsForOldTraces(t *testing.T) {
	old := rt.Trace{Seed: 1, Decisions: []uint64{3, 3}}
	out := render(t, func(w *bytes.Buffer) error { return View(w, old, Options{}) })
	if !strings.Contains(out, "g3") || !strings.Contains(out, "no step detail") {
		t.Fatalf("old-trace fallback missing:\n%s", out)
	}
	if strings.Contains(out, "at t=") {
		t.Fatalf("old trace should not print step times:\n%s", out)
	}
}

func TestViewLimitKeepsTheTailAndCountsTheSkipped(t *testing.T) {
	tr := sampleTrace()
	out := render(t, func(w *bytes.Buffer) error { return View(w, tr, Options{Limit: 2}) })
	if !strings.Contains(out, "3 earlier decisions omitted") {
		t.Fatalf("limit note missing:\n%s", out)
	}
	if strings.Contains(out, "blocked on chan recv") {
		t.Fatalf("limited view should only show the last 2 decisions:\n%s", out)
	}
	if !strings.Contains(out, "#5") {
		t.Fatalf("numbering should continue from the original trace:\n%s", out)
	}
}

func TestStatsAggregatesPicksAndReasons(t *testing.T) {
	out := render(t, func(w *bytes.Buffer) error { return Stats(w, sampleTrace(), Options{}) })
	// sender picked 3 times, receiver 2; busiest first.
	if !strings.Contains(out, "sender  3 picks") || !strings.Contains(out, "receiver  2 picks") {
		t.Fatalf("pick counts wrong:\n%s", out)
	}
	if !strings.Contains(out, "chan send×2") || !strings.Contains(out, "finished×1") {
		t.Fatalf("block histogram missing:\n%s", out)
	}
	if !strings.Contains(out, "biggest virtual-time jumps") || !strings.Contains(out, "2µs passed") {
		t.Fatalf("gap analysis missing:\n%s", out)
	}
}

func TestDiffMarksKeptAndDropped(t *testing.T) {
	orig := rt.Trace{
		Seed:      5,
		Decisions: []uint64{0, 1, 0, 1, 0, 1, 1},
		Labels:    []string{"a", "b"},
	}
	min := rt.Trace{Seed: 5, Decisions: []uint64{0, 1, 1, 1}}
	out := render(t, func(w *bytes.Buffer) error { return Diff(w, orig, min, Options{}) })

	if !strings.Contains(out, "kept 4 of 7 decisions (57.1%)") {
		t.Fatalf("keep summary wrong:\n%s", out)
	}
	if n := strings.Count(out, "keep "); n != 4 {
		t.Fatalf("expected 4 keep lines, got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "drop "); n != 3 {
		t.Fatalf("expected 3 drop lines, got %d:\n%s", n, out)
	}
}

func TestDiffReportsDriftInsteadOfPretending(t *testing.T) {
	orig := rt.Trace{Seed: 5, Decisions: []uint64{0, 1, 0}}
	min := rt.Trace{Seed: 5, Decisions: []uint64{1, 1, 0}} // needs two 1s, original has one
	out := render(t, func(w *bytes.Buffer) error { return Diff(w, orig, min, Options{}) })
	if !strings.Contains(out, "not a subsequence") {
		t.Fatalf("drift not reported:\n%s", out)
	}
	if !strings.Contains(out, "original shape:") || !strings.Contains(out, "minimized shape:") {
		t.Fatalf("fallback shapes missing:\n%s", out)
	}
}

func TestColorAddsEscapesAndPlainDoesNot(t *testing.T) {
	var plain, colored bytes.Buffer
	if err := View(&plain, sampleTrace(), Options{}); err != nil {
		t.Fatal(err)
	}
	if err := View(&colored, sampleTrace(), Options{Color: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("plain output must not contain ANSI escapes:\n%s", plain.String())
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("colored output has no ANSI escapes:\n%s", colored.String())
	}
}
