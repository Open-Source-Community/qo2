# Linux evaluation '26 — Setup & Test Instructions

> **Important:** run this on a Linux machine (Arch, Debian/Ubuntu, Fedora, OpenSUSE, Alpine all work). You need `sudo` access.

## 1. Install `qo`

Open a terminal and run:

```bash
curl -fsSL https://raw.githubusercontent.com/Open-Source-Community/qo2/main/setup.sh | bash
```

The script detects your distro, installs the required tools (Go, tar, gzip), clones the source, builds `qo` as a static binary, and installs it to `/usr/local/bin/qo` with the `qo-check`, `qo-setup`, and `qo-reset` aliases it needs.

Verify:

```bash
qo --help
```

> No network access? Download the prebuilt **`qo`** binary and **`setup.sh`** from
> the drive folder (step 2), save both in your home folder, then:
>
> ```bash
> cd ~
> chmod +x qo setup.sh
> ./setup.sh ./qo
> ```

## 2. Download the test archive

Get the practice archive from the drive:

**https://drive.google.com/drive/folders/1nhKUefmOJ8FNPSUXz4sXmCMmDvnXtsc3?usp=sharing**

Download `test.enc` and save it in your home folder.

## 3. Start the practice session

```bash
cd ~
sudo qo start -m test -i <YOUR_STUDENT_ID> -a ~/test.enc -p osc2026 -k testkey -d 90m
```

- `-m` — mode: `test` for practice sessions (results go to the test dashboard), `eval` for the real event. **Use `test` for now.**
- `-i` — your student ID (the one you registered with, e.g. `2023001`)
- `-a` — path to `test.enc`
- `-p` — password (given above)
- `-k` — starter key (given above)
- `-d` — session duration in minutes

You will land inside the sandbox shell (`ahmed@sandbox:~$`).

## 4. Inside the sandbox

```bash
qo-setup                  # create all 10 test levels
cd ~/challenges/test1
bash tools-test.sh        # sanity check — expect: Total tests passed: 43/43
cat README.md             # read the task
# solve the task, then: 
./check.sh                # get your flag for this level
```

Repeat for the other levels (`test2` … `test10`) — each has its own README.

Useful commands inside the sandbox:

```bash
qo-setup test2            # set up one level
qo-reset test2            # reset a level to its original files
ls ~/challenges          # list all levels
```

## 5. Leaderboard

Live rankings will be shown at: **https://<leaderboard-url>/** _(placeholder — link will be announced)_

During practice, your test submissions appear on the admin dashboard under the **Test run** section, so the organizers can see you are comfortable with the tool before the real event.

## 6. Tips

- `exit` — leave the sandbox (your session ends).
- The real event will use a different archive and password, announced later.
- If anything is broken or a tool is missing in the sandbox, tell the organizers — this practice round exists to catch those issues.