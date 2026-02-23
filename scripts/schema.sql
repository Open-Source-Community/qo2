CREATE TABLE users (
    user_id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE,
    phone TEXT,
    year INTEGER,
    oscian BOOLEAN DEFAULT 0
);
CREATE TABLE sessions (
    session_id INTEGER PRIMARY KEY,
    user_id INTEGER,
    time DATETIME DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    score INTEGER,
    result TEXT,
    FOREIGN KEY (user_id) REFERENCES users (user_id)
);
CREATE TABLE session_questions (
    session_id INTEGER,
    question_id INTEGER,
    PRIMARY KEY (session_id, question_id),
    FOREIGN KEY (session_id) REFERENCES sessions (session_id),
    FOREIGN KEY (question_id) REFERENCES questions (question_id)
);
CREATE TABLE submissions (
    submission_id INTEGER PRIMARY KEY,
    session_id INTEGER, -- Added this to link the answer to the session
    question_id INTEGER,
    answer TEXT,
    score INTEGER,
    FOREIGN KEY (session_id) REFERENCES sessions (session_id),
    FOREIGN KEY (question_id) REFERENCES questions (question_id)
);
CREATE TABLE IF NOT EXISTS "questions" (
	"question_id"	INTEGER,
	"text"	TEXT NOT NULL,
	"topic"	TEXT,
	"difficulty"	INTEGER,
	"model_answer"	TEXT,
	"test_script"	TEXT,
	"setup_script"	TEXT,
	"cleanup_script"	TEXT,
	"source"	TEXT,
	PRIMARY KEY("question_id")
);
