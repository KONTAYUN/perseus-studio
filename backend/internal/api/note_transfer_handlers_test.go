package api

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"webide-backend/internal/workspace"

	_ "modernc.org/sqlite"
)

func TestInlineHTMLAssetsForExportEmbedsLocalImages(t *testing.T) {
	files := newAPITestWorkspace(t)
	if _, err := files.CreateBytes(context.Background(), ".assets/Note/pixel.png", []byte("pngdata")); err != nil {
		t.Fatalf("CreateBytes returned error: %v", err)
	}

	html := `<p>hello</p><img src="/api/files/content?path=.assets%2FNote%2Fpixel.png" data-note-src=".assets/Note/pixel.png" alt="pixel">`
	got := inlineHTMLAssetsForExport(html, "Note.html", files)

	if !strings.Contains(got, `src="data:image/png;base64,cG5nZGF0YQ=="`) {
		t.Fatalf("exported HTML did not inline image data: %s", got)
	}
	if !strings.Contains(got, `data-note-src=".assets/Note/pixel.png"`) {
		t.Fatalf("exported HTML should retain data-note-src metadata: %s", got)
	}
}

func TestSanitizeHTMLFragmentRemovesExecutableMarkup(t *testing.T) {
	got := sanitizeHTMLFragment(`<p onclick="run()">safe<a href="javascript:alert(1)">link</a><img src='javascript:alert(2)' alt="x"><script>alert(3)</script><style>body{color:red}</style></p>`)

	for _, forbidden := range []string{"onclick", "javascript:", "<script", "<style"} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Fatalf("sanitized fragment still contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, `<p>safe<a>link</a><img alt="x"></p>`) {
		t.Fatalf("sanitized fragment removed unexpected safe markup: %s", got)
	}
}

func newAPITestWorkspace(t *testing.T) *workspace.Service {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	files, err := workspace.NewService(db, t.TempDir(), "test-user")
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	if err := files.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if err := files.EnsureWorkspace(); err != nil {
		t.Fatalf("EnsureWorkspace returned error: %v", err)
	}
	return files
}
