package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NotFoundError is returned when an import path cannot be resolved.
// It carries the full list of directories that were checked so the
// caller can print a useful message.
type NotFoundError struct {
	Import   string
	Searched []string
}

func (e *NotFoundError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: not found", e.Import)
	if len(e.Searched) > 0 {
		b.WriteString("\n  searched:")
		for _, s := range e.Searched {
			fmt.Fprintf(&b, "\n    %s", s)
		}
	}
	b.WriteString("\n  hint: put the package in ./vendor or ./source, " +
		"or add its directory to SHIROKO_PATH")
	return b.String()
}

// Fetch resolves an import path relative to the current working dir.
// Prefer FetchFrom when you know where the importing file lives.
func Fetch(importPath string) (string, error) {
	cwd, _ := os.Getwd()
	return FetchFrom(cwd, importPath)
}

// FetchFrom returns the on-disk source directory for an import path,
// resolved relative to `from` — usually the directory of the file that
// declared the import.
//
// Lookup order:
//
//  1. <from>/vendor/<import>
//  2. <from>/<import>
//  3. <project root>/vendor/<import>
//  4. <project root>/source/<import>          project root = nearest
//  5. <project root>/<import>                 dir with a shiroko.shrkomod
//  6. $SHIROKO_PATH/<import>                  first match wins
//  7. $HOME/.shiroko/packages/<import>
//
// No registry, no network, no git. The import path is a plain relative
// path; you decide where packages live and point SHIROKO_PATH at them.
func FetchFrom(from, importPath string) (string, error) {
	if err := ValidateImportPath(importPath); err != nil {
		return "", err
	}

	var searched []string
	try := func(base string) (string, bool) {
		cand := filepath.Join(base, filepath.FromSlash(importPath))
		searched = append(searched, cand)
		if isDir(cand) {
			return cand, true
		}
		return "", false
	}

	// 1 & 2: alongside the importing file.
	if from != "" {
		if d, ok := try(filepath.Join(from, "vendor")); ok {
			return d, nil
		}
		if d, ok := try(from); ok {
			return d, nil
		}
	}

	// 3–5: from the project root, if we can find one.
	if root := FindProjectRoot(from); root != "" && root != from {
		if d, ok := try(filepath.Join(root, "vendor")); ok {
			return d, nil
		}
		if d, ok := try(filepath.Join(root, "source")); ok {
			return d, nil
		}
		if d, ok := try(root); ok {
			return d, nil
		}
	}

	// 6: user-configured search path (colon-separated on Unix).
	for _, base := range filepath.SplitList(os.Getenv("SHIROKO_PATH")) {
		if base == "" {
			continue
		}
		if d, ok := try(base); ok {
			return d, nil
		}
	}

	// 7: user-wide cache.
	if home, err := os.UserHomeDir(); err == nil {
		if d, ok := try(filepath.Join(home, ".shiroko", "packages")); ok {
			return d, nil
		}
	}

	return "", &NotFoundError{Import: importPath, Searched: searched}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
