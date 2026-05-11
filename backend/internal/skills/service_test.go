package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"webide-backend/internal/store"
)

func TestInstallZipSingleSkillNested(t *testing.T) {
	service := newTestService(t)
	raw := makeZip(t, map[string]string{
		"repo-main/skills/foo/SKILL.md":     "---\nname: Foo Skill\ndescription: Helps foo\n---\n# Foo\n",
		"repo-main/skills/foo/assets/a.txt": "asset",
	})
	result, err := service.InstallZip(context.Background(), "test-user-id", raw, true)
	if err != nil {
		t.Fatalf("InstallZip returned error: %v", err)
	}
	if len(result.Installed) != 1 {
		t.Fatalf("expected one installed skill, got %#v", result)
	}
	if result.Installed[0].Name != "Foo Skill" || !result.Installed[0].Enabled {
		t.Fatalf("unexpected installed skill: %#v", result.Installed[0])
	}
	read, err := service.ReadSkill(context.Background(), "test-user-id", result.Installed[0].ID, true)
	if err != nil {
		t.Fatalf("ReadSkill returned error: %v", err)
	}
	if read.Content == "" || read.Truncated {
		t.Fatalf("unexpected read result: %#v", read)
	}
}

func TestInstallZipParsesFoldedDescription(t *testing.T) {
	service := newTestService(t)
	raw := makeZip(t, map[string]string{
		"director-master/SKILL.md": "---\nname: director-master\ndescription: >\n  山音超级导演大师——由 @山音 设计的导演视听技能...\n  与「山音超级编剧大师」联动。\n---\n# Director\n",
	})
	result, err := service.InstallZip(context.Background(), "test-user-id", raw, true)
	if err != nil {
		t.Fatalf("InstallZip returned error: %v", err)
	}
	if got := result.Installed[0].Description; got == ">" || got == "" {
		t.Fatalf("description was not parsed from folded scalar: %q", got)
	}
	want := "山音超级导演大师——由 @山音 设计的导演视听技能... 与「山音超级编剧大师」联动。"
	if result.Installed[0].Description != want {
		t.Fatalf("unexpected folded description:\nwant %q\n got %q", want, result.Installed[0].Description)
	}
}

func TestInstallZipSameNameAndSourceReplacesExisting(t *testing.T) {
	service := newTestService(t)
	raw := makeZip(t, map[string]string{
		"demo-skill/SKILL.md": "---\nname: Demo Skill\ndescription: First\n---\n",
	})
	first, err := service.InstallZip(context.Background(), "test-user-id", raw, true)
	if err != nil {
		t.Fatalf("first InstallZip returned error: %v", err)
	}
	raw = makeZip(t, map[string]string{
		"demo-skill/SKILL.md": "---\nname: Demo Skill\ndescription: Second\n---\n",
	})
	second, err := service.InstallZip(context.Background(), "test-user-id", raw, true)
	if err != nil {
		t.Fatalf("second InstallZip returned error: %v", err)
	}
	if first.Installed[0].ID != second.Installed[0].ID {
		t.Fatalf("expected reinstall to reuse skill id, first=%s second=%s", first.Installed[0].ID, second.Installed[0].ID)
	}
	items, err := service.List(context.Background(), "test-user-id")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].Description != "Second" {
		t.Fatalf("expected one replaced skill with updated description, got %#v", items)
	}
}

func TestInstallZipMultipleCandidates(t *testing.T) {
	service := newTestService(t)
	raw := makeZip(t, map[string]string{
		"a/SKILL.md": "# A\n",
		"b/SKILL.md": "# B\n",
	})
	result, err := service.InstallZip(context.Background(), "test-user-id", raw, true)
	if err != nil {
		t.Fatalf("InstallZip returned error: %v", err)
	}
	if len(result.Candidates) != 2 || result.UploadID == "" {
		t.Fatalf("expected candidates, got %#v", result)
	}
	installed, err := service.InstallSelected(context.Background(), "test-user-id", result.UploadID, []string{result.Candidates[0].ID}, true)
	if err != nil {
		t.Fatalf("InstallSelected returned error: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected one installed selected skill, got %#v", installed)
	}
}

func TestInstallZipRejectsMissingSkill(t *testing.T) {
	service := newTestService(t)
	_, err := service.InstallZip(context.Background(), "test-user-id", makeZip(t, map[string]string{"README.md": "# nope"}), true)
	if err == nil {
		t.Fatalf("expected missing SKILL.md error")
	}
}

func TestInstallZipRejectsZipSlip(t *testing.T) {
	service := newTestService(t)
	_, err := service.InstallZip(context.Background(), "test-user-id", makeZip(t, map[string]string{"../SKILL.md": "# bad"}), true)
	if err == nil {
		t.Fatalf("expected zip slip error")
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := store.NewSkillStore(db)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(t.TempDir(), store)
	if err := service.Ensure(context.Background(), "test-user-id"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return service
}

func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for path, content := range entries {
		file, err := writer.Create(path)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}
