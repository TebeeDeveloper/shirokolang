package cli

import (
	"os"

	"shiroko/internal/project"
)

// loadProject is the CLI's entry point for "the current project".
// All filesystem logic lives in pkg; this just hands over the working
// directory so cli/*.go files don't each re-do the same dance.
func loadProject() (*project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.LoadProject(cwd)
}
