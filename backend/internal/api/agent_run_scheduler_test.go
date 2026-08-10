package api

import (
	"fmt"
	"testing"

	"webide-backend/internal/store"
)

func TestAgentRunSchedulerRotatesAcrossEligibleUsers(t *testing.T) {
	scheduler := newAgentRunScheduler(nil, 4, 2)
	queued := []store.AgentRun{
		{ID: "a1", UserID: "user-a"},
		{ID: "a2", UserID: "user-a"},
		{ID: "b1", UserID: "user-b"},
		{ID: "c1", UserID: "user-c"},
	}

	first, ok := scheduler.chooseCandidate(queued)
	if !ok || first.ID != "a1" {
		t.Fatalf("expected first queued user, got %#v ok=%v", first, ok)
	}
	scheduler.lastUser = "user-a"
	second, ok := scheduler.chooseCandidate(queued)
	if !ok || second.ID != "b1" {
		t.Fatalf("expected rotation to user-b, got %#v ok=%v", second, ok)
	}
	scheduler.lastUser = "user-b"
	third, ok := scheduler.chooseCandidate(queued)
	if !ok || third.ID != "c1" {
		t.Fatalf("expected rotation to user-c, got %#v ok=%v", third, ok)
	}
}

func TestAgentRunSchedulerSkipsUsersAtConcurrencyLimit(t *testing.T) {
	scheduler := newAgentRunScheduler(nil, 4, 2)
	scheduler.active = map[string]string{"a-running-1": "user-a", "a-running-2": "user-a"}
	scheduler.activeUsers = map[string]int{"user-a": 2}
	queued := []store.AgentRun{
		{ID: "a3", UserID: "user-a"},
		{ID: "b1", UserID: "user-b"},
	}

	candidate, ok := scheduler.chooseCandidate(queued)
	if !ok || candidate.ID != "b1" {
		t.Fatalf("expected eligible user-b run, got %#v ok=%v", candidate, ok)
	}
	scheduler.active["b-running-1"] = "user-b"
	scheduler.active["b-running-2"] = "user-b"
	scheduler.activeUsers["user-b"] = 2
	if candidate, ok := scheduler.chooseCandidate(queued); ok {
		t.Fatalf("expected no eligible run, got %#v", candidate)
	}
}

func TestAgentRunSchedulerCapsTwentyUsersAtGlobalLimit(t *testing.T) {
	scheduler := newAgentRunScheduler(nil, 4, 2)
	queued := make([]store.AgentRun, 0, 20)
	for index := 0; index < 20; index++ {
		queued = append(queued, store.AgentRun{ID: fmt.Sprintf("run-%02d", index), UserID: fmt.Sprintf("user-%02d", index)})
	}
	selectedUsers := map[string]bool{}
	for len(scheduler.active) < scheduler.maxGlobal {
		candidate, ok := scheduler.chooseCandidate(queued)
		if !ok {
			t.Fatal("expected another eligible run")
		}
		scheduler.active[candidate.ID] = candidate.UserID
		scheduler.activeUsers[candidate.UserID]++
		scheduler.lastUser = candidate.UserID
		selectedUsers[candidate.UserID] = true
	}
	if len(scheduler.active) != 4 || len(selectedUsers) != 4 {
		t.Fatalf("expected four global slots shared by four users, active=%d users=%d", len(scheduler.active), len(selectedUsers))
	}
	if candidate, ok := scheduler.chooseCandidate(queued); ok {
		t.Fatalf("global limit admitted an extra run: %#v", candidate)
	}
}
