#!/bin/bash

# setup.sh - Installation and build script for qo2
# Supports: Arch Linux, Debian/Ubuntu, and Fedora

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}=== OSC Linux Interview Tool: Setup & Installation ===${NC}"

# 1. Check for root/sudo
if [ "$EUID" -ne 0 ]; then
  echo -e "${YELLOW}Please run with sudo to install dependencies.${NC}"
  # Note: The script will continue and use sudo for specific commands
fi

# 2. Detect Package Manager and Install Dependencies
echo -e "${BLUE}[1/4] Installing dependencies...${NC}"

if command -v pacman >/dev/null; then
    echo -e "Detected ${GREEN}Arch Linux${NC}"
    sudo pacman -Sy --needed --noconfirm go sqlite git tar
elif command -v apt-get >/dev/null; then
    echo -e "Detected ${GREEN}Debian/Ubuntu${NC}"
    sudo apt-get update
    sudo apt-get install -y golang sqlite3 git tar
elif command -v dnf >/dev/null; then
    echo -e "Detected ${GREEN}Fedora${NC}"
    sudo dnf install -y golang sqlite git tar
else
    echo -e "${RED}Unsupported package manager. Please install 'go', 'sqlite3', and 'git' manually.${NC}"
fi

# 3. Verify Database
echo -e "${BLUE}[2/4] Verifying database...${NC}"
if [ -f "linux.db" ]; then
    echo -e "${GREEN}Using existing linux.db file.${NC}"
else
    echo -e "${YELLOW}Warning: linux.db not found in root directory.${NC}"
    echo -e "You may need to run your SQL scripts to initialize the database."
fi

# 4. Git Configuration Check
echo -e "${BLUE}[3/4] Checking Git configuration...${NC}"
if ! git config --global user.email >/dev/null; then
    echo -e "${YELLOW}Setting up default Git identity (OSC Recruit)...${NC}"
    git config --global user.email "recruit@osc.org"
    git config --global user.name "OSC Recruit"
    git config --global init.defaultBranch "master"
else
    echo -e "${GREEN}Git identity already configured.${NC}"
fi

# 5. Build the Application
echo -e "${BLUE}[4/4] Building the application...${NC}"
if [ -f "main.go" ]; then
    go build -o qo main.go
    echo -e "${GREEN}Build successful! Binary created: ./qo${NC}"
else
    echo -e "${RED}Error: main.go not found. Are you in the right directory?${NC}"
    exit 1
fi

echo -e "\n${GREEN}=== Setup Complete ===${NC}"
echo -e "To start the quiz, run:"
echo -e "  ${BLUE}sudo ./qo start${NC}\n"
