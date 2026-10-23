package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"webide-backend/internal/config"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestCORSRejectsUntrustedOriginsBeforeHandler(t *testing.T) {
	s := &Server{cfg: config.Config{AllowedOrigins: "https://studio.example"}}
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"https://evil.example", 403}, {"null", 403}, {"https://app.example/forged", 403},
		{"https://app.example", 200}, {"https://studio.example", 200}, {"", 200},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://app.example/api/files/write", nil)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			s.withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d", rec.Code, tc.want)
			}
			if tc.want == 403 && rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("untrusted origin allowed")
			}
		})
	}
}

func TestFilesWriteReturnsConflictWithoutOverwriting(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	factory := usersvc.NewFactory(db, t.TempDir(), nil)
	files, err := factory.Workspace("user1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.WriteFile(t.Context(), "draft.md", "AI version", "test"); err != nil {
		t.Fatal(err)
	}
	s := &Server{factory: factory}
	req := httptest.NewRequest("POST", "/api/files/write", strings.NewReader(`{"path":"draft.md","content":"stale version","expected_hash":"stale-hash"}`))
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "user1"}))
	rec := httptest.NewRecorder()
	s.handleFilesWrite(rec, req)
	if rec.Code != 409 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	file, err := files.ReadFile("draft.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "AI version" {
		t.Fatalf("content overwritten: %q", file.Content)
	}
}

func TestFirstRegistrationIsSerialized(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := &Server{users: users, sessions: sessions}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 6)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(fmt.Sprintf(`{"username":"user%d","password":"password"}`, i)))
			rec := httptest.NewRecorder()
			s.handleAuthRegister(rec, req)
			responses <- rec
		}()
	}
	wg.Wait()
	close(responses)
	admins := 0
	for rec := range responses {
		if strings.Contains(rec.Body.String(), `"role":"admin"`) {
			admins++
		}
		if rec.Code >= 500 {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	if admins != 1 {
		t.Fatalf("first registrations produced %d admins", admins)
	}
}

func TestUploadRejectsOversizedMultipartBody(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	factory := usersvc.NewFactory(db, t.TempDir(), nil)
	s := &Server{factory: factory}
	reader, writer := io.Pipe()
	defer reader.Close()
	form := multipart.NewWriter(writer)
	contentType := form.FormDataContentType()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		_ = form.WriteField("path", "large.txt")
		file, err := form.CreateFormFile("file", "large.txt")
		if err == nil {
			_, _ = io.CopyN(file, uploadZeroReader{}, 66<<20)
		}
		_ = form.Close()
	}()
	req := httptest.NewRequest("POST", "/api/files/upload", reader)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "user1"}))
	rec := httptest.NewRecorder()
	s.handleFilesUpload(rec, req)
	_ = reader.Close()
	<-done
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	files, err := factory.Workspace("user1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.StatPath("large.txt"); err == nil {
		t.Fatal("oversized upload reached disk")
	}
}

type uploadZeroReader struct{}

func (uploadZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
