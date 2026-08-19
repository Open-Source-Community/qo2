package sandbox

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"sync"
)

//go:embed rootfs.tar.gz
var embeddedRootfs []byte

const target = "/tmp"
const sentinel = "\x00__QO_EOF__\x00"

var (
	Rootfs        = filepath.Join(target, "rootfs")
	ChallengesDir = filepath.Join(target, "rootfs_challenges")
	defaultUser   = "ahmed"
)

// manAllowlist is the finalized set of man topics students may read through the
// sandbox command loop. Admin account-management commands (useradd, passwd,
// usermod, groupadd, groupdel) are intentionally excluded even if a matching
// man page is later added to the rootfs: the allowlist is the enforcement point,
// not the filesystem.
var manAllowlist = map[string]bool{
	"bunzip2": true, "bzip2": true, "cat": true, "chgrp": true, "chmod": true,
	"chown": true, "cp": true, "curl": true, "cut": true, "doas": true,
	"doas.conf": true, "file": true, "find": true, "grep": true, "groups": true,
	"gunzip": true, "gzip": true, "head": true, "id": true, "join": true,
	"kill": true, "killall": true, "less": true, "ln": true, "ls": true,
	"man": true, "man.conf": true, "mkdir": true, "more": true, "mv": true,
	"paste": true, "pgrep": true, "ping": true, "pkill": true, "ps": true,
	"rm": true, "rmdir": true, "sed": true, "sort": true, "ssh": true,
	"stat": true, "su": true, "sudo": true, "sudo.conf": true, "tail": true,
	"tar": true, "top": true, "touch": true, "tr": true, "uname": true,
	"uniq": true, "unzip": true, "uptime": true, "wc": true, "wget": true,
	"whoami": true, "zip": true,
}

// Sandbox session

type SandboxSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	done   chan struct{}
	mu     sync.Mutex
}

func NewSession() (*SandboxSession, error) {
	if err := ExtractRootfs(); err != nil {
		return nil, fmt.Errorf("extracting rootfs: %w", err)
	}

	cmd := exec.Command("/proc/self/exe", "init")
	cmd.Env = append(os.Environ(), "SANDBOX_MODE=session")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	// monitor crash
	go func() {
		_ = cmd.Wait()
	}()

	return &SandboxSession{
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewReader(stdoutPipe),
		done:   make(chan struct{}),
	}, nil
}

func (s *SandboxSession) Run(command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return "", fmt.Errorf("sandbox closed")
	default:
	}

	if _, err := fmt.Fprintln(s.stdin, command); err != nil {
		return "", fmt.Errorf("writing command: %w", err)
	}

	if _, err := fmt.Fprintln(s.stdin, sentinel); err != nil {
		return "", fmt.Errorf("writing sentinel: %w", err)
	}

	var sb strings.Builder
	for {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return sb.String(), fmt.Errorf("reading output: %w", err)
		}
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasSuffix(trimmed, sentinel) {
			sb.WriteString(strings.TrimSuffix(trimmed, sentinel))
			break
		}
		sb.WriteString(line)
	}

	return sb.String(), nil
}

func (s *SandboxSession) Close() error {
	select {
	case <-s.done:
		return nil
	default:
		close(s.done)
	}

	_, _ = fmt.Fprintln(s.stdin, "exit")
	s.stdin.Close()
	err := s.cmd.Wait()

	_ = syscall.Unmount(Rootfs+"/proc", syscall.MNT_FORCE)
	_ = syscall.Unmount(Rootfs+"/dev/null", syscall.MNT_FORCE)

	return err
}

// Rootfs extraction

