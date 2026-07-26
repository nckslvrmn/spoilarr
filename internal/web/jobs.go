package web

import "sync"

// jobRunner runs background protect/reveal work keyed by rating key so the UI can
// poll status instead of blocking on a whole-show operation. One job per key.
type jobRunner struct {
	mu sync.Mutex
	m  map[string]*jobState
}

type jobState struct {
	running bool
	message string
}

// start returns false if a job for key is already in flight.
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

func (j *jobRunner) status(key string) (running bool, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if st := j.m[key]; st != nil {
		return st.running, st.message
	}
	return false, ""
}
