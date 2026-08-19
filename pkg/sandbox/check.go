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

	// setupClientName / resetClientName are the on-demand commands installed
	// inside the chroot. Both are the same static binary; main.go dispatches
	// on argv[0].
	setupClientName = "qo-setup"
	resetClientName = "qo-reset"

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
	if err := os.MkdirAll(filepath.Dir(sock), 0755); err != nil {
		return nil, fmt.Errorf("creating socket dir: %w", err)
	}
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

// handleCheckConn services a single request and closes the connection. It
// dispatches on the first tab-separated token: "check" runs a level check,
// "setup"/"reset" copy a level's pristine files into the student's home.
func handleCheckConn(conn net.Conn, studentID string) {
	defer conn.Close()

	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		fmt.Fprintf(conn, "QOCHECK %d\nbad request: %v\n", checkResponseError, err)
		return
	}
	verb := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 2)[0]
	switch verb {
	case "setup", "reset":
		handleSetupConn(conn, line, verb == "reset")
	default:
		handleCheckConnVerb(conn, line, studentID)
	}
}

// handleCheckConnVerb is the original check path, fed the already-read line.
func handleCheckConnVerb(conn net.Conn, line, studentID string) {
	req, err := readCheckRequestLine(line)
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
		// Prefer the student ID carried in the request (read from QO_STUDENT_ID
		// inside the chroot) over the one captured when the session server
		// started. This keeps flags and leaderboard rows tied to the session
		// the check actually ran in, even if a stale server process lingers.
		reqID := studentID
		if req.studentID != "" {
			reqID = req.studentID
		}
		flag := GenerateUniqueFlag(baseFlag, reqID)
		output = presentCheckSuccess(output, baseFlag, flag)
		if LeaderboardHook != nil {
			LeaderboardHook(reqID, req.levelKey, flag)
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
	levelKey  string
	cwd       string
	uid       uint32
	gid       uint32
	studentID string
	args      []string
}

// readCheckRequestLine parses
//
//	check\t<levelKey>\t<cwd>\t<uid>\t<gid>
//
// with optional trailing fields when the request carries a student ID and/or
// check.sh arguments (added so the student ID travels with the request instead
// of being captured once at session start, and so ./check.sh <args> works):
//
//	check\t<levelKey>\t<cwd>\t<uid>\t<gid>\t<studentID>\t<argCount>\t<arg1>...\t<argN>
func readCheckRequestLine(line string) (checkRequest, error) {
	var req checkRequest
	parts := strings.Split(strings.TrimRight(line, "\n"), "\t")
	if len(parts) < 5 || parts[0] != "check" {
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

	rest := parts[5:]
	if len(rest) == 0 {
		return req, nil
	}
	req.studentID = rest[0]
	rest = rest[1:]
	if len(rest) == 0 {
		return req, nil
	}
	n, err := strconv.Atoi(rest[0])
	if err != nil || n < 0 || len(rest)-1 < n {
		return req, fmt.Errorf("malformed arg count")
	}
	req.args = rest[1 : 1+n]
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

	// Run bash with -s so the script text piped on stdin is executed and the
	// relayed check.sh arguments become positional parameters ($1, $2, ...),
	// exactly as if the student ran ./check.sh <args> directly.
	cmdArgs := []string{"-s", "--"}
	cmdArgs = append(cmdArgs, req.args...)
	cmd := exec.Command("/bin/bash", cmdArgs...)
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
		// The chroot has no ld.so.cache, so the loader must find the host-
		// provisioned libraries via LD_LIBRARY_PATH. This must match the
		// interactive sandbox shell (rootfs.go); without it, forking bash on
		// distros whose glibc default paths differ fails with
		// "error while loading shared libraries".
		"LD_LIBRARY_PATH=/usr/lib:/lib:/lib/x86_64-linux-gnu:/usr/lib/x86_64-linux-gnu:/lib/aarch64-linux-gnu:/usr/lib/aarch64-linux-gnu",
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
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "." || part == ".." {
			return false
		}
	}
	root := filepath.Clean(ChallengesDir)
	joined := filepath.Join(root, clean)
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return false
	}
	return true
}

// WriteCheckStubs places a thin, non-secret stub check.sh for every level found
// in ChallengesDir into the pristine staging tree at the same relative path
// (PristineDir/<key>). The stub contains no test logic and no flag; it only
// relays to the socket. It ships inside every ~/challenges/<level> copy that
// qo-setup creates, so ./check.sh works from the student's home.
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
		stub := filepath.Join(PristineDir, lvl, checkStubName)
		if err := os.MkdirAll(filepath.Dir(stub), 0755); err != nil {
			return n, err
		}
		content := fmt.Sprintf("#!/bin/sh\n/bin/%s %s '%s' \"$@\"\nexit $?\n", checkClientName, checkSocketInChroot, lvl)
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
	return copyClientBin(checkClientName)
}

// CopySetupClients installs /bin/qo-setup plus a /bin/qo-reset symlink. Both
// share the same static binary; main.go dispatches on argv[0] (like qo-check),
// so the sandbox user gets on-demand level setup and reset commands.
func CopySetupClients() error {
	if err := copyClientBin(setupClientName); err != nil {
		return err
	}
	reset := filepath.Join(Rootfs, "bin", resetClientName)
	_ = os.Remove(reset)
	return os.Symlink(setupClientName, reset)
}

// copyClientBin places the running qo binary into the chroot under name.
func copyClientBin(name string) error {
	self, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return fmt.Errorf("resolving self: %w", err)
	}
	dst := filepath.Join(Rootfs, "bin", name)
	_ = os.Remove(dst)
	if err := copyFile(self, dst); err != nil {
		return fmt.Errorf("copying %s: %w", name, err)
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

	// Carry the session's student ID with the request (set by qo start via
	// QO_STUDENT_ID and inherited into the sandbox shell). The server prefers
	// it over the ID captured at session start, so flags and leaderboard rows
	// always match the session that actually ran the check. Any arguments the
	// student passed to ./check.sh are relayed too.
	req := fmt.Sprintf("check\t%s\t%s\t%d\t%d", level, cwd, os.Getuid(), os.Getgid())
	sid := os.Getenv("QO_STUDENT_ID")
	if sid != "" || len(args) > 2 {
		req += "\t" + sid
		req += fmt.Sprintf("\t%d", len(args)-2)
		for _, a := range args[2:] {
			req += "\t" + a
		}
	}
	req += "\n"
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