func ExtractRootfs() error {
	if pathExists(Rootfs) {
		_ = os.RemoveAll(Rootfs)
	}
	if pathExists(ChallengesDir) {
		_ = os.RemoveAll(ChallengesDir)
	}
	_ = os.MkdirAll(ChallengesDir, 0700)

	gzReader, err := gzip.NewReader(bytes.NewReader(embeddedRootfs))
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := strings.TrimPrefix(hdr.Name, "rootfs/")
		dest := filepath.Join(Rootfs, name)
		cleanPath := filepath.Clean(dest)
		if !strings.HasPrefix(cleanPath, Rootfs) {
			return fmt.Errorf("invalid tar path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(cleanPath, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(cleanPath), 0755)
			outFile, _ := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			io.Copy(outFile, tarReader)
			outFile.Close()
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(cleanPath), 0755)
			os.Symlink(hdr.Linkname, cleanPath)
		}
	}

	os.MkdirAll(filepath.Join(Rootfs, "tmp"), 0777)
	os.Chmod(filepath.Join(Rootfs, "tmp"), 01777)
	os.MkdirAll(filepath.Join(Rootfs, "home/ahmed"), 0777)
	os.Chmod(filepath.Join(Rootfs, "home/ahmed"), 01777)
	os.MkdirAll(filepath.Join(Rootfs, "root"), 0700)
	os.MkdirAll(filepath.Join(Rootfs, "etc"), 0755)
	os.MkdirAll(filepath.Join(Rootfs, "bin"), 0755)
	os.MkdirAll(filepath.Join(Rootfs, "var/mail"), 0777)
	os.MkdirAll(filepath.Join(Rootfs, "var/spool/mail"), 0777)
	os.Chmod(filepath.Join(Rootfs, "var/mail"), 01777)
	os.Chmod(filepath.Join(Rootfs, "var/spool/mail"), 01777)

	// Copy host assets to sandbox home if the assets directory exists
	assetsDir := "assets"
	if pathExists(assetsDir) {
		files, err := os.ReadDir(assetsDir)
		if err == nil {
			for _, f := range files {
				if !f.IsDir() {
					srcPath := filepath.Join(assetsDir, f.Name())
					dstPath := filepath.Join(Rootfs, "home/ahmed", f.Name())
					
					src, err := os.Open(srcPath)
					if err != nil {
						continue
					}
					
					dst, err := os.Create(dstPath)
					if err != nil {
						src.Close()
						continue
					}
					
					io.Copy(dst, src)
					src.Close()
					dst.Close()
					
					// ensure the file is readable by the sandbox user
					os.Chmod(dstPath, 0644)
					// chown to ahmed (uid=1000, gid=1000) so the sandbox user can modify it
					os.Chown(dstPath, 1000, 1000)
				}
			}
		}
	}

	// Create a mock sudo script
	sudoPath := filepath.Join(Rootfs, "bin/sudo")
	os.WriteFile(sudoPath, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755)

	// Ensure /lib, /lib64, /usr/lib64 all point to /usr/lib so that
	// provisioned libraries from any distro layout are found correctly.
	ensureLibSymlinks()

	// Provision required tools if they exist on the host
	tools := []string{
		"bash", "ls", "cat", "grep", "git", "useradd", "userdel", "groupadd", "groupdel",
		"passwd", "usermod", "id", "groups", "zip", "gzip", "bzip2", "tar", "find",
		"tail", "head", "stat", "df", "du", "free", "chmod", "chown", "wc", "tee",
		"sed", "touch", "rm", "mkdir", "zcat", "cp", "mv", "cut", "sort", "uniq",
		"whoami", "pwd", "ping", "unzip", "pgrep", "pkill", "nano", "vim", "awk",
	}
	for _, tool := range tools {
		_ = provisionTool(tool)
	}

	// Copy nano syntax files if available on host
	if pathExists("/usr/share/nano") {
		_ = copyDir("/usr/share/nano", filepath.Join(Rootfs, "usr/share/nano"))
	}

	// Ensure /bin/sh points to our new bash provisioned from the host
	shPath := filepath.Join(Rootfs, "bin/sh")
	os.Remove(shPath)
	os.Symlink("bash", shPath)

	// Setup default git identity
	_ = setupGitConfig()

	// Ensure standard system groups exist for tools like useradd
	_ = setupStandardGroups()

	return nil
}

// Sandbox init

