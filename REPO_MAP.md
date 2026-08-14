# QO2 — REPO_MAP

## Project Overview

QO2 is a Go CLI/TUI tool that lets instructors build encrypted, time-locked Linux challenge archives and students solve them inside isolated sandboxes. The sandbox uses Linux namespace isolation (`CLONE_NEWUTS`, `CLONE_NEWPID`, `CLONE_NEWNS`) + `chroot` into an embedded root filesystem (~24 MB plain BusyBox-style directory). Student commands are sent over a pipe to a subprocess inside the sandbox and executed via `/bin/sh -c`. There is no Docker, no user namespace, no seccomp, and no cgroups — isolation is partial and relies on chroot + PID separation.

## Directory Structure

```
qo2/
├── main.go                      # Entry: "init" → sandbox subprocess; no args → TUI; else → Cobra CLI
├── go.mod / go.sum              # Go module (github.com/ahmedYasserM/qo), Go 1.26
├── README.md                    # Docs, usage, known unimplemented features
├── .gitignore                   # *.db, scratch/, .idea/, qo binary
├── .vscode/launch.json          # Debug config (runs as root)
├── assets/                      # Sample data files copied into sandbox
│   ├── code.js / code.py        #   Todo-list scripts (reference for challenges)
│   └── customer.csv / org.csv   #   Fake CSV data (100 rows each)
├── cmd/
│   ├── root.go                  # Cobra root command (`qo`)
│   ├── build.go                 # `qo build` — encrypt challenge folder → .enc archive
│   └── start.go                 # `sudo qo start` — extract rootfs, decrypt, launch sandbox
├── pkg/
│   ├── archive/
│   │   ├── encrypt.go           # AES-256-CTR bulk encryption + AES-256-GCM for unlock-time
│   │   ├── decrypt.go           # Stream-decrypt + time-gate extraction into sandbox rootfs
│   │   └── utils.go             # PBKDF2 key derivation, folder structure validation
│   ├── database/
│   │   ├── types.go             # Client interface, Question/Session structs, GradeWithSandbox()
│   │   ├── supabase.go          # Supabase backend (hardcoded keys, anonymous auth)
│   │   └── sqlite.go            # Local SQLite backend (pure Go, no CGO)
│   ├── logger/
│   │   └── log.go               # Colored emoji-prefixed logger (Info/Warn/Error/Success)
│   ├── sandbox/
│   │   ├── rootfs.go            # Core sandbox engine (658 lines) — clone(), chroot, provisioning
│   │   └── rootfs.tar.gz        # Embedded root filesystem (~24 MB, compressed)
│   ├── rootfs.tar.gz            # Duplicate (~10 MB), possibly stale
│   ├── scripts/                 # Near-duplicate of scripts/ at repo root
│   └── tui/
│       └── render.go            # Bubble Tea TUI — info form + interactive quiz (520 lines)
└── scripts/
    ├── setup.sh                 # Multi-distro build/install (Arch, Debian, Fedora)
    ├── install.sh               # Simple install from GitHub
    ├── inject.sh                # Copy binary + ldd deps into rootfs
    ├── gen-example.sh           # Generate sample challenge folder with 3 levels
    ├── schema.sql               # Empty placeholder
    ├── questions.sql            # 17 INSERTs — Linux challenge questions
    ├── clear_data.sh            # SQLite: wipe user/session data, keep questions
    ├── export_users.py          # Export users + sessions + submissions to JSON
    └── export_markdown.py       # Export results as Markdown report
```

## Entry Points

| Mode | Command | What happens |
|---|---|---|
| Sandbox init | `main.go` with `os.Args[1] == "init"` | Called internally via `/proc/self/exe init` — runs the sandboxed command loop |
| Interactive TUI | `qo` (no args) | Bubble Tea app → info form → Supabase/SQLite quiz → sandbox grading |
| Instructor CLI | `qo build -f <folder> -p <password> -k <key> -u <unlock-time>` | Encrypts challenge folder |
| Student CLI | `sudo qo start -i <id> -a <archive> -p <password> -k <key>` | Root-required; extracts rootfs, decrypts, launches sandbox |

## Key Modules

### `pkg/sandbox/rootfs.go` (658 lines)
Core sandbox engine. `NewSession()` calls `clone()` with `CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS` to spawn a child running `/proc/self/exe init`. `StartSandBox()` does `syscall.Chroot()`, mounts `/proc`, drops privileges to a non-root user via `Setuid/Setgid`. `provisionTool()` copies host binaries + their `ldd`-resolved shared libraries into the chroot. The sandbox processes commands over a pipe with a sentinel terminator; thread-safe via `sync.Mutex`. Depends on `golang.org/x/sys` (syscall) and embedded `rootfs.tar.gz`.

### `pkg/archive/encrypt.go` / `decrypt.go`
Encryption layer. Uses AES-256-CTR for bulk tar data (random salt + nonce per archive). Embeds an encrypted unlock-time file (AES-256-GCM) inside the tar, decrypted with a separate "starter key". Decryption checks `time.Now() >= unlockTime` and refuses to extract before that time. Depends on `golang.org/x/crypto` (PBKDF2, AES).

