package sandbox

// check.go - check.sh execution over a per-session Unix domain socket.
//
// The real check.sh scripts live in the protected ChallengesDir (root-only,
// outside the chroot). Inside the chroot, each level gets a thin, non-secret
// stub (see WriteCheckStubs) that relays a request to this package's check
// server. The server runs in the privileged parent process that launched the
// sandbox session and stays alive for the whole session. On a request it forks
// a short-lived child, chroot()s it into the same rootfs the student is using,
// and pipes the real script's content into bash via stdin - the script text is
// never written anywhere the student's shell can read.
//
// Security note (deliberate deviation from a strict 0600 root socket): the stub
// runs as the dropped sandbox user (uid/gid 1000), and a 0600 root-owned unix
// socket is unreachable from that uid - the only legitimate client. The socket
// is therefore 0660 owned by root:1000: still root-owned, session-scoped, and
// not world-accessible, while remaining reachable by the sandbox user.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const (
	// checkSocketInChroot is the socket path as seen from inside the chroot.
	checkSocketInChroot = "/tmp/qo-check.sock"
	checkSocketName     = "qo-check.sock"
	checkClientName     = "qo-check"
	checkStubName       = "check.sh"

	// checkResponseOK / checkResponseFail / checkResponseError are the exit
	// codes the server reports: 0 = level passed, 1 = level not yet solved,
	// 2 = internal error (missing script, bad request, ...).
	checkResponseOK    = 0
	checkResponseFail  = 1
	checkResponseError = 2
)

var (
	// keyTokenRe matches the historical hardcoded key print found at the end of
	// old check.sh scripts, e.g. key="LVL-1-K7Q4X". It is used to discover a
	// base flag when no .base_flag / flag.txt file is present.
	keyTokenRe = regexp.MustCompile(`(?i)\bkey\s*=\s*"([^"]+)"`)

	// LeaderboardHook is a best-effort callback invoked once per successfully
	// completed level. cmd wires it to the optional Supabase sync.
	LeaderboardHook func(studentID, levelKey, flag string)
)

// checkSocketHostPath returns the host-side path of the session socket.
func checkSocketHostPath() string {
	return filepath.Join(Rootfs, "tmp", checkSocketName)
}

// StartCheckServer binds the per-session socket and starts accepting requests.
// It must run in the privileged parent process before the sandbox child spawns.
func StartCheckServer(studentID string) (net.Listener, error) {
	sock := checkSocketHostPath()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("binding check socket: %w", err)
	}
	// Root-owned, sandbox-user group so the in-chroot stub (running as the
	// dropped sandbox user) can connect; not world-accessible.
	_ = os.Chown(sock, 0, 1000)
	_ = os.Chmod(sock, 0660)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleCheckConn(conn, studentID)
		}
	}()

	return ln, nil
}

// handleCheckConn services a single stub request and closes the connection.
func handleCheckConn(conn net.Conn, studentID string) {
	defer conn.Close()

	req, err := readCheckRequest(conn)
	if err != nil {
		fmt.Fprintf(conn, "QOCHECK %d\nbad request: %v\n", checkResponseError, err)
		return
	}

	output, code, baseFlag, err := runLevelCheck(req)
	if err != nil {
		fmt.Fprintf(conn, "QOCHECK %d\n%s\n", checkResponseError, err)
		return
	}

	if code == checkResponseOK && baseFlag != "" {
		flag := GenerateUniqueFlag(baseFlag, studentID)
		output = presentCheckSuccess(output, baseFlag, flag)
		if LeaderboardHook != nil {
			LeaderboardHook(studentID, req.levelKey, flag)
		}
	} else if code == checkResponseOK {
		// No base flag found for this level: nothing secret to reveal, but the
		// instructor should add .base_flag. Log and still report success.
		fmt.Fprintf(os.Stderr, "[qo] level %q passed but no base flag file (.base_flag/flag.txt/key=) was found\n", req.levelKey)
	}

	fmt.Fprintf(conn, "QOCHECK %d\n%s", code, output)
}

