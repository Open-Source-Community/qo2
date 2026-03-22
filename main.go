package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ahmedYasserM/qo/pkg/sandbox"
	"github.com/ahmedYasserM/qo/pkg/workflow"
)

func runWorkflow() {
	questions := []workflow.Question{
		{
			Prompt:    "Create a file named test.txt",
			TimeLimit: 300 * time.Second,
		},
		{
			Prompt:    "List files in current directory",
			TimeLimit: 300 * time.Second,
		},
	}

	workflow.Run(questions)
}

func main() {
	// internal sandbox init
	if len(os.Args) > 1 && os.Args[1] == "init" {
		persistent := os.Getenv("SANDBOX_PERSISTENT") == "1"

		var err error
		if persistent {
			// interactive persistent shell
			err = sandbox.StartSandBox(true, "")
		} else {
			// single-command execution
			// (dual mode shpuld be allowed in fututre for certain questions)
			cmd := os.Getenv("SANDBOX_CMD")
			err = sandbox.StartSandBox(false, cmd)
		}

		if err != nil {
			fmt.Println("Sandbox error:", err)
			os.Exit(1)
		}
		return
	}

	// (temporary, before TUI)
	if len(os.Args) > 1 && os.Args[1] == "workflow" {
		runWorkflow()
		return
	}

	fmt.Println("Usage: qo workflow")
}
