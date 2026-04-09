package workflow

import (
	//"bufio"
	"fmt"
	"os"

	//"strings"
	"time"
	//"github.com/ahmedYasserM/qo/pkg/sandbox"
)

type Question struct {
	Prompt    string
	TimeLimit time.Duration
}

func Run(questions []Question) {
	for i, q := range questions {
		fmt.Printf("\nQuestion %d: %s\n", i+1, q.Prompt)

		timer := time.AfterFunc(q.TimeLimit, func() {
			fmt.Println("\ntime")
			os.Exit(0)
		})

		// start one persistent sandbox session for this question
		// err := sandbox.RunIsolatedSession()

		// if err != nil {
		// 	fmt.Println("Error:", err)
		// }

		timer.Stop() // stop timer if exits early
	}
}
