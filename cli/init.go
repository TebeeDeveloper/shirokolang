package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/pkg"
)

// InitCmd implements `shirocc init <name>`. It creates a runnable
// project with the conventional layout:
//
//	<name>/
//	  shiroko.shrkomod
//	  source/          (.shrko sources)
//	  build/           (compiled output)
//	  test/            (tests)
func InitCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc init <name>")
	}
	name := args[0]
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid project name %q", name)
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, name)

	for _, sub := range []string{"", "build", "source", "test"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}

	modPath := filepath.Join(dir, pkg.ShrkoModName)
	mainPath := filepath.Join(dir, "source", "main.shrko")
	testPath := filepath.Join(dir, "test", "main_test.shrko")

	var created []string

	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		mf := &pkg.ModFile{
			Path:    modPath,
			Name:    name,
			Version: "0.1.0",
			Entry:   "source/main.shrko",
			Output:  "build/main.go",
		}
		if err := mf.Save(); err != nil {
			return err
		}
		created = append(created, modPath)
	}

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

// tests for ` + name + `
func main() {
	// TODO: write tests
}
`
		if err := os.WriteFile(testPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, testPath)
	}

	if len(created) == 0 {
		fmt.Printf("project %s already exists at %s\n", name, dir)
		return nil
	}
	for _, p := range created {
		fmt.Printf("created %s\n", p)
	}
	return nil
}
