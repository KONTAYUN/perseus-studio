// Package runqueue owns bounded in-process agent execution. Jobs for the same
// workspace key never overlap; unrelated users may run concurrently.
package runqueue

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrClosed    = errors.New("agent scheduler is stopping")
	ErrFull      = errors.New("agent queue is full; try again later")
	ErrDuplicate = errors.New("agent run is already scheduled")
)

type job struct {
	id, key string
	ctx     context.Context
	cancel  context.CancelFunc
	run     func(context.Context)
	active  bool
	done    func()
}

type Snapshot struct {
	Running   int  `json:"running"`
	Queued    int  `json:"queued"`
	Capacity  int  `json:"capacity"`
	Accepting bool `json:"accepting"`
}

type Manager struct {
	mu                sync.Mutex
	workers, capacity int
	closed            bool
	jobs              map[string]*job
	queue             []*job
	keys              map[string]bool
	running           int
	drained           chan struct{}
}

func New(workers, capacity int) *Manager {
	if workers < 1 {
		workers = 2
	}
	if capacity < 1 {
		capacity = 64
	}
	return &Manager{workers: workers, capacity: capacity, jobs: map[string]*job{}, keys: map[string]bool{}, drained: make(chan struct{})}
}

func (m *Manager) Submit(id, key string, run func(context.Context), cleanup ...func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if _, exists := m.jobs[id]; exists {
		return ErrDuplicate
	}
	if len(m.jobs) >= m.capacity {
		return ErrFull
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{id: id, key: key, ctx: ctx, cancel: cancel, run: run}
	if len(cleanup) > 0 {
		j.done = cleanup[0]
	}
	m.jobs[id] = j
	m.queue = append(m.queue, j)
	m.dispatchLocked()
	return nil
}

func (m *Manager) dispatchLocked() {
	for index := 0; index < len(m.queue) && m.running < m.workers; {
		j := m.queue[index]
		if m.keys[j.key] {
			index++
			continue
		}
		m.queue = append(m.queue[:index], m.queue[index+1:]...)
		m.keys[j.key] = true
		j.active = true
		m.running++
		go m.execute(j)
	}
}

func (m *Manager) execute(j *job) {
	defer func() {
		if j.done != nil {
			j.done()
		}
		j.cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.jobs, j.id)
		delete(m.keys, j.key)
		m.running--
		if !m.closed {
			m.dispatchLocked()
		}
		if m.closed && m.running == 0 {
			close(m.drained)
		}
	}()
	if j.ctx.Err() == nil {
		j.run(j.ctx)
	}
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return false
	}
	j.cancel()
	if !j.active {
		delete(m.jobs, id)
		if j.done != nil {
			j.done()
		}
		for index, candidate := range m.queue {
			if candidate == j {
				m.queue = append(m.queue[:index], m.queue[index+1:]...)
				break
			}
		}
	}
	return true
}

// Shutdown stops admission, cancels pending and running work, then waits for
// active callbacks to release their resources. It is safe to call repeatedly.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		for id, j := range m.jobs {
			j.cancel()
			if !j.active {
				delete(m.jobs, id)
				if j.done != nil {
					j.done()
				}
			}
		}
		m.queue = nil
		if m.running == 0 {
			close(m.drained)
		}
	}
	m.mu.Unlock()
	select {
	case <-m.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{Running: m.running, Queued: len(m.queue), Capacity: m.capacity, Accepting: !m.closed}
}

func (m *Manager) Contains(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.jobs[id]
	return exists
}