func StartSandBox() error {
	syscall.Sethostname([]byte("sandbox"))

	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("mount private failed: %w", err)
	}

	os.MkdirAll(filepath.Join(Rootfs, "dev"), 0755)
	os.WriteFile(filepath.Join(Rootfs, "dev/null"), nil, 0666)
	if err := syscall.Mount("/dev/null", filepath.Join(Rootfs, "dev/null"), "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("mount /dev/null failed: %w", err)
	}

	if err := syscall.Chroot(Rootfs); err != nil {
		return fmt.Errorf("chroot error: %w", err)
	}
	if err := os.Chdir("/home/ahmed"); err != nil {
		return err
	}

	// mount /proc
	os.MkdirAll("/proc", 0755)
	syscall.Mount("proc", "/proc", "proc", 0, "")

	// drop to user unless root is requested
	sandboxUser := os.Getenv("SANDBOX_USER")
	if sandboxUser == "" {
		sandboxUser = defaultUser
	}

	if sandboxUser != "root" {
		if err := dropToUser(sandboxUser); err != nil {
			return err
		}
	} else {
		// Explicitly set root environment for Admin Mode
		os.Setenv("HOME", "/root")
		os.Setenv("USER", "root")
		os.Setenv("LOGNAME", "root")
	}

	switch os.Getenv("SANDBOX_MODE") {
	case "session":
		return runSessionLoop()
	case "cli":
		return runInteractiveShell()
	default:
		return runInteractiveShell()
	}
}

// StartSandboxSession is the CLI entry point used by the privileged parent. It
// forks a child that chroots into the rootfs and drops into an interactive
// shell, while the parent keeps running (holding the check socket listener).
// No private mount namespace is used here so the /dev/null and /proc mounts the
// sandbox child makes stay visible to the short-lived check children the parent
// forks later.
func StartSandboxSession() error {
	signal.Ignore(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	cmd := exec.Command("/proc/self/exe", "init")
	cmd.Env = append(os.Environ(), "SANDBOX_MODE=cli")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting sandbox session: %w", err)
	}
	err := cmd.Wait()

	// Best-effort cleanup of the mounts the sandbox child made in the shared
	// mount namespace.
	_ = syscall.Unmount(filepath.Join(Rootfs, "proc"), syscall.MNT_FORCE)
	_ = syscall.Unmount(filepath.Join(Rootfs, "dev", "null"), syscall.MNT_FORCE)

	return err
}

func runSessionLoop() error {
	defer syscall.Unmount("/proc", 0)
	reader := bufio.NewReader(os.Stdin)
	cwd := "/home/ahmed"

	for {
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			line = strings.TrimRight(line, "\n")

			if line == sentinel {
				break
			}

			lines = append(lines, line)
		}
		cmdStr := strings.TrimSpace(strings.Join(lines, "\n"))
		if cmdStr == "" {
			fmt.Println(sentinel)
			continue
		}
		if cmdStr == "exit" {
			break
		}

		if strings.HasPrefix(cmdStr, "man ") {
			fields := strings.Fields(strings.TrimPrefix(cmdStr, "man "))
			if len(fields) == 0 {
				fmt.Println("usage: man <topic>")
				fmt.Println(sentinel)
				continue
			}
			topic := fields[0]
			if !manAllowlist[topic] {
				fmt.Printf("No man page for %s\n", topic)
				fmt.Println(sentinel)
				continue
			}
			cmdStr = fmt.Sprintf(
				"f=$(find /usr/share/man -name '%s.*' 2>/dev/null | head -1); "+
					"[ -z \"$f\" ] && echo 'No man page for %s' && exit; "+
					"case \"$f\" in *.gz) zcat \"$f\";; *) cat \"$f\";; esac | sed 's/.\x08//g'",
				topic, topic,
			)
		}
		// persistent cd handling
		if strings.HasPrefix(cmdStr, "cd ") && !strings.Contains(cmdStr, "\n") {
			path := strings.TrimSpace(strings.TrimPrefix(cmdStr, "cd "))
			var newCwd string
			if filepath.IsAbs(path) {
				newCwd = path
			} else {
				newCwd = filepath.Join(cwd, path)
			}
			if fi, err := os.Stat(newCwd); err != nil || !fi.IsDir() {
				fmt.Printf("cd: no such directory: %s\n", path)
			} else {
				cwd = newCwd
			}
			fmt.Println(sentinel)
			continue
		}

		if cmdStr == "" {
			fmt.Println(sentinel)
			continue
		}

		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		cmd.Dir = cwd
		cmd.Env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=" + os.Getenv("HOME"),
			"USER=" + os.Getenv("USER"),
			"LOGNAME=" + os.Getenv("LOGNAME"),
			"TERM=xterm",
			"MANPATH=/usr/share/man:/usr/local/share/man",
			"PAGER=less",
			"MANPAGER=less",
			// Ensure dynamic linker finds libs regardless of distro layout
			"LD_LIBRARY_PATH=/usr/lib:/lib:/lib/x86_64-linux-gnu:/usr/lib/x86_64-linux-gnu:/lib/aarch64-linux-gnu:/usr/lib/aarch64-linux-gnu",
		}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		err := cmd.Run()
		if err != nil {
			out.WriteString(fmt.Sprintf("sh error: %v\n", err))
		}
		fmt.Print(out.String())
		fmt.Println(sentinel)
	}
	return nil
}

