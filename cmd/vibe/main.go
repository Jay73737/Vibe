package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"time"

	"github.com/vibe-vcs/vibe/internal/branch"
	"github.com/vibe-vcs/vibe/internal/core"
	"github.com/vibe-vcs/vibe/internal/history"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit()
	case "add":
		cmdAdd()
	case "commit":
		cmdCommit()
	case "status":
		cmdStatus()
	case "log":
		cmdLog()
	case "branch":
		cmdBranch()
	case "branches":
		cmdBranches()
	case "switch":
		cmdSwitch()
	case "destroy":
		cmdDestroy()
	case "sessions":
		cmdSessions()
	case "restore":
		cmdRestore()
	case "diff":
		cmdDiff()
	case "revert":
		cmdRevert()
	case "blame":
		cmdBlame()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "vibe: unknown command '%s'\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`vibe - modern version control

Usage: vibe <command> [arguments]

Core Commands:
  init              Create a new vibe repository
  add <files>       Stage files for commit
  commit -m "msg"   Create a new commit from staged files
  status            Show working tree status
  log               Show commit history

Branch & Session Commands:
  branch <name>     Create a new branch
  branches          List all branches
  switch <name>     Switch branch (auto-saves session)
  destroy <name>    Delete a branch and its sessions
  sessions          List saved sessions
  restore <id>      Restore a saved session

