package roles

import (
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestInitAndLoad(t *testing.T) {
	mgr := NewManager(tempDir(t))
	if err := mgr.Init("alice", "token123"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	rf, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rf.Owner != "alice" {
		t.Fatalf("expected owner 'alice', got %q", rf.Owner)
	}
	if len(rf.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(rf.Users))
	}
	if rf.Users[0].Role != Admin {
		t.Fatalf("expected admin role, got %s", rf.Users[0].Role)
	}
}

func TestInitAutoToken(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("bob", "")

	user, err := mgr.GetUser("bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Token == "" {
		t.Fatal("expected auto-generated token")
	}
	if len(user.Token) != 32 {
		t.Fatalf("expected 32-char token, got %d", len(user.Token))
	}
}

func TestGrant(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "admintoken")

	if err := mgr.Grant("bob", Contributor, "bobtoken"); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	user, err := mgr.GetUser("bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != Contributor {
		t.Fatalf("expected contributor, got %s", user.Role)
	}
	if user.Token != "bobtoken" {
		t.Fatalf("expected 'bobtoken', got %s", user.Token)
	}
}

func TestGrantUpdateExisting(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "admintoken")
	mgr.Grant("bob", Reader, "bobtoken")
	mgr.Grant("bob", Contributor, "")

	user, _ := mgr.GetUser("bob")
	if user.Role != Contributor {
		t.Fatalf("expected contributor after update, got %s", user.Role)
	}
	if user.Token != "bobtoken" {
		t.Fatal("token should be preserved when not provided on update")
	}
}

func TestGrantInvalidRole(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "tok")

	if err := mgr.Grant("bob", Role("superadmin"), ""); err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestRevoke(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "tok")
	mgr.Grant("bob", Reader, "bobtok")

	if err := mgr.Revoke("bob"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err := mgr.GetUser("bob")
	if err == nil {
		t.Fatal("expected bob to be gone after revoke")
	}
}

func TestRevokeOwner(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "tok")

	if err := mgr.Revoke("alice"); err == nil {
		t.Fatal("expected error revoking owner")
	}
}

func TestRevokeNonExistent(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "tok")

	if err := mgr.Revoke("nobody"); err == nil {
		t.Fatal("expected error revoking non-existent user")
	}
}

func TestGetUserByToken(t *testing.T) {
	mgr := NewManager(tempDir(t))
	mgr.Init("alice", "alicetok")
	mgr.Grant("bob", Contributor, "bobtok")

	user, err := mgr.GetUserByToken("bobtok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "bob" {
		t.Fatalf("expected bob, got %s", user.Name)
	}

	_, err = mgr.GetUserByToken("invalidtoken")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestPermissionHelpers(t *testing.T) {
	if !CanRead(Reader) || !CanRead(Contributor) || !CanRead(Admin) {
		t.Fatal("all roles should be able to read")
	}
	if CanWrite(Reader) {
		t.Fatal("reader should not be able to write")
	}
	if !CanWrite(Contributor) || !CanWrite(Admin) {
		t.Fatal("contributor and admin should be able to write")
	}
	if CanManage(Contributor) || CanManage(Reader) {
		t.Fatal("only admin should be able to manage")
	}
	if !CanManage(Admin) {
		t.Fatal("admin should be able to manage")
	}
}

func TestLoadNotConfigured(t *testing.T) {
	mgr := NewManager(tempDir(t))
	_, err := mgr.Load()
	if err == nil {
		t.Fatal("expected error loading unconfigured roles")
	}
}
