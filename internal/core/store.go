package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ObjectStore is a content-addressable store for Vibe objects.
// Objects are stored as files in .vibe/objects/<first2>/<rest> (like Git).
type ObjectStore struct {
	Root string // path to .vibe directory
}

func NewObjectStore(vibeDir string) *ObjectStore {
	return &ObjectStore{Root: vibeDir}
}

func (s *ObjectStore) objectDir() string {
	return filepath.Join(s.Root, "objects")
}

func (s *ObjectStore) objectPath(h Hash) string {
	hex := h.String()
	return filepath.Join(s.objectDir(), hex[:2], hex[2:])
}

// HasObject checks if an object exists in the store.
func (s *ObjectStore) HasObject(h Hash) bool {
	_, err := os.Stat(s.objectPath(h))
	return err == nil
}

// WriteBlob stores a blob and returns its hash.
func (s *ObjectStore) WriteBlob(data []byte) (Hash, error) {
	header := []byte("blob\x00")
	content := append(header, data...)
	h := HashBytes(content)
	if err := s.writeRaw(h, content); err != nil {
		return Hash{}, err
	}
	return h, nil
}

// ReadBlob retrieves a blob by hash.
func (s *ObjectStore) ReadBlob(h Hash) ([]byte, error) {
	content, err := s.readRaw(h)
	if err != nil {
		return nil, err
	}
	// Strip "blob\x00" header
	for i, b := range content {
		if b == 0 {
			return content[i+1:], nil
		}
	}
	return nil, fmt.Errorf("invalid blob object %s", h.Short())
}

// WriteTree stores a tree and returns its hash.
func (s *ObjectStore) WriteTree(tree *Tree) (Hash, error) {
	data, err := json.Marshal(tree)
	if err != nil {
		return Hash{}, fmt.Errorf("marshal tree: %w", err)
	}
	header := []byte("tree\x00")
	content := append(header, data...)
	h := HashBytes(content)
	if err := s.writeRaw(h, content); err != nil {
		return Hash{}, err
	}
	return h, nil
}

// ReadTree retrieves a tree by hash.
func (s *ObjectStore) ReadTree(h Hash) (*Tree, error) {
	content, err := s.readRaw(h)
	if err != nil {
		return nil, err
	}
	// Strip header
	var payload []byte
	for i, b := range content {
		if b == 0 {
			payload = content[i+1:]
			break
		}
	}
	if payload == nil {
		return nil, fmt.Errorf("invalid tree object %s", h.Short())
	}
	var tree Tree
	if err := json.Unmarshal(payload, &tree); err != nil {
		return nil, fmt.Errorf("unmarshal tree %s: %w", h.Short(), err)
	}
	return &tree, nil
}

// WriteCommit stores a commit and returns its hash.
func (s *ObjectStore) WriteCommit(commit *Commit) (Hash, error) {
	data, err := json.Marshal(commit)
	if err != nil {
		return Hash{}, fmt.Errorf("marshal commit: %w", err)
	}
	header := []byte("commit\x00")
	content := append(header, data...)
	h := HashBytes(content)
	if err := s.writeRaw(h, content); err != nil {
		return Hash{}, err
	}
	return h, nil
}

// ReadCommit retrieves a commit by hash.
func (s *ObjectStore) ReadCommit(h Hash) (*Commit, error) {
	content, err := s.readRaw(h)
	if err != nil {
		return nil, err
	}
	var payload []byte
	for i, b := range content {
		if b == 0 {
			payload = content[i+1:]
			break
		}
	}
	if payload == nil {
		return nil, fmt.Errorf("invalid commit object %s", h.Short())
	}
	var commit Commit
	if err := json.Unmarshal(payload, &commit); err != nil {
		return nil, fmt.Errorf("unmarshal commit %s: %w", h.Short(), err)
	}
	return &commit, nil
}

// writeRaw writes raw bytes to the object store, creating directories as needed.
func (s *ObjectStore) writeRaw(h Hash, data []byte) error {
	p := s.objectPath(h)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create object dir: %w", err)
	}
	// Content-addressable: if it exists, it's already correct
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	if err := os.WriteFile(p, data, 0444); err != nil {
		return fmt.Errorf("write object %s: %w", h.Short(), err)
	}
	return nil
}

// readRaw reads raw bytes from the object store.
func (s *ObjectStore) readRaw(h Hash) ([]byte, error) {
	data, err := os.ReadFile(s.objectPath(h))
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", h.Short(), err)
	}
	return data, nil
}
