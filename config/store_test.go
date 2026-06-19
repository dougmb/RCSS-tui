package config

import (
	"path/filepath"
	"testing"
)

// TestStoreUpsertActiveRemove covers the in-memory account operations (no disk
// I/O, so the user's real config is never touched).
func TestStoreUpsertActiveRemove(t *testing.T) {
	var s Store
	s.Upsert(NewAccount("drive:"))
	s.Upsert(NewAccount("work:"))
	if len(s.Accounts) != 2 {
		t.Fatalf("want 2 accounts, got %d", len(s.Accounts))
	}

	// Upsert replaces by key (RemoteName) rather than duplicating.
	c, _ := s.Get("drive:")
	c.SourceRoot = "/data"
	s.Upsert(c)
	if len(s.Accounts) != 2 {
		t.Fatalf("upsert should replace; got %d accounts", len(s.Accounts))
	}
	if got, ok := s.Get("drive:"); !ok || got.SourceRoot != "/data" {
		t.Fatalf("expected updated SourceRoot, got %+v ok=%v", got, ok)
	}

	s.SetActive("work:")
	if a, ok := s.Active(); !ok || a.RemoteName != "work:" {
		t.Fatalf("active should be work:, got %+v ok=%v", a, ok)
	}

	// Removing the active account falls back to a remaining one.
	s.Remove("work:")
	if s.Has("work:") {
		t.Error("work: should be removed")
	}
	if a, ok := s.Active(); !ok || a.RemoteName != "drive:" {
		t.Fatalf("active should fall back to drive:, got %+v ok=%v", a, ok)
	}

	// Removing the last account leaves no active account.
	s.Remove("drive:")
	if _, ok := s.Active(); ok {
		t.Error("expected no active account after removing all")
	}
}

// TestRemoteBase checks the remote base path renders cleanly, including the
// empty-destination (account root) case and stray slashes.
func TestRemoteBase(t *testing.T) {
	cases := []struct {
		name, remote, dest, want string
	}{
		{"root", "drive:", "", "drive:"},
		{"subfolder", "drive:", "Backups", "drive:/Backups"},
		{"stray slashes", "drive:/", "/Backups/", "drive:/Backups"},
	}
	for _, tc := range cases {
		got := Config{RemoteName: tc.remote, RemoteDestination: tc.dest}.RemoteBase()
		if got != tc.want {
			t.Errorf("%s: RemoteBase()=%q want %q", tc.name, got, tc.want)
		}
	}
}

// TestValidateAllowsRootDestination checks an empty RemoteDestination (account
// root) is valid, while RemoteName and SourceRoot stay required.
func TestValidateAllowsRootDestination(t *testing.T) {
	// Empty destination = account root, so a config with only remote + source set
	// must validate (NewAccount now defaults the destination to empty).
	if err := (Config{RemoteName: "drive:", SourceRoot: "/data"}).Validate(); err != nil {
		t.Errorf("empty destination should be valid: %v", err)
	}
	if err := (Config{SourceRoot: "/data"}).Validate(); err == nil {
		t.Error("missing remote_name should fail validation")
	}
	if err := (Config{RemoteName: "drive:"}).Validate(); err == nil {
		t.Error("missing source_root should fail validation")
	}
	if err := (Config{RemoteName: "drive:", SourceRoot: "/data", RetentionDays: -1}).Validate(); err == nil {
		t.Error("negative retention should fail validation")
	}
}

// TestResolveLogFilePerAccount checks each account gets an isolated default log
// and that an explicit LogFile is honored.
func TestResolveLogFilePerAccount(t *testing.T) {
	got, err := NewAccount("drive:").ResolveLogFile()
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(got); base != "backup-drive.log" {
		t.Errorf("want backup-drive.log, got %s", base)
	}

	c := NewAccount("x:")
	c.LogFile = filepath.FromSlash("/var/log/custom.log")
	if got, _ := c.ResolveLogFile(); got != c.LogFile {
		t.Errorf("explicit log not honored: %s", got)
	}

	if got, _ := (Config{}).ResolveLogFile(); filepath.Base(got) != "backup.log" {
		t.Errorf("want backup.log for accountless config, got %s", filepath.Base(got))
	}
}
