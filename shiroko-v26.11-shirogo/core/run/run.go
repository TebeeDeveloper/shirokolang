package run

import (
	"os"
	"os/exec"
)

// RunGo shells out to `go run <path>`. The child's stdin/stdout/stderr
// are wired directly to ours so the program sees a real terminal.
func RunGo(path string) error {
	cmd := exec.Command("go", "run", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
