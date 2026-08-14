# Multi-Distro Compatibility Specifications

The QO2 testing tool targets full support across the following 5 Linux distributions:
- **Debian**
- **Ubuntu**
- **Fedora**
- **Kali Linux**
- **Pop!_OS**

## Verification Criteria per Distro

1. **Tool Installation**:
   - `pkg/scripts/setup.sh` correctly identifies the package manager (`apt-get` for Debian/Ubuntu/Kali/Pop!_OS, `dnf` for Fedora, `pacman` for Arch).
   - All core system dependencies (`golang`, `sqlite3`, `git`, `tar`, `zip`, `gzip`, `bzip2`) install cleanly without conflicts.

2. **Sandbox Provisioning**:
   - Core tools required by evaluation challenges (`bash`, `ls`, `cat`, `grep`, `git`, `useradd`, `passwd`, `chmod`, `chown`, `ping`, `unzip`, `pgrep`, `pkill`, `nano`, `vim`, `awk`) are copied via `ldd` dynamic link resolution.
   - Shared library locations (`/usr/lib`, `/lib/x86_64-linux-gnu`, `/lib64`) resolve correctly inside the chroot via `ensureLibSymlinks()`.

3. **Editor Functionality**:
   - `nano`: Provisioned with full syntax highlighting support (`/usr/share/nano`).
   - `vim`: Provisioned with best-effort runtime library search paths.

4. **Secured Script Extraction**:
   - Challenge validation scripts (`check.sh`, `setup.sh`, `cleanup.sh`) extract to root-only `/tmp/rootfs_challenges/` (mode `0700`), remaining inaccessible from inside the student chroot.
