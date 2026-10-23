package usersvc

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"webide-backend/internal/skills"
	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

type Factory struct {
	db           *sql.DB
	userDataRoot string
	skillStore   *store.SkillStore
	mu           sync.Mutex
	workspaces   map[string]*workspace.Service
	skillsMap    map[string]*skills.Service
}

func NewFactory(db *sql.DB, userDataRoot string, skillStore *store.SkillStore) *Factory {
	return &Factory{
		db:           db,
		userDataRoot: filepath.Clean(userDataRoot),
		skillStore:   skillStore,
		workspaces:   make(map[string]*workspace.Service),
		skillsMap:    make(map[string]*skills.Service),
	}
}

func (f *Factory) Workspace(userID string) (*workspace.Service, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required for workspace access")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if svc, ok := f.workspaces[userID]; ok {
		return svc, nil
	}
	root := filepath.Join(f.userDataRoot, userID, "workspace")
	svc, err := workspace.NewService(f.db, root, userID)
	if err != nil {
		return nil, fmt.Errorf("create workspace service: %w", err)
	}
	if err := svc.Migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate workspace: %w", err)
	}
	if err := svc.EnsureWorkspace(); err != nil {
		return nil, fmt.Errorf("ensure workspace: %w", err)
	}
	recoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	recoveryErr := svc.RecoverPendingPatches(recoveryCtx)
	cancel()
	if recoveryErr != nil {
		log.Printf("[workspace] pending patch recovery requires inspection user=%s error=%v", userID, recoveryErr)
	}
	f.workspaces[userID] = svc
	return svc, nil
}

func (f *Factory) Skills(userID string) (*skills.Service, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required for skills access")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if svc, ok := f.skillsMap[userID]; ok {
		return svc, nil
	}
	root := filepath.Join(f.userDataRoot, userID, "skills")
	svc := skills.NewService(root, f.skillStore)
	if err := svc.Ensure(context.Background(), userID); err != nil {
		return nil, fmt.Errorf("ensure skills: %w", err)
	}
	f.skillsMap[userID] = svc
	return svc, nil
}

func (f *Factory) UploadsDir(userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID is required for uploads access")
	}
	dir := filepath.Join(f.userDataRoot, userID, "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (f *Factory) EnsureUserDirs(userID string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	base := filepath.Join(f.userDataRoot, userID)
	for _, sub := range []string{"workspace", "uploads", "skills"} {
		if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (f *Factory) EnsureRoot() error {
	return os.MkdirAll(f.userDataRoot, 0o755)
}

func (f *Factory) RemoveUserData(userID string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	f.mu.Lock()
	delete(f.workspaces, userID)
	delete(f.skillsMap, userID)
	f.mu.Unlock()
	return os.RemoveAll(filepath.Join(f.userDataRoot, userID))
}
