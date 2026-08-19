# QO2 Threat Model

Scope: the `qo start` CLI evaluation flow (`ctf-improvements`). The interview/TUI
flow is out of scope.

## Participants and trust

| Party | Trust | Notes |
|---|---|---|
| Instructor | Fully trusted | Builds archives, runs `qo build`, hosts `qo start`. |
| `qo start` parent process | Fully trusted | Runs as root for the whole session. Holds the check socket, reads the protected scripts, computes flags, best-effort leaderboard sync. |
| Sandboxed student shell | **Untrusted** | Rootless (`ahmed`, uid/gid 1000) by default, confined by chroot. |
| Check child process | Semitrusted | Forked by the parent, `chroot()`ed into the same rootfs, runs the real check script. Dropped to the student's uid when the session is unprivileged. |
| Leaderboard endpoint | Untrusted for correctness | Optional; failures are logged and ignored. |

## Isolation boundaries

- **Namespaces**: the CLI sandbox uses **chroot only** (no `CLONE_NEWNS`). This
  is deliberate: the sandbox child's `/dev/null` and `/proc` mounts must remain
  visible to the check children the parent forks later, so those mounts are made
  in the shared (host) mount namespace. The TUI `NewSession` path uses
  `CLONE_NEWUTS|CLONE_NEWPID|CLONE_NEWNS`.
- **No user namespace, no seccomp, no cgroups.** Accepted risk for this event;
  hardening is out of scope.
- The student shell shares the host PID and network namespaces. `ps`/`ping`
  reflect host state. Accepted.

## Secrecy properties (the load-bearing ones)

1. **Real check scripts never enter the chroot.** `DecryptTarArchive` routes
   `check.sh`, `setup.sh`, `cleanup.sh`, `flag.txt`, `.base_flag` to the
   root-only `/tmp/rootfs_challenges` (mode `0700`), which the student's chroot
   cannot reach. Only thin, secret-free stubs land inside the chroot.
2. **The script body is piped via stdin, never written to the filesystem.** The
   parent reads the real script content and feeds it to a forked child's bash
   over a pipe; the content exists only in memory and in the child's stdin.
3. **Base flags never reach the student's view.** `GenerateUniqueFlag` HMACs
   `(base_flag, student_id)`. On success the parent replaces every occurrence of
   the base flag in the script output with the per-student flag (which also
   rewrites the historical `key="..."` print in place) and appends a
   congratulations line if needed.
4. **Socket is session-scoped and not world-accessible.** Bound inside the
   freshly extracted rootfs (`/tmp/rootfs/tmp/qo-check.sock`), mode `0660`,
   root-owned, group = sandbox user (uid/gid 1000). Root-owned + group-restricted
   + not world-accessible. A strict `0600` root socket is unreachable from the
   dropped-privilege shell that must connect to it, so `0660` is the smallest
   deviation that remains functional while preserving root ownership and
   session scoping. The socket dies with the session (rootfs is wiped on the
   next `ExtractRootfs`).
5. **Level keys are validated.** The stub's level identity is checked against a
   path-containment rule before the parent touches `ChallengesDir`; `..`/
   absolute/odd characters are rejected (applies to the `check`, `setup`, and
   `reset` socket verbs alike).
6. **The student never sees `/tmp`.** Non-secret level data is extracted to the
   root-only pristine staging tree (`/tmp/rootfs_pristine`, mode `0700`) and
   shipped to `~/challenges/<level>` only via the `qo-setup`/`qo-reset` socket
   verbs. The student works exclusively from their home directory; a corrupted
   working copy is restorable at any time by re-running `qo-setup`/`qo-reset`,
   which wipes and re-copies from pristine.

## Attack surface and responses

- **Student reads real check.sh / base flag**: prevented by properties 1-3. The
  copied `/bin/qo-check` client inside the chroot is the same binary as the
  host's; it contains no level secrets (embedded rootfs is the student's own
  rootfs; the Supabase anon key is publishable).
- **Student forges another student's flag**: requires the base flag, which never
  leaves the parent (property 3).
- **Student replays/brute-forces the socket**: the socket only runs the real
  check for the requested level and returns output; no new capability. `exit 0`
  is still gated on the check passing. The `setup`/`reset` verbs only copy
  non-secret pristine data into the requester's own home; they cannot reach
  `ChallengesDir`.
- **Student alters the stub**: the stub carries no secrets; tampering only
  breaks their own `./check.sh`.
- **Student corrupts their working copy**: `qo-setup`/`qo-reset` restore the
  level from pristine; the pristine tree is root-only and re-extracted fresh on
  every `qo start`.
- **Student reads `/proc` of host processes**: possible (no PID namespace);
  accepted risk. Avoid placing session secrets in host process env/cmdlines.
- **Network exfiltration**: sandbox shares the host network. The base flag never
  enters the chroot, so there is nothing secret to exfiltrate.
- **Check child compromise**: the child is `chroot()`ed and dropped to the
  student's uid; it runs instructor-supplied scripts against a view of the
  filesystem the student controls — this is the intended grading interaction.
  A malicious instructor script is out of threat model.

## Leaderboard (optional)

Best-effort single send of `{student_id, question_id, flag}` on success. No
retry queue, no idempotency key (deliberate). Failures are logged locally;
coverage relies on a manual Google Form path outside the tool.

## Known accepted risks (deliberate, not bugs)

- **sudo wrapper is a no-op** (`exec "$@"` as the same user) — documented, not
  a privilege-escalation vector; it exists so scripts that invoke `sudo` don't
  fail. No fix intended.
- **Man allowlist excludes admin-account tools** (`useradd`, `passwd`,
  `usermod`, `groupadd`, `groupdel`) even when provisioned.
- **Duration flag** is parsed but has no effect.
- **Root requirement**: `qo start` and the check server must run as root.