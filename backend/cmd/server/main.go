package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/api"
	"webide-backend/internal/config"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
	"webide-backend/internal/usersvc"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	userStore := store.NewUserStore(db)
	if err := userStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate users: %v", err)
	}
	sessionStore := store.NewSessionStore(db)
	if err := sessionStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate sessions: %v", err)
	}
	if err := sessionStore.DeleteExpired(context.Background(), time.Now()); err != nil {
		log.Fatalf("cleanup sessions: %v", err)
	}

	if cfg.AdminUsername != "" && cfg.AdminPassword != "" {
		count, _ := userStore.CountUsers(context.Background())
		if count == 0 {
			if _, err := userStore.CreateUser(context.Background(), cfg.AdminUsername, cfg.AdminPassword, "admin"); err != nil {
				log.Fatalf("create initial admin: %v", err)
			}
			log.Printf("created initial admin user: %s", cfg.AdminUsername)
		}
	}

	taskStore := store.NewTaskStore(db)
	if err := taskStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	configStore := store.NewConfigStore(db)
	if err := configStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate config database: %v", err)
	}
	conversationStore := store.NewConversationStore(db)
	if err := conversationStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate conversation database: %v", err)
	}
	diagnosticStore := store.NewDiagnosticStore(db)
	if err := diagnosticStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate diagnostic database: %v", err)
	}
	usageStore := store.NewUsageStore(db)
	if err := usageStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate usage database: %v", err)
	}
	if info := store.LastDBRuntimeInfo(); info.RiskyWALMount {
		_, _ = diagnosticStore.Append(context.Background(), store.DiagnosticLogInput{
			UserID:         "system",
			Stage:          "sqlite_storage_mode",
			Level:          "warn",
			ErrorMessage:   "SQLite WAL is enabled on a network-style mount; use SQLITE_JOURNAL_MODE=DELETE for reliable persistence.",
			RequestSummary: info.MountType + " " + info.MountPoint,
		})
	}
	agentRunStore := store.NewAgentRunStore(db)
	if err := agentRunStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate agent run database: %v", err)
	}
	runningRuns, err := agentRunStore.ListRunningRuns(context.Background(), 1000)
	if err != nil {
		log.Fatalf("list interrupted agent runs: %v", err)
	}
	for _, run := range runningRuns {
		messageID := ""
		if run.UserMessageID.Valid {
			messageID = run.UserMessageID.String
		}
		if _, _, requeueErr := agentRunStore.RequeueInterruptedRunWithEvent(context.Background(), store.RequeueInterruptedAgentRunInput{
			RunID: run.ID, UserID: run.UserID,
			Event: store.AgentEventInput{
				ID:             fmt.Sprintf("evt_requeue_%s_%d", run.ID, run.EventHeadSeq+1),
				ConversationID: run.ConversationID, MessageID: messageID,
				Type: "run_requeued_after_restart", Level: "info",
				Payload: map[string]any{"reason": "service_restart", "resume_mode": "checkpoint"},
			},
		}); requeueErr != nil {
			log.Fatalf("requeue interrupted agent run %s: %v", run.ID, requeueErr)
		}
	}
	if len(runningRuns) > 0 {
		log.Printf("requeued %d interrupted agent run(s) from checkpoint", len(runningRuns))
	}
	skillStore := store.NewSkillStore(db)
	if err := skillStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate skills database: %v", err)
	}

	factory := usersvc.NewFactory(db, cfg.UserDataRoot, skillStore)
	if err := factory.EnsureRoot(); err != nil {
		log.Fatalf("ensure user data root: %v", err)
	}

	if err := taskStore.RequeueRunningTasks(context.Background()); err != nil {
		log.Fatalf("requeue running tasks: %v", err)
	}

	broker := tasks.NewBroker()
	runBroker := tasks.NewBroker()
	aiService := ai.NewService(configStore, factory, conversationStore, diagnosticStore)
	aiService.AttachUsageStore(usageStore)
	aiService.AttachRunStore(agentRunStore)
	manager := tasks.NewManager(taskStore, conversationStore, diagnosticStore, aiService, factory, broker, tasks.Config{
		WorkerCount:  cfg.WorkerCount,
		PollInterval: cfg.PollInterval,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	manager.Start(ctx)

	handler := api.NewServer(taskStore, agentRunStore, configStore, conversationStore, diagnosticStore, usageStore, userStore, sessionStore, broker, runBroker, factory, aiService, cfg)
	handler.StartChatAttachmentCleanup(ctx)
	handler.StartAgentRunScheduler(ctx)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	log.Printf("backend listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
