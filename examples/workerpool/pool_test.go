package workerpool

import (
	"reflect"
	"sort"
	"testing"

	"github.com/arshnah/detsim/rt"
)

func makeJobs(n int) []Job {
	jobs := make([]Job, n)
	for i := 0; i < n; i++ {
		jobs[i] = Job{ID: i, Value: i}
	}
	return jobs
}

func runSeed(seed int64) []Result {
	s := rt.NewSched(seed)
	results, err := Run(s, 4, makeJobs(20))
	if err != nil {
		panic(err)
	}
	return results
}

func TestAllJobsProcessedExactlyOnce(t *testing.T) {
	results := runSeed(1)
	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}
	seen := make(map[int]bool)
	for _, r := range results {
		if seen[r.JobID] {
			t.Fatalf("job %d processed more than once", r.JobID)
		}
		seen[r.JobID] = true
		if r.Sum != r.JobID*r.JobID {
			t.Fatalf("job %d: expected sum %d, got %d", r.JobID, r.JobID*r.JobID, r.Sum)
		}
	}
}

func TestSameSeedIsDeterministic(t *testing.T) {
	a := runSeed(7)
	b := runSeed(7)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed produced different result orderings:\n%v\n%v", a, b)
	}
}

func TestManySeedsNoLostOrDuplicatedWork(t *testing.T) {
	for seed := int64(0); seed < 500; seed++ {
		results := runSeed(seed)
		if len(results) != 20 {
			t.Fatalf("seed %d: expected 20 results, got %d", seed, len(results))
		}
		ids := make([]int, len(results))
		for i, r := range results {
			ids[i] = r.JobID
		}
		sort.Ints(ids)
		for i, id := range ids {
			if id != i {
				t.Fatalf("seed %d: job IDs not exactly 0..19, got %v", seed, ids)
			}
		}
	}
}
