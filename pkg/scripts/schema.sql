BEGIN TRANSACTION;
DROP TABLE IF EXISTS "questions";
CREATE TABLE "questions" (
	"question_id"	INTEGER,
	"text"	TEXT NOT NULL,
	"topic"	TEXT,
	"difficulty"	INTEGER,
	"model_answer"	TEXT,
	"test_script"	TEXT,
	"setup_script"	TEXT,
	"cleanup_script"	TEXT,
	"source"	TEXT,
    "oneShot"   INTEGER,
	PRIMARY KEY("question_id")
);
DROP TABLE IF EXISTS "users";
CREATE TABLE users (
    user_id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE,
    phone TEXT,
    year INTEGER,
    oscian BOOLEAN DEFAULT false
);
DROP TABLE IF EXISTS "sessions";
CREATE TABLE sessions (
    session_id INTEGER PRIMARY KEY,
    user_id INTEGER,
    time DATETIME DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    score INTEGER,
    result TEXT,
    FOREIGN KEY (user_id) REFERENCES users (user_id)
);
DROP TABLE IF EXISTS "submissions";
CREATE TABLE submissions (
    submission_id INTEGER PRIMARY KEY,
    session_id INTEGER, -- Added this to link the answer to the session
    question_id INTEGER,
    answer TEXT,
    score INTEGER,
    result text,
    FOREIGN KEY (session_id) REFERENCES sessions (session_id),
    FOREIGN KEY (question_id) REFERENCES questions (question_id)
);

