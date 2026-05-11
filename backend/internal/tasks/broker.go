package tasks

import "sync"

type Event struct {
	Type string `json:"type"`
	Task any    `json:"task,omitempty"`
	Log  any    `json:"log,omitempty"`
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

func (b *Broker) Subscribe(taskID string) (<-chan Event, func()) {
	ch := make(chan Event, 32)

	b.mu.Lock()
	if _, ok := b.subscribers[taskID]; !ok {
		b.subscribers[taskID] = make(map[chan Event]struct{})
	}
	b.subscribers[taskID][ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, ok := b.subscribers[taskID]; ok {
			if _, exists := subs[ch]; exists {
				delete(subs, ch)
				close(ch)
			}
			if len(subs) == 0 {
				delete(b.subscribers, taskID)
			}
		}
	}

	return ch, unsubscribe
}

func (b *Broker) Publish(taskID string, event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers[taskID] {
		select {
		case ch <- event:
		default:
		}
	}
}
