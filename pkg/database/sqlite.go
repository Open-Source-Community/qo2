package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ahmedYasserM/qo/pkg/sandbox"
	_ "modernc.org/sqlite"
)

type LocalClient struct {
	DnsURI string
	db     *sql.DB
}

func (client *LocalClient) Initialize() error {
	db, err := sql.Open("sqlite", client.DnsURI)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to db:%s", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("Failed to connect to db:%s", err))
	}
	client.db = db

	return nil
}

func (client *LocalClient) SaveUserInfo(user *User) error {
	// check if student id exists on system
	// this would be good to do within the form
	// var id int
	// if row := db.QueryRow("SELECT * FROM users WHERE user_id =?").Scan(&id); row != sql.ErrNoRows {
	// 	// autofill the data in the text box or something
	// and if edited delete and reinsert, could use on duplicate key but dont want this automation
	// }
	stmt, err := client.db.Prepare("INSERT INTO users(name, email, phone, year, oscian) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	result, err := stmt.Exec(user.Name, user.Email, user.Phone, user.Year, user.Oscian)
	if err != nil {
		return err
	}
	user.ID, err = result.LastInsertId()
	return err
}

func (client *LocalClient) GenerateQuestionSet(title string, instructions string) (*QuestionSet, error) {
	// for each topic and difficulty, select one question
	// thus, 3 questions for each topic: easy (1), medium (2) and hard (3)
	rows, err := client.db.Query(`SELECT question_id, text, topic, difficulty, test_script, setup_script, cleanup_script, oneShot
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
			&(q.ID),
			&(q.Text),
			&(q.Topic),
			&(q.Difficulty),
			&_test_script,
			&_setup_script,
			&_cleanup_script,
			&_oneShot,
		)
		if err != nil {
			return nil, err
		}
		if _test_script.Valid {
			q.TestScript = _test_script.String
		}
		if _setup_script.Valid {
			q.SetupScript = _setup_script.String
		}
		if _cleanup_script.Valid {
			q.CleanupScript = _cleanup_script.String
		}
		if _oneShot.Valid {
			q.OneShot = _oneShot.Bool
		}
		qs = append(qs, q)

	}
	return &QuestionSet{Title: title, Instructions: instructions, Questions: qs}, nil
}

func (client *LocalClient) InitializeSession(user *User) (*Session, error) {
	err := client.Initialize()
	if err != nil {
		return nil, fmt.Errorf("initializing db: %w", err)
	}
	if err := client.SaveUserInfo(user); err != nil {
		return nil, fmt.Errorf("saving user: %w", err)
	}

	qs, err := client.GenerateQuestionSet("Interview", "")
	if err != nil {
		client.db.Close()
		return nil, fmt.Errorf("generating question set: %w", err)
	}

	stmt, err := client.db.Prepare("INSERT INTO sessions(user_id, time, notes, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	result, err := stmt.Exec(user.ID, time.Now(), nil, 0, nil)
	if err != nil {
		return nil, err
	}
	session_id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	sbSession, err := sandbox.NewSession()
	if err != nil {
		client.db.Close()
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	return &Session{
		ID:          session_id,
		Time:        time.Now().Local().Format(time.RFC3339),
		User:        user,
		QuestionSet: qs,
		Client:      client,
		Sandbox:     sbSession,
	}, nil
}

// SaveSession persists the session results and closes the sandbox.
func (client *LocalClient) SaveSession(session *Session) error {
	// close sandbox first — done executing commands
	if session.Sandbox != nil {
		_ = session.Sandbox.Close()
		session.Sandbox = nil
	}
	defer client.Close()
	totalScore := 0
	for _, q := range session.QuestionSet.Questions {
		totalScore += q.Score
	}
	session.Score = totalScore
	session.Result = fmt.Sprintf("%d/%d", session.Score, len(session.QuestionSet.Questions))
	stmt, err := client.db.Prepare(`UPDATE sessions 
									SET score = ?, result = ?
									WHERE session_id = ?;`)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(session.Score, session.Result, session.ID)
	if err != nil {
		return err
	}
	return nil
}

func (client *LocalClient) SubmitQuestion(session *Session, currentQuestion int) error {
	q := session.QuestionSet.Questions[currentQuestion]
	submissionStmt, err := client.db.Prepare("INSERT INTO submissions(session_id, question_id, answer, score, result) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer submissionStmt.Close()
	_, err = submissionStmt.Exec(session.ID, q.ID, q.Answer, q.Score, q.Result)
	return err
}

func (client *LocalClient) Close() {
	client.db.Close()
}
