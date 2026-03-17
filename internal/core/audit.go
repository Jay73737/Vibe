package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry represents a single entry in the audit log.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	User      string    `json:"user,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Source    string    `json:"source"` // "cli" or "server"
	IP        string    `json:"ip,omitempty"`
}

// AuditLog provides append-only audit logging for a Vibe repository.
type AuditLog struct {
	mu      sync.Mutex
	logPath string
}

// NewAuditLog creates an audit logger for the given .vibe directory.
func NewAuditLog(vibeDir string) *AuditLog {
	return &AuditLog{logPath: filepath.Join(vibeDir, "audit.log")}
}

// Log appends an audit entry.
func (a *AuditLog) Log(action, user, detail, source, ip string) error {
	entry := AuditEntry{
		Timestamp: time.Now().UTC(),
		Action:    action,
		User:      user,
		Detail:    detail,
		Source:    source,
		IP:        ip,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	f, err := os.OpenFile(a.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}

// Read returns the last N entries from the audit log (0 = all).
func (a *AuditLog) Read(limit int) ([]AuditEntry, error) {
	data, err := os.ReadFile(a.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []AuditEntry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
