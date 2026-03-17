package core

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ImportGit clones a git repo and converts it into a Vibe repository.
// It clones into targetDir, removes .git, and initializes .vibe with all files committed.
func ImportGit(gitURL, targetDir, author string) (*Repo, error) {
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}

	// Clone the git repo
	cmd := exec.Command("git", "clone", "--depth", "1", gitURL, absTarget)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// Remove .git directory
	gitDir := filepath.Join(absTarget, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return nil, fmt.Errorf("remove .git: %w", err)
	}

	// Initialize as a Vibe repo
	repo, err := InitRepo(absTarget)
	if err != nil {
		return nil, fmt.Errorf("init vibe repo: %w", err)
	}

	// Add all files
	count := 0
	err = filepath.WalkDir(absTarget, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(absTarget, path)
		if err := repo.AddToIndex(relPath); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("add files: %w", err)
	}

	if count == 0 {
		return repo, nil
	}

	// Create initial commit
	msg := fmt.Sprintf("Imported from %s", stripGitSuffix(gitURL))
	if _, err := repo.CreateCommit(author, msg); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return repo, nil
}

func stripGitSuffix(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return url
}
