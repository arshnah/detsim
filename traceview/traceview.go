// Package traceview renders rt traces for humans: an annotated timeline of the
// scheduler's decisions, per-goroutine stats, and a keep/drop diff showing exactly
// which decisions a minimized trace kept. Plain text out, ANSI color optional.
package traceview

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/arshnah/detsim/rt"
)

// Options control rendering. Color emits ANSI escapes; Limit caps how many decisions
// View prints (0 means all).
type Options struct {
	Color bool
	Limit int
}

var palette = []string{"39", "203", "71", "214", "170", "75", "230", "135"}

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

func (o Options) color(code, s string) string {
	if !o.Color {
		return s
	}
	return "\x1b[38;5;" + code + "m" + s + ansiReset
}

func (o Options) wrap(code, s string) string {
	if !o.Color {
		return s
	}
	return code + s + ansiReset
}

func goroutineColor(id uint64) string {
	return palette[id%uint64(len(palette))]
}

func (o Options) gname(tr rt.Trace, id uint64) string {
	return o.color(goroutineColor(id), tr.Name(id))
}

// Header renders the one-line summary every subcommand shares.
func Header(tr rt.Trace) string {
	span := virtualSpan(tr)
	spanStr := "no time advanced"
	if span > 0 {
		spanStr = "t=0 → " + span.String()
	}
	return fmt.Sprintf("seed %d · %d %s · %d %s · %s",
		tr.Seed,
		len(tr.Decisions), plural("decision", len(tr.Decisions)),
		len(tr.Labels), plural("goroutine", len(tr.Labels)),
		spanStr)
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func virtualSpan(tr rt.Trace) rt.VirtualTime {
	var maxAt rt.VirtualTime
	for _, s := range tr.Steps {
		if s.At > maxAt {
			maxAt = s.At
		}
	}
	return maxAt
}

// View writes the annotated decision timeline: one line per pick, with the virtual
// time it ran at, the goroutine by name, and what that goroutine did when its turn
// ended. Traces recorded before steps existed fall back to bare goroutine ids.
func View(w io.Writer, tr rt.Trace, opt Options) error {
	fmt.Fprintln(w, opt.wrap(ansiBold, Header(tr)))
	fmt.Fprintln(w)

	decisions := tr.Decisions
	if opt.Limit > 0 && len(decisions) > opt.Limit {
		skipped := len(decisions) - opt.Limit
		fmt.Fprintf(w, "%s\n", opt.wrap(ansiDim, fmt.Sprintf("... %d earlier decisions omitted, use -limit=0 to see everything ...", skipped)))
		decisions = decisions[skipped:]
	}
	startOffset := len(tr.Decisions) - len(decisions)

	numWidth := len(fmt.Sprint(len(tr.Decisions)))
	for i, id := range decisions {
		num := i + 1 + startOffset
		line := fmt.Sprintf("  %s %s %s  %s",
			opt.wrap(ansiDim, fmt.Sprintf("#%*d", numWidth, num)),
			opt.gname(tr, id),
			opt.wrap(ansiDim, describeStep(tr, i+startOffset)),
			opt.wrap(ansiDim, stepTime(tr, i+startOffset)),
		)
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func describeStep(tr rt.Trace, decisionIdx int) string {
	if decisionIdx >= len(tr.Steps) {
		return "picked (trace has no step detail, recorded before detsim-trace existed)"
	}
	s := tr.Steps[decisionIdx]
	switch s.After {
	case "finished":
		return "ran to completion"
	case "":
		return "ran, kept running to the end of the trace"
	default:
		return "blocked on " + s.After
	}
}

func stepTime(tr rt.Trace, decisionIdx int) string {
	if decisionIdx >= len(tr.Steps) {
		return ""
	}
	return "at t=" + tr.Steps[decisionIdx].At.String()
}

// Stats writes per-goroutine pick counts, what each one blocked on, and the largest
// virtual-time gaps in the run.
func Stats(w io.Writer, tr rt.Trace, opt Options) error {
	fmt.Fprintln(w, opt.wrap(ansiBold, Header(tr)))
	fmt.Fprintln(w)

	type perG struct {
		id      uint64
		picks   int
		blocked map[string]int
		first   int
		last    int
	}
	byG := map[uint64]*perG{}
	var order []uint64
	for i, id := range tr.Decisions {
		g := byG[id]
		if g == nil {
			g = &perG{id: id, blocked: map[string]int{}, first: i, last: i}
			byG[id] = g
			order = append(order, id)
		}
		g.picks++
		g.last = i
		if i < len(tr.Steps) {
			reason := tr.Steps[i].After
			if reason == "" {
				reason = "(ran to end)"
			}
			g.blocked[reason]++
		}
	}
	sort.Slice(order, func(a, b int) bool {
		if byG[order[a]].picks != byG[order[b]].picks {
			return byG[order[a]].picks > byG[order[b]].picks
		}
		return order[a] < order[b]
	})

	fmt.Fprintln(w, opt.wrap(ansiBold, "goroutines, busiest first:"))
	for _, id := range order {
		g := byG[id]
		var reasons []string
		for r, n := range g.blocked {
			reasons = append(reasons, fmt.Sprintf("%s×%d", r, n))
		}
		sort.Strings(reasons)
		fmt.Fprintf(w, "  %s  %d picks (first at decision %d, last at %d)  %s\n",
			opt.gname(tr, id), g.picks, g.first+1, g.last+1,
			strings.Join(reasons, ", "))
	}

	if len(tr.Steps) > 1 {
		type gap struct {
			at rt.VirtualTime
			id uint64
		}
		var gaps []gap
		for i := 1; i < len(tr.Steps); i++ {
			d := tr.Steps[i].At - tr.Steps[i-1].At
			if d > 0 {
				gaps = append(gaps, gap{at: d, id: tr.Steps[i].Goroutine})
			}
		}
		sort.Slice(gaps, func(a, b int) bool { return gaps[a].at > gaps[b].at })
		if len(gaps) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, opt.wrap(ansiBold, "biggest virtual-time jumps:"))
			n := len(gaps)
			if n > 3 {
				n = 3
			}
			for _, g := range gaps[:n] {
				fmt.Fprintf(w, "  %s passed before %s ran again\n", g.at.String(), opt.gname(tr, g.id))
			}
		}
	}
	return nil
}

// Diff renders a minimized trace against the original it came from: every original
// decision marked as kept or dropped, so you can see the skeleton the minimizer
// decided was actually load-bearing. A minimized trace should be a subsequence of the
// original; if replay leniency made it drift, that's reported instead of rendered.
func Diff(w io.Writer, orig, min rt.Trace, opt Options) error {
	fmt.Fprintln(w, opt.wrap(ansiBold, "original: ")+Header(orig))
	fmt.Fprintln(w, opt.wrap(ansiBold, "minimized: ")+Header(min))
	fmt.Fprintln(w)

	keptAt := subsequenceMarks(orig.Decisions, min.Decisions)
	if keptAt == nil {
		fmt.Fprintf(w, "%s\n", opt.wrap(ansiYellow, "the minimized trace is not a subsequence of the original, replay drifted somewhere; showing both shapes instead"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, opt.wrap(ansiBold, "original shape:"))
		fmt.Fprintln(w, "  "+shapeString(orig.Decisions))
		fmt.Fprintln(w, opt.wrap(ansiBold, "minimized shape:"))
		fmt.Fprintln(w, "  "+shapeString(min.Decisions))
		return nil
	}

	fmt.Fprintf(w, "the minimizer kept %s of %d decisions (%.1f%%)\n",
		opt.wrap(ansiGreen, fmt.Sprint(len(min.Decisions))), len(orig.Decisions),
		100*float64(len(min.Decisions))/float64(max(1, len(orig.Decisions))))
	fmt.Fprintln(w)

	for i, id := range orig.Decisions {
		if keptAt[i] {
			fmt.Fprintf(w, "  %s %s  #%d\n", opt.wrap(ansiGreen, "keep "), opt.gname(orig, id), i+1)
		} else {
			fmt.Fprintf(w, "  %s %s  #%d\n", opt.wrap(ansiDim, "drop "), opt.gname(orig, id), i+1)
		}
	}
	return nil
}

func shapeString(decisions []uint64) string {
	parts := make([]string, 0, len(decisions))
	for _, d := range decisions {
		parts = append(parts, fmt.Sprintf("g%d", d))
	}
	return strings.Join(parts, " → ")
}

// subsequenceMarks returns one bool per original decision, true where that decision is
// part of the subsequence, or nil if min is not a subsequence of orig.
func subsequenceMarks(orig, min []uint64) []bool {
	marks := make([]bool, len(orig))
	j := 0
	for i, o := range orig {
		if j < len(min) && o == min[j] {
			marks[i] = true
			j++
		}
	}
	if j != len(min) {
		return nil
	}
	return marks
}
