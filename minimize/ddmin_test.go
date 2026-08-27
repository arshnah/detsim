package minimize

import (
	"reflect"
	"testing"
)

func containsSubsequence(haystack, needle []uint64) bool {
	if len(needle) == 0 {
		return true
	}
	i := 0
	for _, v := range haystack {
		if v == needle[i] {
			i++
			if i == len(needle) {
				return true
			}
		}
	}
	return false
}

func TestDdminReducesToTheRequiredMarkers(t *testing.T) {
	decisions := []uint64{1, 2, 3, 4, 5, 99, 6, 7, 8, 9, 10, 88, 11, 12}
	required := []uint64{99, 88}

	stillFails := func(c []uint64) bool {
		return containsSubsequence(c, required)
	}

	result := Ddmin(decisions, stillFails)
	if !containsSubsequence(result, required) {
		t.Fatalf("minimized trace %v lost the required markers %v", result, required)
	}
	if len(result) != len(required) {
		t.Fatalf("expected ddmin to shrink to exactly %v, got %v", required, result)
	}
}

func TestDdminOnAlreadyMinimalTraceIsANoop(t *testing.T) {
	decisions := []uint64{1, 2}
	stillFails := func(c []uint64) bool { return len(c) == 2 }

	result := Ddmin(decisions, stillFails)
	if !reflect.DeepEqual(result, decisions) {
		t.Fatalf("expected no change, got %v", result)
	}
}

func TestDdminNeverCallsStillFailsWithTheSameTraceTwice(t *testing.T) {
	decisions := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	calls := 0
	seen := make(map[string]bool)

	stillFails := func(c []uint64) bool {
		calls++
		key := traceKey(c)
		if seen[key] {
			t.Fatalf("stillFails called twice with the same trace %v", c)
		}
		seen[key] = true
		return len(c) >= 3
	}

	Ddmin(decisions, stillFails)
	if calls == 0 {
		t.Fatal("expected at least one stillFails call")
	}
}
