package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"shiroko/pkg"
)

// InitCmd implements `shirocc init <name>`. It creates a runnable
// project directory with an entry point (package main + func main)
// and an empty shiroko.shrkomod.
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

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	mainPath := filepath.Join(dir, "main.shrko")
	modPath := filepath.Join(dir, pkg.ShrkoModName)

	var created []string

	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		content := "package main\nimport {\n\t\"std/fmt\"\n}\nfunc main() {\n\tfmt.println(\"hello, world\")\n}\n"
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		return err
	}
	created = append(created, mainPath)
	}

	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		content := fmt.Sprintf("# shiroko.shrkomod for %s\n#\n# List the packages this one imports, one per line:\n#   <name>\n#   <package>/<module>\n#\n# Empty means no dependencies.\n", name)
		if err := os.WriteFile(modPath, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, modPath)
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
