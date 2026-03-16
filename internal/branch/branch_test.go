package branch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vibe-vcs/vibe/internal/core"
)

func setupTestRepo(t *testing.T) *core.Repo {
	t.Helper()
	dir := t.TempDir()
	repo, err := core.InitRepo(dir)
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Create and commit a file so we have a base commit
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddToIndex("base.txt"); err != nil {
		t.Fatalf("AddToIndex: %v", err)
	}
	if _, err := repo.CreateCommit("tester", "initial commit"); err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	return repo
}

func TestCreateBranch(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	if err := mgr.Create("feature"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Should fail on duplicate
	if err := mgr.Create("feature"); err == nil {
		t.Fatal("expected error creating duplicate branch")
	}
}

func TestCreateBranchNoCommits(t *testing.T) {
	dir := t.TempDir()
	repo, _ := core.InitRepo(dir)
	mgr := NewManager(repo)

	if err := mgr.Create("feature"); err == nil {
		t.Fatal("expected error creating branch with no commits")
	}
}

func TestListBranches(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	mgr.Create("dev")
	mgr.Create("feature")

	branches, current, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if current != "main" {
		t.Fatalf("expected current branch 'main', got %q", current)
	}
	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %d: %v", len(branches), branches)
	}
}

func TestSwitchBranch(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	mgr.Create("feature")

	if err := mgr.Switch("feature", false); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	branch, _, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if branch != "feature" {
		t.Fatalf("expected branch 'feature', got %q", branch)
	}

	// base.txt should still exist after switch
	data, err := os.ReadFile(filepath.Join(repo.WorkDir, "base.txt"))
	if err != nil {
		t.Fatalf("base.txt should exist after switch: %v", err)
	}
	if string(data) != "base content" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestSwitchAutoSavesSession(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	// Create a file and stage it on main
	os.WriteFile(filepath.Join(repo.WorkDir, "wip.txt"), []byte("work in progress"), 0644)
	repo.AddToIndex("wip.txt")

	mgr.Create("feature")
	mgr.Switch("feature", false)

	// Check that a session was saved for main
	sessions, err := mgr.Sessions("main")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected a session to be saved for main")
	}
	if sessions[0].Branch != "main" {
		t.Fatalf("session branch: got %q, want 'main'", sessions[0].Branch)
	}
}

func TestSwitchNoSession(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	mgr.Create("feature")
	mgr.Switch("feature", true) // --no-session

	sessions, _ := mgr.Sessions("main")
	if len(sessions) != 0 {
		t.Fatal("expected no sessions with --no-session flag")
	}
}

func TestDestroyBranch(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	mgr.Create("throwaway")

	if err := mgr.Destroy("throwaway"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// Should fail to destroy again
	if err := mgr.Destroy("throwaway"); err == nil {
		t.Fatal("expected error destroying non-existent branch")
	}
}

func TestDestroyCurrentBranch(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	if err := mgr.Destroy("main"); err == nil {
		t.Fatal("expected error destroying current branch")
	}
}

func TestRestoreSession(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	// Create and stage a file on main
	os.WriteFile(filepath.Join(repo.WorkDir, "restore-me.txt"), []byte("important work"), 0644)
	repo.AddToIndex("restore-me.txt")

	// Switch to feature (auto-saves session)
	mgr.Create("feature")
	mgr.Switch("feature", false)

	// restore-me.txt should be gone after checkout
	if _, err := os.Stat(filepath.Join(repo.WorkDir, "restore-me.txt")); err == nil {
		t.Fatal("restore-me.txt should not exist on feature branch")
	}

	// Switch back to main
	mgr.Switch("main", true)

	// Get session ID and restore it
	sessions, _ := mgr.Sessions("main")
	if len(sessions) == 0 {
		t.Fatal("no sessions found")
	}

	if err := mgr.Restore(sessions[0].ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Check the index was restored
	idx, _ := repo.ReadIndex()
	if _, ok := idx.Entries["restore-me.txt"]; !ok {
		t.Fatal("restore-me.txt should be in index after restore")
	}
}

func TestInvalidBranchName(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := NewManager(repo)

	cases := []string{"", "has space", "has/slash"}
	for _, name := range cases {
		if err := mgr.Create(name); err == nil {
			t.Fatalf("expected error for branch name %q", name)
		}
	}
}
