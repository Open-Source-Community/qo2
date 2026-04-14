package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
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

type SelectionMode int

const (
	ModeByTopic SelectionMode = iota
	ModeGlobal
	ModeByDifficulty
)

type SessionConfig struct {
	Mode             SelectionMode
	TopicCounts      map[string]int
	GlobalCount      int
	GlobalTopics     []string
	DifficultyCounts map[int]int
}

type Session struct {
	id                int64
	user              *User
	time              string
	notes             string
	score             int
	result            string
	questionSet       *QuestionSet
	currentQuestion   int
	currentTopicIndex int
	currentDifficulty int
	db                *sql.DB
	sandbox           *sandbox.SandboxSession // one persistent sandbox for the whole session
	config            SessionConfig
	topicFetched      map[string]int
	difficultyFetched map[int]int
	globalFetched    int
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

	// Runtime migration: ensure submissions table has a result column
	_, _ = db.Exec("ALTER TABLE submissions ADD COLUMN result TEXT")

	return db, nil
}

func FetchTopicsWithCounts() (map[string]int, error) {
	db, err := initDB("linux.db")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT topic, count(*) FROM questions GROUP BY topic`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var topic string
		var count int
		if err := rows.Scan(&topic, &count); err != nil {
			return nil, err
		}
		counts[topic] = count
	}
	return counts, nil
}

func FetchDifficultyWithCounts() (map[int]int, error) {
	db, err := initDB("linux.db")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT difficulty, count(*) FROM questions GROUP BY difficulty`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[int]int)
	for rows.Next() {
		var diff, count int
		if err := rows.Scan(&diff, &count); err != nil {
			return nil, err
		}
		counts[diff] = count
	}
	return counts, nil
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

// Fetches a question batch of the given size depending on the session's current difficulty and topic.
// NOTE: this is currently used with batchSize=1. If this is eventually changed, make sure to handle
// the remaining questions in the batch. Do you want to immediately discard them and switch to
// the new topic/difficulty? Do you want to add  them to submissions even if they were
// discarded before they were displayed?
// I leave this chore to you, kind successor :D
func (session *Session) fetchQuestionBatch(batchSize int) ([]*Question, error) {
	stmt, err := session.db.Prepare(`SELECT question_id, text, topic, difficulty, model_answer, test_script, setup_script, cleanup_script, source, oneShot
							FROM questions q
							WHERE difficulty = ?
							AND topic = ?
							AND NOT EXISTS(
								SELECT 1
								FROM submissions s
								WHERE s.question_id = q.question_id
								AND s.session_id = ?)
							ORDER BY RANDOM()
							LIMIT ?`)
	if err != nil {
		return nil, err
	}
	currentTopic := session.GetCurrentTopic()

	rows, err := stmt.Query(session.currentDifficulty, currentTopic, session.id, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var (
		qs                                                                   []*Question
		_model_answer, _test_script, _setup_script, _cleanup_script, _source sql.NullString
		_oneShot                                                             sql.NullInt64
	)
	for rows.Next() {
		q := Question{}
		err = rows.Scan(
			&q.id,
			&q.text,
			&q.topic,
			&q.difficulty,
			&_model_answer,
			&_test_script,
			&_setup_script,
			&_cleanup_script,
			&_source,
			&_oneShot,
		)
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
		qs = append(qs, &q)
	}
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return nil, errors.New("empty questions")
	}
	return qs, nil
}

func initializeQuestionSet(title string, instructions string, db *sql.DB, config SessionConfig) (*QuestionSet, error) {
	var topics []string

	switch config.Mode {
	case ModeByTopic:
		for topic := range config.TopicCounts {
			topics = append(topics, topic)
		}
	case ModeGlobal:
		for _, topic := range config.GlobalTopics {
			topics = append(topics, topic)
		}
	case ModeByDifficulty:
		// topic doesn't matter for difficulty mode, but we need at least one to start the loop
		rows, err := db.Query(`SELECT DISTINCT topic FROM questions`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err == nil {
				topics = append(topics, t)
			}
		}
	}

	if len(topics) == 0 {
		// fallback to random topics if none selected (backward compatibility or empty config)
		rows, err := db.Query(`SELECT DISTINCT topic FROM questions ORDER BY RANDOM()`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var topic sql.NullString
			if err := rows.Scan(&topic); err == nil && topic.Valid {
				topics = append(topics, topic.String)
			}
		}
	}

	SortTopics(topics)
	return &QuestionSet{title: title, instructions: instructions, topics: topics}, nil

}

