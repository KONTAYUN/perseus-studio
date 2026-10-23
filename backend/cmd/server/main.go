package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/api"
	"webide-backend/internal/config"
	"webide-backend/internal/runqueue"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
	"webide-backend/internal/usersvc"
)

func main() {
	cfg := config.Load()
	lockPath, err := store.DatabaseRuntimeLockPath(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("resolve database ownership: %v", err)
	}
	dbLock, err := store.AcquireRuntimeLock(lockPath)
	if err != nil {
		log.Fatalf("acquire database ownership: %v", err)
	}
	defer dbLock.Close()
	rootLock, err := store.AcquireRuntimeLock(filepath.Join(cfg.UserDataRoot, ".server.lock"))
	if err != nil {
		log.Fatalf("acquire workspace ownership: %v", err)
	}
	defer rootLock.Close()

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
	if count, err := agentRunStore.FailStaleRunningRuns(context.Background(), 0, "上次服务停止或本地状态保存中断，本轮已暂停，可点击继续本轮。"); err != nil {
		log.Fatalf("recover stale agent runs: %v", err)
	} else if count > 0 {
		log.Printf("marked %d stale agent run(s) as recoverable failed", count)
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
	scheduler := runqueue.New(cfg.AgentConcurrency, cfg.AgentQueueCapacity)
	manager := tasks.NewManager(taskStore, conversationStore, diagnosticStore, aiService, factory, broker, tasks.Config{
		WorkerCount:  cfg.WorkerCount,
		PollInterval: cfg.PollInterval,
	}, scheduler)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	manager.Start(ctx)

	handler := api.NewServer(taskStore, agentRunStore, configStore, conversationStore, diagnosticStore, usageStore, userStore, sessionStore, broker, runBroker, factory, aiService, cfg, scheduler)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := handler.ShutdownRuns(shutdownCtx); err != nil {
			log.Printf("agent shutdown: %v", err)
		}
		if err := manager.Wait(shutdownCtx); err != nil {
			log.Printf("task shutdown: %v", err)
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	log.Printf("backend listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
	<-shutdownDone
}
