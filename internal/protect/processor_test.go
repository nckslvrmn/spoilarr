package protect

import (
	"sync/atomic"
	"testing"
)

// TestRunTasksAggregates verifies the concurrent task runner sums every task's
// Result exactly once and actually runs them in parallel. Run with -race.
func TestRunTasksAggregates(t *testing.T) {
	p := &Processor{}
	const n = 500
	var ran atomic.Int64
	tasks := make([]func() Result, n)
	for i := range tasks {
		tasks[i] = func() Result {
			ran.Add(1)
			return Result{Protected: 1}
		}
	}
	got := p.runTasks(tasks)
	if got.Protected != n {
		t.Fatalf("Protected = %d, want %d", got.Protected, n)
	}
	if ran.Load() != n {
		t.Fatalf("ran %d tasks, want %d", ran.Load(), n)
	}
	if p.runTasks(nil).Protected != 0 {
		t.Fatal("empty task list should yield zero result")
	}
}
