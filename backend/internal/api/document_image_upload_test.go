package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
	"webide-backend/internal/workspace"
)

func TestDocumentImageUploadValidatesBytesAndIsolatesUsers(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	factory := usersvc.NewFactory(db, t.TempDir(), nil)
	server := &Server{factory: factory}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=")
	for _, tc := range []struct {
		name   string
		data   []byte
		status int
	}{
		{"valid bytes with misleading extension.txt", png, 201},
		{"fake.png", []byte("ordinary text"), 415},
		{"unsafe.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 415},
		{"too-large.png", append(png, make([]byte, 16<<20)...), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			part, err := form.CreateFormFile("file", tc.name)
			if err != nil {
				t.Fatal(err)
			}
			part.Write(tc.data)
			form.Close()
			req := httptest.NewRequest("POST", "/api/documents/images", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "user1"}))
			rec := httptest.NewRecorder()
			server.handleDocumentImages(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if rec.Code != 201 {
				return
			}
			var image workspace.DocumentImage
			if err := json.Unmarshal(rec.Body.Bytes(), &image); err != nil {
				t.Fatal(err)
			}
			files, _ := factory.Workspace("user1")
			tree, err := files.Tree()
			if err != nil || len(tree) != 0 {
				t.Fatalf("upload visible: %+v %v", tree, err)
			}
			if image.Name != tc.name || filepath.Ext(image.Path) != ".png" {
				t.Fatal(image)
			}
			for _, user := range []string{"user1", "user2"} {
				request := httptest.NewRequest("GET", "/api/files/image?path="+image.Path, nil)
				request = request.WithContext(context.WithValue(request.Context(), ctxSessionKey, &SessionInfo{UserID: user}))
				response := httptest.NewRecorder()
				server.handleFilesImage(response, request)
				if user == "user1" && (response.Code != 200 || !bytes.Equal(response.Body.Bytes(), png)) {
					t.Fatalf("own image unavailable: %d", response.Code)
				}
				if user == "user2" && response.Code != 404 {
					t.Fatalf("other user image exposed: %d", response.Code)
				}
			}
		})
	}
}