func InitializeSession(user *User, config SessionConfig) (*Session, error) {
	db, err := initDB("linux.db")
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to db:%s", err))
	}
	if err := saveUserInfo(user, db); err != nil {
		return nil, fmt.Errorf("saving user: %w", err)
	}
	qs, err := initializeQuestionSet("Interview", "", db, config)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("generating question set: %w", err)
	}

	session := &Session{time: time.Now().Local().Format(time.RFC3339),
		user:         user,
		questionSet:  qs,
		db:           db,
		currentQuestion: -1,
		currentTopicIndex: 0,
		currentDifficulty: 0,
		config:       config,
		topicFetched: make(map[string]int),
		difficultyFetched: make(map[int]int),
	}

	session.AdvanceQuestion() // smoothly loop to find first valid topic/difficulty
	if len(session.questionSet.questions) == 0 {
		db.Close()
		return nil, fmt.Errorf("generating question set: no questions found in database")
	}

	//insert session metadata
	sessionStmt, err := db.Prepare("INSERT INTO sessions(user_id, time, notes, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		panic("Failed to initialize session")
	}
	result, err := sessionStmt.Exec(session.user.user_id, session.time, session.notes, session.score, session.result)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize session: %w", err)

	}
	session_id, _ := result.LastInsertId()
	session.id = session_id

	// Check if any topic requires root access
	needsRoot := false
	for _, topic := range session.questionSet.topics {
		// Use a more inclusive check for the user management topic
		if topic == "User & Group Management" || topic == "Users & Groups" {
			needsRoot = true
			break
		}
	}

	if needsRoot {
		os.Setenv("SANDBOX_USER", "root")
	} else {
		os.Unsetenv("SANDBOX_USER")
	}

	sbSession, err := sandbox.NewSession()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}
	session.sandbox = sbSession

	return session, nil
}

func (session *Session) IncreaseDifficulty(reverse bool) error {
	if reverse {
		if session.currentDifficulty > 0 {
			session.currentDifficulty--
		} else {
			return errors.New("Minimum difficulty level!")

		}
	} else {
		if session.currentDifficulty < 3 { // one less than max bc well increment it
			session.currentDifficulty++
		} else {
			return errors.New("maximum difficulty level!")

		}
	}

	session.result = fmt.Sprintf("%d/%d", session.score, len(session.questionSet.questions))

	updateStmt, err := session.db.Prepare(`UPDATE sessions SET score = ?, result = ? WHERE session_id = ?;`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	_, err = updateStmt.Exec(session.score, session.result, session.id)
	return err
}

func (session *Session) AdvanceTopic(reverse bool) error {
	if reverse {
		if session.currentTopicIndex > 0 {
			session.currentTopicIndex--
			session.currentDifficulty = 0
		} else {
			return errors.New("No previous topics!")
		}
	} else {
		if session.currentTopicIndex < len(session.questionSet.topics)-1 {
			session.currentTopicIndex++
			session.currentDifficulty = 0

		} else {
			return errors.New("No more topics!")

		}
	}
	return nil
}

func (session *Session) GetCurrentQuestion() (*Question, error) {
	if session.currentQuestion == -1 {
		return nil, errors.New("No more questions!")

	} else if session.currentQuestion < len(session.questionSet.questions) {
		return session.questionSet.questions[session.currentQuestion], nil
	} else {
		return nil, errors.New("BUG: Current index exceeds length of existing questions")
	}

}

func (session *Session) GetCurrentTopic() string {
	return session.questionSet.topics[session.currentTopicIndex]
}

func (session *Session) GetCurrentDifficulty() string {
	switch session.currentDifficulty {
	case 1:
		return "level 1"
	case 2:
		return "level 2"
	case 3:
		return "level 3"
	default:
		return "unknown"
	}

}

func (session *Session) AdvanceQuestion() {
	// happy case, there are questions already
	if session.currentQuestion < len(session.questionSet.questions)-1 {
		session.currentQuestion++
		return
	}

	for {
		switch session.config.Mode {
		case ModeByTopic:
			currentTopic := session.GetCurrentTopic()
			targetTopicCount := session.config.TopicCounts[currentTopic]
			if targetTopicCount > 0 && session.topicFetched[currentTopic] >= targetTopicCount {
				if session.AdvanceTopic(false) == nil {
					continue
				} else {
					goto Done
				}
			}
		case ModeGlobal:
			if session.config.GlobalCount > 0 && session.globalFetched >= session.config.GlobalCount {
				goto Done
			}
		case ModeByDifficulty:
			// In difficulty mode, difficulty is our primary driver.
			// AdvanceQuestion will loop through topics just to find questions of the current difficulty.
			targetDiffCount := session.config.DifficultyCounts[session.currentDifficulty]
			if targetDiffCount > 0 && session.difficultyFetched[session.currentDifficulty] >= targetDiffCount {
				// difficulty met? try increasing difficulty
				if session.currentDifficulty < 3 {
					session.currentDifficulty++
					continue
				} else {
					goto Done
				}
			}
		}

		q, err := session.fetchQuestionBatch(1)
		if err == nil {
			session.questionSet.questions = append(session.questionSet.questions, q...)
			session.currentQuestion++
			session.topicFetched[q[0].topic]++
			session.difficultyFetched[q[0].difficulty]++
			session.globalFetched++
			return
		}

		// fetch failed? try next combo
		if session.config.Mode == ModeByDifficulty {
			// for difficulty mode, just try next topic for the SAME difficulty
			if session.AdvanceTopic(false) == nil {
				continue
			} else {
				// topic exhausted? try next difficulty if possible
				if session.currentDifficulty < 3 {
					session.currentDifficulty++
					session.currentTopicIndex = 0
					continue
				} else {
					goto Done
				}
			}
		} else {
			// standard logic
			if session.IncreaseDifficulty(false) == nil {
				continue
			}
			if session.AdvanceTopic(false) == nil {
				continue
			} else {
				goto Done
			}
		}
	}

Done:
	session.currentQuestion = -1
	session.Finalize()
}

func (session *Session) Finalize() error {

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

func (session *Session) SubmitAnswer(answer string, result string) error {
	q, _ := session.GetCurrentQuestion()
	q.answer = answer
	q.result = result
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
