package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	UserRolePending = "pending"
	UserRoleUser    = "user"
	UserRoleAdmin   = "admin"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'user',
  disabled INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS system_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);
`)
	return err
}

func (s *UserStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *UserStore) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0`, UserRoleAdmin).Scan(&count)
	return count, err
}

func (s *UserStore) CreateUser(ctx context.Context, username, password, role string) (User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 2 || len(username) > 32 {
		return User{}, fmt.Errorf("username must be 2-32 characters")
	}
	if password == "" {
		return User{}, fmt.Errorf("password is required")
	}
	if !IsValidUserRole(role) {
		return User{}, fmt.Errorf("role must be pending, user or admin")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	now := time.Now().UTC()
	id := nextStoreID("user", now)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO users (id, username, password_hash, role, disabled, created_at, updated_at)
VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, username, string(hash), role, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, fmt.Errorf("username already exists")
		}
		return User{}, err
	}
	return User{ID: id, Username: username, Role: role, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *UserStore) Authenticate(ctx context.Context, username, password string) (User, error) {
	user, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return User{}, fmt.Errorf("invalid username or password")
	}
	if user.Disabled {
		return User{}, fmt.Errorf("account is disabled")
	}
	if user.Role == UserRolePending {
		return User{}, fmt.Errorf("account is pending approval")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return User{}, fmt.Errorf("invalid username or password")
	}
	return user, nil
}

func (s *UserStore) GetUserByUsername(ctx context.Context, username string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, disabled, created_at, updated_at
FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *UserStore) GetUser(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, disabled, created_at, updated_at
FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *UserStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, password_hash, role, disabled, created_at, updated_at
FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) UpdateRole(ctx context.Context, id, role string) error {
	if !IsValidUserRole(role) {
		return fmt.Errorf("role must be pending, user or admin")
	}
	if role != UserRoleAdmin {
		if err := s.ensureNotLastAdmin(ctx, id); err != nil {
			return err
		}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, role, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *UserStore) SetDisabled(ctx context.Context, id string, disabled bool) error {
	if disabled {
		if err := s.ensureNotLastAdmin(ctx, id); err != nil {
			return err
		}
	}
	v := 0
	if disabled {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET disabled = ?, updated_at = ? WHERE id = ?`, v, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *UserStore) ResetPassword(ctx context.Context, id, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("password is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, string(hash), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *UserStore) DeleteUser(ctx context.Context, id string) error {
	if err := s.ensureNotLastAdmin(ctx, id); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *UserStore) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *UserStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC())
	return err
}

func (s *UserStore) IsRegistrationAllowed(ctx context.Context) (bool, error) {
	val, err := s.GetSetting(ctx, "allow_registration")
	if err != nil {
		return false, err
	}
	if val == "" {
		return true, nil
	}
	return val == "true", nil
}

func (s *UserStore) SetRegistrationAllowed(ctx context.Context, allowed bool) error {
	v := "false"
	if allowed {
		v = "true"
	}
	return s.SetSetting(ctx, "allow_registration", v)
}

func (s *UserStore) DefaultUserRole(ctx context.Context) (string, error) {
	val, err := s.GetSetting(ctx, "default_user_role")
	if err != nil {
		return "", err
	}
	if val == "" {
		return UserRolePending, nil
	}
	if !IsValidUserRole(val) {
		return UserRolePending, nil
	}
	return val, nil
}

func (s *UserStore) SetDefaultUserRole(ctx context.Context, role string) error {
	if !IsValidUserRole(role) {
		return fmt.Errorf("role must be pending, user or admin")
	}
	return s.SetSetting(ctx, "default_user_role", role)
}

func IsValidUserRole(role string) bool {
	switch role {
	case UserRolePending, UserRoleUser, UserRoleAdmin:
		return true
	default:
		return false
	}
}

func (s *UserStore) ensureNotLastAdmin(ctx context.Context, id string) error {
	user, err := s.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if user.Role != UserRoleAdmin {
		return nil
	}
	count, err := s.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return fmt.Errorf("cannot modify the last admin")
	}
	return nil
}

func scanUser(s scanner) (User, error) {
	var u User
	var disabled int
	err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &disabled, &u.CreatedAt, &u.UpdatedAt)
	u.Disabled = disabled != 0
	return u, err
}
