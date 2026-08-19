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

	// The sandbox contains copies of this binary as /bin/qo-check, /bin/qo-setup
	// and /bin/qo-reset (qo-reset is a symlink to qo-setup), used by the level
	// check stubs and the on-demand setup/reset commands to relay requests to
	// the privileged parent.
	switch filepath.Base(os.Args[0]) {
	case "qo-check":
		os.Exit(sandbox.RunCheckClient(os.Args[1:]))
	case "qo-setup":
		os.Exit(sandbox.RunSetupClient(os.Args[1:]))
	case "qo-reset":
		os.Exit(sandbox.RunResetClient(os.Args[1:]))
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
