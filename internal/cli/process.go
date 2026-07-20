package cli

import (
	"os"
)

func stopProcess(process *os.Process) error { return process.Kill() }
