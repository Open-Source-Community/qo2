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

	"github.com/ahmedYasserM/qo/pkg/logger"
)

//go:embed rootfs.tar.gz
var embeddedRootfs []byte

const target = "/tmp"

var (
	Rootfs      string = filepath.Join(target, "rootfs")
	defaultUser string = "ahmed"
)

// PathExists checks if a file or directory exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// ExtractRootfs extracts the tar-archived rootfs folder in /tmp
func ExtractRootfs() error {
	if pathExists(Rootfs) {
		_ = syscall.Unmount(filepath.Join(Rootfs, "proc"), syscall.MNT_FORCE) // force unmount of /proc to handle possible previous exits using external kill signal
		_ = syscall.Unmount(filepath.Join(Rootfs, "dev/null"), syscall.MNT_FORCE)

		if err := os.RemoveAll(Rootfs); err != nil {
			return err
		}
	}

	gzReader, err := gzip.NewReader(io.NopCloser(bytes.NewReader(embeddedRootfs)))
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // done
		}
		if err != nil {
			return err
		}

		destPath := filepath.Join(target, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}

			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}

			if err := os.Symlink(header.Linkname, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func dropToUser(username string) error {
	passwdBytes, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return err
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
		return err
	}
	if err := syscall.Setuid(uid); err != nil {
		return err
	}

	// Set environment variables
	os.Setenv("HOME", homeDir)
	os.Setenv("USER", username)
	os.Setenv("LOGNAME", username)

	return nil
}

func StartSandBox(persistent bool, command string) error {

	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := ExtractRootfs(); err != nil {
			return err
		}
		if err := syscall.Chroot(Rootfs); err != nil {
			return err
		}

		if err := os.Chdir("/tmp"); err != nil {
			return err
		}

		if err := os.MkdirAll("/tmp", 0777); err != nil {
			return err
		}

		uid := 1000
		gid := 1000
		_ = os.Chown("/tmp", uid, gid)

		if err := os.MkdirAll("/dev", 0755); err != nil {
			return err
		}

		devNull := "/dev/null"

		if _, err := os.Stat(devNull); os.IsNotExist(err) {
			f, err := os.Create(devNull)
			if err != nil {
				return err
			}
			f.Close()
		}

		//mount real /dev/null
		if err := syscall.Mount("/dev/null", devNull, "", syscall.MS_BIND, ""); err != nil {
			return err
		}

		if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
			return err
		}

		if err := dropToUser(defaultUser); err != nil {
			return err
		}

		logger.Info("You are now inside the isolated enviornemnt.")

		//command := os.Getenv("SANDBOX_CMD")

		// if command == "" {
		// 	return fmt.Errorf("no command provided to sandbox")
		// }

		if persistent {
			reader := bufio.NewReader(os.Stdin)
			cwd := "/tmp"
			for {
				fmt.Print("> ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input == "exit" || input == "" {
					break
				}

				parts := strings.Fields(input)
				if parts[0] == "cd" && len(parts) > 1 {
					var target string

					if len(parts) < 2 {
						target = "/tmp"
					} else if filepath.IsAbs(parts[1]) {
						target = parts[1]
					} else {
						target = filepath.Join(cwd, parts[1])
					}

					target = filepath.Clean(target)

					fi, err := os.Stat(target)
					if err != nil || !fi.IsDir() {
						fmt.Println("cd: no such directory:", target)
						continue // not update cwd
					}

					cwd = target
					continue
				}

				cmd := exec.Command("/bin/sh", "-c", input)
				cmd.Dir = cwd // run command in current working directory
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Run()
			}
		} else {
			command := os.Getenv("SANDBOX_CMD")
			if command == "" {
				return fmt.Errorf("no command provided to sandbox")
			}

			cmd := exec.Command("/bin/sh", "-c", command)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}

		_ = syscall.Unmount("/proc", 0)     // unmount /proc
		_ = syscall.Unmount("/dev/null", 0) // unmount /dev/null

		return nil

	}

	cmd := exec.Command("/proc/self/exe", "init")
	//cmd.Args = []string{"init"}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	if err := cmd.Run(); err != nil {
		return err
	}

	err := syscall.Unmount(Rootfs+"/proc", 0)

	return err
}

func RunIsolatedSession() error {
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS,
	}

	// set env to indicate persistent shell
	cmd.Env = append(os.Environ(), "SANDBOX_PERSISTENT=1")

	return cmd.Run()
}
