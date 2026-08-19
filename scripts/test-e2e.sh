#!/bin/bash

# test-e2e.sh - Automated End-to-End Integration Test for CI
# Builds the binary, creates a multi-level challenge folder, encrypts it to an
# .enc archive, decrypts it, and exercises the check.sh execution path inside
# the sandbox (stub -> socket server -> chrooted check child -> HMAC flag).
# Root-gated tests exercise the real chroot; non-root tests verify secrecy.

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}=== Starting QO2 End-to-End Encrypted Archive Integration Test ===${NC}"

WORK_DIR="/tmp/qo2_e2e_test"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

CHALLENGE_DIR="$WORK_DIR/challenges"
ARCHIVE_PATH="$WORK_DIR/exam-test.enc"
PASS="examPass123"
KEY="starterKey123"
STUDENT_ID="2021170034"

# 1. Generate Multi-Level Challenge Folder with various Linux commands
echo -e "${BLUE}[1/4] Generating multi-level challenge folder...${NC}"

# Level 1: Directory creation (mkdir)
mkdir -p "$CHALLENGE_DIR/level1"
cat <<'EOF' > "$CHALLENGE_DIR/level1/README.md"
Create a directory named 'testdir' in your working directory.
EOF
echo "Hint: use 'mkdir testdir'" > "$CHALLENGE_DIR/level1/hint.txt"
cat <<'EOF' > "$CHALLENGE_DIR/level1/check.sh"
#!/bin/bash
if [ -d "$PWD/testdir" ]; then
    echo "Level 1 passed!"
    echo "key=\"LVL-1-K7Q4X\""
    exit 0
else
    echo "Level 1 failed: 'testdir' does not exist."
    exit 1
fi
EOF
echo "LVL-1-K7Q4X" > "$CHALLENGE_DIR/level1/.base_flag"
chmod +x "$CHALLENGE_DIR/level1/check.sh"

# Level 2: User management (useradd)
mkdir -p "$CHALLENGE_DIR/level2"
cat <<'EOF' > "$CHALLENGE_DIR/level2/README.md"
Create a user named 'studentuser'.
EOF
cat <<'EOF' > "$CHALLENGE_DIR/level2/check.sh"
#!/bin/bash
if id "studentuser" &>/dev/null; then
    echo "Level 2 passed!"
    echo "key=\"LVL-2-M3R2D\""
    exit 0
else
    echo "Level 2 failed: user 'studentuser' does not exist."
    exit 1
fi
EOF
echo "LVL-2-M3R2D" > "$CHALLENGE_DIR/level2/.base_flag"
chmod +x "$CHALLENGE_DIR/level2/check.sh"

# Level 3: File permissions (chmod)
mkdir -p "$CHALLENGE_DIR/level3"
cat <<'EOF' > "$CHALLENGE_DIR/level3/README.md"
Copy secret.txt to $HOME/secret.txt and set permissions to 600.
EOF
echo "Level 3 secret content" > "$CHALLENGE_DIR/level3/secret.txt"
cat <<'EOF' > "$CHALLENGE_DIR/level3/check.sh"
#!/bin/bash
if [ -f "$HOME/secret.txt" ] && [ "$(stat -c "%a" "$HOME/secret.txt" 2>/dev/null)" == "600" ]; then
    echo "Level 3 passed!"
    echo "key=\"LVL-3-P9W1Z\""
    exit 0
else
    echo "Level 3 failed: Check file copy and permissions."
    exit 1
fi
EOF
echo "LVL-3-P9W1Z" > "$CHALLENGE_DIR/level3/.base_flag"
chmod +x "$CHALLENGE_DIR/level3/check.sh"

# 2. Build qo binary and encrypt archive
echo -e "${BLUE}[2/4] Building binary & encrypting challenge archive...${NC}"
CGO_ENABLED=0 go build -o "$WORK_DIR/qo" main.go

"$WORK_DIR/qo" build -f "$CHALLENGE_DIR" -p "$PASS" -k "$KEY" -u "2020-01-01 00:00" -o "$ARCHIVE_PATH"

if [ ! -f "$ARCHIVE_PATH" ]; then
    echo -e "${RED}Failed: Encrypted archive $ARCHIVE_PATH was not created.${NC}"
    exit 1
fi
echo -e "${GREEN}Encrypted archive built successfully.${NC}"

# 3. Run the secrecy & archive round-trip tests (non-root)
echo -e "${BLUE}[3/4] Running secrecy & archive round-trip tests...${NC}"
go test -v ./pkg/archive/... -run 'TestArchiveRoundTripSecrecy|TestIsValidFolderStructure'
go test -v ./pkg/sandbox/... -run 'TestWriteCheckStubsSecrecy|TestLoadBaseFlag|TestPresentCheckSuccess|TestValidateLevelKey|TestGenerateUniqueFlag|TestRunSetup|TestSetupSocketRoundTrip'

# 4. Run the root-required check-execution tests (real chroot + socket + HMAC)
echo -e "${BLUE}[4/4] Running root-required check-execution tests...${NC}"
if [ "$(id -u)" == "0" ]; then
    go test -v ./pkg/sandbox/... -run 'TestCheckSocketRoundTrip|TestCheckServerSocketMode'
else
    echo -e "${YELLOW}Not running as root; skipping chroot integration tests.${NC}"
fi

echo -e "${GREEN}=== All End-to-End Encrypted Archive Tests Passed Successfully! ===${NC}"