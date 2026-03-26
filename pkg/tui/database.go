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
	user    *User
	time       string
	notes      string
	score      string
	result     string
	questionsSet  *QuestionSet
	db				*sql.DB
}

type Question struct {
	// basic data
	id int
	text           string
	topic          string
	difficulty     int
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

func generateQuestionSet(title string, instructions string, db *sql.DB) (*QuestionSet, error) {
	// for each topic and difficulty, select one question
	// thus, 3 questions for each topic: easy (1), medium (2) and hard (3)
	rows, err := db.Query(`SELECT question_id, text, topic, difficulty, model_answer, test_script, setup_script, cleanup_script, source
							FROM (
								SELECT *, ROW_NUMBER() OVER(PARTITION BY topic, difficulty ORDER BY RANDOM()) as rn
								FROM questions
								WHERE difficulty IN ('1', '2', '3')
							)
							WHERE rn = 1;`)
	if err != nil {
		return nil, err
	}
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
		print(q.String())
		qs = append(qs, q)

	}
	return &QuestionSet{title: title, instructions: instructions, questions: qs}, nil

}

func InitializeSession(user *User) *Session {
	db, err := initDB("linux.db")
	if err != nil {
		panic(err)
	}
	saveUserInfo(user,db)

	qs, err := generateQuestionSet("Interview", "", db)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate question set: %s", err))
	}
	session := &Session{time: time.Now().Local().Format(time.RFC3339),
		user:   user,
		questionsSet: qs, db: db,}
	return session
}

func (session Session) SaveSession() error {
	//insert session metadata
	db:= session.db
	sessionStmt, err:= db.Prepare("INSERT INTO sessions(user_id, time, notes, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	result, err:= sessionStmt.Exec(session.user.user_id, session.time, session.notes, session.score, session.result)
		if err != nil {
		return err
	}
	session_id, err:= result.LastInsertId()
	if err != nil {
		return err
	}
	//insert submissions
	submissionStmt, err:= db.Prepare("INSERT INTO submissions(session_id, question_id, answer, score) values(?, ?, ?, ?)")
		if err != nil {
		return err
	}
	for _,question:=range(session.questionsSet.questions){
		_, err= submissionStmt.Exec(session_id, question.id, question.answer,question.score)
		if err != nil {
			return err
		}
	}
	return nil

}

func CloseDB(db *sql.DB){
	db.Close()
}