package cmd

// start.go - Student Command
//
// This command is used by students to start their test session.
//
// Workflow:
// 1. Prompts the student to enter their Student ID (used for reports and logs).
// 2. Verifies the provided starter key and unlock time to decrypt the archive.
//    - The test will not start before the scheduled unlock time.
// 3. Sets up a sandboxed environment using Linux namespaces (isolates processes, users, and filesystem).
// 4. Extracts the challenge folder into the sandbox and launches an interactive shell for the student.
// 5. Monitors activity and logs commands executed by the student.
// 6. When time ends or the student chooses to finish, generates a single-page PDF report with their results.
//
// Flags:
// -i  --id 			 	 	 Student ID (required)
// -a, --archive  		 Path to the encrypted archive file (required).
// -p, --password 		 Password used for encrypt the archive (required)
// -k, --key           Starter key used for encryption (required).
// -d, --duration      Total duration of the test in minutes (required).
// -o, --output        Directory to save logs and PDF report (optional, default: eval-results)
//
// Usage Example:
// eval start -a ./test.enc -p foo -k bar -d 1h30m -o ./results

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ahmedYasserM/qo/pkg/archive"
	"github.com/ahmedYasserM/qo/pkg/database"
	"github.com/ahmedYasserM/qo/pkg/logger"
	"github.com/ahmedYasserM/qo/pkg/sandbox"
	"github.com/spf13/cobra"
)

var (
	idStr         string
	id            uint64
	archivePath   string
	utKeyStart    string
	passwordStart string
	testDuration  time.Duration
	outputLogDir  string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a test session in a sandboxed environment.",
	RunE: func(cmd *cobra.Command, args []string) error {

		if os.Geteuid() != 0 {
			logger.Error(fmt.Errorf("this program must be run as root"))
			os.Exit(1)
		}

		var err error
		id, err = strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			logger.Error(fmt.Errorf("invalid student ID: %w", err))
			os.Exit(1)
		}
		if !validStartModes()[startMode] {
			logger.Error(fmt.Errorf("invalid mode %q: must be 'eval' or 'test'", startMode))
			os.Exit(1)
		}
		database.SubmissionTable = "submissions"
		database.SessionSource = "eval"
		if startMode == "test" {
			database.SubmissionTable = "test_submissions"
			database.SessionSource = "test"
		}
		os.Setenv("QO_STUDENT_ID", idStr)

		// Expand a literal leading ~ before touching the filesystem.
		archivePath = expandHome(archivePath)
		outputLogDir = expandHome(outputLogDir)

		if err := sandbox.ExtractRootfs(); err != nil {
			return err
		}

		if err := archive.DecryptTarArchive(archivePath, passwordStart, utKeyStart); err != nil {
			return err
		}

		logger.Success(fmt.Sprintf("%s folder is unpacked and decrypted successfully.", archivePath))

		// check.sh execution setup: thin stubs inside the chroot relay over a
		// Unix socket to this privileged parent, which runs the real scripts.
		if _, err := sandbox.WriteCheckStubs(); err != nil {
			return fmt.Errorf("writing check stubs: %w", err)
		}
		if err := sandbox.CopyCheckClient(); err != nil {
			return fmt.Errorf("copying check client: %w", err)
		}
		if err := sandbox.CopySetupClients(); err != nil {
			return fmt.Errorf("copying setup clients: %w", err)
		}
		ln, err := sandbox.StartCheckServer(idStr)
		if err != nil {
			return err
		}
		defer ln.Close()

		// Record the student's arrival in the background (best-effort) so the
		// admin "who entered / who didn't" page is up to date. The mode flag
		// decides which Supabase tables get used for this session's data.
		go database.ReportStudentEntry(idStr)

		sandbox.LeaderboardHook = database.SendLeaderboardFlag
		err = sandbox.StartSandboxSession()

		return err
	},
}

// startMode is the destination for this session's data. "eval" writes to the
// real submissions/table; "test" writes to test_submissions/session_logs so the
// practice run before the event never pollutes final-day data.
var startMode string

func validStartModes() map[string]bool {
	return map[string]bool{"eval": true, "test": true}
}

// expandHome expands a literal leading ~ or ~/ to the invoking user's home
// directory. The shell normally expands an unquoted ~; this covers quoted
// values passed through scripts.
func expandHome(p string) string {
	if p == "~" {
		p = "~/"
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Flags
	startCmd.Flags().StringVarP(&idStr, "id", "i", "0", "Student ID (required)")
	startCmd.Flags().StringVarP(&startMode, "mode", "m", "eval", "Destination for this session: 'eval' (real leaderboard) or 'test' (practice tables)")
	startCmd.Flags().StringVarP(&archivePath, "archive", "a", "", "Path to the encrypted archive file (required)")
	startCmd.Flags().StringVarP(&passwordStart, "password", "p", "", "Password used for encrypt the archive (required)")
	startCmd.Flags().StringVarP(&utKeyStart, "key", "k", "", "Starter key used for decryption (required)")
	startCmd.Flags().DurationVarP(&testDuration, "duration", "d", 0, "Total duration of the test (e.g., 90m, 1h30m) (optional)")
	startCmd.Flags().StringVarP(&outputLogDir, "output", "o", "eval-results", "Output directory for logs and PDF reports")

	startCmd.MarkFlagRequired("id")
	startCmd.MarkFlagRequired("archive")
	startCmd.MarkFlagRequired("password")
	startCmd.MarkFlagRequired("key")

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
}