History Commands:
  diff              Show changes in working tree vs last commit
  diff <hash> <hash> Compare two commits
  revert <hash>     Revert repo to a previous commit
  blame <file>      Show per-line authorship`)
}

func cmdInit() {
	dir := "."
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}
	repo, err := core.InitRepo(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Initialized empty vibe repository in %s\n", repo.VibeDir)
}

func cmdAdd() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe add <file> [file...]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, arg := range os.Args[2:] {
		if arg == "." {
			if err := addAll(repo); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			continue
		}
		relPath, err := filepath.Rel(repo.WorkDir, filepath.Join(mustGetwd(), arg))
		if err != nil {
			relPath = arg
		}
		if err := repo.AddToIndex(relPath); err != nil {
			fmt.Fprintf(os.Stderr, "error adding %s: %v\n", arg, err)
			os.Exit(1)
		}
	}
}

func addAll(repo *core.Repo) error {
	return filepath.WalkDir(repo.WorkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == core.VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(repo.WorkDir, path)
		return repo.AddToIndex(relPath)
	})
}

func cmdCommit() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	message := ""
	author := getAuthor()

	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-m" || os.Args[i] == "--message") && i+1 < len(os.Args) {
			message = os.Args[i+1]
			i++
		}
	}
	if message == "" {
		fmt.Fprintln(os.Stderr, "error: commit message required (-m \"message\")")
		os.Exit(1)
	}

	h, err := repo.CreateCommit(author, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	fmt.Printf("[%s %s] %s\n", branchName, h.Short(), message)
}

func cmdStatus() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	fmt.Printf("On branch %s\n\n", branchName)

	staged, modified, untracked, err := repo.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(staged) > 0 {
		fmt.Println("Staged files:")
		for _, f := range staged {
			fmt.Printf("  \033[32m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(modified) > 0 {
		fmt.Println("Modified (not staged):")
		for _, f := range modified {
			fmt.Printf("  \033[33m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(untracked) > 0 {
		fmt.Println("Untracked files:")
		for _, f := range untracked {
			fmt.Printf("  \033[31m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(staged) == 0 && len(modified) == 0 && len(untracked) == 0 {
		fmt.Println("Nothing to report, working tree clean.")
	}
}

func cmdLog() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_, commitHash, err := repo.Head()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if commitHash.IsZero() {
		fmt.Println("No commits yet.")
		return
	}

	h := commitHash
	for !h.IsZero() {
		commit, err := repo.Store.ReadCommit(h)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading commit: %v\n", err)
			break
		}
		fmt.Printf("\033[33mcommit %s\033[0m\n", h.String())
		fmt.Printf("Author: %s\n", commit.Author)
		fmt.Printf("Date:   %s\n", commit.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
		fmt.Println()
		for _, line := range strings.Split(commit.Message, "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
		h = commit.ParentHash
	}
}

func cmdBranch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe branch <name>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Create(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created branch '%s'\n", os.Args[2])
}

func cmdBranches() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	branches, current, err := mgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, b := range branches {
		if b == current {
			fmt.Printf("* \033[32m%s\033[0m\n", b)
		} else {
			fmt.Printf("  %s\n", b)
		}
	}
}

func cmdSwitch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe switch <branch> [--no-session]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	target := os.Args[2]
	noSession := false
	for _, arg := range os.Args[3:] {
		if arg == "--no-session" {
			noSession = true
		}
	}

	mgr := branch.NewManager(repo)
	if err := mgr.Switch(target, noSession); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Switched to branch '%s'\n", target)
	if !noSession {
		fmt.Println("(previous work auto-saved as session)")
	}
}

func cmdDestroy() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe destroy <branch>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Destroy(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Destroyed branch '%s'\n", os.Args[2])
}

func cmdSessions() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	filterBranch := ""
	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-b" || os.Args[i] == "--branch") && i+1 < len(os.Args) {
			filterBranch = os.Args[i+1]
			i++
		}
	}

	mgr := branch.NewManager(repo)
	sessions, err := mgr.Sessions(filterBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No saved sessions.")
		return
	}

	for _, s := range sessions {
		fmt.Printf("\033[33m%s\033[0m\n", s.ID)
		fmt.Printf("  Branch: %s\n", s.Branch)
		fmt.Printf("  Date:   %s\n", s.Timestamp.Format("Mon Jan 2 15:04:05 2006"))
		fmt.Printf("  Files:  %d staged, %d modified\n", len(s.Index), len(s.WorkingFiles))
		fmt.Println()
	}
}

func cmdRestore() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe restore <session-id>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Restore(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Restored session '%s'\n", os.Args[2])
}

func cmdDiff() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := history.NewManager(repo)

	if len(os.Args) >= 4 {
		// diff <old-hash> <new-hash>
		oldHash, err := core.HashFromHex(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[2])
			os.Exit(1)
		}
		newHash, err := core.HashFromHex(os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[3])
			os.Exit(1)
		}
		diffs, err := mgr.DiffCommits(oldHash, newHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, d := range diffs {
			fmt.Print(history.FormatDiff(&d))
		}
		return
	}

	// diff working tree vs HEAD
	diffs, err := mgr.DiffWorkingTree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(diffs) == 0 {
		fmt.Println("No changes.")
		return
	}
	for _, d := range diffs {
		fmt.Print(history.FormatDiff(&d))
	}
}

func cmdRevert() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe revert <commit-hash>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	targetHash, err := core.HashFromHex(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[2])
		os.Exit(1)
	}

	mgr := history.NewManager(repo)
	newHash, err := mgr.Revert(targetHash, getAuthor())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	fmt.Printf("[%s %s] Revert to %s\n", branchName, newHash.Short(), targetHash.Short())
}

func cmdBlame() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe blame <file>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	relPath := os.Args[2]
	mgr := history.NewManager(repo)
	lines, err := mgr.Blame(relPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, bl := range lines {
		ts := ""
		if t, ok := bl.Timestamp.(time.Time); ok {
			ts = t.Format("2006-01-02")
		}
		fmt.Printf("\033[33m%s\033[0m %-12s %s \033[2m%4d\033[0m | %s\n",
			bl.CommitHash.Short(), bl.Author, ts, bl.LineNum, bl.Content)
	}
}

func getAuthor() string {
	if name := os.Getenv("VIBE_AUTHOR"); name != "" {
		return name
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return dir
}
