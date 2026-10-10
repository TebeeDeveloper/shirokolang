package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shiroko/pkg"
)

// project is a resolved shiroko project: its root directory, parsed
// modfile, and the concrete entry/output paths the compiler will use.
type project struct {
	Root   string
	Mod    *pkg.ModFile
	Entry  string
	Output string
}

// loadProject walks up from the current directory looking for a
// shiroko.shrkomod and resolves the entry and output paths.
//
// entry:  modfile `entry`  > <root>/source/main.shrko
// output: modfile `output` > <root>/build/<base>.go
func loadProject() (*project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	root := pkg.FindProjectRoot(cwd)
	if root == "" {
		return nil, fmt.Errorf(
			"no shiroko.shrkomod found in %s or any parent directory", cwd)
	}

	mf, err := pkg.LoadModFile(root)
	if err != nil {
		return nil, err
	}

	p := &project{Root: root, Mod: mf}

	switch {
	case mf.Entry != "":
		p.Entry = filepath.Join(root, mf.Entry)
	default:
		p.Entry = filepath.Join(root, "source", "main.shrko")
	}

	switch {
	case mf.Output != "":
		p.Output = filepath.Join(root, mf.Output)
	default:
		base := strings.TrimSuffix(filepath.Base(p.Entry), ".shrko") + ".go"
		p.Output = filepath.Join(root, "build", base)
	}

	return p, nil
}
