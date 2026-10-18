package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/internal/project"
)

// ModCmd implements `shirocc mod <name>` and `shirocc mod <package>/<module>`.
//
// It creates a library package directory under the current working dir:
//
//   <cwd>/<name>/
//     shiroko.shrkomod      name only — no entry, no output
//     <base>.shrko          starter source file
//
// Libraries are consumed via `import { "..." }` blocks in the .shrko
// files that use them. The shrkomod carries only metadata, so a
// library's is minimal.
func ModCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc mod <name> | <package>/<module>")
	}
	name := args[0]
	if err := project.ValidateImportPath(name); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(cwd, filepath.FromSlash(name))
	pkgName := filepath.Base(name)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	modPath := filepath.Join(dir, project.ShrkoModName)
	srcPath := filepath.Join(dir, pkgName+".shrko")

	var created []string

	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		mod := &project.ModFile{
			Path:    modPath,
			Name:    name,
			Version: "0.1.0",
		}
		if err := mod.Save(); err != nil {
			return err
		}
		created = append(created, modPath)
	}

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		content := fmt.Sprintf(`package main

// %s — add your code here.
`, name)
		if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, srcPath)
	}

	if len(created) == 0 {
		fmt.Printf("package %s already exists at %s\n", name, dir)
		return nil
	}
	for _, p := range created {
		fmt.Printf("created %s\n", p)
	}
	return nil
}
