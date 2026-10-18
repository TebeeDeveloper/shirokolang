package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shiroko/internal/project"
)

// InitCmd implements `shirocc init`.
//
// Two behaviors:
//
//   - `init <name>` (no slash): create a new full project at
//     ./<name>/ with shiroko.shrkomod, source/main.shrko,
//     test/main_test.shrko, and build/. Errors if ./<name>/ already
//     exists.
//
//   - `init <pkg>/<path>` (slashed): create a subpackage. The first
//     segment names the target project:
//     1. if it matches the current project's name, use CWD;
//     2. else if ./<pkg>/ exists as a project, use it;
//     3. else create ./<pkg>/ as a full project first.
//     Then create <pkg>/source/<path>/<last>.shrko.
//
// Examples (from ~/tb/_shiroko/shiroko/mat, where mat is a project):
//
//	init proc        → ./proc/           (new sibling project)
//	init mat/proc    → mat/source/proc/proc.shrko
//	init mat/a/b     → mat/source/a/b/b.shrko
//	init helper/util → creates ./helper/ as a full project,
//	                   then helper/source/util/util.shrko
func InitCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc init <name> | <package>/<module>")
	}
	raw := strings.Trim(args[0], "/")
	if raw == "" {
		return fmt.Errorf("invalid name %q", args[0])
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid name %q", args[0])
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// No slash → full project.
	if !strings.Contains(raw, "/") {
		return initProject(cwd, raw)
	}

	// Slashed → subpackage.
	segs := strings.Split(raw, "/")
	pkg := segs[0]
	rest := segs[1:]
	if len(rest) == 0 {
		// Shouldn't happen — "has slash" guarantees at least two
		// segments after the split of a validated string.
		return fmt.Errorf("invalid name %q", args[0])
	}

	// Resolve the target project root.
	root, err := resolveTargetProject(cwd, pkg)
	if err != nil {
		return err
	}

	return initSubpackage(root, rest)
}

// resolveTargetProject returns the root directory of the project
// named pkg, creating it as a fresh project at ./pkg/ if it doesn't
// already exist.
//
// Resolution order:
//
//  1. If CWD is inside a project whose name == pkg, return its root.
//  2. If ./pkg/shiroko.shrkomod or ./pkg/ contains .shrko files,
//     return ./pkg.
//  3. Otherwise create ./pkg/ as a full project and return it.
func resolveTargetProject(cwd, pkg string) (string, error) {
	// 1: current project?
	if root, err := project.FindProjectRoot(cwd); err == nil && root != "" {
		name := ""
		if mod, err := project.LoadModFile(root); err == nil && mod != nil {
			name = mod.Name
		}
		if name == "" {
			name = filepath.Base(root)
		}
		if name == pkg {
			return root, nil
		}
	}

	// 2: sibling project at ./pkg/?
	cand := filepath.Join(cwd, pkg)
	if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
		return cand, nil
	}

	// 3: create a new project at ./pkg/.
	if err := initProject(cwd, pkg); err != nil {
		return "", err
	}
	return cand, nil
}

// initProject creates a full project at <cwd>/<name>/.
func initProject(cwd, name string) error {
	root := filepath.Join(cwd, name)

	// Refuse if the project already exists (has a shrkomod).
	if _, err := os.Stat(filepath.Join(root, project.ShrkoModName)); err == nil {
		return fmt.Errorf("project %q already exists at %s", name, root)
	}

	srcDir := filepath.Join(root, "source")
	testDir := filepath.Join(root, "test")
	buildDir := filepath.Join(root, "build")

	for _, d := range []string{srcDir, testDir, buildDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	mainPath := filepath.Join(srcDir, "main.shrko")
	testPath := filepath.Join(testDir, "main_test.shrko")
	modPath := filepath.Join(root, project.ShrkoModName)

	var created []string

	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		content := `package main

import {
    "std/fmt"
}

func main() {
    fmt.println("hello, world")
}
`
		if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, mainPath)
	}

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		content := `package main

import {
    "std/fmt"
    "std/os"
}

// assertEq prints a pass/fail line and exits with code 1 on failure.
// Keeps the test suite usable from CI without a real test framework.
func assertEq(name string, got int, want int) {
    if got != want {
        fmt.println("FAIL:", name, "got", got, "want", want)
        os.exit(1)
    }
    fmt.println("ok:  ", name)
}

func main() {
    fmt.println("running tests")

    assertEq("one plus one", 1 + 1, 2)
    assertEq("two times three", 2 * 3, 6)
    assertEq("subtraction", 10 - 4, 6)

    fmt.println("all tests passed")
}
`
		if err := os.WriteFile(testPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, testPath)
	}

	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		mod := &project.ModFile{
			Path:    modPath,
			Name:    name,
			Version: "0.1.0",
			Entry:   "source/main.shrko",
			Output:  "build/main.go",
			Test:    "test/main_test.shrko",
		}
		if err := mod.Save(); err != nil {
			return err
		}
		created = append(created, modPath)
	}

	if len(created) == 0 {
		fmt.Printf("project %s already exists at %s\n", name, root)
		return nil
	}
	for _, p := range created {
		fmt.Printf("created %s\n", p)
	}
	return nil
}

// initSubpackage creates <root>/source/<path>/<last>.shrko, where
// <path> is the rest of the argument after the project segment.
func initSubpackage(root string, pathSegs []string) error {
	if len(pathSegs) == 0 {
		return fmt.Errorf("empty subpackage path")
	}
	last := pathSegs[len(pathSegs)-1]

	srcDir := filepath.Join(append(
		[]string{root, "source"}, pathSegs...)...)
	filePath := filepath.Join(srcDir, last+".shrko")

	if _, err := os.Stat(filePath); err == nil {
		fmt.Printf("subpackage %s already exists at %s\n",
			strings.Join(pathSegs, "/"), srcDir)
		return nil
	}

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}

	content := fmt.Sprintf(`package %s

// %s — add your code here.
`, last, strings.Join(pathSegs, "/"))

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("created %s\n", filePath)
	return nil
}
