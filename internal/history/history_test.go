package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jay73737/Vibe/internal/core"
)

func setupTestRepo(t *testing.T) *core.Repo {
	t.Helper()
	dir := t.TempDir()
	repo, err := core.InitRepo(dir)
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	return repo
}

func commitFile(t *testing.T, repo *core.Repo, path, content, message string) core.Hash {
	t.Helper()
	absPath := filepath.Join(repo.WorkDir, path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddToIndex(path); err != nil {
		t.Fatalf("AddToIndex: %v", err)
	}
	h, err := repo.CreateCommit("tester", message)
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	return h
}

func TestDiffBasic(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"

	lines := Diff(old, new)

	hasAdded := false
	hasRemoved := false
	for _, l := range lines {
		if l.Type == DiffAdded && l.Content == "modified" {
			hasAdded = true
		}
		if l.Type == DiffRemoved && l.Content == "line2" {
			hasRemoved = true
		}
	}
	if !hasAdded {
		t.Fatal("expected added line 'modified'")
	}
	if !hasRemoved {
		t.Fatal("expected removed line 'line2'")
	}
}

func TestDiffEmpty(t *testing.T) {
	lines := Diff("same\n", "same\n")
	for _, l := range lines {
		if l.Type != DiffContext {
			t.Fatalf("expected all context lines, got %v", l)
		}
	}
}

func TestDiffAddedFile(t *testing.T) {
	lines := Diff("", "new content\n")
	if len(lines) == 0 {
		t.Fatal("expected diff lines for new file")
	}
	if lines[0].Type != DiffAdded {
		t.Fatalf("expected added line, got %v", lines[0].Type)
	}
}

func TestDiffWorkingTree(t *testing.T) {
	repo := setupTestRepo(t)
	h := commitFile(t, repo, "test.txt", "original\n", "first commit")
	_ = h

	// Modify the file
	os.WriteFile(filepath.Join(repo.WorkDir, "test.txt"), []byte("modified\n"), 0644)

	mgr := NewManager(repo)
	diffs, err := mgr.DiffWorkingTree()
	if err != nil {
		t.Fatalf("DiffWorkingTree: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Status != "modified" {
		t.Fatalf("expected modified, got %s", diffs[0].Status)
	}
}

func TestDiffCommits(t *testing.T) {
	repo := setupTestRepo(t)
	h1 := commitFile(t, repo, "test.txt", "version 1\n", "first")
	h2 := commitFile(t, repo, "test.txt", "version 2\n", "second")

	mgr := NewManager(repo)
	diffs, err := mgr.DiffCommits(h1, h2)
	if err != nil {
		t.Fatalf("DiffCommits: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Status != "modified" {
		t.Fatalf("expected modified, got %s", diffs[0].Status)
	}
}

func TestRevert(t *testing.T) {
	repo := setupTestRepo(t)
	h1 := commitFile(t, repo, "test.txt", "original\n", "first")
	_ = commitFile(t, repo, "test.txt", "changed\n", "second")

	mgr := NewManager(repo)
	_, err := mgr.Revert(h1, "tester")
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}

	// File should be back to original
	data, err := os.ReadFile(filepath.Join(repo.WorkDir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Fatalf("expected 'original\\n', got %q", data)
	}
}

func TestBlame(t *testing.T) {
	repo := setupTestRepo(t)
	commitFile(t, repo, "test.txt", "line1\nline2\n", "first")
	commitFile(t, repo, "test.txt", "line1\nmodified\nline3\n", "second")

	mgr := NewManager(repo)
	lines, err := mgr.Blame("test.txt")
	if err != nil {
		t.Fatalf("Blame: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 blame lines, got %d", len(lines))
	}

	// line1 should be attributed to the first commit (older)
	// modified and line3 should be attributed to the second commit (newer)
	if lines[0].Author != "tester" {
		t.Fatalf("expected author 'tester', got %q", lines[0].Author)
	}
}

func TestFormatDiff(t *testing.T) {
	fd := &FileDiff{
		Path:   "test.txt",
		Status: "modified",
		Lines: []DiffLine{
			{Type: DiffContext, Content: "same"},
			{Type: DiffRemoved, Content: "old"},
			{Type: DiffAdded, Content: "new"},
		},
	}
	output := FormatDiff(fd)
	if output == "" {
		t.Fatal("expected non-empty formatted diff")
	}
}
