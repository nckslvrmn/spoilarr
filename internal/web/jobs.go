package web

import "sync"

// jobRunner tracks background protect/reveal operations keyed by rating key, so
// the UI can poll status without blocking the request while a whole show is
// processed. One job per key at a time.
type jobRunner struct {
	mu sync.Mutex
	m  map[string]*jobState
}

type jobState struct {
	running bool
	message string
}

// start launches fn in the background unless a job for key is already running.
// Returns false if one was already in flight.
func (j *jobRunner) start(key string, fn func() string) bool {
	j.mu.Lock()
	if j.m == nil {
		j.m = map[string]*jobState{}
	}
	if st := j.m[key]; st != nil && st.running {
		j.mu.Unlock()
		return false
	}
	j.m[key] = &jobState{running: true}
	j.mu.Unlock()

	go func() {
		msg := fn()
		j.mu.Lock()
		j.m[key] = &jobState{running: false, message: msg}
		j.mu.Unlock()
	}()
	return true
}

// status reports whether a job for key is running and its last message.
func (j *jobRunner) status(key string) (running bool, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if st := j.m[key]; st != nil {
		return st.running, st.message
	}
	return false, ""
}
