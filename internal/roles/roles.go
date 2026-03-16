package roles

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Role defines the access level for a user.
type Role string

const (
	Admin       Role = "admin"
	Contributor Role = "contributor"
	Reader      Role = "reader"
)

// ValidRoles is the set of allowed roles.
var ValidRoles = map[Role]bool{
	Admin:       true,
	Contributor: true,
	Reader:      true,
}

// UserEntry represents a single user in the roles file.
type UserEntry struct {
	Name  string `json:"name"`
	Role  Role   `json:"role"`
	Token string `json:"token"` // per-user auth token
}

// RolesFile is the on-disk roles database stored in .vibe/roles.json.
type RolesFile struct {
	Owner string      `json:"owner"` // repo creator (always admin)
	Users []UserEntry `json:"users"`
}

// Manager handles role operations.
type Manager struct {
	VibeDir string
}

func NewManager(vibeDir string) *Manager {
	return &Manager{VibeDir: vibeDir}
}

func (m *Manager) filePath() string {
	return filepath.Join(m.VibeDir, "roles.json")
}

// Init creates the roles file with the given owner as admin.
func (m *Manager) Init(ownerName, ownerToken string) error {
	if ownerToken == "" {
		ownerToken = GenerateToken()
	}
	rf := &RolesFile{
		Owner: ownerName,
		Users: []UserEntry{
			{Name: ownerName, Role: Admin, Token: ownerToken},
		},
	}
	return m.save(rf)
}

// Load reads the roles file.
func (m *Manager) Load() (*RolesFile, error) {
	data, err := os.ReadFile(m.filePath())
	if err != nil {
		return nil, fmt.Errorf("roles not configured (run 'vibe roles init <name>')")
	}
	var rf RolesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse roles: %w", err)
	}
	return &rf, nil
}

// Grant assigns a role to a user. Creates the user if they don't exist.
func (m *Manager) Grant(name string, role Role, token string) error {
	if !ValidRoles[role] {
		return fmt.Errorf("invalid role '%s' (use admin, contributor, or reader)", role)
	}
	rf, err := m.Load()
	if err != nil {
		return err
	}

	found := false
	for i, u := range rf.Users {
		if u.Name == name {
			rf.Users[i].Role = role
			if token != "" {
				rf.Users[i].Token = token
			}
			found = true
			break
		}
	}
	if !found {
		if token == "" {
			token = GenerateToken()
		}
		rf.Users = append(rf.Users, UserEntry{Name: name, Role: role, Token: token})
	}

	return m.save(rf)
}

// Revoke removes a user's access.
func (m *Manager) Revoke(name string) error {
	rf, err := m.Load()
	if err != nil {
		return err
	}
	if name == rf.Owner {
		return fmt.Errorf("cannot revoke the repo owner '%s'", name)
	}

	newUsers := make([]UserEntry, 0, len(rf.Users))
	found := false
	for _, u := range rf.Users {
		if u.Name == name {
			found = true
			continue
		}
		newUsers = append(newUsers, u)
	}
	if !found {
		return fmt.Errorf("user '%s' not found", name)
	}
	rf.Users = newUsers
	return m.save(rf)
}

// GetUser finds a user by name.
func (m *Manager) GetUser(name string) (*UserEntry, error) {
	rf, err := m.Load()
	if err != nil {
		return nil, err
	}
	for _, u := range rf.Users {
		if u.Name == name {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("user '%s' not found", name)
}

// GetUserByToken finds a user by their auth token.
func (m *Manager) GetUserByToken(token string) (*UserEntry, error) {
	rf, err := m.Load()
	if err != nil {
		return nil, err
	}
	for _, u := range rf.Users {
		if u.Token == token {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("invalid token")
}

// CanWrite returns true if the role allows writing (admin or contributor).
func CanWrite(role Role) bool {
	return role == Admin || role == Contributor
}

// CanManage returns true if the role allows managing users (admin only).
func CanManage(role Role) bool {
	return role == Admin
}

// CanRead returns true if the role allows reading (all roles).
func CanRead(role Role) bool {
	return ValidRoles[role]
}

func (m *Manager) save(rf *RolesFile) error {
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath(), data, 0600)
}

// GenerateToken creates a random 32-character hex token.
func GenerateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
