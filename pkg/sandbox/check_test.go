package sandbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// setPaths temporarily repoints the package-level rootfs/challenge locations so
// tests run in isolated temp dirs without touching a live session.
func setPaths(t *testing.T) (restore func()) {
	t.Helper()
	oldRoot, oldChal, oldPristine := Rootfs, ChallengesDir, PristineDir
	dir := t.TempDir()
	Rootfs = filepath.Join(dir, "rootfs")
	ChallengesDir = filepath.Join(dir, "challenges")
	PristineDir = filepath.Join(dir, "pristine")
	os.MkdirAll(Rootfs, 0755)
	os.MkdirAll(ChallengesDir, 0700)
	os.MkdirAll(PristineDir, 0700)
	return func() {
		Rootfs, ChallengesDir, PristineDir = oldRoot, oldChal, oldPristine
	}
}

func TestValidateLevelKey(t *testing.T) {
	valid := []string{"level1", "Level-7", "challenges/level1", "a/b/c", "a b"}
	for _, k := range valid {
		if !validateLevelKey(k) {
			t.Errorf("expected %q to be valid", k)
		}
	}
	invalid := []string{"", ".", "..", "/etc", "../escape", "level/../../x", "a'z", "a\tb", "a\x00b"}
	for _, k := range invalid {
		if validateLevelKey(k) {
			t.Errorf("expected %q to be invalid", k)
		}
	}
}

func TestLoadBaseFlag(t *testing.T) {
	restore := setPaths(t)
	defer restore()

	levelDir := filepath.Join(ChallengesDir, "level1")
	os.MkdirAll(levelDir, 0700)

	// No flag anywhere -> error.
	if _, err := loadBaseFlag(levelDir); err == nil {
		t.Errorf("expected error with no base flag present")
	}

	// .base_flag wins over flag.txt.
	os.WriteFile(filepath.Join(levelDir, ".base_flag"), []byte("BASE-FROM-FILE\n"), 0600)
	os.WriteFile(filepath.Join(levelDir, "flag.txt"), []byte("BASE-FROM-TXT"), 0600)
	if f, err := loadBaseFlag(levelDir); err != nil || f != "BASE-FROM-FILE" {
		t.Errorf("expected .base_flag to win, got %q (err %v)", f, err)
	}

	// Fallback to key="..." inside check.sh when no flag file exists.
	os.Remove(filepath.Join(levelDir, ".base_flag"))
	os.Remove(filepath.Join(levelDir, "flag.txt"))
	os.WriteFile(filepath.Join(levelDir, "check.sh"), []byte("#!/bin/bash\necho 'ok'\necho 'key=\"LVL-1-K7Q4X\"'\nexit 0\n"), 0755)
	if f, err := loadBaseFlag(levelDir); err != nil || f != "LVL-1-K7Q4X" {
		t.Errorf("expected fallback key extraction, got %q (err %v)", f, err)
	}
}

func TestPresentCheckSuccess(t *testing.T) {
	const base = "LVL-1-K7Q4X"
	const flag = "aabbccddeeff0011"

	// Old-style key="..." print is rewritten in place with the per-student flag.
	out := presentCheckSuccess("Level 1 passed!\nYour key is: key=\"LVL-1-K7Q4X\"\n", base, flag)
	if strings.Contains(out, base) {
		t.Errorf("base flag leaked into output: %q", out)
	}
	if !strings.Contains(out, flag) {
		t.Errorf("per-student flag missing from output: %q", out)
	}

	// No key print -> congratulations line appended.
	out2 := presentCheckSuccess("Level 1 passed!\n", base, flag)
	if !strings.HasSuffix(out2, "Congratulations! Your flag is: "+flag+"\n") {
		t.Errorf("expected appended congratulations, got %q", out2)
	}
}

