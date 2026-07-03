package tui

import (
	"database/sql"
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
	id           int64
	text         string
	topic        string
	difficulty   int
	model_answer string
	oneShot      bool
	attempted    bool

	// accompanying scripts
	test_script    string
	setup_script   string
	cleanup_script string
	source         string

	// non-database fields: response
	answer        string
	score         int
	result        string
	sandboxOutput string // stdout captured from sandbox execution

}

func (q Question) String() string {
	return fmt.Sprintf(
		"[ID: %d\tDifficulty: %d] (%s)\nQuestion: %s\nModel Answer: %s\nSource: %s",
		q.id,
		q.difficulty,
		q.topic,
		q.text,
		q.model_answer,
		q.source,
	)
}

// gradeWithModelAnswer is the fallback grader when no test_script is present.
func (q *Question) gradeWithModelAnswer() {
	userAns := strings.TrimSpace(q.answer)
	modelAns := strings.TrimSpace(q.model_answer)
	if strings.EqualFold(userAns, modelAns) {
		q.score = 1
	} else {
		q.score = 0
	}
}

// attempt to run script in sandbox - usefull for debugging (read error)
func (q *Question) gradeWithSandbox(s *sandbox.SandboxSession) error {
	if s == nil || q.test_script == "" {
		q.gradeWithModelAnswer()
		return nil
	}

	// run setup script if present
	if q.setup_script != "" {
		if _, err := s.Run(q.setup_script); err != nil {
			return fmt.Errorf("setup_script failed: %w", err)
		}
	}

	// write the candidate's answer as a shell command, then run the test
	if q.answer != "" {
		if _, err := s.Run(q.answer); err != nil {
			// answer failed to execute — still run the test to get a score
		}
	}

	output, err := s.Run(q.test_script)
	if err != nil {
		return fmt.Errorf("test_script failed: %w", err)
	}
	q.sandboxOutput = output

	if strings.Contains(output, "PASS") {
		q.score = 1
		q.result = "pass"
	} else {
		q.score = 0
		q.result = "fail"
	}

	// run cleanup script regardless of result
	if q.cleanup_script != "" {
		_, _ = s.Run(q.cleanup_script)
	}

	return nil
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
	rows, err := db.Query(`SELECT question_id, text, topic, difficulty, model_answer, test_script, setup_script, cleanup_script, source, oneShot
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
		qs                                                                   []*Question
		q                                                                    *Question
		_model_answer, _test_script, _setup_script, _cleanup_script, _source sql.NullString
		_oneShot                                                             sql.NullInt64
	)
	for rows.Next() {
		q = &Question{}
		err = rows.Scan(
			&(q.id),
			&(q.text),
			&(q.topic),
			&(q.difficulty),
			&_model_answer,
			&_test_script,
			&_setup_script,
			&_cleanup_script,
			&_source,
			&_oneShot,
		)
		if err != nil {
			return nil, err
		}
		if _model_answer.Valid {
			q.model_answer = _model_answer.String
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
		if _source.Valid {
			q.source = _source.String
		}
		if _oneShot.Valid {
			q.oneShot = _oneShot.Int64 == 1
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

func (session *Session) GradeCurrentQuestion(index int) error {
	if index < 0 || index >= len(session.questionSet.questions) {
		return fmt.Errorf("question index %d out of range", index)
	}
	q := session.questionSet.questions[index]
	return q.gradeWithSandbox(session.sandbox)
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

func (session *Session) SubmitAnswer(currentQuestion int, answer string) error {
	q := session.questionSet.questions[currentQuestion]
	q.answer = answer
	session.GradeCurrentQuestion(currentQuestion)

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
