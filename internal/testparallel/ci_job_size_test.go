package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestCICappedJobsPreserveEveryTestAndExternalAssignment(t *testing.T) {
	t.Parallel()
	const limit = 100
	for _, workers := range []int{2, 4} {
		t.Run(fmt.Sprintf("workers-%d", workers), func(t *testing.T) {
			t.Parallel()
			for _, size := range []int{0, 1, 99, 100, 101, 199, 200, 201, 1678, 2004} {
				names := make([]string, size)
				for i := range names {
					names[i] = fmt.Sprintf("Test%04d", i)
				}
				seen := make(map[string]int, size)
				for external := range 2 {
					const pkg = "example.com/complete-tests"
					selected := selectExternalShard(pkg, names, external, 2)
					jobs := makeTestJobs(pkg, names, workers, limit, external, 2)
					if !reflect.DeepEqual(jobs, makeTestJobs(pkg, names, workers, limit, external, 2)) {
						t.Fatalf("size %d shard %d: nondeterministic jobs", size, external)
					}
					wantJobs := min(len(selected), max(workers, (len(selected)+limit-1)/limit))
					if len(jobs) != wantJobs {
						t.Fatalf("size %d shard %d: jobs=%d want=%d", size, external, len(jobs), wantJobs)
					}
					for i, job := range jobs {
						if job.pkg != pkg || job.shard != i || job.shardCount != len(jobs) || len(job.names) == 0 || len(job.names) > limit {
							t.Fatalf("size %d shard %d: invalid job %+v", size, external, job)
						}
						for _, name := range job.names {
							if externalShardForName(pkg, name, 2) != external {
								t.Fatalf("size %d: external assignment changed for %s", size, name)
							}
							seen[name]++
						}
					}
				}
				if len(seen) != len(names) {
					t.Fatalf("size %d: distinct names=%d want=%d", size, len(seen), len(names))
				}
				for _, name := range names {
					if seen[name] != 1 {
						t.Fatalf("size %d: %s selected %d times, want once", size, name, seen[name])
					}
				}
			}
		})
	}
}
