package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

type Config struct {
	WorkerCount  int
	PollInterval time.Duration
}

type Manager struct {
	store         *store.TaskStore
	conversations *store.ConversationStore
	diagnostics   *store.DiagnosticStore
	ai            *ai.Service
	factory       *usersvc.Factory
	broker        *Broker
	cfg           Config
	wg            sync.WaitGroup
}

type MockPayload struct {
	Steps      int    `json:"steps"`
	DelayMS    int    `json:"delay_ms"`
	FailAtStep int    `json:"fail_at_step"`
	Message    string `json:"message"`
}

type AIAgentPayload struct {
	ConversationID  string              `json:"conversation_id"`
	UserMessageID   string              `json:"user_message_id"`
	UserMessage     string              `json:"user_message"`
	ModelProviderID string              `json:"model_provider_id"`
	UserRole        string              `json:"user_role,omitempty"`
	Attachments     json.RawMessage     `json:"attachments"`
	CreatedFromChat bool                `json:"created_from_chat"`
	DecisionReason  string              `json:"decision_reason"`
	Goal            string              `json:"goal"`
	ExpectedOutputs []ai.ExpectedOutput `json:"expected_outputs"`
	SuggestedSteps  []string            `json:"suggested_steps"`
}

func NewManager(taskStore *store.TaskStore, conversationStore *store.ConversationStore, diagnosticStore *store.DiagnosticStore, aiService *ai.Service, factory *usersvc.Factory, broker *Broker, cfg Config) *Manager {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 2
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1500 * time.Millisecond
	}
	return &Manager{
		store:         taskStore,
		conversations: conversationStore,
		diagnostics:   diagnosticStore,
		ai:            aiService,
		factory:       factory,
		broker:        broker,
		cfg:           cfg,
	}
}

func (m *Manager) Start(ctx context.Context) {
	for index := 0; index < m.cfg.WorkerCount; index++ {
		workerID := fmt.Sprintf("worker-%d", index+1)
		m.wg.Add(1)
		go func(id string) {
			defer m.wg.Done()
			m.runWorker(ctx, id)
		}(workerID)
	}
}

func (m *Manager) runWorker(ctx context.Context, workerID string) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, err := m.store.ClaimNextTask(ctx, workerID)
		if err != nil {
			time.Sleep(m.cfg.PollInterval)
			continue
		}
		if task == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		m.publishTask(ctx, task.ID)
		m.executeTask(ctx, *task, workerID)
	}
}
