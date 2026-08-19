package sandbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedPristine creates a level's non-secret tree in PristineDir, the way the
// decryptor + WriteCheckStubs would.
func seedPristine(t *testing.T, key string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(PristineDir, key, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func homeLevel(t *testing.T, key string) string {
	t.Helper()
	p := filepath.Join(Rootfs, "home", "ahmed", key)
	return p
}

func TestRunSetupCopiesLevelToHome(t *testing.T) {
	restore := setPaths(t)
	defer restore()

	seedPristine(t, "challenges/level1", map[string]string{
		"README.md":     "level 1 task",
		"check.sh":      "#!/bin/sh\n/bin/qo-check /tmp/qo-check.sock 'challenges/level1'\n",
		"data/file.txt": "data",
	})
	// WriteCheckStubs writes the stub executable; mirror that so the copy
	// preserves the mode.
	if err := os.Chmod(filepath.Join(PristineDir, "challenges", "level1", "check.sh"), 0755); err != nil {
		t.Fatal(err)
	}

	sum, err := runSetup("level1", 1000, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sum, "ready in ~/challenges/level1") {
		t.Errorf("unexpected summary: %q", sum)
	}

	for _, rel := range []string{"README.md", "check.sh", "data/file.txt"} {
		p := filepath.Join(homeLevel(t, "challenges/level1"), rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s in home copy: %v", rel, err)
		}
	}
	// Executable stub survives the copy.
	fi, err := os.Stat(filepath.Join(homeLevel(t, "challenges/level1"), "check.sh"))
	if err != nil || fi.Mode()&0111 == 0 {
		t.Errorf("home copy stub not executable (fi=%v err=%v)", fi, err)
	}

	// reset behaves like setup: wipe + re-copy pristine.
	os.Remove(filepath.Join(homeLevel(t, "challenges/level1"), "data", "file.txt"))
	if _, err := runSetup("level1", 1000, 1000, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(homeLevel(t, "challenges/level1"), "data", "file.txt")); err != nil {
		t.Errorf("reset did not restore file.txt: %v", err)
	}
}

func TestRunSetupAll(t *testing.T) {
	restore := setPaths(t)
	defer restore()

	seedPristine(t, "challenges/level1", map[string]string{"README.md": "one"})
	seedPristine(t, "challenges/level2", map[string]string{"README.md": "two"})
	os.WriteFile(filepath.Join(PristineDir, "challenges", "stray.txt"), []byte("not a level"), 0644)

	sum, err := runSetup("", 1000, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sum, "2 levels") {
		t.Errorf("unexpected summary: %q", sum)
	}
	for _, lvl := range []string{"level1", "level2"} {
		if _, err := os.Stat(filepath.Join(homeLevel(t, "challenges/"+lvl), "README.md")); err != nil {
			t.Errorf("level %s not copied home: %v", lvl, err)
		}
	}
}

func TestRunSetupRejectsBadKeys(t *testing.T) {
	restore := setPaths(t)
	defer restore()
	seedPristine(t, "challenges/level1", map[string]string{"README.md": "one"})

	for _, bad := range []string{"../etc", "/etc", "a/../../x", "level/..", "..", "."} {
		if _, err := runSetup(bad, 1000, 1000, false); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
	if _, err := runSetup("level99", 1000, 1000, false); err == nil {
		t.Error("expected unknown level to fail")
	}
}

func TestSetupSocketRoundTrip(t *testing.T) {
	restore := setPaths(t)
	defer restore()
	seedPristine(t, "challenges/level1", map[string]string{"README.md": "one"})

	ln, err := StartCheckServer("2021170034")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.Dial("unix", checkSocketHostPath())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("setup\tlevel1\t1000\t1000\n")); err != nil {
		t.Fatal(err)
	}
	br := newConnReader(conn)
	header, err := br.readString()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(header, "QOSETUP 0") {
		t.Fatalf("expected success header, got %q (%s)", header, br.rest())
	}
	if _, err := os.Stat(filepath.Join(homeLevel(t, "challenges/level1"), "README.md")); err != nil {
		t.Errorf("level not copied over the socket: %v", err)
	}

	// Malformed request -> error code.
	conn2, err := net.Dial("unix", checkSocketHostPath())
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	if _, err := fmt.Fprintln(conn2, "setup\tlevel1\t1000"); err != nil {
		t.Fatal(err)
	}
	br2 := newConnReader(conn2)
	header2, _ := br2.readString()
	if !strings.HasPrefix(header2, "QOSETUP 1") {
		t.Fatalf("expected error header for malformed request, got %q", header2)
	}
}
