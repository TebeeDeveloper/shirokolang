package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/pkg"
)

// ModCmd implements `shirocc mod <name>` and `shirocc mod <package>/<module>`.
//
// It creates a new package directory under the current working dir:
//
//   <cwd>/<name>/
//     shiroko.shrkomod      — dependency list (initially empty)
//     <base>.shrko          — starter source file
//
// Safe to re-run. Existing files are left alone.
func ModCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: shirocc mod <name> | <package>/<module>")
	}
	name := args[0]
	if err := pkg.ValidateImportPath(name); err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, filepath.FromSlash(name))
	pkgName := filepath.Base(name)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	modPath := filepath.Join(dir, pkg.ShrkoModName)
	srcPath := filepath.Join(dir, pkgName+".shrko")

	var created []string

	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		content := fmt.Sprintf(`# shiroko.shrkomod for %s
		#
		# List the packages this one imports, one per line:
		#   <name>
		#   <package>/<module>
		#
		# Empty means no dependencies.
		`, name)
		if err := os.WriteFile(modPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, modPath)
	}

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		content := fmt.Sprintf(`package %s

		// %s — add your code here.
		`, pkgName, name)
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
