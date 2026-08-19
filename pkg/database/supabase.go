package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmedYasserM/qo/pkg/logger"
	"github.com/ahmedYasserM/qo/pkg/sandbox"
	"github.com/google/uuid"
	"github.com/supabase-community/gotrue-go/types"
	supabase "github.com/supabase-community/supabase-go"
)

const (
	API_URL     string = "https://rtfamwipfqysagxwbtjd.supabase.co"
	API_KEY     string = "sb_publishable_qDI7vzI16KtLBD1fHA-X1A_trhf4blM"
	QONGIF_FILE string = "qonfig.json"
)

// SubmissionTable selects where level submissions are recorded. Empty means
// auto-detect from the level key prefix ("test1".."test10" -> test_submissions,
// anything else -> submissions). cmd/start.go sets it explicitly via -m/--mode
// so the instructor chooses the destination table up front.
var SubmissionTable string

// SessionSource tags session-log entries (eval|test). Set by -m/--mode.
var SessionSource = "eval"

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "qo")
}

type SupabaseClient struct {
	sb           *supabase.Client
	userID       uuid.UUID
	existingUser bool
}

func (client *SupabaseClient) Initialize() error {
	sb, err := supabase.NewClient(
		API_URL,
		API_KEY,
		&supabase.ClientOptions{},
	)
	if err != nil {
		log.Fatalf("Failed to init client: %v", err)
	}

	configDir := getConfigDir()
	sessionBytes, err := os.ReadFile(filepath.Join(configDir, QONGIF_FILE))
	session := types.Session{}
	if err == nil {
		err = json.Unmarshal(sessionBytes, &session)
		if err == nil {
			sb.UpdateAuthSession(session)
			sb.EnableTokenAutoRefresh(session)
			client.userID = session.User.ID
		}

	}
	if _, count, err := sb.From("users").Select("*", "exact", false).Execute(); err == nil && count > 0 {
		client.existingUser = true
	} else {

		// Anonymous sign-in = Signup with no email/phone/password.
		resp, err := sb.Auth.Signup(types.SignupRequest{})
		if err != nil {
			log.Fatalf("Failed to sign up: %v", err)
		}
		sessionBytes, _ := json.Marshal(resp.Session)
		err = os.MkdirAll(configDir, 0o755)
		if err != nil {
			log.Fatalf("Failed to create config dir: %v", err)
		}
		err = os.WriteFile(filepath.Join(configDir, QONGIF_FILE), sessionBytes, 0o600)
		if err != nil {
			log.Fatalf("Failed to create config file: %v", err)

		}
		// fmt.Printf("Anonymous user ID: %s\n", resp.User.ID)
		// fmt.Printf("Access token: %s\n", resp.Session.AccessToken)
		// fmt.Printf("Refresh token: %s\n", resp.Session.RefreshToken)
		client.userID = resp.User.ID
		sb.UpdateAuthSession(resp.Session)
		sb.EnableTokenAutoRefresh(resp.Session)

		client.existingUser = false

	}
	client.sb = sb
	return nil
}
func (client *SupabaseClient) initializeSessionRow(session *Session) error {
	var res []Session
	_, err := client.sb.From("sessions").Insert(map[string]interface{}{
		"user_id": client.userID, "time": session.Time, "notes": session.Notes,
		"score": session.Score, "result": session.Result,
	}, true, "session_id", "representation", "").ExecuteTo(&res)
	if err != nil {
		return err
	}
	session.ID = res[0].ID
	return nil

}

func (client *SupabaseClient) updateSessionScore(session *Session) error {
	_, _, err := client.sb.From("sessions").Update(map[string]interface{}{
		"notes": session.Notes, "score": session.Score, "result": session.Result,
	}, "minimal", "").Eq("session_id", fmt.Sprintf("%d", session.ID)).Execute()
	if err != nil {
		return err
	}
	return nil

}

func (client *SupabaseClient) SaveUserInfo(user *User) error {
	if client.existingUser {
		return nil // should not even take info in the tui
	}
	_, _, err := client.sb.From("users").Insert(map[string]interface{}{
		"user_id": client.userID,
		"name":    user.Name, "email": user.Email, "phone": user.Phone,
		"year": user.Year, "oscian": user.Oscian,
	}, false, "", "minimal", "").Execute()
	if err != nil {
		return err
	}
	return nil
}

func (client *SupabaseClient) GenerateQuestionSet(title string, instructions string) (*QuestionSet, error) {
	var questions []*Question
	_, err := client.sb.From("questions").Select("*", "", false).ExecuteTo(&questions)
	if err != nil {
		return nil, err
	}
	return &QuestionSet{Title: title, Instructions: instructions, Questions: questions}, nil
}

