package tui

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	user_id int
	name    string
	email   string
	phone   string
	year    int
	oscian  bool
}

type Session struct {
	session_id int
	user_id    int
	time       string
	notes      string
	score      string
	result     string
	questions  []Question
}

type Question struct {
	text           string
	topic          string
	difficulty     int
	answer         string
	test_script    string
	setup_script   string
	cleanup_script string
	source         string
}

var (
	db *sql.DB
)

func initDB(dsnURI string) error {

	dbconn, err := sql.Open("sqlite", dsnURI)
	defer db.Close()
	if err != nil {
		return err
	}
	db = dbconn
	return nil
}

func saveUserInfo(user *User) error {
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
	stmt.Exec(user.name, user.email, user.phone, user.year, user.oscian)
	stmt.Close()
	return nil
}
func generateQuestionSet(title string, instructions string) (*QuestionSet, error) {
	// for each topic and difficulty, select one question
	// thus, 3 questions for each topic: easy (1), medium (2) and hard (3)
	rows, err := db.Query(`SELECT question_id, text, topic, difficulty, model_answer, test_script, setup_script, cleanup_script, source
							FROM (
								SELECT *, ROW_NUMBER() OVER(PARTITION BY topic, difficulty ORDER BY RANDOM()) as rn
								FROM questions
								WHERE difficulty IN ('easy', 'medium', 'hard')
							)
							WHERE rn = 1;`)
	if err != nil {
		return nil, err
	}
	var (
		qs                                                             []Question
		q                                                              Question
		qid                                                            int
		_answer, _test_script, _setup_script, _cleanup_script, _source sql.NullString
	)
	for rows.Next() {
		err = rows.Scan(
			&qid,
			&q.text,
			&q.topic,
			&q.difficulty,
			&_answer,
			&_test_script,
			&_setup_script,
			&_cleanup_script,
			&_source,
		)

		if _answer.Valid {
			q.answer = _answer.String
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
	return &QuestionSet{title: title, instructions: instructions, questions: qs}, nil

}

func InitializeSession(user *User) *Session {
	initDB("linux.db")
	saveUserInfo(user)

	qs, err := generateQuestionSet("Interview", "")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate question set: %s", err))
	}
	session := &Session{time: time.Now().Local().Format("DateTime"),
		user_id:   user.user_id,
		questions: qs.questions}
	return session
}

func SaveSession() {

}