### `pkg/database/types.go` + `supabase.go` + `sqlite.go`
Abstracts persistence behind a `Client` interface. Two implementations: Supabase (remote, hardcoded credentials, anonymous auth) and SQLite (local file). `GradeWithSandbox()` orchestrates: setup script → student answer → test script → cleanup script, all inside the sandbox process. Depends on `github.com/supabase-community/supabase-go` and `modernc.org/sqlite`.

### `pkg/tui/render.go` (520 lines)
Two-screen Bubble Tea app. `infoModel` (5-field user form) → `questionModel` (quiz with textarea, progress bar, spinner, pass/fail output display). `gradeAnswer()` runs `GradeWithSandbox()` async via `tea.Cmd`. Depends on `charmbracelet/bubbletea`, `bubbles`, `lipgloss`.

## Tech Stack

- **Language**: Go 1.26
- **CLI**: Cobra + coloredcobra
- **TUI**: Charm Bubble Tea v1, Bubbles v0.21, Lipgloss v1
- **Database**: pure-Go SQLite (modernc.org/sqlite) or Supabase REST client
- **Encryption**: AES-256-CTR/GCM, PBKDF2 (100k SHA-256 iterations)
- **Sandboxing**: Linux namespaces (UTS + PID + mount) + chroot — **no Docker, no user ns, no seccomp, no cgroups**
- **Embedded rootfs**: ~24 MB gzip'd tar built from host tools at compile time

## Existing Session/User Handling

- **Session lifecycle**: User info form → `InitializeSession()` (DB init, user save, question fetch, sandbox create, session row persist) → interactive quiz → `SaveSession()` (close sandbox, calc score, update DB).
- **Per-user isolation**: Each user gets their own sandbox subprocess (`clone()`). The sandbox chroots to a fresh copy of the rootfs. Sessions are serialized per-process via `sync.Mutex`. **No mechanism prevents concurrent sessions for the same user.**
- **State persistence**: Supabase stores sessions server-side; SQLite stores locally. Student ID parsed from CLI `-i` flag.
- **No auth beyond anon Supabase**: TUI mode uses Supabase anonymous auth; CLI mode uses only a numeric student ID.

## Existing Network/Resource Controls

- **Network**: **None.** Sandbox shares the host network stack — no `CLONE_NEWNET`. Students can `ping`, `curl` (if provisioned), access localhost services.
- **Rate limiting**: **None.** No limit on submissions, sandbox spawns, API calls, or concurrent sessions.
- **Resource caps**: **None.** No cgroups — CPU, memory, disk, and process count are unbounded.
- **Time limits**: The unlock-time gating (archive decryption) is the only temporal control. Duration flag (`-d`) is accepted but documented as "not implemented yet".

## Config & Deployment

- **No Dockerfile** or docker-compose exists.
- **No CI/CD** config found.
- **Environment config**: None via env vars — Supabase URL/key are hardcoded in source. SQLite path is a struct field with no env-var override.
- **Root requirement**: `sudo qo start` and the TUI sandbox (`clone()` with namespaces) require root. VS Code debug config has `"asRoot": true`.
- **Install scripts**: `scripts/setup.sh` (multi-distro) and `scripts/install.sh` (simple clone+go build). Both fetch source from GitHub and build the binary.

## Known Gaps/TODOs

- **No user namespace**: `CLONE_NEWUSER` is not used — the sandbox child shares the host UID/GID namespace. `dropToUser()` only reduces privileges within the existing namespace; a compromised sandbox can still interact with host user IDs.
- **No network namespace**: Sandbox can access all host network interfaces. This is the biggest isolation gap for multi-tenant deployment.
- **No seccomp**: No system call filtering. Students can call `mount`, `reboot`, `keyctl`, etc. inside their namespace (some will fail due to chroot, but not all).
- **No cgroups**: No CPU/memory/disk/process limits. A student can fork-bomb or OOM the host.
- **No rate limiting**: One-shot per question is the only throttle. No API rate limiting, no sandbox spawn limits.
- **Hardcoded secrets**: Supabase API URL and publishable anon key are in plaintext in `pkg/database/supabase.go:17-18`.
- **Host-dependent sandbox**: Tools are copied from the host at runtime via `provisionTool()` / `ldd`. The sandbox content varies by host distro — not reproducible.
- **Root requirement**: The entire sandbox mechanism requires root privileges, which is risky for any service exposing this to users.
- **Duration flag unimplemented**: `-d` is parsed but has no effect (README confirms).
- **Legacy non-sandboxed grading**: `Question.Test()` in `types.go` runs commands locally on the host (commented as temporary, pending "integrated run feature").
- **Duplicate rootfs**: Two copies of `rootfs.tar.gz` exist (`pkg/` and `pkg/sandbox/`) with different sizes (~10 MB vs ~24 MB) — one likely stale.
