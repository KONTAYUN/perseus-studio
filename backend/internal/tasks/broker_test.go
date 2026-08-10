package tasks

import "testing"

func TestBrokerCoalescesWakeupsWithoutBlockingPublisher(t *testing.T) {
	broker := NewBroker()
	notifications, unsubscribe := broker.Subscribe("run-1")
	defer unsubscribe()

	for index := 0; index < 10_000; index++ {
		broker.Publish("run-1", Event{Type: "event"})
	}

	select {
	case <-notifications:
	default:
		t.Fatal("expected a pending database wake-up notification")
	}
	stats := broker.Stats()
	if stats.Published != 10_000 {
		t.Fatalf("published = %d, want 10000", stats.Published)
	}
	if stats.Coalesced == 0 {
		t.Fatal("expected redundant notifications to be coalesced")
	}
}

func TestBrokerRetainsOnlyLatestTransientState(t *testing.T) {
	broker := NewBroker()
	broker.PublishLatest("run-1", "preview-1", Event{Type: "preview", Log: 1})
	broker.PublishLatest("run-1", "preview-1", Event{Type: "preview", Log: 2})

	items := broker.Latest("run-1")
	if len(items) != 1 || items[0].Log != 2 {
		t.Fatalf("latest state = %#v, want one event with log 2", items)
	}
	broker.DeleteLatestTopic("run-1")
	if items := broker.Latest("run-1"); len(items) != 0 {
		t.Fatalf("latest state was not cleared: %#v", items)
	}
}
