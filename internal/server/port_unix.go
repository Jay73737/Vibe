//go:build !windows

package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// isAddrInUse returns true if the error is "address already in use".
func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return sysErr.Err == syscall.EADDRINUSE
		}
	}
	return false
}

// killExistingVibe finds the process using the given port and kills it,
// but only if it is a vibe process. Returns true if a process was killed.
func killExistingVibe(port int) bool {
	// Use lsof to find the PID on the port
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", port)).Output()
	if err != nil {
		return false
	}

	pid := strings.TrimSpace(string(out))
	if pid == "" {
		return false
	}

	// Check if the PID is a vibe process
	commPath := fmt.Sprintf("/proc/%s/comm", pid)
	comm, err := os.ReadFile(commPath)
	if err != nil {
		// Fallback: use ps
		psOut, err := exec.Command("ps", "-p", pid, "-o", "comm=").Output()
		if err != nil {
			return false
		}
		comm = psOut
	}

	if !strings.Contains(strings.ToLower(string(comm)), "vibe") {
		return false
	}

	// Kill it
	err = exec.Command("kill", pid).Run()
	return err == nil
}
