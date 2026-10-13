package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/pkg"
)

// InitCmd implements `shirocc init <name>`. It creates:
//
//	<name>/shiroko.shrkomod          metadata: name, version, entry, output, test
//	<name>/source/main.shrko         starter program
//	<name>/test/main_test.shrko      starter test
//	<name>/build/                    output directory
func InitCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc init <name>")
	}
	name := args[0]
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid project name %q", name)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := filepath.Join(cwd, name)
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
	modPath := filepath.Join(root, pkg.ShrkoModName)

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
		mod := &pkg.ModFile{
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
