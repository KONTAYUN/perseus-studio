package tasks

import (
	"sync"
	"sync/atomic"
)

type Event struct {
	Type string `json:"type"`
	Task any    `json:"task,omitempty"`
	Log  any    `json:"log,omitempty"`
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan struct{}]struct{}
	latest      map[string]map[string]Event
	published   atomic.Uint64
	coalesced   atomic.Uint64
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]map[chan struct{}]struct{}),
		latest:      make(map[string]map[string]Event),
	}
}

// Subscribe returns a coalescing wake-up signal. Durable event payloads must be
// read from their store after every signal; this channel is not an event log.
func (b *Broker) Subscribe(taskID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	b.mu.Lock()
	if _, ok := b.subscribers[taskID]; !ok {
		b.subscribers[taskID] = make(map[chan struct{}]struct{})
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
	_ = event
	b.notify(taskID)
}

// PublishLatest stores a replaceable transient state before waking subscribers.
// It is suitable for previews, where intermediate frames may be coalesced but
// the newest complete state must remain recoverable.
func (b *Broker) PublishLatest(topic, key string, event Event) {
	b.mu.Lock()
	if b.latest[topic] == nil {
		b.latest[topic] = make(map[string]Event)
	}
	b.latest[topic][key] = event
	b.mu.Unlock()
	b.notify(topic)
}

func (b *Broker) Latest(topic string) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := make([]Event, 0, len(b.latest[topic]))
	for _, event := range b.latest[topic] {
		items = append(items, event)
	}
	return items
}

func (b *Broker) DeleteLatest(topic, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.latest[topic], key)
	if len(b.latest[topic]) == 0 {
		delete(b.latest, topic)
	}
}

func (b *Broker) DeleteLatestTopic(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.latest, topic)
}

func (b *Broker) notify(taskID string) {
	b.published.Add(1)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers[taskID] {
		select {
		case ch <- struct{}{}:
		default:
			b.coalesced.Add(1)
		}
	}
}

type BrokerStats struct {
	Published uint64 `json:"published"`
	Coalesced uint64 `json:"coalesced"`
}

func (b *Broker) Stats() BrokerStats {
	return BrokerStats{
		Published: b.published.Load(),
		Coalesced: b.coalesced.Load(),
	}
}
