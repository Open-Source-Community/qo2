package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ahmedYasserM/qo/cmd"
	"github.com/ahmedYasserM/qo/pkg/logger"
	"github.com/ahmedYasserM/qo/pkg/sandbox"
	"github.com/ahmedYasserM/qo/pkg/tui"
)

func main() {

	// The sandbox contains a copy of this binary as /bin/qo-check, used by the
	// per-level check.sh stubs to relay requests to the privileged parent.
	if filepath.Base(os.Args[0]) == "qo-check" {
		os.Exit(sandbox.RunCheckClient(os.Args[1:]))
	}

	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := sandbox.StartSandBox(); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "sandbox init error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) == 1 {
		if err := tui.StartTUI(); err != nil {
			logger.Error(err)
			os.Exit(1)
		}
		return
	}

	if err := cmd.Execute(); err != nil {
		logger.Error(err)
		os.Exit(1)
	}
}
