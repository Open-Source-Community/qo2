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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

//go:embed rootfs.tar.gz
var embeddedRootfs []byte

const target = "/tmp"
const sentinel = "\x00__QO_EOF__\x00"

var (
	Rootfs      = filepath.Join(target, "rootfs")
	defaultUser = "ahmed"
)

// ─── Sandbox session ─────────────────────────────────────────────

type SandboxSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	done   chan struct{}
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
		err := cmd.Wait()
		fmt.Fprintf(os.Stderr, "\n[SANDBOX EXITED]: %v\n", err)
	}()

	return &SandboxSession{
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewReader(stdoutPipe),
		done:   make(chan struct{}),
	}, nil
}

func (s *SandboxSession) Run(command string) (string, error) {
	select {
	case <-s.done:
		return "", fmt.Errorf("sandbox closed")
	default:
	}

	if _, err := fmt.Fprintln(s.stdin, command); err != nil {
		return "", fmt.Errorf("writing command: %w", err)
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
		if trimmed == sentinel {
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
	os.MkdirAll(filepath.Join(Rootfs, "home/ahmed"), 0755)
	os.MkdirAll(filepath.Join(Rootfs, "bin"), 0755)

	return nil
}

// Sandbox init

func StartSandBox() error {
	syscall.Sethostname([]byte("sandbox"))

	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("mount private failed: %w", err)
	}
	if err := syscall.Chroot(Rootfs); err != nil {
		return fmt.Errorf("chroot error: %w", err)
	}
	if err := os.Chdir("/tmp"); err != nil {
		return err
	}

	// mount /proc
	os.MkdirAll("/proc", 0755)
	syscall.Mount("proc", "/proc", "proc", 0, "")

	// safe /dev/null
	os.MkdirAll("/dev", 0755)

	unix.Mknod("/dev/null", unix.S_IFCHR|0666, int(unix.Mkdev(1, 3)))
	if err := dropToUser(defaultUser); err != nil {
		return err
	}

	switch os.Getenv("SANDBOX_MODE") {
	case "session":
		return runSessionLoop()
	default:
		return runInteractiveShell()
	}
}

func runSessionLoop() error {
	defer syscall.Unmount("/proc", 0)
	reader := bufio.NewReader(os.Stdin)
	cwd := "/tmp"

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		cmdStr := strings.TrimSpace(line)

		if cmdStr == "exit" {
			break
		}

		// persistent cd handling
		if strings.HasPrefix(cmdStr, "cd ") {
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
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		cmd.Run()
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
