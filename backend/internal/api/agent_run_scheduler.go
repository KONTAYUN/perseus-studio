package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"webide-backend/internal/store"
)

type agentRunScheduler struct {
	server     *Server
	maxGlobal  int
	maxPerUser int
	wake       chan struct{}
	startOnce  sync.Once
	started    atomic.Bool

	mu          sync.Mutex
	active      map[string]string
	activeUsers map[string]int
	lastUser    string
}

func newAgentRunScheduler(server *Server, maxGlobal, maxPerUser int) *agentRunScheduler {
	if maxGlobal <= 0 {
		maxGlobal = 4
	}
	if maxPerUser <= 0 {
		maxPerUser = 2
	}
	return &agentRunScheduler{
		server: server, maxGlobal: maxGlobal, maxPerUser: maxPerUser,
		wake: make(chan struct{}, 1), active: map[string]string{}, activeUsers: map[string]int{},
	}
}

func (s *Server) StartAgentRunScheduler(ctx context.Context) {
	if s == nil || s.runScheduler == nil {
		return
	}
	s.runScheduler.Start(ctx)
}

func (s *Server) wakeAgentScheduler() {
	if s == nil || s.runScheduler == nil {
		return
	}
	s.runScheduler.Wake()
}

func (s *Server) ensureAgentQueueCapacity(ctx context.Context, userID string) error {
	if s == nil || s.runs == nil {
		return nil
	}
	global, user, err := s.runs.QueueCounts(ctx, userID)
	if err != nil {
		return err
	}
	if s.cfg.AgentMaxQueuedGlobal > 0 && global >= s.cfg.AgentMaxQueuedGlobal {
		return &store.AgentQueueFullError{Scope: "global", Limit: s.cfg.AgentMaxQueuedGlobal}
	}
	if s.cfg.AgentMaxQueuedPerUser > 0 && user >= s.cfg.AgentMaxQueuedPerUser {
		return &store.AgentQueueFullError{Scope: "user", Limit: s.cfg.AgentMaxQueuedPerUser}
	}
	return nil
}

func (q *agentRunScheduler) Start(ctx context.Context) {
	q.startOnce.Do(func() {
		q.started.Store(true)
		go q.loop(ctx)
		q.Wake()
	})
}

func (q *agentRunScheduler) Ready() bool {
	return q != nil && q.started.Load()
}

func (q *agentRunScheduler) Wake() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *agentRunScheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-ticker.C:
		}
		for q.dispatchOne(ctx) {
		}
	}
}

func (q *agentRunScheduler) dispatchOne(ctx context.Context) bool {
	q.mu.Lock()
	if len(q.active) >= q.maxGlobal {
		q.mu.Unlock()
		return false
	}
	q.mu.Unlock()

	queued, err := q.server.runs.ListQueuedRuns(ctx, 200)
	if err != nil {
		slog.Error("agent scheduler failed to list queued runs", "error", err)
		return false
	}
	candidate, ok := q.chooseCandidate(queued)
	if !ok {
		return false
	}
	claimed, claimedOK, err := q.server.runs.ClaimQueuedRun(ctx, candidate.UserID, candidate.ID)
	if err != nil {
		slog.Error("agent scheduler failed to claim run", "run_id", candidate.ID, "user_id", candidate.UserID, "error", err)
		return false
	}
	if !claimedOK {
		return true
	}

	q.mu.Lock()
	q.active[claimed.ID] = claimed.UserID
	q.activeUsers[claimed.UserID]++
	q.lastUser = claimed.UserID
	activeSlots := len(q.active)
	q.mu.Unlock()
	queueWait := time.Duration(0)
	if claimed.QueuedAt.Valid {
		queueWait = time.Since(claimed.QueuedAt.Time)
	}
	slog.Info("agent run dispatched",
		"run_id", claimed.ID,
		"conversation_id", claimed.ConversationID,
		"user_id", claimed.UserID,
		"queue_wait_ms", queueWait.Milliseconds(),
		"active_slots", activeSlots,
		"max_slots", q.maxGlobal,
	)
	go func() {
		defer q.release(claimed.ID, claimed.UserID)
		q.server.executePersistedConversationRun(claimed)
	}()
	return true
}

func (q *agentRunScheduler) chooseCandidate(queued []store.AgentRun) (store.AgentRun, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.active) >= q.maxGlobal || len(queued) == 0 {
		return store.AgentRun{}, false
	}
	users := make([]string, 0)
	firstByUser := map[string]store.AgentRun{}
	for _, run := range queued {
		if q.activeUsers[run.UserID] >= q.maxPerUser {
			continue
		}
		if _, exists := firstByUser[run.UserID]; exists {
			continue
		}
		users = append(users, run.UserID)
		firstByUser[run.UserID] = run
	}
	if len(users) == 0 {
		return store.AgentRun{}, false
	}
	start := 0
	for index, userID := range users {
		if userID == q.lastUser {
			start = (index + 1) % len(users)
			break
		}
	}
	return firstByUser[users[start]], true
}

func (q *agentRunScheduler) release(runID, userID string) {
	q.mu.Lock()
	delete(q.active, runID)
	if q.activeUsers[userID] <= 1 {
		delete(q.activeUsers, userID)
	} else {
		q.activeUsers[userID]--
	}
	q.mu.Unlock()
	q.Wake()
}

func (s *Server) executePersistedConversationRun(run store.AgentRun) {
	user, err := s.users.GetUser(context.Background(), run.UserID)
	if err != nil || user.Disabled || user.Role == store.UserRolePending {
		if err == nil {
			err = errors.New("run user is not allowed to execute")
		}
		s.failQueuedConversationRun(run, err)
		return
	}
	if !run.UserMessageID.Valid || run.UserMessageID.String == "" {
		s.failQueuedConversationRun(run, errors.New("queued run has no user message"))
		return
	}
	message, err := s.conversations.GetMessage(context.Background(), run.UserID, run.UserMessageID.String)
	if err != nil {
		s.failQueuedConversationRun(run, err)
		return
	}
	attachments := s.payloadsFromStoredMessageAttachments(context.Background(), run.UserID, message)
	s.executeConversationRun(run.UserID, user.Role, run.ConversationID, message.ID, run.ModelProviderID, message.Content, message.AttachmentsJSON, attachments, run.ThinkingMode, run.ID)
}

func (s *Server) failQueuedConversationRun(run store.AgentRun, cause error) {
	message := "queued agent run could not be restored"
	if cause != nil {
		message = cause.Error()
	}
	_, _, err := s.finalizeRunStrict(context.Background(), run.UserID, run.ID, store.AgentRunFailed, "queue_restore_failed", store.UpdateAgentRunInput{
		ErrorMessage: &message, SetCompletedAt: true,
	}, store.AgentEventInput{
		ConversationID: run.ConversationID, Type: "run_failed", Level: "error",
		Payload: map[string]any{"failure_class": "queue_restore_failed", "message": message},
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("agent scheduler failed to finalize queued run", "run_id", run.ID, "conversation_id", run.ConversationID, "user_id", run.UserID, "error", err)
	}
}