func TestWriteCheckStubsSecrecy(t *testing.T) {
	restore := setPaths(t)
	defer restore()

	// Simulate post-decrypt state.
	for _, lvl := range []string{"level1", "challenges/level2"} {
		dir := filepath.Join(ChallengesDir, lvl)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "check.sh"), []byte("#!/bin/bash\nif true; then echo 'pass'; exit 0; fi\n"), 0755)
		os.WriteFile(filepath.Join(dir, ".base_flag"), []byte("TOP-SECRET-BASE"), 0600)
	}
	os.WriteFile(filepath.Join(ChallengesDir, "level1", "secret.txt"), []byte("stuff"), 0644)

	n, err := WriteCheckStubs()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 stubs, got %d", n)
	}

	// Stub exists in the pristine staging tree, is executable, and contains no
	// test logic or secrets.
	for _, lvl := range []string{"level1", "challenges/level2"} {
		stub := filepath.Join(PristineDir, lvl, "check.sh")
		b, err := os.ReadFile(stub)
		if err != nil {
			t.Fatalf("stub %s not written: %v", stub, err)
		}
		content := string(b)
		for _, secret := range []string{"TOP-SECRET-BASE", "exit 0", "echo 'pass'", "if true"} {
			if strings.Contains(content, secret) {
				t.Errorf("stub %s leaks %q", stub, secret)
			}
		}
		if !strings.Contains(content, "/bin/qo-check") {
			t.Errorf("stub %s does not invoke the socket client", stub)
		}
		fi, err := os.Stat(stub)
		if err != nil || fi.Mode()&0111 == 0 {
			t.Errorf("stub %s not executable", stub)
		}
	}

	// No secret file ever lands under the rootfs tree or the pristine tree.
	for _, base := range []string{Rootfs, PristineDir} {
		err = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && (info.Name() == ".base_flag" || info.Name() == "flag.txt" || info.Name() == "setup.sh" || info.Name() == "cleanup.sh") {
				t.Errorf("secret file leaked into %s: %s", base, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckServerSocketMode(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("socket ownership semantics require root")
	}
	restore := setPaths(t)
	defer restore()
	os.MkdirAll(filepath.Join(Rootfs, "tmp"), 0755)

	ln, err := StartCheckServer("2021170034")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	fi, err := os.Lstat(checkSocketHostPath())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0660 {
		t.Errorf("expected socket mode 0660, got %v", fi.Mode().Perm())
	}
	// Not world-writable/readable is the load-bearing secrecy property.
	if fi.Mode().Perm()&0007 != 0 {
		t.Errorf("socket is world-accessible: %v", fi.Mode().Perm())
	}
}

// TestCheckSocketRoundTrip exercises the full check-execution path: stub ->
// socket server -> chrooted child running the real script -> HMAC flag. It
// requires root (chroot + dropping to the sandbox user). The uid-1000 subtest
// runs as the sandbox user (default session); the uid-0 subtest runs as the
// admin session user and also validates the mechanics inside a user namespace
// where only uid 0 is mapped.
func TestCheckSocketRoundTrip(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root: chroot into sandbox rootfs")
	}

	if err := ExtractRootfs(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(Rootfs)
		_ = os.RemoveAll(ChallengesDir)
	}()

	const baseFlag = "LVL-1-K7Q4X"
	levelDir := filepath.Join(ChallengesDir, "level1")
	os.MkdirAll(levelDir, 0700)
	os.WriteFile(filepath.Join(levelDir, "check.sh"),
		[]byte("#!/bin/bash\nif [ -d \"$PWD/answer\" ]; then echo 'Level 1 passed!'; echo \"key=\\\"LVL-1-K7Q4X\\\"\"; exit 0; else echo 'Level 1 failed: missing answer dir'; exit 1; fi\n"), 0755)
	os.WriteFile(filepath.Join(levelDir, ".base_flag"), []byte(baseFlag), 0600)

	// Simulate the student's work inside the sandbox (in their home, where
	// qo-setup would have copied the level).
	os.MkdirAll(filepath.Join(Rootfs, "home", "ahmed", "level1", "answer"), 0755)

	// The sandbox child normally bind-mounts /dev/null before the shell starts;
	// replicate it so the check child has a working /dev/null.
	os.MkdirAll(filepath.Join(Rootfs, "dev"), 0755)
	os.WriteFile(filepath.Join(Rootfs, "dev", "null"), nil, 0666)
	if err := syscall.Mount("/dev/null", filepath.Join(Rootfs, "dev", "null"), "", syscall.MS_BIND, ""); err != nil {
		t.Fatalf("mount /dev/null: %v", err)
	}
	defer syscall.Unmount(filepath.Join(Rootfs, "dev", "null"), syscall.MNT_FORCE)

	if _, err := WriteCheckStubs(); err != nil {
		t.Fatal(err)
	}
	ln, err := StartCheckServer("2021170034")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	for _, tt := range []struct {
		name string
		uid  uint32
	}{
		{"sandbox-user", 1000},
		{"admin-user", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			header, body := runCheckRequest(t, tt.uid)
			if strings.HasPrefix(header, "QOCHECK 2") {
				// Inside a user namespace only uid 0 is mapped, so dropping to
				// uid 1000 legitimately fails there; real root hosts map it.
				if tt.uid != 0 && (strings.Contains(body, "operation not permitted") || strings.Contains(body, "permission denied")) {
					t.Skip("user namespace does not map uid 1000")
				}
				t.Fatalf("internal error: %q %q", header, body)
			}
			if !strings.HasPrefix(header, "QOCHECK 0") {
				t.Fatalf("expected success header, got %q %q", header, body)
			}
			if !strings.Contains(body, "Level 1 passed!") {
				t.Errorf("diagnostic output missing: %q", body)
			}
			if strings.Contains(body, baseFlag) {
				t.Errorf("base flag leaked into output: %q", body)
			}
			expected := GenerateUniqueFlag(baseFlag, "2021170034")
			if !strings.Contains(body, expected) {
				t.Errorf("per-student flag missing from output: %q", body)
			}
		})
	}
}

// runCheckRequest dials the session socket using the qo-check protocol for the
// given uid (as the sandbox user would) and returns the header and body.
func runCheckRequest(t *testing.T, uid uint32) (string, string) {
	t.Helper()
	conn, err := net.Dial("unix", checkSocketHostPath())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := fmt.Sprintf("check\tlevel1\t/home/ahmed/level1\t%d\t%d\n", uid, uid)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	br := newConnReader(conn)
	header, err := br.readString()
	if err != nil {
		t.Fatal(err)
	}
	return header, br.rest()
}

// connReader is a tiny helper to read the QOCHECK header line then the rest.
type connReader struct {
	c *net.Conn
}

func newConnReader(c net.Conn) *connReader {
	return &connReader{c: &c}
}

func (r *connReader) readString() (string, error) {
	buf := make([]byte, 0, 32)
	tmp := make([]byte, 1)
	for {
		n, err := (*r.c).Read(tmp)
		if n == 1 {
			buf = append(buf, tmp[0])
			if tmp[0] == '\n' {
				return string(buf), nil
			}
		}
		if err != nil {
			return string(buf), err
		}
	}
}

func (r *connReader) rest() string {
	buf := make([]byte, 4096)
	n, _ := (*r.c).Read(buf)
	return string(buf[:n])
}
