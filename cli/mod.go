package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/pkg"
)

// ModCmd implements `shirocc mod <name>` and `shirocc mod <package>/<module>`.
//
// Inside a shiroko project (i.e. under a directory with a
// shiroko.shrkomod), the new package is created under <root>/source/
// so `import { "<name>" }` just works. Otherwise it is created relative
// to the current working directory.
//
// Only a .shrko file is created. Sub-packages do not carry a
// shiroko.shrkomod — imports live in source files, project metadata
// lives at the project root.
func ModCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc mod <name> | <package>/<module>")
	}
	name := args[0]
	if err := pkg.ValidateImportPath(name); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	base := cwd
	if root := pkg.FindProjectRoot(cwd); root != "" {
		base = filepath.Join(root, "source")
	}
	dir := filepath.Join(base, filepath.FromSlash(name))
	pkgName := filepath.Base(name)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	srcPath := filepath.Join(dir, pkgName+".shrko")

	if _, err := os.Stat(srcPath); err == nil {
		fmt.Printf("package %s already exists at %s\n", name, dir)
		return nil
	}

	content := fmt.Sprintf(
		"package %s\n\n// %s — add your code here.\n", pkgName, name)
	if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("created %s\n", srcPath)
	return nil
}
