package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ahmedYasserM/qo/pkg/sandbox"
	_ "modernc.org/sqlite"
)

var PreferredTopicOrder = []string{
	"Linux",
	"Archiving & Compression",
	"File System Navigation",
	"Text processing",
	"User & Group Management",
	"Permissions",
	"Pipelining & Redirection",
	"Git & GitHub",
}

func SortTopics(topics []string) {
	orderMap := make(map[string]int)
	for i, topic := range PreferredTopicOrder {
		orderMap[topic] = i
	}

	sort.Slice(topics, func(i, j int) bool {
		idxI, okI := orderMap[topics[i]]
		idxJ, okJ := orderMap[topics[j]]

		if okI && okJ {
			return idxI < idxJ
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return topics[i] < topics[j]
	})
}

type User struct {
	user_id int64
	name    string
	email   string
	phone   string
	year    int
	oscian  bool
}

type Session struct {
	id          int64
	user        *User
	time        string
	notes       string
	score       int
	result      string
	questionSet *QuestionSet
	db          *sql.DB
	sandbox     *sandbox.SandboxSession // one persistent sandbox for the whole session

}

type Question struct {
	// basic data
	id         int64
	text       string
	topic      string
	difficulty int
	oneShot    bool

	// accompanying scripts
	test_script    string
	setup_script   string
	cleanup_script string

	// non-database fields: response
	attempted bool
	answer    string
	score     int
	result    string
}

func (q Question) String() string {
	return fmt.Sprintf(
		"[ID: %d\tDifficulty: %d] (%s)\nQuestion: %s",
		q.id,
		q.difficulty,
		q.topic,
		q.text,
	)
}

// attempt to run script in sandbox - usefull for debugging (read error)
func (q *Question) gradeWithSandbox(s *sandbox.SandboxSession) (string, error) {
	// 1. Run Setup
	if q.setup_script != "" {
		setupOutput, errSetup := s.Run(q.setup_script)
		if errSetup != nil || strings.Contains(setupOutput, "sh error:") {
			q.score = 0
			q.result = "error"
			_, errClean := s.Run(q.cleanup_script)
			return "", errors.Join(fmt.Errorf("setup failed: %w", errSetup), errClean)
		}
	}

	// 2. Run Student Answer
	output, errAns := s.Run(q.answer)
	if errAns != nil || strings.Contains(output, "sh error:") {
		q.score = 0
		q.result = "fail"
		_, errClean := s.Run(q.cleanup_script)
		return output, errors.Join(fmt.Errorf("running answer failed: %w", errAns), errClean)
	}

	// 3. Run Test Script
	testOutput, errTest := s.Run(q.test_script)

	// 4. Evaluate Results based on Test Output
	if errTest != nil || strings.Contains(testOutput, "sh error:") {
		q.score = 0
		q.result = "fail"
		if errTest != nil {
			errTest = fmt.Errorf("running test failed: %w", errTest)
		}
	} else {
		q.score = 1
		q.result = "pass"
	}

	// 5. Run Cleanup
	cleanOutput, errClean := s.Run(q.cleanup_script)
	if errClean != nil || strings.Contains(cleanOutput, "sh error:") {
		errClean = fmt.Errorf("cleanup failed: %w", errClean)
	}
	return output, errors.Join(errTest, errClean)
}

func initDB(dsnURI string) (*sql.DB, error) {

	db, err := sql.Open("sqlite", dsnURI)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}
func saveUserInfo(user *User, db *sql.DB) error {
	// check if student id exists on system
	// this would be good to do within the form
	// var id int
	// if row := db.QueryRow("SELECT * FROM users WHERE user_id =?").Scan(&id); row != sql.ErrNoRows {
	// 	// autofill the data in the text box or something
	// and if edited delete and reinsert, could use on duplicate key but dont want this automation
	// }
	stmt, err := db.Prepare("INSERT INTO users(name, email, phone, year, oscian) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	result, err := stmt.Exec(user.name, user.email, user.phone, user.year, user.oscian)
	if err != nil {
		return err
	}
	user.user_id, err = result.LastInsertId()
	return err
}

func generateQuestionSet(title string, instructions string, db *sql.DB) (*QuestionSet, error) {
	// for each topic and difficulty, select one question
	// thus, 3 questions for each topic: easy (1), medium (2) and hard (3)
	rows, err := db.Query(`SELECT question_id, text, topic, difficulty, test_script, setup_script, cleanup_script, oneShot
							FROM (
								SELECT *, ROW_NUMBER() OVER(PARTITION BY topic, difficulty ORDER BY RANDOM()) as rn
								FROM questions
								WHERE difficulty IN ('1', '2', '3')
							)
							WHERE rn = 1;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		qs                                           []*Question
		q                                            *Question
		_test_script, _setup_script, _cleanup_script sql.NullString
		_oneShot                                     sql.NullBool
	)
	for rows.Next() {
		q = &Question{}
		err = rows.Scan(
			&(q.id),
			&(q.text),
			&(q.topic),
			&(q.difficulty),
			&_test_script,
			&_setup_script,
			&_cleanup_script,
			&_oneShot,
		)
		if err != nil {
			return nil, err
		}
		if _test_script.Valid {
			q.test_script = _test_script.String
		}
		if _setup_script.Valid {
			q.setup_script = _setup_script.String
		}
		if _cleanup_script.Valid {
			q.cleanup_script = _cleanup_script.String
		}
		if _oneShot.Valid {
			q.oneShot = _oneShot.Bool
		}
		qs = append(qs, q)

	}
	return &QuestionSet{title: title, instructions: instructions, questions: qs}, nil
}

func InitializeSession(user *User) (*Session, error) {
	db, err := initDB("linux.db")
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to db:%s", err))
	}
	if err := saveUserInfo(user, db); err != nil {
		return nil, fmt.Errorf("saving user: %w", err)
	}

	qs, err := generateQuestionSet("Interview", "", db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("generating question set: %w", err)
	}

	stmt, err := db.Prepare("INSERT INTO sessions(user_id, time, notes, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	result, err := stmt.Exec(user.user_id, time.Now(), nil, 0, nil)
	if err != nil {
		return nil, err
	}
	session_id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	sbSession, err := sandbox.NewSession()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	return &Session{
		id:          session_id,
		time:        time.Now().Local().Format(time.RFC3339),
		user:        user,
		questionSet: qs,
		db:          db,
		sandbox:     sbSession,
	}, nil
}

// SaveSession persists the session results and closes the sandbox.
func (session *Session) SaveSession() error {
	// close sandbox first — done executing commands
	if session.sandbox != nil {
		_ = session.sandbox.Close()
		session.sandbox = nil
	}
	defer CloseDB(session.db)
	totalScore := 0
	for _, q := range session.questionSet.questions {
		totalScore += q.score
	}
	session.score = totalScore
	session.result = fmt.Sprintf("%d/%d", session.score, len(session.questionSet.questions))
	stmt, err := session.db.Prepare(`UPDATE sessions 
									SET score = ?, result = ?
									WHERE session_id = ?;`)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(session.score, session.result, session.id)
	if err != nil {
		return err
	}
	return nil
}

func (session *Session) SubmitQuestion(currentQuestion int) error {
	q := session.questionSet.questions[currentQuestion]
	submissionStmt, err := session.db.Prepare("INSERT INTO submissions(session_id, question_id, answer, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer submissionStmt.Close()
	_, err = submissionStmt.Exec(session.id, q.id, q.answer, q.score, q.result)
	if err != nil {
		return err
	}

	// Incremental update for session score ensure sessions table always has latest progress
	session.score += q.score
	session.result = fmt.Sprintf("%d/%d", session.score, len(session.questionSet.questions))

	updateStmt, err := session.db.Prepare(`UPDATE sessions SET score = ?, result = ? WHERE session_id = ?;`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	_, err = updateStmt.Exec(session.score, session.result, session.id)
	return err
}

func CloseDB(db *sql.DB) {
	db.Close()
}
