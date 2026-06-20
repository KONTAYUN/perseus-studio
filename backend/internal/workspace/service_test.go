package workspace

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReorderPathPersistsMixedManualOrder(t *testing.T) {
	ctx := context.Background()
	db, root := newTestDBAndRoot(t)
	files := newTestServiceWithDBAndRoot(t, db, root)

	if _, err := files.MkdirNew("folder"); err != nil {
		t.Fatalf("MkdirNew returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "a.txt", "a"); err != nil {
		t.Fatalf("CreateFile a returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "b.txt", "b"); err != nil {
		t.Fatalf("CreateFile b returned error: %v", err)
	}
	if _, err := files.ReorderPath("folder", "", "a.txt", "b.txt"); err != nil {
		t.Fatalf("ReorderPath returned error: %v", err)
	}

	reloaded, err := NewService(db, root, "test-user")
	if err != nil {
		t.Fatalf("NewService reload returned error: %v", err)
	}
	if err := reloaded.Migrate(ctx); err != nil {
		t.Fatalf("Migrate reload returned error: %v", err)
	}
	if err := reloaded.EnsureWorkspace(); err != nil {
		t.Fatalf("EnsureWorkspace reload returned error: %v", err)
	}
	tree, err := reloaded.Tree()
	if err != nil {
		t.Fatalf("Tree returned error: %v", err)
	}
	if got, want := treeNames(tree), []string{"a.txt", "folder", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected manual order after reload: got %#v want %#v", got, want)
	}

	if _, err := reloaded.Rename("folder", "renamed"); err != nil {
		t.Fatalf("Rename returned error: %v", err)
	}
	tree, err = reloaded.Tree()
	if err != nil {
		t.Fatalf("Tree after rename returned error: %v", err)
	}
	if got, want := treeNames(tree), []string{"a.txt", "renamed", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected manual order after rename: got %#v want %#v", got, want)
	}

	if _, err := reloaded.MoveToTrash("renamed"); err != nil {
		t.Fatalf("MoveToTrash returned error: %v", err)
	}
	tree, err = reloaded.Tree()
	if err != nil {
		t.Fatalf("Tree after trash returned error: %v", err)
	}
	if got, want := treeNames(tree), []string{"a.txt", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order after trash cleanup: got %#v want %#v", got, want)
	}
}

func TestReorderPathMovesAcrossFolders(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	for _, path := range []string{"source", "target"} {
		if _, err := files.MkdirNew(path); err != nil {
			t.Fatalf("MkdirNew %s returned error: %v", path, err)
		}
	}
	if _, err := files.CreateFile(ctx, "source/move.txt", "move"); err != nil {
		t.Fatalf("CreateFile move returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "target/a.txt", "a"); err != nil {
		t.Fatalf("CreateFile a returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "target/b.txt", "b"); err != nil {
		t.Fatalf("CreateFile b returned error: %v", err)
	}

	result, err := files.ReorderPath("source/move.txt", "target", "target/a.txt", "target/b.txt")
	if err != nil {
		t.Fatalf("ReorderPath returned error: %v", err)
	}
	if result.NewPath != "target/move.txt" || result.OldPath != "source/move.txt" {
		t.Fatalf("unexpected path change result: %#v", result)
	}
	if _, err := files.ReadFile("target/move.txt"); err != nil {
		t.Fatalf("ReadFile moved path returned error: %v", err)
	}

	tree, err := files.Tree()
	if err != nil {
		t.Fatalf("Tree returned error: %v", err)
	}
	target := findTreeNode(tree, "target")
	if target == nil {
		t.Fatal("expected target folder in tree")
	}
	if got, want := treeNames(target.Children), []string{"a.txt", "move.txt", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected target child order: got %#v want %#v", got, want)
	}
}

func TestReorderPathRejectsMovingFolderIntoItself(t *testing.T) {
	files := newTestService(t)

	if _, err := files.MkdirNew("folder"); err != nil {
		t.Fatalf("MkdirNew folder returned error: %v", err)
	}
	if _, err := files.MkdirNew("folder/child"); err != nil {
		t.Fatalf("MkdirNew child returned error: %v", err)
	}
	if _, err := files.ReorderPath("folder", "folder", "", ""); !errors.Is(err, ErrMoveIntoSelf) {
		t.Fatalf("expected ErrMoveIntoSelf for self target, got %v", err)
	}
	if _, err := files.ReorderPath("folder", "folder/child", "", ""); !errors.Is(err, ErrMoveIntoSelf) {
		t.Fatalf("expected ErrMoveIntoSelf for child target, got %v", err)
	}
}

func TestMoveHTMLNoteRewritesLocalAssetReferences(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateBytes(ctx, ".assets/Note/cover.png", []byte("image")); err != nil {
		t.Fatalf("CreateBytes asset returned error: %v", err)
	}
	html := `<p>hello</p><img src="/api/files/content?path=.assets%2FNote%2Fcover.png" data-note-src=".assets/Note/cover.png" alt="cover">`
	if _, err := files.CreateFile(ctx, "Note.html", "<p>draft</p>"); err != nil {
		t.Fatalf("CreateFile note returned error: %v", err)
	}
	if _, err := files.WriteFile(ctx, "Note.html", html, "test_write"); err != nil {
		t.Fatalf("WriteFile note returned error: %v", err)
	}
	if _, err := files.MkdirNew("archive"); err != nil {
		t.Fatalf("MkdirNew returned error: %v", err)
	}

	result, err := files.MovePath("Note.html", "archive")
	if err != nil {
		t.Fatalf("MovePath returned error: %v", err)
	}
	if result.NewPath != "archive/Note.html" {
		t.Fatalf("unexpected move result: %#v", result)
	}
	read, err := files.ReadFile("archive/Note.html")
	if err != nil {
		t.Fatalf("ReadFile moved note returned error: %v", err)
	}
	if !strings.Contains(read.Content, `data-note-src="../.assets/Note/cover.png"`) {
		t.Fatalf("moved note did not rewrite data-note-src: %s", read.Content)
	}
	if !strings.Contains(read.Content, `src="../.assets/Note/cover.png"`) {
		t.Fatalf("moved note did not rewrite src: %s", read.Content)
	}
	if _, _, err := files.ReadBytes(".assets/Note/cover.png"); err != nil {
		t.Fatalf("asset should remain at original workspace path: %v", err)
	}
	versions, err := files.ListFileVersions(ctx, "archive/Note.html", 10)
	if err != nil {
		t.Fatalf("ListFileVersions moved note returned error: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected moved note to retain prior versions, got %#v", versions)
	}
}

func TestSearchFindsPathsAndSkipsTrash(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.MkdirNew("docs"); err != nil {
		t.Fatalf("MkdirNew docs returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "docs/Alpha Note.md", "hello"); err != nil {
		t.Fatalf("CreateFile alpha returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "docs/beta.txt", "hello"); err != nil {
		t.Fatalf("CreateFile beta returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "trashed-alpha.txt", "gone"); err != nil {
		t.Fatalf("CreateFile trash target returned error: %v", err)
	}
	if _, err := files.MoveToTrash("trashed-alpha.txt"); err != nil {
		t.Fatalf("MoveToTrash returned error: %v", err)
	}

	results, err := files.Search("alpha", 20)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if got, want := searchPaths(results), []string{"docs/Alpha Note.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected search results: got %#v want %#v", got, want)
	}
	if results[0].ParentPath != "docs" || results[0].Type != "file" {
		t.Fatalf("unexpected result metadata: %#v", results[0])
	}

	results, err = files.Search("docs", 20)
	if err != nil {
		t.Fatalf("Search docs returned error: %v", err)
	}
	if got := searchPaths(results); !reflect.DeepEqual(got, []string{"docs", "docs/Alpha Note.md", "docs/beta.txt"}) {
		t.Fatalf("unexpected path search results: %#v", got)
	}

	results, err = files.Search("", 20)
	if err != nil {
		t.Fatalf("Search empty returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty query to return no results, got %#v", results)
	}
}

func TestFindFilesSupportsQueryGlobAndSkipsIgnoredDirs(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.MkdirNew("src"); err != nil {
		t.Fatalf("MkdirNew src returned error: %v", err)
	}
	if _, err := files.MkdirNew("node_modules"); err != nil {
		t.Fatalf("MkdirNew node_modules returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "src/App.jsx", "export default function App() {}"); err != nil {
		t.Fatalf("CreateFile App returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "node_modules/App.jsx", "ignored"); err != nil {
		t.Fatalf("CreateFile ignored returned error: %v", err)
	}

	results, err := files.FindFiles("app", "*.jsx", 20)
	if err != nil {
		t.Fatalf("FindFiles returned error: %v", err)
	}
	if got, want := searchPaths(results), []string{"src/App.jsx"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected find results: got %#v want %#v", got, want)
	}
}

func TestSearchTextReturnsLineMetadata(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.MkdirNew("src"); err != nil {
		t.Fatalf("MkdirNew src returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "src/App.jsx", "one\nconst needle = true;\nthree\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	results, limited, err := files.SearchText("needle", false, 20, 1)
	if err != nil {
		t.Fatalf("SearchText returned error: %v", err)
	}
	if limited {
		t.Fatal("did not expect limited search")
	}
	if len(results) != 1 {
		t.Fatalf("expected one match, got %#v", results)
	}
	if results[0].Path != "src/App.jsx" || results[0].Line != 2 || results[0].Column != 7 || results[0].Before != "one" || results[0].After != "three" {
		t.Fatalf("unexpected search result metadata: %#v", results[0])
	}
}

func TestSearchTextScopedFiltersPathsAndGlobs(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.MkdirNew("docs"); err != nil {
		t.Fatalf("MkdirNew docs returned error: %v", err)
	}
	if _, err := files.MkdirNew("src"); err != nil {
		t.Fatalf("MkdirNew src returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "docs/a.md", "needle docs\n"); err != nil {
		t.Fatalf("CreateFile docs/a returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "docs/b.txt", "needle txt\n"); err != nil {
		t.Fatalf("CreateFile docs/b returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "src/a.md", "needle src\n"); err != nil {
		t.Fatalf("CreateFile src/a returned error: %v", err)
	}

	results, limited, err := files.SearchTextScoped("needle", false, 20, 0, TextSearchOptions{
		PathScope:    "docs",
		IncludeGlobs: []string{"*.md"},
		ExcludeGlobs: []string{"docs/skip*"},
	})
	if err != nil {
		t.Fatalf("SearchTextScoped returned error: %v", err)
	}
	if limited {
		t.Fatal("did not expect limited search")
	}
	if got, want := textSearchPaths(results), []string{"docs/a.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected scoped search paths: got %#v want %#v", got, want)
	}
}

func TestReadTextFileRangeAndEditTextFile(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateFile(ctx, "draft.txt", "one\ntwo\nthree\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	read, err := files.ReadTextFileRange("draft.txt", 2, 2, 20000)
	if err != nil {
		t.Fatalf("ReadTextFileRange returned error: %v", err)
	}
	if read.Content != "two\n" || read.StartLine != 2 || read.EndLine != 2 || read.TotalLines != 4 {
		t.Fatalf("unexpected ranged read: %#v", read)
	}
	if read.NumberedContent != "2: two\n" {
		t.Fatalf("unexpected numbered content: %q", read.NumberedContent)
	}

	result, replacements, err := files.EditTextFile(ctx, "draft.txt", "two", "TWO", "test_edit", false)
	if err != nil {
		t.Fatalf("EditTextFile returned error: %v", err)
	}
	if replacements != 1 || result.Path != "draft.txt" {
		t.Fatalf("unexpected edit result: result=%#v replacements=%d", result, replacements)
	}
	after, err := files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile after edit returned error: %v", err)
	}
	if after.Content != "one\nTWO\nthree\n" {
		t.Fatalf("unexpected edited content: %q", after.Content)
	}
}

func TestEditTextFileSmartUsesTrimmedLineMatch(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateFile(ctx, "draft.txt", "intro\n  target line  \nend\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	result, err := files.EditTextFileSmart(ctx, "draft.txt", "target line\n", "replacement\n", "test_edit", false)
	if err != nil {
		t.Fatalf("EditTextFileSmart returned error: %v", err)
	}
	if result.MatchKind != TextEditMatchTrimmedLines || result.Replacements != 1 {
		t.Fatalf("unexpected smart edit result: %#v", result)
	}
	after, err := files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile after edit returned error: %v", err)
	}
	if after.Content != "intro\nreplacement\nend\n" {
		t.Fatalf("unexpected edited content: %q", after.Content)
	}
}

func TestEditTextFileSmartRejectsOmissionPlaceholder(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateFile(ctx, "draft.txt", "one\ntwo\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	_, err := files.EditTextFileSmart(ctx, "draft.txt", "one\n", "ONE\n其余不变", "test_edit", false)
	if err == nil || !strings.Contains(err.Error(), "omission placeholder") {
		t.Fatalf("expected omission placeholder error, got %v", err)
	}
}

func TestTrashItemsRestoreAndConflict(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateFile(ctx, "docs/a.txt", "hello"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	moved, err := files.MoveToTrash("docs/a.txt")
	if err != nil {
		t.Fatalf("MoveToTrash returned error: %v", err)
	}

	tree, err := files.Tree()
	if err != nil {
		t.Fatalf("Tree returned error: %v", err)
	}
	if hasTreePath(tree, TrashDirName) {
		t.Fatalf("trash root should not be listed in workspace tree: %#v", tree)
	}
	if _, err := files.ReadFile(moved.NewPath); err == nil {
		t.Fatal("expected trash file to be blocked by normal read API")
	}
	if _, _, err := files.List(TrashDirName, 2, 50); err == nil {
		t.Fatal("expected trash folder to be blocked by normal list API")
	}

	items, err := files.TrashItems()
	if err != nil {
		t.Fatalf("TrashItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one trash item, got %#v", items)
	}
	if items[0].Path != moved.NewPath || items[0].OriginalPath != "docs/a.txt" {
		t.Fatalf("unexpected trash metadata: %#v", items[0])
	}

	if _, err := files.CreateFile(ctx, "docs/a.txt", "conflict"); err != nil {
		t.Fatalf("CreateFile conflict target returned error: %v", err)
	}
	if _, err := files.RestoreTrashItem(items[0].Path, false); !errors.Is(err, ErrPathExists) {
		t.Fatalf("expected ErrPathExists, got %v", err)
	}
	if _, err := files.RestoreTrashItem(items[0].Path, true); err != nil {
		t.Fatalf("RestoreTrashItem overwrite returned error: %v", err)
	}
	read, err := files.ReadFile("docs/a.txt")
	if err != nil {
		t.Fatalf("ReadFile restored path returned error: %v", err)
	}
	if read.Content != "hello" {
		t.Fatalf("expected restored content to overwrite target, got %q", read.Content)
	}
	items, err = files.TrashItems()
	if err != nil {
		t.Fatalf("TrashItems after restore returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected trash to be empty after restore, got %#v", items)
	}
}

func TestDeleteTrashItemPermanently(t *testing.T) {
	ctx := context.Background()
	files := newTestService(t)

	if _, err := files.CreateFile(ctx, "delete.txt", "bye"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	moved, err := files.MoveToTrash("delete.txt")
	if err != nil {
		t.Fatalf("MoveToTrash returned error: %v", err)
	}
	if _, err := files.DeleteTrashItem(moved.NewPath); err != nil {
		t.Fatalf("DeleteTrashItem returned error: %v", err)
	}
	items, err := files.TrashItems()
	if err != nil {
		t.Fatalf("TrashItems returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected trash to be empty after permanent delete, got %#v", items)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	db, root := newTestDBAndRoot(t)
	return newTestServiceWithDBAndRoot(t, db, root)
}

func newTestDBAndRoot(t *testing.T) (*sql.DB, string) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db, t.TempDir()
}

func newTestServiceWithDBAndRoot(t *testing.T, db *sql.DB, root string) *Service {
	t.Helper()

	files, err := NewService(db, root, "test-user")
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

func treeNames(nodes []TreeNode) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}
	return names
}

func searchPaths(nodes []SearchResult) []string {
	paths := make([]string, 0, len(nodes))
	for _, node := range nodes {
		paths = append(paths, node.Path)
	}
	return paths
}

func textSearchPaths(nodes []TextSearchResult) []string {
	paths := make([]string, 0, len(nodes))
	for _, node := range nodes {
		paths = append(paths, node.Path)
	}
	return paths
}

func findTreeNode(nodes []TreeNode, path string) *TreeNode {
	for index := range nodes {
		if nodes[index].Path == path {
			return &nodes[index]
		}
		if found := findTreeNode(nodes[index].Children, path); found != nil {
			return found
		}
	}
	return nil
}

func hasTreePath(nodes []TreeNode, path string) bool {
	for _, node := range nodes {
		if node.Path == path || hasTreePath(node.Children, path) {
			return true
		}
	}
	return false
}
