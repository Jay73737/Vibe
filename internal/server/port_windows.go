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

// WSAEADDRINUSE is the Windows socket error for "address already in use".
const WSAEADDRINUSE syscall.Errno = 10048

// isAddrInUse returns true if the error is "address already in use".
func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return sysErr.Err == WSAEADDRINUSE
		}
	}
	return false
}

// killExistingVibe finds the process using the given port and kills it,
// but only if it is a vibe.exe process. Returns true if a process was killed.
func killExistingVibe(port int) bool {
	// Use netstat to find the PID on the port
	out, err := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%d | findstr LISTENING", port)).Output()
	if err != nil {
		return false
	}

	// Parse PID from netstat output (last column)
	pid := ""
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 5 {
			pid = fields[len(fields)-1]
			break
		}
	}
	if pid == "" || pid == "0" {
		return false
	}

	// Check if the PID is a vibe process
	out, err = exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	outStr := strings.ToLower(string(out))
	if !strings.Contains(outStr, "vibe") {
		return false
	}

	// Kill it
	err = exec.Command("taskkill", "/PID", pid, "/F").Run()
	return err == nil
}