// checkRequest is a parsed request line from the stub.
type checkRequest struct {
	levelKey string
	cwd      string
	uid      uint32
	gid      uint32
}

// readCheckRequest parses "check\t<levelKey>\t<cwd>\t<uid>\t<gid>".
func readCheckRequest(conn net.Conn) (checkRequest, error) {
	var req checkRequest
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return req, err
	}
	parts := strings.Split(strings.TrimRight(line, "\n"), "\t")
	if len(parts) != 5 || parts[0] != "check" {
		return req, fmt.Errorf("malformed request")
	}
	uid, errU := strconv.ParseUint(parts[3], 10, 32)
	gid, errG := strconv.ParseUint(parts[4], 10, 32)
	if errU != nil || errG != nil {
		return req, fmt.Errorf("malformed uid/gid")
	}
	req.levelKey = parts[1]
	req.cwd = parts[2]
	req.uid = uint32(uid)
	req.gid = uint32(gid)
	return req, nil
}

// runLevelCheck runs the real check.sh for a level inside a fresh chrooted
// child and returns its combined output, exit code, and the level's base flag.
func runLevelCheck(req checkRequest) (string, int, string, error) {
	if !validateLevelKey(req.levelKey) {
		return "", checkResponseError, "", fmt.Errorf("invalid level %q", req.levelKey)
	}
	levelDir := filepath.Join(ChallengesDir, req.levelKey)

	// The real script is read by the privileged parent and piped into bash via
	// stdin. Its content never exists as a file inside the chroot.
	script, err := os.ReadFile(filepath.Join(levelDir, checkStubName))
	if err != nil {
		return "", checkResponseError, "", fmt.Errorf("check script unavailable for %q: %v", req.levelKey, err)
	}
	baseFlag, _ := loadBaseFlag(levelDir)

	// The check must run in the same directory (and as the same user) the
	// student was in when they invoked ./check.sh.
	cwd := req.cwd
	if cwd == "" || !filepath.IsAbs(cwd) {
		cwd = "/home/ahmed"
	}
	hostCwd := filepath.Join(Rootfs, cwd)
	if !strings.HasPrefix(filepath.Clean(hostCwd), filepath.Clean(Rootfs)) {
		return "", checkResponseError, "", fmt.Errorf("invalid cwd %q", req.cwd)
	}

	home := "/home/ahmed"
	user := "ahmed"
	if req.uid == 0 {
		home = "/root"
		user = "root"
	}

	cmd := exec.Command("/bin/bash")
	cmd.Dir = cwd // applied via chdir after the chroot, relative to the new root
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: Rootfs,
	}
	// Drop to the sandbox user only when the session runs unprivileged; a root
	// session (admin mode) needs no credential change.
	if req.uid != 0 {
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid:    req.uid,
			Gid:    req.gid,
			Groups: []uint32{req.gid},
		}
	}
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + home,
		"USER=" + user,
		"LOGNAME=" + user,
		"TERM=xterm",
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", checkResponseError, "", err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return "", checkResponseError, "", fmt.Errorf("starting check child: %v", err)
	}
	if _, err := io.WriteString(stdin, string(script)); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return out.String(), checkResponseError, "", err
	}
	_ = stdin.Close()

	code := checkResponseOK
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
			if code != checkResponseFail {
				code = checkResponseFail
			}
		} else {
			code = checkResponseFail
		}
	}
	return out.String(), code, baseFlag, nil
}

// presentCheckSuccess replaces every occurrence of the base flag in the script
// output with the per-student flag (so the historical key="..." print is
// transformed in place), and appends a congratulations line when the flag did
// not already appear.
func presentCheckSuccess(output, baseFlag, flag string) string {
	out := output
	if baseFlag != "" {
		out = strings.ReplaceAll(out, baseFlag, flag)
	}
	if !strings.Contains(out, flag) {
		out = strings.TrimRight(out, "\n") + "\n\nCongratulations! Your flag is: " + flag + "\n"
	}
	return out
}

