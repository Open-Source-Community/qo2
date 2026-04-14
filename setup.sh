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
echo -e "${BLUE}[1/5] Installing dependencies...${NC}"

if command -v pacman >/dev/null; then
    echo -e "Detected ${GREEN}Arch Linux${NC}"
    sudo pacman -Sy --needed --noconfirm go sqlite git tar kitty zip gzip bzip2
elif command -v apt-get >/dev/null; then
    echo -e "Detected ${GREEN}Debian/Ubuntu${NC}"
    sudo apt-get update
    sudo apt-get install -y golang sqlite3 git tar kitty zip gzip bzip2
elif command -v dnf >/dev/null; then
    echo -e "Detected ${GREEN}Fedora${NC}"
    sudo dnf install -y golang sqlite git tar kitty zip gzip bzip2
else
    echo -e "${RED}Unsupported package manager. Please install 'go', 'sqlite3', 'git', 'gzip', and 'bzip2' manually.${NC}"
fi

# 3. Clone or Enter Directory
echo -e "${BLUE}[2/5] Fetching source code...${NC}"
if [ -f "main.go" ] && [ -d "pkg" ]; then
    echo -e "${GREEN}Already in project directory.${NC}"
else
    if [ ! -d "qo2" ]; then
        echo -e "Cloning repository..."
        git clone -b interview https://github.com/Open-Source-Community/qo2.git
    else
        echo -e "${GREEN}Directory 'qo2' already exists. Updating...${NC}"
        cd qo2
        git checkout interview 2>/dev/null || git checkout -b interview origin/interview
        git pull origin interview
        cd ..
    fi
    cd qo2
fi

# 4. Git Configuration Check
echo -e "${BLUE}[3/5] Checking Git configuration...${NC}"
if ! git config --global user.email >/dev/null; then
    echo -e "${YELLOW}Setting up default Git identity (OSC Recruit)...${NC}"
    git config --global user.email "recruit@osc.org"
    git config --global user.name "OSC Recruit"
    git config --global init.defaultBranch "master"
else
	echo -e "${GREEN}Git identity already configured.${NC}"
fi

# 5. Managing submodules
# echo -e "${BLUE}[4/5] Setting up submodules...${NC}"
# if [ ! -d "recruit-data" ]; then
#     echo -e "Adding Linux-25-Recruit submodule..."
#     git submodule add https://github.com/Open-Source-Community/Linux-25-Recruit.git recruit-data || true
# fi
# git submodule update --init --recursive
# echo -e "${GREEN}Submodules updated successfully.${NC}"

# 6. Build the Application
echo -e "${BLUE}[5/5] Building the application...${NC}"
if [ -f "main.go" ]; then
    go build -o qo main.go
    echo -e "${GREEN}Build successful! Binary created: ./qo${NC}"
else
    echo -e "${RED}Error: main.go not found. Build failed.${NC}"
    exit 1
fi

echo -e "\n${GREEN}=== Setup Complete ===${NC}"
echo -e "To start the quiz, run:"
if [ $(basename "$PWD") == "qo2" ]; then
    echo -e "  ${BLUE}sudo ./qo start${NC}\n"
else
    echo -e "  ${BLUE}cd qo2 && sudo ./qo start${NC}\n"
fi
