package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vibe-vcs/vibe/internal/core"
)

func setupTestServer(t *testing.T) (*Server, *core.Repo) {
	t.Helper()
	dir := t.TempDir()
	repo, err := core.InitRepo(dir)
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Create files and commit
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	repo.AddToIndex("hello.txt")
	repo.AddToIndex("main.go")
	repo.CreateCommit("tester", "initial commit")

	cfg := DefaultConfig()
	cfg.RepoPath = dir

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	return srv, repo
}

func TestHandleInfo(t *testing.T) {
	srv, _ := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/info")
	if err != nil {
		t.Fatalf("GET /api/info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var info map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&info)

	if info["branch"] != "main" {
		t.Fatalf("expected branch 'main', got %v", info["branch"])
	}
}

func TestHandleRefs(t *testing.T) {
	srv, _ := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/refs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result struct {
		Refs map[string]string `json:"refs"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if _, ok := result.Refs["main"]; !ok {
		t.Fatal("expected 'main' in refs")
	}
}

func TestHandleManifest(t *testing.T) {
	srv, _ := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result struct {
		Branch string                          `json:"branch"`
		Head   string                          `json:"head"`
		Files  map[string]map[string]interface{} `json:"files"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}
	if _, ok := result.Files["hello.txt"]; !ok {
		t.Fatal("expected hello.txt in manifest")
	}
}

func TestHandleBlob(t *testing.T) {
	srv, repo := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Get the blob hash for hello.txt
	_, headHash, _ := repo.Head()
	commit, _ := repo.Store.ReadCommit(headHash)
	tree, _ := repo.Store.ReadTree(commit.TreeHash)

	var helloHash string
	for _, entry := range tree.Entries {
		if entry.Name == "hello.txt" {
			helloHash = entry.Hash.String()
		}
	}

	resp, err := http.Get(ts.URL + "/api/blob/" + helloHash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body []byte
	body, _ = os.ReadFile(resp.Request.URL.Path) // won't work, read from resp instead
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	body = buf[:n]

	if string(body) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", body)
	}
}

func TestAuthMiddleware(t *testing.T) {
	dir := t.TempDir()
	repo, _ := core.InitRepo(dir)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644)
	repo.AddToIndex("test.txt")
	repo.CreateCommit("tester", "init")

	cfg := DefaultConfig()
	cfg.RepoPath = dir
	cfg.Auth.Token = "secret123"

	srv, _ := New(cfg)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// No token — should get 401
	resp, _ := http.Get(ts.URL + "/api/info")
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}

	// With token — should get 200
	req, _ := http.NewRequest("GET", ts.URL+"/api/info", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with token, got %d", resp.StatusCode)
	}

	// Query param token — should also work
	resp, _ = http.Get(ts.URL + "/api/info?token=secret123")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with query token, got %d", resp.StatusCode)
	}
}

func TestHandleCommit(t *testing.T) {
	srv, repo := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, headHash, _ := repo.Head()
	resp, err := http.Get(ts.URL + "/api/commit/" + headHash.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var commit core.Commit
	json.NewDecoder(resp.Body).Decode(&commit)
	if commit.Message != "initial commit" {
		t.Fatalf("expected 'initial commit', got %q", commit.Message)
	}
}

func TestHandle404(t *testing.T) {
	srv, _ := setupTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	fakeHash := "0000000000000000000000000000000000000000000000000000000000000000"
	resp, _ := http.Get(ts.URL + "/api/blob/" + fakeHash)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
