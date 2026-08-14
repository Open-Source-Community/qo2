package archive

import (
	"os"
	"path/filepath"
	"testing"
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
