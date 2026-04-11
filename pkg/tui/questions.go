package tui

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

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
	return runScript(question.setup_script, WORKDIR)
}

func (question *Question) Test() string {
	question.Setup()
	// comment this later; this will be handled by integrated run feature that zeyad is working on
	runScript(question.answer, WORKDIR)
	// end comment block
	err := runScript(question.test_script, WORKDIR)
	var note string
	if err != nil {
		note = fmt.Sprintf("Test failed: %s", err.Error())
		question.score = 0
	} else {
		note = ""
		question.score = 1
	}
	question.Cleanup()
	return note
}

func (question *Question) Cleanup() error {
	return runScript(question.cleanup_script, WORKDIR)

}
