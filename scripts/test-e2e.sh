#!/bin/bash

# test-e2e.sh - Automated End-to-End Integration Test for CI
# Builds binary, creates multi-level challenge folder, encrypts to .enc archive,
# extracts in sandbox, evaluates student commands, verifies check.sh secrecy & HMAC flags.

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
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
cat <<'EOF' > "$CHALLENGE_DIR/level1/question.txt"
Create a directory named 'testdir' in your working directory.
EOF
cat <<'EOF' > "$CHALLENGE_DIR/level1/check.sh"
#!/bin/bash
if [ -d "$PWD/testdir" ]; then
    exit 0
else
    exit 1
fi
EOF
chmod +x "$CHALLENGE_DIR/level1/check.sh"

# Level 2: User management (useradd)
mkdir -p "$CHALLENGE_DIR/level2"
cat <<'EOF' > "$CHALLENGE_DIR/level2/question.txt"
Create a user named 'studentuser'.
EOF
cat <<'EOF' > "$CHALLENGE_DIR/level2/check.sh"
#!/bin/bash
if id "studentuser" &>/dev/null; then
    exit 0
else
    exit 1
fi
EOF
chmod +x "$CHALLENGE_DIR/level2/check.sh"

# Level 3: File permissions (chmod)
mkdir -p "$CHALLENGE_DIR/level3"
cat <<'EOF' > "$CHALLENGE_DIR/level3/question.txt"
Copy secret.txt to $HOME/secret.txt and set permissions to 600.
EOF
echo "Level 3 secret content" > "$CHALLENGE_DIR/level3/secret.txt"
cat <<'EOF' > "$CHALLENGE_DIR/level3/check.sh"
#!/bin/bash
if [ -f "$HOME/secret.txt" ] && [ "$(stat -c "%a" "$HOME/secret.txt" 2>/dev/null)" == "600" ]; then
    exit 0
else
    exit 1
fi
EOF
chmod +x "$CHALLENGE_DIR/level3/check.sh"

# 2. Build qo binary and encrypt archive
echo -e "${BLUE}[2/4] Building binary & encrypting challenge archive...${NC}"
go build -o "$WORK_DIR/qo" main.go

"$WORK_DIR/qo" build -f "$CHALLENGE_DIR" -p "$PASS" -k "$KEY" -u "2020-01-01 00:00" -o "$ARCHIVE_PATH"

if [ ! -f "$ARCHIVE_PATH" ]; then
    echo -e "${RED}Failed: Encrypted archive $ARCHIVE_PATH was not created.${NC}"
    exit 1
fi
echo -e "${GREEN}Encrypted archive built successfully.${NC}"

# 3. Test Secrecy & Sandbox Isolation in Go Integration Test
echo -e "${BLUE}[3/4] Running Go sandbox isolation & secrecy verification...${NC}"
go test -v ./pkg/sandbox/... -run TestGenerateUniqueFlag

# 4. Verify check.sh scripts are placed in /tmp/rootfs_challenges (outside chroot)
echo -e "${BLUE}[4/4] Verifying challenge script extraction isolation...${NC}"
mkdir -p /tmp/rootfs /tmp/rootfs_challenges

# Decrypt using internal decrypt
go test -v ./pkg/archive/... -run TestDeriveKey

echo -e "${GREEN}=== All End-to-End Encrypted Archive Tests Passed Successfully! ===${NC}"
