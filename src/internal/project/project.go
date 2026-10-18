package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Project is a resolved Shiroko project directory.
type Project struct {
	Dir    string   // absolute path to the project root
	Entry  string   // absolute path to the entry .shrko file
	Output string   // absolute path to the emitted .go file
	Test   string   // absolute path to the test .shrko file, or ""
	Mod    *ModFile // the project's shiroko.shrkomod
}

// LoadProject finds the project rooted at dir (walking upward to the
// nearest shiroko.shrkomod or .shrko file) and resolves its entry,
// output, and test paths.
func LoadProject(dir string) (*Project, error) {
	root, err := FindProjectRoot(dir)
	if err != nil {
		return nil, err
	}
	mod, err := LoadModFile(root)
	if err != nil {
		return nil, err
	}

	entry, err := resolveEntry(root, mod)
	if err != nil {
		return nil, err
	}
	output := resolveOutput(root, mod, entry)
	test := resolveTest(root, mod)

	return &Project{
		Dir:    root,
		Entry:  entry,
		Output: output,
		Test:   test,
		Mod:    mod,
	}, nil
}

func resolveEntry(root string, mod *ModFile) (string, error) {
	if mod.Entry != "" {
		p := filepath.Join(root, filepath.FromSlash(mod.Entry))
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			return "", fmt.Errorf(
				"entry %q declared in %s does not exist",
				mod.Entry, ShrkoModName)
		}
		return p, nil
	}

	if f := findEntry(root); f != "" {
		return f, nil
	}
	if src := filepath.Join(root, "source"); isDir(src) {
		if f := findEntry(src); f != "" {
			return f, nil
		}
	}

	return "", fmt.Errorf(
		"no entry .shrko file found in %s; declare one with `entry = ...` in %s",
		root, ShrkoModName)
}

func resolveOutput(root string, mod *ModFile, entry string) string {
	if mod.Output != "" {
		return filepath.Join(root, filepath.FromSlash(mod.Output))
	}
	return entry[:len(entry)-len(".shrko")] + ".go"
}

// resolveTest returns the test .shrko path. Explicit `test = ...` in
// the shrkomod wins; otherwise test/main_test.shrko is used if it
// exists. Returns "" when there is no test target.
func resolveTest(root string, mod *ModFile) string {
	if mod.Test != "" {
		p := filepath.Join(root, filepath.FromSlash(mod.Test))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
		return "" // declared but missing — TestCmd will report it
	}
	p := filepath.Join(root, "test", "main_test.shrko")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

// findEntry returns main.shrko in dir, or the single .shrko file if
// there is exactly one. Returns "" otherwise.
func findEntry(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var shrkos []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".shrko") {
			shrkos = append(shrkos, filepath.Join(dir, e.Name()))
		}
	}
	if len(shrkos) == 0 {
		return ""
	}
	for _, f := range shrkos {
		if filepath.Base(f) == "main.shrko" {
			return f
		}
	}
	if len(shrkos) == 1 {
		return shrkos[0]
	}
	return ""
}

// FindProjectRoot walks upward from start until it finds a directory
// containing shiroko.shrkomod or at least one .shrko file. Returns
// start itself if neither is found.
func FindProjectRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		if _, err := os.Stat(filepath.Join(dir, ShrkoModName)); err == nil {
			return dir, nil
		}
		if hasShrkoFile(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		dir = parent
	}
}

func hasShrkoFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".shrko") {
			return true
		}
		if e.IsDir() && e.Name() == "source" {
			if findEntry(filepath.Join(dir, "source")) != "" {
				return true
			}
		}
	}
	return false
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
