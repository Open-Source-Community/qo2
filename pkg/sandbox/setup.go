package sandbox

// setup.go - on-demand level setup and reset over the session socket.
//
// The pristine, non-secret level data lives root-only in PristineDir (outside
// the chroot), extracted there by the archive decryptor plus the stub written
// by WriteCheckStubs. The qo-setup / qo-reset commands inside the chroot relay
// a request over the same Unix socket as qo-check; the privileged parent then
// copies the requested level's pristine tree into the student's home
// (~/challenges/<level>) and hands it to them. This keeps students out of /tmp
// entirely and lets them restore a corrupted working copy at any time.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// handleSetupConn services a "setup\t<level>\t<uid>\t<gid>" or
// "reset\t<level>\t<uid>\t<gid>" request. A bare or "*" level sets up all
// levels. Responds "QOSETUP <0|1>\n<message>\n" (QORESET for resets).
func handleSetupConn(conn net.Conn, line string, reset bool) {
	verb := "SETUP"
	if reset {
		verb = "RESET"
	}
	parts := strings.Split(strings.TrimRight(line, "\n"), "\t")
	if len(parts) != 4 {
		fmt.Fprintf(conn, "QO%s 1\nbad request\n", verb)
		return
	}
	uid, errU := strconv.ParseUint(parts[2], 10, 32)
	gid, errG := strconv.ParseUint(parts[3], 10, 32)
	if errU != nil || errG != nil {
		fmt.Fprintf(conn, "QO%s 1\nmalformed uid/gid\n", verb)
		return
	}
	summary, err := runSetup(parts[1], uint32(uid), uint32(gid), reset)
	if err != nil {
		fmt.Fprintf(conn, "QO%s 1\n%s\n", verb, err)
		return
	}
	fmt.Fprintf(conn, "QO%s 0\n%s\n", verb, summary)
}

// runSetup copies the pristine files of one or all levels into the requester's
// home directory. reset is accepted but behaves identically: re-copying pristine
// over the working copy is what resets a level.
func runSetup(rawKey string, uid, gid uint32, reset bool) (string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey != "" && rawKey != "*" && rawKey != "all" && !validateLevelKey(rawKey) {
		return "", fmt.Errorf("invalid level %q", rawKey)
	}
	key := normalizeLevelKey(rawKey)
	if key == "" || key == "*" || key == "all" {
		return setupAll(uid, gid)
	}
	if !validateLevelKey(key) {
		return "", fmt.Errorf("invalid level %q", rawKey)
	}
	if _, err := copyLevelToHome(key, uid, gid); err != nil {
		return "", err
	}
	return fmt.Sprintf("level %s ready in ~/%s", rawKey, key), nil
}

// setupAll copies every level found under PristineDir/challenges into home.
func setupAll(uid, gid uint32) (string, error) {
	base := filepath.Join(PristineDir, "challenges")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("no challenge levels available: %w", err)
	}
	done, failed := 0, 0
	var errs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		key := filepath.Join("challenges", e.Name())
		if !validateLevelKey(key) {
			continue
		}
		if _, err := copyLevelToHome(key, uid, gid); err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("%s: %v", key, err))
			continue
		}
		done++
	}
	if failed > 0 {
		return fmt.Sprintf("set up %d levels, %d failed: %s", done, failed, strings.Join(errs, "; ")),
			fmt.Errorf("%d levels failed", failed)
	}
	return fmt.Sprintf("set up %d levels in ~/challenges", done), nil
}

// copyLevelToHome wipes the student's home copy of a level and re-copies it
// from the root-only pristine tree, chowning it to the requester (best-effort
// for non-root test runs). Returns the number of files copied.
func copyLevelToHome(levelKey string, uid, gid uint32) (int, error) {
	if !validateLevelKey(levelKey) {
		return 0, fmt.Errorf("invalid level %q", levelKey)
	}
	src := filepath.Join(PristineDir, levelKey)
	if !pathExists(src) {
		return 0, fmt.Errorf("level %q is not available", levelKey)
	}
	home := homeDirFor(uid)
	dst := filepath.Join(Rootfs, home, levelKey)
	if err := os.RemoveAll(dst); err != nil {
		return 0, err
	}
	if err := copyDir(src, dst); err != nil {
		return 0, err
	}
	if os.Getuid() == 0 {
		_ = chownTree(dst, int(uid), int(gid))
	}
	return countFiles(dst), nil
}

// homeDirFor mirrors the check runner: the admin session (uid 0) works in
// /root, the sandbox user works in /home/ahmed.
func homeDirFor(uid uint32) string {
	if uid == 0 {
		return "/root"
	}
	return "/home/ahmed"
}

// normalizeLevelKey maps the student-facing name ("" / "all" / "level1") onto
// the canonical archive key ("challenges/level1").
func normalizeLevelKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || key == "*" || key == "all" {
		return key
	}
	if strings.HasPrefix(key, "challenges/") {
		return key
	}
	return "challenges/" + key
}

// chownTree best-effort chowns a copied tree to the requester.
func chownTree(root string, uid, gid int) error {
	return filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		return os.Chown(p, uid, gid)
	})
}

// countFiles returns the number of regular files under root.
func countFiles(root string) int {
	n := 0
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// RunSetupClient is the qo-setup entry point: relay a setup request for the
// given level (or all levels when empty) and print the server's response.
func RunSetupClient(args []string) int {
	return runSetupClient(args, "setup")
}

// RunResetClient is the qo-reset entry point (qo-reset is a symlink to the
// same binary; main.go dispatches on argv[0]).
func RunResetClient(args []string) int {
	return runSetupClient(args, "reset")
}

func runSetupClient(args []string, verb string) int {
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [level1|level2|...|all]\n", verb)
		return 1
	}
	key := ""
	if len(args) == 1 {
		key = args[0]
	}

	conn, err := net.Dial("unix", checkSocketInChroot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s unavailable: %v\n", verb, err)
		return 1
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(conn, "%s\t%s\t%d\t%d\n", verb, key, os.Getuid(), os.Getgid()); err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", verb, err)
		return 1
	}

	br := bufio.NewReader(conn)
	header, err := br.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", verb, err)
		return 1
	}
	body, _ := io.ReadAll(br)
	fmt.Print(string(body))
	if strings.HasSuffix(strings.TrimSpace(header), " 0") {
		return 0
	}
	return 1
}