func runInteractiveShell() error {
	cmd := exec.Command("/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// helpers

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func dropToUser(username string) error {
	passwdPath := "/etc/passwd"
	//passwdPath := filepath.Join(Rootfs, "etc/passwd")
	passwdBytes, err := os.ReadFile(passwdPath)
	if err != nil {
		return fmt.Errorf("cannot read chroot /etc/passwd: %w", err)
	}

	var uid, gid int
	var homeDir string
	for _, line := range strings.Split(string(passwdBytes), "\n") {
		if strings.HasPrefix(line, username+":") {
			parts := strings.Split(line, ":")
			uid, _ = strconv.Atoi(parts[2])
			gid, _ = strconv.Atoi(parts[3])
			homeDir = parts[5]
			break
		}
	}

	if uid == 0 && username != "root" {
		return fmt.Errorf("user %s not found in chroot /etc/passwd", username)
	}

	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups failed: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid failed: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid failed: %w", err)
	}

	os.Setenv("HOME", homeDir)
	os.Setenv("USER", username)
	os.Setenv("LOGNAME", username)
	return nil
}
func setupGitConfig() error {
	config := "[user]\n\tname = OSC Recruit\n\temail = recruit@osc.org\n[init]\n\tdefaultBranch = master\n"
	return os.WriteFile(filepath.Join(Rootfs, "home/ahmed/.gitconfig"), []byte(config), 0644)
}

func provisionTool(name string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}

	// Determine destination (keep it standard in /bin)
	dst := filepath.Join(Rootfs, "bin", name)
	os.Remove(dst)
	if err := copyFile(path, dst); err != nil {
		return err
	}

	// Find all shared library dependencies using ldd
	out, err := exec.Command("ldd", path).Output()
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		var libSrc string

		if strings.Contains(line, "=>") {
			// Standard lib line: "libfoo.so.X => /real/path/libfoo.so.X (0x...)"
			parts := strings.Fields(line)
			if len(parts) >= 3 && strings.HasPrefix(parts[2], "/") {
				libSrc = parts[2]
			}
		} else if strings.HasPrefix(line, "/") {
			// Dynamic linker: "/lib64/ld-linux-x86-64.so.2 (0x...)"
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				libSrc = parts[0]
			}
		}

		if libSrc == "" {
			continue
		}

		// Provision the library preserving host paths (cross-distro compatible)
		_ = provisionLib(libSrc)
	}
	return nil
}

