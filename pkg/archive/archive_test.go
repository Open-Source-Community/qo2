package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmedYasserM/qo/pkg/sandbox"
)

func TestDeriveKey(t *testing.T) {
	salt := []byte("1234567890123456")
	key1 := DeriveKey("password", salt)
	key2 := DeriveKey("password", salt)
	key3 := DeriveKey("different", salt)

	if len(key1) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key1))
	}

	if string(key1) != string(key2) {
		t.Errorf("Expected deterministic key derivation")
	}

	if string(key1) == string(key3) {
		t.Errorf("Expected different key for different password")
	}
}

func TestIsValidFolderStructure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "challenge_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Empty dir should pass
	if err := IsValidFolderStructure(tempDir); err != nil {
		t.Errorf("Expected empty dir to be valid, got: %v", err)
	}

	// File in root should fail
	rootFile := filepath.Join(tempDir, "stray.txt")
	os.WriteFile(rootFile, []byte("stray"), 0644)
	if err := IsValidFolderStructure(tempDir); err == nil {
		t.Errorf("Expected error for stray file in root")
	}
	os.Remove(rootFile)

	// Level dir without check.sh should fail
	level1 := filepath.Join(tempDir, "level1")
	os.MkdirAll(level1, 0755)
	if err := IsValidFolderStructure(tempDir); err == nil {
		t.Errorf("Expected error for missing check.sh")
	}

	// Non-executable check.sh should fail
	checkSh := filepath.Join(level1, "check.sh")
	os.WriteFile(checkSh, []byte("#!/bin/sh\nexit 0"), 0644)
	if err := IsValidFolderStructure(tempDir); err == nil {
		t.Errorf("Expected error for non-executable check.sh")
	}

	// Executable check.sh should pass
	os.Chmod(checkSh, 0755)
	if err := IsValidFolderStructure(tempDir); err != nil {
		t.Errorf("Expected valid structure, got: %v", err)
	}
}

// TestArchiveRoundTripSecrecy builds a challenge archive, decrypts it, and
// verifies the secrecy routing: secret files land only in the protected
// challenges directory (0700), public files land inside the chroot tree.
func TestArchiveRoundTripSecrecy(t *testing.T) {
	oldRoot, oldChal := sandbox.Rootfs, sandbox.ChallengesDir
	tmp := t.TempDir()
	sandbox.Rootfs = filepath.Join(tmp, "rootfs")
	sandbox.ChallengesDir = filepath.Join(tmp, "challenges")
	defer func() {
		sandbox.Rootfs, sandbox.ChallengesDir = oldRoot, oldChal
	}()
	os.MkdirAll(sandbox.Rootfs, 0755)
	os.MkdirAll(sandbox.ChallengesDir, 0700)

	// Build a challenge folder with both secret and public files.
	chalDir := filepath.Join(tmp, "challenges")
	levelDir := filepath.Join(chalDir, "level1")
	os.MkdirAll(levelDir, 0755)
	os.WriteFile(filepath.Join(levelDir, "check.sh"), []byte("#!/bin/bash\necho ok\nexit 0\n"), 0755)
	os.WriteFile(filepath.Join(levelDir, ".base_flag"), []byte("BASE-SECRET"), 0600)
	os.WriteFile(filepath.Join(levelDir, "question.txt"), []byte("task"), 0644)
	os.WriteFile(filepath.Join(levelDir, "secret.txt"), []byte("data"), 0644)

	arch := filepath.Join(tmp, "exam.enc")
	if err := CreateEncryptedTarArchive(chalDir, arch, "2020-01-01 00:00", "pass", "key"); err != nil {
		t.Fatal(err)
	}
	if err := DecryptTarArchive(arch, "pass", "key"); err != nil {
		t.Fatal(err)
	}

	// The archive preserves the top-level challenge folder name, so levels are
	// nested under "challenges/". Secrets must land only in the protected dir.
	if _, err := os.Stat(filepath.Join(sandbox.ChallengesDir, "challenges", "level1", "check.sh")); err != nil {
		t.Errorf("check.sh not in protected dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox.ChallengesDir, "challenges", "level1", ".base_flag")); err != nil {
		t.Errorf(".base_flag not in protected dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox.Rootfs, "tmp", "challenges", "level1", "check.sh")); err == nil {
		t.Errorf("check.sh leaked into rootfs tree")
	}
	if _, err := os.Stat(filepath.Join(sandbox.Rootfs, "tmp", "challenges", "level1", ".base_flag")); err == nil {
		t.Errorf(".base_flag leaked into rootfs tree")
	}

	// Public files must be readable inside the chroot tree.
	if _, err := os.Stat(filepath.Join(sandbox.Rootfs, "tmp", "challenges", "level1", "question.txt")); err != nil {
		t.Errorf("question.txt missing from rootfs tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox.Rootfs, "tmp", "challenges", "level1", "secret.txt")); err != nil {
		t.Errorf("secret.txt missing from rootfs tree: %v", err)
	}

	// Protected directory must be root-only.
	fi, err := os.Stat(sandbox.ChallengesDir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0700 {
		t.Errorf("protected dir mode = %v, want 0700", fi.Mode().Perm())
	}
}