func (client *SupabaseClient) InitializeSession(user *User) (*Session, error) {

	err := client.Initialize()
	if err != nil {
		return nil, fmt.Errorf("initializing db: %w", err)
	}
	if err := client.SaveUserInfo(user); err != nil {
		return nil, fmt.Errorf("saving user: %w", err)
	}

	qs, err := client.GenerateQuestionSet("Interview", "")
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("generating question set: %w", err)
	}
	sbSession, err := sandbox.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	session := &Session{
		ID:          0,
		Time:        time.Now().Local().Format(time.RFC3339),
		User:        user,
		QuestionSet: qs,
		Client:      client,
		Sandbox:     sbSession,
	}

	err = client.initializeSessionRow(session)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("initializing session entry: %w", err)
	}
	return session, nil

}

// SaveSession persists the session results and closes the sandbox.
func (client *SupabaseClient) SaveSession(session *Session) error {
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
	err := client.updateSessionScore(session)
	if err != nil {
		return err
	}
	return nil
}

func (client *SupabaseClient) SubmitQuestion(session *Session, currentQuestion int) error {
	q := session.QuestionSet.Questions[currentQuestion]
	_, _, err := client.sb.From("submissions").Insert(map[string]interface{}{
		"session_id": session.ID, "question_id": q.ID,
		"answer": q.Answer, "score": q.Score, "result": q.Result,
	}, false, "", "minimal", "").Execute()
	if err != nil {
		logger.Warn(fmt.Sprintf("Leaderboard submit failed (network fallback active): %v", err))
	}
	return nil
}

func (client *SupabaseClient) Close() {
	//
}

// SendLeaderboardFlag reports a successfully completed level to the leaderboard
// as a single best-effort send of {student_id, question_id, flag}. On failure it
// logs locally and does nothing further: there is deliberately no retry queue
// and no idempotency key. This is the check-execution integration point for the
// CLI flow and must not exit the process, so it deliberately avoids the
// log.Fatalf paths in Initialize.
//
// The destination table is chosen by SubmissionTable (set via qo start -m):
// "test_submissions" for the practice archive, "submissions" for the real
// event. When SubmissionTable is empty the level-key prefix is used as a
// fallback ("test1".."test10" -> test_submissions).
func SendLeaderboardFlag(studentID, questionID, flag string) {
	sb, err := supabase.NewClient(
		API_URL,
		API_KEY,
		&supabase.ClientOptions{},
	)
	if err != nil {
		logger.Warn(fmt.Sprintf("Leaderboard sync skipped (client init failed): %v", err))
		return
	}

	// No session is needed: the client already authenticates every request with
	// the anon key (Authorization: Bearer <key>), which PostgREST maps to the
	// "anon" role. RLS policies grant that role INSERT access to the tables we
	// write, so the Signup() session dance is both unnecessary and a failure
	// point (anonymous sign-ins are disabled on the project).

	table := SubmissionTable
	if table == "" {
		table = "submissions"
		if strings.HasPrefix(questionID, "test") {
			table = "test_submissions"
		}
	}

	_, _, err = sb.From(table).Insert(map[string]interface{}{
		"student_id":  studentID,
		"question_id": questionID,
		"flag":        flag,
	}, false, "", "minimal", "").Execute()
	if err != nil {
		logger.Warn(fmt.Sprintf("Leaderboard sync failed (%s): %v", table, err))
	}
}

// ReportStudentEntry records that a student's sandbox session was started, so
// the admin "who entered / who didn't" page knows who actually showed up. It is
// best-effort and never blocks session startup: the caller should run it in a
// goroutine. The source column is SessionSource (eval|test).
func ReportStudentEntry(studentID string) {
	sb, err := supabase.NewClient(
		API_URL,
		API_KEY,
		&supabase.ClientOptions{},
	)
	if err != nil {
		logger.Warn(fmt.Sprintf("Session entry sync skipped (client init failed): %v", err))
		return
	}

	// Same as SendLeaderboardFlag: authenticate with the anon key directly and
	// rely on the RLS insert policy for the anon role.

	_, _, err = sb.From("session_logs").Insert(map[string]interface{}{
		"student_id": studentID,
		"source":     SessionSource,
	}, false, "", "minimal", "").Execute()
	if err != nil {
		logger.Warn(fmt.Sprintf("Session entry sync failed: %v", err))
	}
}
