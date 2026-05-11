package main

import (
	"context"
	"log"
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
	agentRunStore := store.NewAgentRunStore(db)
	if err := agentRunStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate agent run database: %v", err)
	}
	skillStore := store.NewSkillStore(db)
	if err := skillStore.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate skills database: %v", err)
	}

	factory := usersvc.NewFactory(db, cfg.UserDataRoot, skillStore)
	if err := factory.EnsureRoot(); err != nil {
		log.Fatalf("ensure user data root: %v", err)
	}

	if err := os.MkdirAll(cfg.UploadsRoot, 0o755); err != nil {
		log.Fatalf("ensure uploads: %v", err)
	}
	if err := taskStore.RequeueRunningTasks(context.Background()); err != nil {
		log.Fatalf("requeue running tasks: %v", err)
	}

	broker := tasks.NewBroker()
	runBroker := tasks.NewBroker()
	aiService := ai.NewService(configStore, factory, conversationStore, diagnosticStore)
	aiService.AttachRunStore(agentRunStore)
	manager := tasks.NewManager(taskStore, conversationStore, diagnosticStore, aiService, factory, broker, tasks.Config{
		WorkerCount:  cfg.WorkerCount,
		PollInterval: cfg.PollInterval,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	manager.Start(ctx)

	handler := api.NewServer(taskStore, agentRunStore, configStore, conversationStore, diagnosticStore, userStore, sessionStore, broker, runBroker, factory, aiService, cfg)
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler.Routes(),
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
