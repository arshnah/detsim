// Package minimize implements delta-debugging (ddmin) over a scheduler decision trace, shrinking
// a failing seed's trace down to a minimal one that still reproduces the failure.
package minimize

import (
	"fmt"
	"strings"
)

type span struct {
	start, end int
}

// Ddmin shrinks decisions to the smallest subsequence for which stillFails still returns true.
func Ddmin(decisions []uint64, stillFails func([]uint64) bool) []uint64 {
	cached := memoize(stillFails)

	c := append([]uint64(nil), decisions...)
	n := 2

	for len(c) >= 2 {
		reducedThisRound := false

		for _, sp := range split(len(c), n) {
			complement := without(c, sp)
			if cached(complement) {
				c = complement
				if n > 2 {
					n--
				}
				reducedThisRound = true
				break
			}
		}

		if reducedThisRound {
			continue
		}
		if n >= len(c) {
			break
		}
		n = min(n*2, len(c))
	}

	return c
}

func split(length, n int) []span {
	if n > length {
		n = length
	}
	if n == 0 {
		return nil
	}
	chunkSize := (length + n - 1) / n
	var spans []span
	for start := 0; start < length; start += chunkSize {
		end := start + chunkSize
		if end > length {
			end = length
		}
		spans = append(spans, span{start, end})
	}
	return spans
}

func without(c []uint64, sp span) []uint64 {
	out := make([]uint64, 0, len(c)-(sp.end-sp.start))
	out = append(out, c[:sp.start]...)
	out = append(out, c[sp.end:]...)
	return out
}

func memoize(stillFails func([]uint64) bool) func([]uint64) bool {
	seen := make(map[string]bool)
	return func(c []uint64) bool {
		key := traceKey(c)
		if result, ok := seen[key]; ok {
			return result
		}
		result := stillFails(c)
		seen[key] = result
		return result
	}
}

func traceKey(c []uint64) string {
	var b strings.Builder
	for _, d := range c {
		fmt.Fprintf(&b, "%d,", d)
	}
	return b.String()
}
