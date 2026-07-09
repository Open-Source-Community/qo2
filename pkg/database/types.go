package database

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ahmedYasserM/qo/pkg/sandbox"
)

type Client interface {
	Initialize() error
	SaveUserInfo(user *User) error

	GenerateQuestionSet(title string, instructions string) (*QuestionSet, error)
	SaveSession(*Session) error
	SubmitQuestion(session *Session, currentQuestion int) error
	Close()
}

type User struct {
	ID     int64
	Name   string
	Email  string
	Phone  string
	Year   int
	Oscian bool
}
type Question struct {
	//basic data
	ID         int64  `json:"question_id"`
	Text       string `json:"text"`
	Topic      string `json:"topic"`
	Difficulty int    `json:"difficulty"`
	OneShot    bool   `json:"oneShot"`

	// accompanying scripts
	TestScript    string `json:"test_script"`
	SetupScript   string `json:"setup_script"`
	CleanupScript string `json:"cleanup_script"`

	// non-database fields: response
	Attempted bool
	Answer    string
	Score     int
	Result    string
}
type QuestionSet struct {
	Title        string
	Instructions string
	Questions    []*Question
}
type Session struct {
	ID          int64 `json:"session_id"`
	User        *User
	Time        string `json:"time"`
	Notes       string `json:"notes"`
	Score       int    `json:"score"`
	Result      string `json:"result"`
	QuestionSet *QuestionSet
	Client      Client
	Sandbox     *sandbox.SandboxSession // one persistent sandbox for the whole session
}

func (q Question) String() string {
	return fmt.Sprintf(
		"[ID: %d\tDifficulty: %d] (%s)\nQuestion: %s",
		q.ID,
		q.Difficulty,
		q.Topic,
		q.Text,
	)
}

// attempt to run script in sandbox - usefull for debugging (read error)
func (q *Question) GradeWithSandbox(s *sandbox.SandboxSession) (string, error) {
	// 1. Run Setup
	if q.SetupScript != "" {
		setupOutput, errSetup := s.Run(q.SetupScript)
		if errSetup != nil || strings.Contains(setupOutput, "sh error:") {
			q.Score = 0
			q.Result = "error"
			_, errClean := s.Run(q.CleanupScript)
			return "", errors.Join(fmt.Errorf("setup failed: %w", errSetup), errClean)
		}
	}

	// 2. Run Student Answer
	output, errAns := s.Run(q.Answer)
	if errAns != nil || strings.Contains(output, "sh error:") {
		q.Score = 0
		q.Result = "fail"
		_, errClean := s.Run(q.CleanupScript)
		return output, errors.Join(fmt.Errorf("running answer failed: %w", errAns), errClean)
	}

	// 3. Run Test Script
	testOutput, errTest := s.Run(q.TestScript)

	// 4. Evaluate Results based on Test Output
	if errTest != nil || strings.Contains(testOutput, "sh error:") {
		q.Score = 0
		q.Result = "fail"
		if errTest != nil {
			errTest = fmt.Errorf("running test failed: %w", errTest)
		}
	} else {
		q.Score = 1
		q.Result = "pass"
	}

	// 5. Run Cleanup
	cleanOutput, errClean := s.Run(q.CleanupScript)
	if errClean != nil || strings.Contains(cleanOutput, "sh error:") {
		errClean = fmt.Errorf("cleanup failed: %w", errClean)
	}
	return output, errors.Join(errTest, errClean)
}

// temporary safety measure that should be handled once integrated run in qo sandbox is merged
const WORKDIR = "./.test"

type Result struct {
	note   string
	passed bool
}

func runScript(script string, workDir string) error {
	if script == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Dir = workDir
	// It's often helpful to capture combined output for debugging,
	// but for simple execution, just Run() is enough.
	return cmd.Run()
}

func (question *Question) Setup() error {
	return runScript(question.SetupScript, WORKDIR)
}

func (question *Question) Test() string {
	question.Setup()
	// comment this later; this will be handled by integrated run feature that zeyad is working on
	runScript(question.Answer, WORKDIR)
	// end comment block
	err := runScript(question.TestScript, WORKDIR)
	var note string
	if err != nil {
		note = fmt.Sprintf("Test failed: %s", err.Error())
		question.Score = 0
	} else {
		note = ""
		question.Score = 1
	}
	question.Cleanup()
	return note
}

func (question *Question) Cleanup() error {
	return runScript(question.CleanupScript, WORKDIR)

}
