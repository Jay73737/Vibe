package core

import (
	"os"
	"testing"
	"time"
)

func tempVibeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func TestWriteReadBlob(t *testing.T) {
	store := NewObjectStore(tempVibeDir(t))
	data := []byte("hello world")

	h, err := store.WriteBlob(data)
	if err != nil {
		t.Fatalf("WriteBlob: %v", err)
	}
	if h.IsZero() {
		t.Fatal("expected non-zero hash")
	}

	got, err := store.ReadBlob(h)
	if err != nil {
		t.Fatalf("ReadBlob: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestBlobContentAddressable(t *testing.T) {
	store := NewObjectStore(tempVibeDir(t))
	data := []byte("same content")

	h1, _ := store.WriteBlob(data)
	h2, _ := store.WriteBlob(data)

	if h1 != h2 {
		t.Fatal("same content should produce same hash")
	}
}

func TestWriteReadTree(t *testing.T) {
	store := NewObjectStore(tempVibeDir(t))

	blobHash, _ := store.WriteBlob([]byte("file content"))
	tree := &Tree{
		Entries: []TreeEntry{
			{Name: "test.txt", Type: BlobObject, Hash: blobHash, Mode: 0644},
		},
	}

	treeHash, err := store.WriteTree(tree)
	if err != nil {
		t.Fatalf("WriteTree: %v", err)
	}

	got, err := store.ReadTree(treeHash)
	if err != nil {
		t.Fatalf("ReadTree: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "test.txt" {
		t.Fatalf("unexpected tree entries: %+v", got.Entries)
	}
	if got.Entries[0].Hash != blobHash {
		t.Fatal("tree entry hash mismatch")
	}
}

func TestWriteReadCommit(t *testing.T) {
	store := NewObjectStore(tempVibeDir(t))

	now := time.Now().UTC().Truncate(time.Second)
	commit := &Commit{
		TreeHash:  HashBytes([]byte("fake tree")),
		Author:    "tester",
		Message:   "initial commit",
		Timestamp: now,
	}

	h, err := store.WriteCommit(commit)
	if err != nil {
		t.Fatalf("WriteCommit: %v", err)
	}

	got, err := store.ReadCommit(h)
	if err != nil {
		t.Fatalf("ReadCommit: %v", err)
	}
	if got.Author != "tester" || got.Message != "initial commit" {
		t.Fatalf("unexpected commit: %+v", got)
	}
	if got.TreeHash != commit.TreeHash {
		t.Fatal("tree hash mismatch")
	}
}

func TestHasObject(t *testing.T) {
	store := NewObjectStore(tempVibeDir(t))

	if store.HasObject(HashBytes([]byte("nope"))) {
		t.Fatal("should not have object before writing")
	}

	h, _ := store.WriteBlob([]byte("exists"))
	if !store.HasObject(h) {
		t.Fatal("should have object after writing")
	}
}

func TestHashFromHex(t *testing.T) {
	original := HashBytes([]byte("test"))
	hex := original.String()

	parsed, err := HashFromHex(hex)
	if err != nil {
		t.Fatalf("HashFromHex: %v", err)
	}
	if parsed != original {
		t.Fatal("round-trip hash mismatch")
	}
}

func TestRepoInitAndCommit(t *testing.T) {
	dir := t.TempDir()

	repo, err := InitRepo(dir)
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Create a test file
	testFile := "hello.txt"
	if err := os.WriteFile(dir+"/"+testFile, []byte("hello vibe"), 0644); err != nil {
		t.Fatal(err)
	}

	// Add and commit
	if err := repo.AddToIndex(testFile); err != nil {
		t.Fatalf("AddToIndex: %v", err)
	}

	h, err := repo.CreateCommit("tester", "first commit")
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	if h.IsZero() {
		t.Fatal("expected non-zero commit hash")
	}

	// Verify HEAD points to the commit
	branch, headHash, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected branch 'main', got %q", branch)
	}
	if headHash != h {
		t.Fatal("HEAD should point to new commit")
	}

	// Verify we can read the commit back
	commit, err := repo.Store.ReadCommit(h)
	if err != nil {
		t.Fatalf("ReadCommit: %v", err)
	}
	if commit.Message != "first commit" {
		t.Fatalf("unexpected message: %s", commit.Message)
	}
}

func TestRepoStatus(t *testing.T) {
	dir := t.TempDir()
	repo, _ := InitRepo(dir)

	// Create files
	os.WriteFile(dir+"/staged.txt", []byte("staged"), 0644)
	os.WriteFile(dir+"/untracked.txt", []byte("untracked"), 0644)

	// Stage one file
	repo.AddToIndex("staged.txt")

	staged, _, untracked, err := repo.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if len(staged) != 1 || staged[0] != "staged.txt" {
		t.Fatalf("expected staged.txt in staged, got %v", staged)
	}

	foundUntracked := false
	for _, f := range untracked {
		if f == "untracked.txt" {
			foundUntracked = true
		}
	}
	if !foundUntracked {
		t.Fatalf("expected untracked.txt in untracked, got %v", untracked)
	}
}
