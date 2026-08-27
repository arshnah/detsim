package workerpool

import "github.com/arshnah/detsim/rt"

// Job is a unit of work to be processed by a worker.
type Job struct {
	ID    int
	Value int
}

// Result is the outcome of processing a Job, holding the original JobID and a computed Sum.
type Result struct {
	JobID int
	Sum   int
}

// Run distributes jobs across numWorkers goroutines on the deterministic scheduler and
// collects results. It returns all results once every job has been processed.
func Run(s *rt.Sched, numWorkers int, jobs []Job) ([]Result, error) {
	var results []Result

	s.Go(func() {
		jobCh := rt.NewChan[Job](s, len(jobs))
		resultCh := rt.NewChan[Result](s, len(jobs))
		workersDone := rt.NewWaitGroup(s)
		collectorDone := rt.NewWaitGroup(s)

		for _, j := range jobs {
			jobCh.Send(j)
		}
		jobCh.Close()

		workersDone.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			s.Go(func() {
				defer workersDone.Done()
				for {
					j, ok := jobCh.RecvOK()
					if !ok {
						return
					}
					resultCh.Send(Result{JobID: j.ID, Sum: j.Value * j.Value})
				}
			})
		}

		collectorDone.Add(1)
		s.Go(func() {
			defer collectorDone.Done()
			for {
				r, ok := resultCh.RecvOK()
				if !ok {
					return
				}
				results = append(results, r)
			}
		})

		workersDone.Wait()
		resultCh.Close()
		collectorDone.Wait()
	})

	err := s.Run()
	return results, err
}
