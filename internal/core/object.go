package core

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// ObjectType represents the type of a stored object.
type ObjectType string

const (
	BlobObject   ObjectType = "blob"
	TreeObject   ObjectType = "tree"
	CommitObject ObjectType = "commit"
)

// Hash is a SHA-256 content hash used as an object identifier.
type Hash [32]byte

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func (h Hash) Short() string {
	return h.String()[:8]
}

func (h Hash) IsZero() bool {
	return h == Hash{}
}

// HashFromHex parses a hex-encoded hash string.
func HashFromHex(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil {
		return h, err
	}
	copy(h[:], b)
	return h, nil
}

// HashBytes computes the SHA-256 hash of raw data.
func HashBytes(data []byte) Hash {
	return sha256.Sum256(data)
}

// Blob represents a file's content.
type Blob struct {
	Data []byte
}

// TreeEntry is a single entry in a tree (directory listing).
type TreeEntry struct {
	Name string     `json:"name"`
	Type ObjectType `json:"type"`
	Hash Hash       `json:"hash"`
	Mode uint32     `json:"mode"` // e.g., 0644 for files, 0755 for executable, 040000 for dirs
}

// Tree represents a directory snapshot — a list of named entries.
type Tree struct {
	Entries []TreeEntry `json:"entries"`
}

// Commit represents a single point-in-time snapshot of the repository.
type Commit struct {
	TreeHash    Hash      `json:"tree"`
	ParentHash  Hash      `json:"parent,omitempty"`       // zero hash for initial commit
	MergeParent Hash      `json:"merge_parent,omitempty"` // second parent for merge commits
	Author      string    `json:"author"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}