// loadBaseFlag resolves the level's base flag, preferring .base_flag, then
// flag.txt, then falling back to the historical key="..." print inside check.sh.
func loadBaseFlag(levelDir string) (string, error) {
	for _, name := range []string{".base_flag", "flag.txt"} {
		if b, err := os.ReadFile(filepath.Join(levelDir, name)); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s, nil
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(levelDir, checkStubName)); err == nil {
		if m := keyTokenRe.FindStringSubmatch(string(b)); m != nil {
			return m[1], nil
		}
	}
	return "", fmt.Errorf("no base flag in %s", levelDir)
}

// validateLevelKey rejects keys that could escape ChallengesDir or break the
// shell quoting used in the generated stub.
func validateLevelKey(levelKey string) bool {
	if levelKey == "" {
		return false
	}
	if strings.ContainsAny(levelKey, "'\x00\t\n") {
		return false
	}
	if filepath.IsAbs(levelKey) {
		return false
	}
	clean := filepath.Clean(levelKey)
	if clean == "." || clean == ".." {
		return false
	}
	root := filepath.Clean(ChallengesDir)
	joined := filepath.Join(root, clean)
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return false
	}
	return true
}

// WriteCheckStubs places a thin, non-secret stub check.sh for every level found
// in ChallengesDir into the chroot at the same relative path (Rootfs/tmp/<key>).
// The stub contains no test logic and no flag; it only relays to the socket.
func WriteCheckStubs() (int, error) {
	if !pathExists(ChallengesDir) {
		return 0, nil
	}
	var levels []string
	err := filepath.Walk(ChallengesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != checkStubName {
			return nil
		}
		rel, err := filepath.Rel(ChallengesDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		if validateLevelKey(rel) {
			levels = append(levels, rel)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	n := 0
	for _, lvl := range levels {
		stub := filepath.Join(Rootfs, "tmp", lvl, checkStubName)
		if err := os.MkdirAll(filepath.Dir(stub), 0755); err != nil {
			return n, err
		}
		content := fmt.Sprintf("#!/bin/sh\n/bin/%s %s '%s'\nexit $?\n", checkClientName, checkSocketInChroot, lvl)
		if err := os.WriteFile(stub, []byte(content), 0755); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// CopyCheckClient copies the running qo binary into the chroot as /bin/qo-check
// so the stub has a Unix-socket client. The binary contains no level secrets:
// check.sh content and base flags are read at runtime by the parent from the
// protected directory, never embedded.
func CopyCheckClient() error {
	self, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return fmt.Errorf("resolving self: %w", err)
	}
	dst := filepath.Join(Rootfs, "bin", checkClientName)
	_ = os.Remove(dst)
	if err := copyFile(self, dst); err != nil {
		return fmt.Errorf("copying check client: %w", err)
	}
	return os.Chmod(dst, 0755)
}

// RunCheckClient is the qo-check entry point. It connects to the session socket,
// relays a check request for its level, prints the response verbatim, and exits
// with the reported code. main.go dispatches to it when argv[0] is "qo-check".
func RunCheckClient(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: qo-check <socket> <level>")
		return checkResponseError
	}
	sock := args[0]
	level := args[1]
	cwd, _ := os.Getwd()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check unavailable: %v\n", err)
		return checkResponseError
	}
	defer conn.Close()

	req := fmt.Sprintf("check\t%s\t%s\t%d\t%d\n", level, cwd, os.Getuid(), os.Getgid())
	if _, err := io.WriteString(conn, req); err != nil {
		fmt.Fprintf(os.Stderr, "check request failed: %v\n", err)
		return checkResponseError
	}

	br := bufio.NewReader(conn)
	header, err := br.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "check response failed: %v\n", err)
		return checkResponseError
	}
	var code int
	if _, err := fmt.Sscanf(strings.TrimSpace(header), "QOCHECK %d", &code); err != nil {
		fmt.Fprintf(os.Stderr, "malformed check response: %v\n", err)
		return checkResponseError
	}
	if _, err := io.Copy(os.Stdout, br); err != nil {
		return checkResponseError
	}
	return code
}
