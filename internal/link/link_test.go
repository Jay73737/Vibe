package link

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jay73737/Vibe/internal/core"
)

func setupSourceRepo(t *testing.T) *core.Repo {
	t.Helper()
	dir := t.TempDir()
	repo, err := core.InitRepo(dir)
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Create files and commit
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello world"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	repo.AddToIndex("readme.txt")
	repo.AddToIndex("main.go")
	repo.CreateCommit("author", "initial commit")

	return repo
}

func TestLinkLocal(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, err := Link(targetDir, source.WorkDir)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}

	// Verify link config exists
	mgr := NewManager(linked)
	config, manifest, err := mgr.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if config.Source != source.WorkDir {
		t.Fatalf("expected source %s, got %s", source.WorkDir, config.Source)
	}
	if config.SourceType != "local" {
		t.Fatalf("expected local, got %s", config.SourceType)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("expected 2 files in manifest, got %d", len(manifest.Files))
	}
}

func TestLinkManifestNotCached(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, _ := Link(targetDir, source.WorkDir)
	mgr := NewManager(linked)
	_, manifest, _ := mgr.Status()

	// Files should not be cached yet (hybrid mode)
	for name, info := range manifest.Files {
		if info.Cached {
			t.Fatalf("file %s should not be cached initially", name)
		}
	}
}

func TestFetchSingleFile(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, _ := Link(targetDir, source.WorkDir)
	mgr := NewManager(linked)

	data, err := mgr.Fetch("readme.txt")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", data)
	}

	// File should exist on disk now
	diskData, err := os.ReadFile(filepath.Join(linked.WorkDir, "readme.txt"))
	if err != nil {
		t.Fatalf("file not on disk: %v", err)
	}
	if string(diskData) != "hello world" {
		t.Fatalf("disk content mismatch: %q", diskData)
	}

	// Manifest should show it as cached
	_, manifest, _ := mgr.Status()
	if !manifest.Files["readme.txt"].Cached {
		t.Fatal("readme.txt should be cached after fetch")
	}
}

func TestPullAll(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, _ := Link(targetDir, source.WorkDir)
	mgr := NewManager(linked)

	count, err := mgr.Pull()
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 files pulled, got %d", count)
	}

	// All files should be cached
	_, manifest, _ := mgr.Status()
	for name, info := range manifest.Files {
		if !info.Cached {
			t.Fatalf("%s should be cached after pull", name)
		}
	}

	// Pull again should fetch 0
	count, _ = mgr.Pull()
	if count != 0 {
		t.Fatalf("expected 0 on second pull, got %d", count)
	}
}

func TestSync(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, _ := Link(targetDir, source.WorkDir)
	mgr := NewManager(linked)

	// Add a new file to source and commit
	os.WriteFile(filepath.Join(source.WorkDir, "new.txt"), []byte("new file"), 0644)
	source.AddToIndex("new.txt")
	source.CreateCommit("author", "add new file")

	// Sync should detect the change
	changed, err := mgr.Sync()
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if changed != 1 {
		t.Fatalf("expected 1 changed file, got %d", changed)
	}

	// Manifest should have 3 files now
	_, manifest, _ := mgr.Status()
	if len(manifest.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(manifest.Files))
	}
}

func TestLinkNotVibeRepo(t *testing.T) {
	targetDir := t.TempDir()
	notARepo := t.TempDir()

	_, err := Link(targetDir, notARepo)
	if err == nil {
		t.Fatal("expected error linking to non-vibe directory")
	}
}

func TestFetchNonExistentFile(t *testing.T) {
	source := setupSourceRepo(t)
	targetDir := t.TempDir()

	linked, _ := Link(targetDir, source.WorkDir)
	mgr := NewManager(linked)

	_, err := mgr.Fetch("nonexistent.txt")
	if err == nil {
		t.Fatal("expected error fetching non-existent file")
	}
}
