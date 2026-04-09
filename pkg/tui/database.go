package tui

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type User struct {
	user_id int64
	name    string
	email   string
	phone   string
	year    int
	oscian  bool
}

type Session struct {
	id int64
	user    *User
	time       string
	notes      string
	score      string
	result     string
	questionSet  *QuestionSet
	currentQuestion int
	currentTopicIndex int
	currentDifficulty int
	db				*sql.DB
}

type Question struct {
	// basic data
	id int
	text           string
	topic          string
	difficulty     int // starting from zero, this makes the default field initialization (0) work fine
	model_answer         string

	// accompanying scripts
	test_script    string
	setup_script   string
	cleanup_script string
	source         string

	// non-database fields: response
	answer string
	score int
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

func initDB(dsnURI string) (*sql.DB, error) {
	var err error
	db, err := sql.Open("sqlite", dsnURI)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
    	return nil, err
    }
	return db, err
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
	if err != nil {
		return err
	}
	return nil
}

// fetches a batch of the given size depending on the session's current difficulty and topic
func (session *Session) fetchQuestionBatch(batchSize int) ([]Question, error){
	stmt, err := session.db.Prepare(`SELECT question_id, text, topic, difficulty, model_answer, test_script, setup_script, cleanup_script, source
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
	currentTopic:=session.questionSet.topics[session.currentTopicIndex]

	rows, err := stmt.Query(session.currentDifficulty, currentTopic, session.id, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var (
		qs                                                             []Question
		q                                                              Question
		_model_answer, _test_script, _setup_script, _cleanup_script, _source sql.NullString
	)
	for rows.Next() {
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
		qs = append(qs, q)
	}
	return qs, nil
}



func initializeQuestionSet(title string, instructions string, db *sql.DB) (*QuestionSet, error) {
	//fetch topics
	var (
		topics []string
		topic sql.NullString)

	rows, err := db.Query(`SELECT DISTINCT topic FROM questions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&topic); err != nil {
			return nil, err
		}
		if topic.Valid {
            topics = append(topics, topic.String)
        }

	}
	if err = rows.Err(); err != nil {
        return nil, err
    }

	return &QuestionSet{title: title, instructions: instructions, topics: topics}, nil

}



func InitializeSession(user *User) *Session {
	db, err := initDB("linux.db")
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to db:%s", err))
	}
	saveUserInfo(user,db)

	qs, err := initializeQuestionSet("Interview", "", db)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize question set: %s", err))
	}

	session := &Session{time: time.Now().Local().Format(time.RFC3339),
		user:   user,
		questionSet: qs, db: db, currentQuestion: -1, currentTopicIndex: 0, currentDifficulty: 0,}

	q, err:= session.fetchQuestionBatch(3)
	if err != nil {
		panic(err)
	}else if len(q)==0{
		panic("Failed to fetch any questions!")
	}
	session.currentQuestion=0
	session.questionSet.questions=q
		//insert session metadata
	sessionStmt, err:= db.Prepare("INSERT INTO sessions(user_id, time, notes, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		panic("Failed to initialize session")
	}
	result, err:= sessionStmt.Exec(session.user.user_id, session.time, session.notes, session.score, session.result)
		if err != nil {
			panic("Failed to initialize session")

	}
	session_id, _:= result.LastInsertId()

	session.id=session_id
	return session
}

func (session *Session) IncreaseDifficulty(reverse bool){
	if reverse{
		if session.currentDifficulty > 0{
			session.currentDifficulty--
		}
	}else
	{
		if session.currentDifficulty < 2{
			session.currentDifficulty++
		}
	}
}

func (session *Session) AdvanceTopic(reverse bool){
	if reverse{
		if session.currentTopicIndex > 0{
			session.currentTopicIndex--
		}
	}else
	{
		if session.currentTopicIndex < len(session.questionSet.topics){
			session.currentTopicIndex++
		}
	}
}

func (session *Session) SaveSession() error {
	//insert submissions
	submissionStmt, err:= session.db.Prepare("INSERT INTO submissions(session_id, question_id, answer, score) values(?, ?, ?, ?)")
		if err != nil {
		return err
	}
	for _,question:=range(session.questionSet.questions){
		_, err= submissionStmt.Exec(session.id, question.id, question.answer,question.score)
		if err != nil {
			return err
		}
	}
	return nil

}

func CloseDB(db *sql.DB){
	db.Close()
}