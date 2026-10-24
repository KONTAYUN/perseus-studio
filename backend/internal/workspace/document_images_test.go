package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func uploadTestImage(t *testing.T, s *Service) DocumentImage {
	t.Helper()
	image, err := s.SaveDocumentImage(t.Context(), []byte("fixture image"), ".png", "示意图.png")
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func ageTestImage(t *testing.T, s *Service, path string, now time.Time) {
	t.Helper()
	old := now.Add(-DocumentImageGrace - time.Hour).Unix()
	_, err := s.db.Exec(`UPDATE document_image_leases SET touched_at=?,orphan_since=? WHERE user_id=? AND path=?`, old, old, s.userID, path)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDocumentImagesHiddenButAvailableInLibrary(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	image := uploadTestImage(t, s)
	legacy := "图片/59cb6f94-646d-4f3c-bbf5-b0e11d615135-old.png"
	for _, path := range []string{legacy, "图片/my-photo.png"} {
		if _, err := s.CreateBytes(ctx, path, []byte("fixture image")); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Name != "图片" || len(tree[0].Children) != 1 || tree[0].Children[0].Name != "my-photo.png" {
		t.Fatalf("unexpected tree: %+v", tree)
	}
	items, err := s.DocumentImages(ctx)
	if err != nil || len(items) != 3 {
		t.Fatalf("%+v %v", items, err)
	}
	for _, item := range items {
		if item.Path == image.Path && item.Name != "示意图.png" {
			t.Fatal(item)
		}
	}
	if _, err := s.ReadDocumentImage(ctx, image.Path); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{DocumentImageDir, "59cb6f94"} {
		results, err := s.Search(query, 20)
		if err != nil || len(results) != 0 {
			t.Fatalf("hidden search: %+v %v", results, err)
		}
	}
	if err := os.Remove(filepath.Join(s.rootAbs, "图片", "my-photo.png")); err != nil {
		t.Fatal(err)
	}
	tree, err = s.Tree()
	if err != nil || len(tree) != 0 {
		t.Fatalf("legacy folder still visible: %+v %v", tree, err)
	}
	listed, _, err := s.List("", 3, 100)
	if err != nil || len(listed) != 0 {
		t.Fatalf("hidden list: %+v %v", listed, err)
	}
}

func TestDocumentImageCollectionGraceAndPersistedLeases(t *testing.T) {
	s := newTestService(t)
	image := uploadTestImage(t, s)
	now := time.Now()
	if n, err := s.CollectDocumentImages(t.Context(), now.Add(6*24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("new upload removed: %d %v", n, err)
	}
	reloaded, err := NewService(s.db, s.rootAbs, s.userID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := reloaded.CollectDocumentImages(t.Context(), now.Add(8*24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("orphan not collected: %d %v", n, err)
	}
	if _, err := s.ReadFile(image.Path); !IsNotExist(err) {
		t.Fatalf("file remains: %v", err)
	}
}

func TestDocumentImageCollectionRetainsEveryReferenceAndUserFiles(t *testing.T) {
	for _, source := range []string{"document", "history", "trash", "html", "encoded", "shared"} {
		t.Run(source, func(t *testing.T) {
			s := newTestService(t)
			image := uploadTestImage(t, s)
			ctx := t.Context()
			now := time.Now()
			text := "![图](" + image.Src + ")"
			if source == "html" {
				text = `<img src="` + image.Src + `">`
			}
			if source == "encoded" {
				text = strings.ReplaceAll(image.Src, "a", "%61")
			}
			if _, err := s.CreateFile(ctx, "文稿.md", text); err != nil {
				t.Fatal(err)
			}
			if source == "history" {
				if _, err := s.WriteFile(ctx, "文稿.md", "没有图片", "test"); err != nil {
					t.Fatal(err)
				}
			}
			if source == "trash" {
				if _, err := s.MoveToTrash("文稿.md"); err != nil {
					t.Fatal(err)
				}
			}
			if source == "shared" {
				if _, err := s.CreateFile(ctx, "另一篇.md", text); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(s.rootAbs, "文稿.md")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.CreateBytes(ctx, "user-image.png", []byte("user image")); err != nil {
				t.Fatal(err)
			}
			ageTestImage(t, s, image.Path, now)
			if n, err := s.CollectDocumentImages(ctx, now); err != nil || n != 0 {
				t.Fatalf("referenced image removed: %d %v", n, err)
			}
			if _, err := s.ReadFile(image.Path); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ReadFile("user-image.png"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDocumentImageCollectionReadRenewsLeaseAndScanFailureRetainsFiles(t *testing.T) {
	s := newTestService(t)
	image := uploadTestImage(t, s)
	now := time.Now()
	ageTestImage(t, s, image.Path, now)
	if _, err := s.ReadDocumentImage(t.Context(), image.Path); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CollectDocumentImages(t.Context(), now); err != nil || n != 0 {
		t.Fatalf("displayed image removed: %d %v", n, err)
	}
	ageTestImage(t, s, image.Path, now)
	if _, err := s.db.Exec(`DROP TABLE file_versions`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CollectDocumentImages(t.Context(), now); err == nil || n != 0 {
		t.Fatalf("incomplete scan deleted files: %d %v", n, err)
	}
	if _, err := s.ReadFile(image.Path); err != nil {
		t.Fatal(err)
	}
}
