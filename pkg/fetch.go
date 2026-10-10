package pkg

import (
	"fmt"
	"os"
	"path/filepath"
)

// Fetch returns the on-disk source directory for an import path.
//
// Lookup order:
//   1. <project root>/vendor/<import>       — vendored with the project
//   2. <project root>/<import>              — sibling of the main file
//   3. $SHIROKO_PATH/<import>               — first match wins
//   4. $HOME/.shiroko/packages/<import>     — user-wide install
//
// No registry, no network, no git. The import path is a plain relative
// path; you decide where packages live and point SHIROKO_PATH at them.
func Fetch(importPath string) (string, error) {
	if err := ValidateImportPath(importPath); err != nil {
		return "", err
	}

	// 1. vendor/ next to the project.
	cwd, _ := os.Getwd()
	if cwd != "" {
		cand := filepath.Join(cwd, "vendor", importPath)
		if isDir(cand) {
			return cand, nil
		}
		// 2. sibling directory.
		cand = filepath.Join(cwd, importPath)
		if isDir(cand) {
			return cand, nil
		}
	}

	// 3. SHIROKO_PATH, colon-separated on Unix.
	for _, base := range filepath.SplitList(os.Getenv("SHIROKO_PATH")) {
		if base == "" {
			continue
		}
		cand := filepath.Join(base, importPath)
		if isDir(cand) {
			return cand, nil
		}
	}

	// 4. user-wide cache.
	home, err := os.UserHomeDir()
	if err == nil {
		cand := filepath.Join(home, ".shiroko", "packages", importPath)
		if isDir(cand) {
			return cand, nil
		}
	}

	return "", fmt.Errorf(
		"%s: not found (looked in ./vendor, ., $SHIROKO_PATH, ~/.shiroko/packages)",
			      importPath)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
