package store

import (
	"context"
	"database/sql"
	"time"
)

type Session struct {
	Token     string
	UserID    string
	Username  string
	Role      string
	Expires   time.Time
	CreatedAt time.Time
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS user_sessions (
  token TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
`)
	return err
}

func (s *SessionStore) CreateSession(ctx context.Context, token, userID string, expires time.Time) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_sessions (token, user_id, expires_at, created_at)
VALUES (?, ?, ?, ?)`,
		token, userID, expires.UTC(), now)
	return err
}

func (s *SessionStore) GetSession(ctx context.Context, token string, now time.Time) (Session, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT s.token, u.id, u.username, u.role, s.expires_at, s.created_at, u.disabled
FROM user_sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = ?`, token)
	var session Session
	var disabled int
	if err := row.Scan(&session.Token, &session.UserID, &session.Username, &session.Role, &session.Expires, &session.CreatedAt, &disabled); err != nil {
		if err == sql.ErrNoRows {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}
	if now.After(session.Expires) || disabled != 0 {
		if err := s.DeleteSession(ctx, token); err != nil {
			return Session{}, false, err
		}
		return Session{}, false, nil
	}
	return session, true, nil
}

func (s *SessionStore) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE token = ?`, token)
	return err
}

func (s *SessionStore) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ?`, userID)
	return err
}

func (s *SessionStore) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE expires_at <= ?`, now.UTC())
	return err
}