// provisionLib copies a host library into the sandbox at its exact host path.
// This is cross-distro compatible: it preserves paths like
//   - /usr/lib/libc.so.6            (Arch/Fedora)
//   - /lib/x86_64-linux-gnu/libc.so.6 (Ubuntu/Debian)
//
// Additionally, every library is ALSO copied into rootfs/usr/lib/<basename>
// as a universal fallback, so the dynamic linker can always find it even
// without a valid ld.so.cache (which may be stale or missing in the sandbox).
func provisionLib(hostPath string) error {
	// Resolve any symlinks on the HOST to get to the real file
	realPath, err := filepath.EvalSymlinks(hostPath)
	if err != nil {
		realPath = hostPath
	}

	// Copy the real file into the sandbox at its exact path.
	// os.MkdirAll/file ops follow sandbox symlinks (e.g. /lib -> usr/lib)
	sandboxDst := filepath.Join(Rootfs, realPath)
	if err := os.MkdirAll(filepath.Dir(sandboxDst), 0755); err != nil {
		return err
	}
	if err := copyFile(realPath, sandboxDst); err != nil {
		return err
	}

	// If the original hostPath was a symlink, recreate that symlink in the sandbox
	// so that both the original name and real name resolve correctly.
	if realPath != hostPath {
		sandboxLink := filepath.Join(Rootfs, hostPath)
		os.MkdirAll(filepath.Dir(sandboxLink), 0755)
		if _, err := os.Lstat(sandboxLink); os.IsNotExist(err) {
			// Use relative symlink target if in the same directory
			target := realPath
			if filepath.Dir(hostPath) == filepath.Dir(realPath) {
				target = filepath.Base(realPath)
			}
			_ = os.Symlink(target, sandboxLink)
		}
	}

	// Belt-and-suspenders: ALSO copy to flat /usr/lib/<basename>.
	// This ensures discoverability on ALL distros regardless of ld.so.cache
	// or multiarch subdirectory layout (e.g. Ubuntu's x86_64-linux-gnu/).
	flatDst := filepath.Join(Rootfs, "usr/lib", filepath.Base(realPath))
	if flatDst != sandboxDst {
		_ = copyFile(realPath, flatDst)
	}
	// Also symlink with original basename if different
	origBase := filepath.Base(hostPath)
	if origBase != filepath.Base(realPath) {
		origFlat := filepath.Join(Rootfs, "usr/lib", origBase)
		if _, err := os.Lstat(origFlat); os.IsNotExist(err) {
			_ = os.Symlink(filepath.Base(realPath), origFlat)
		}
	}

	return nil
}

// ensureLibSymlinks guarantees that /lib, /lib64, /usr/lib64, and /usr/bin
// inside the sandbox all resolve correctly, regardless of host distro layout.
// - Arch/Fedora: /usr/lib contains everything; /lib64 & /lib are already symlinks.
// - Ubuntu/Debian: libraries live under /lib/x86_64-linux-gnu/ which the os
//   functions redirect through the /lib -> usr/lib symlink we ensure here.
func ensureLibSymlinks() {
	usrLib := filepath.Join(Rootfs, "usr/lib")
	os.MkdirAll(usrLib, 0755)

	// /lib -> usr/lib
	libPath := filepath.Join(Rootfs, "lib")
	if fi, err := os.Lstat(libPath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		os.RemoveAll(libPath)
		os.Symlink("usr/lib", libPath)
	}

	// /lib64 -> usr/lib
	lib64Path := filepath.Join(Rootfs, "lib64")
	if fi, err := os.Lstat(lib64Path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		os.RemoveAll(lib64Path)
		os.Symlink("usr/lib", lib64Path)
	}

	// /usr/lib64 -> lib (so /usr/lib64 -> usr/lib too)
	usrLib64 := filepath.Join(Rootfs, "usr/lib64")
	if fi, err := os.Lstat(usrLib64); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		os.RemoveAll(usrLib64)
		os.Symlink("lib", usrLib64)
	}

	// /usr/bin -> /bin so provisioned tools in /bin are found via /usr/bin PATH
	usrBin := filepath.Join(Rootfs, "usr/bin")
	if fi, err := os.Lstat(usrBin); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		os.RemoveAll(usrBin)
		os.Symlink("../bin", usrBin)
	}
}

func setupStandardGroups() error {
	groups := []string{
		"bin:x:1:",
		"daemon:x:2:",
		"sys:x:3:",
		"adm:x:4:",
		"tty:x:5:",
		"disk:x:6:",
		"lp:x:7:",
		"mail:x:8:",
		"kmem:x:9:",
		"wheel:x:10:",
		"utmp:x:22:",
		"staff:x:50:",
	}

	groupPath := filepath.Join(Rootfs, "etc/group")
	f, err := os.OpenFile(groupPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, g := range groups {
		f.WriteString(g + "\n")
	}
	return nil
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}

	si, err := os.Stat(src)
	if err == nil {
		os.Chmod(dst, si.Mode())
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}
