#!/bin/bash

# Script to clear all user, session, and submission data while preserving questions.

DB_FILE="linux.db"

if [ ! -f "$DB_FILE" ]; then
    echo "Error: $DB_FILE not found."
    exit 1
fi

echo "Clearing historical data from $DB_FILE..."

sqlite3 "$DB_FILE" <<EOF
DELETE FROM submissions;
DELETE FROM sessions;
DELETE FROM users;
-- Reset auto-increment counters
DELETE FROM sqlite_sequence WHERE name IN ('submissions', 'sessions', 'users');
VACUUM;
EOF

if [ $? -eq 0 ]; then
    echo "Successfully cleared historical data. Questions preserved."
else
    echo "An error occurred while clearing data."
fi
