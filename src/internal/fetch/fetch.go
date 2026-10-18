package fetch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shiroko/internal/project"
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
		"add its directory to SHIROKO_PATH, or run `shirocc import` " +
		"from the package's project root")
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
//  6. <shirocc install>/libs/<import>         (or $SHIROKO_LIBS)
//  7. <shirocc install>/libs/<first>/source/<rest>   for vendored
//     multi-segment imports like "mat/proc"
//  8. $SHIROKO_PATH/<import>                  first match wins
//  9. $HOME/.shiroko/packages/<import>
//
// No registry, no network, no git. The import path is a plain relative
// path; you decide where packages live and point SHIROKO_PATH at them.
func FetchFrom(from, importPath string) (string, error) {
	if err := project.ValidateImportPath(importPath); err != nil {
		return "", err
	}

	var searched []string
	try := func(base string) (string, bool) {
		cand := filepath.Join(base, filepath.FromSlash(importPath))
		searched = append(searched, cand)
		if isDir(cand) && hasShrkoFiles(cand) {
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
	if root, err := project.FindProjectRoot(from); err == nil && root != "" && root != from {
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

	// 6 & 7: shirocc's vendored libs.
	if libs := libsDir(); libs != "" {
		// 6: direct path — libs/<import>
		if d, ok := try(libs); ok {
			return d, nil
		}
		// 7: vendored project layout — libs/<first>/source/<rest>
		//
		// When `shirocc import` copies a project, the project keeps
		// its `source/` subdir. So a subpackage like "mat/proc"
		// lives at libs/mat/source/proc, not libs/mat/proc.
		// 7: vendored project layout — libs/<first>/source/<rest>
		if i := strings.IndexByte(importPath, '/'); i > 0 {
			// 7: vendored project layout — libs/<first>/source[/<rest>]
			first := importPath
			rest := ""
			if i := strings.IndexByte(importPath, '/'); i > 0 {
				first = importPath[:i]
				rest = importPath[i+1:]
			}
			cand := filepath.Join(libs, first, "source")
			if rest != "" {
				cand = filepath.Join(cand, filepath.FromSlash(rest))
			}
			searched = append(searched, cand)
			if hasShrkoFiles(cand) {
				return cand, nil
			}
		}
	}

	// 8: user-configured search path (colon-separated on Unix).
	for _, base := range filepath.SplitList(os.Getenv("SHIROKO_PATH")) {
		if base == "" {
			continue
		}
		if d, ok := try(base); ok {
			return d, nil
		}
	}

	// 9: user-wide cache.
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

// libsDir returns the shirocc install's libs/ directory, or "" if it
// can't be located. Mirrors cli.LibsDir but lives here so fetch stays
// free of a cli import.
func libsDir() string {
	if p := os.Getenv("SHIROKO_LIBS"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "libs")
}

func hasShrkoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".shrko") {
			return true
		}
	}
	return false
}
