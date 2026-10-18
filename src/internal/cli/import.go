package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"shiroko/internal/loader"
	"shiroko/internal/project"
)

// ShiroPkgsName is the registry file inside libs/.
const ShiroPkgsName = "shiropkgs.shiropkgs"

// LibsDirName is the vendored-package directory that sits next to
// the shirocc binary.
const LibsDirName = "libs"

// LibsDir returns the libs/ directory for this install:
//
//  1. $SHIROKO_LIBS, if set — absolute path to a custom libs dir.
//  2. <dir of shirocc binary>/libs
//
// The directory is not created here; callers that write into it
// ensure it exists.
func LibsDir() string {
	if p := os.Getenv("SHIROKO_LIBS"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), LibsDirName)
}

// ShiropkgsPath returns the path to libs/shiropkgs.shiropkgs.
func ShiropkgsPath() string {
	d := LibsDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, ShiroPkgsName)
}

// ImportCmd implements `shirocc import`. It must be run from a
// project root (a directory containing shiroko.shrkomod, or a
// source/ dir, or a lone .shrko file).
//
// It does three things, in this order:
//
//  1. Register: append the project's own name to
//     libs/shiropkgs.shiropkgs, unless already there.
//  2. Scan: walk every .shrko file under the project root, collect
//     every import { "..." } path they declare, and append each one
//     (deduped) to the registry. This records what the project
//     depends on so the paths are available for later import.
//  3. Copy: recursively copy the project folder into libs/<name>/,
//     skipping build/, vendor/, libs/, and dotfiles.
//
// No fetching and no recursion: the command doesn't try to resolve
// the scanned imports. The user imports each dependency by running
// `shirocc import` from that dependency's own project root.
func ImportCmd() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := project.FindProjectRoot(cwd)
	if err != nil {
		return err
	}
	if root == "" {
		return fmt.Errorf(
			"not inside a project (no shiroko.shrkomod or .shrko found)")
	}
	if root != cwd {
		return fmt.Errorf(
			"must be run at the project root %q (current dir is %q)",
			root, cwd)
	}

	name, err := projectName(root)
	if err != nil {
		return err
	}

	libs := LibsDir()
	if libs == "" {
		return fmt.Errorf("cannot locate libs/ directory")
	}
	if err := os.MkdirAll(libs, 0o755); err != nil {
		return err
	}

	regPath := filepath.Join(libs, ShiroPkgsName)

	// Step 1: register the project's own name.
	added, err := registerName(regPath, name)
	if err != nil {
		return err
	}
	if added {
		fmt.Printf("registered %s in %s\n", name, regPath)
	} else {
		fmt.Printf("%s already registered\n", name)
	}

	// Step 2: scan the project for imports and register each one.
	imports := scanProjectImports(root)
	if len(imports) > 0 {
		var added []string
		for _, imp := range imports {
			ok, err := registerName(regPath, imp)
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"warning: could not register %q: %v\n", imp, err)
				continue
			}
			if ok {
				added = append(added, imp)
			}
		}
		if len(added) > 0 {
			sort.Strings(added)
			fmt.Printf("registered %d import(s): %s\n",
				len(added), strings.Join(added, ", "))
		} else {
			fmt.Printf("all %d import(s) already registered\n", len(imports))
		}
	}

	// Step 3: copy the project folder.
	dst := filepath.Join(libs, name)
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := copyTree(root, dst); err != nil {
		return err
	}
	fmt.Printf("copied %s -> %s\n", root, dst)
	return nil
}

// RemovePackageCmd implements `shirocc remove-package <name>`.
//
// It does two things:
//
//  1. Removes the matching line from libs/shiropkgs.shiropkgs.
//  2. Deletes libs/<name>/ recursively.
//
// If neither the registry entry nor the directory exists, it errors.
// If only one exists, it cleans up what's there and reports the
// missing half as a warning.
func RemovePackageCmd(name string) error {
	if name == "" {
		return fmt.Errorf("remove-package: empty package name")
	}
	if strings.HasPrefix(name, "std/") {
		return fmt.Errorf("cannot remove built-in package %q", name)
	}
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("remove-package: invalid name %q", name)
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
		return fmt.Errorf("remove-package: invalid name %q", name)
	}

	libs := LibsDir()
	if libs == "" {
		return fmt.Errorf("cannot locate libs/ directory")
	}

	regPath := filepath.Join(libs, ShiroPkgsName)
	inRegistry, err := removeFromRegistry(regPath, name)
	if err != nil {
		return err
	}

	dst := filepath.Join(libs, filepath.FromSlash(name))
	dirExists := false
	if fi, statErr := os.Stat(dst); statErr == nil && fi.IsDir() {
		dirExists = true
	}

	if !inRegistry && !dirExists {
		return fmt.Errorf("package %q not found", name)
	}

	if inRegistry {
		fmt.Printf("removed %s from %s\n", name, regPath)
	} else {
		fmt.Fprintf(os.Stderr,
			"warning: %s not in registry\n", name)
	}

	if dirExists {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		fmt.Printf("deleted %s\n", dst)
	} else {
		fmt.Fprintf(os.Stderr,
			"warning: %s not present in libs/\n", name)
	}

	return nil
}

// removeFromRegistry rewrites the registry file without the matching
// non-comment line. Returns (true, nil) when a line was removed,
// (false, nil) when the name wasn't there.
//
// Writes atomically: builds the new content in memory and renames a
// temp file over the original, so a crash mid-write can't corrupt
// the registry.
func removeFromRegistry(path, name string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	removed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !removed &&
			trimmed != "" &&
			!strings.HasPrefix(trimmed, "#") &&
			trimmed == name {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return false, nil
	}

	// Collapse any trailing blank lines down to a single terminator
	// so we don't leave a growing tail of "\n\n\n..." after many
	// remove operations.
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	out := strings.Join(kept, "\n") + "\n"

	// Atomic write.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, nil
}

// projectName reads the name from shiroko.shrkomod, falling back to
// the directory basename when the manifest is absent or nameless.
func projectName(root string) (string, error) {
	mod, err := project.LoadModFile(root)
	if err == nil && mod != nil && mod.Name != "" {
		return mod.Name, nil
	}
	base := filepath.Base(root)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf("cannot determine project name for %q", root)
	}
	return base, nil
}

// scanProjectImports walks every .shrko file under root, skipping
// build/, vendor/, libs/, and dotdirs, and returns the deduped set of
// import paths declared in `import { ... }` blocks.
//
// Malformed or unreadable files produce a warning on stderr and are
// skipped — one broken file doesn't block the import.
func scanProjectImports(root string) []string {
	seen := map[string]bool{}
	var out []string

	_ = filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip unreadable entries silently
			}
			base := filepath.Base(path)
			if info.IsDir() {
				if base == "build" || base == "vendor" ||
					base == "libs" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(base, ".shrko") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"warning: %s: %v\n", path, err)
				return nil
			}
			imps, err := loader.ParseImports(string(data))
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"warning: %s: %v\n", path, err)
				return nil
			}
			for _, imp := range imps {
				if imp == "" || seen[imp] {
					continue
				}
				seen[imp] = true
				out = append(out, imp)
			}
			return nil
		})

	return out
}

// registerName appends name to the registry file unless it's already
// listed. Returns (true, nil) if a line was appended, (false, nil) if
// the name was already present.
//
// The file is read in full once, deduped against, then appended to.
// The append payload is prefixed with "\n" when the file does not
// already end with a newline — this prevents a fresh entry from
// gluing onto the last line of a file that was written without a
// trailing terminator.
func registerName(path, name string) (bool, error) {
	// Read the file, or synthesize the header on a fresh file.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		header := "# shiropkgs.shiropkgs\n" +
			"#\n" +
			"# Package registry for this shirocc build.\n" +
			"# One import path per line. Blank lines and # comments are ignored.\n\n"
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			return false, err
		}
		data = []byte(header)
	} else if err != nil {
		return false, err
	}

	// Already present?
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == name {
			return false, nil
		}
	}

	// Build the append payload. Prepend "\n" if the file doesn't
	// already end with one, so the new entry always starts on its
	// own line.
	payload := name + "\n"
	if len(data) > 0 && data[len(data)-1] != '\n' {
		payload = "\n" + payload
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(payload); err != nil {
		return false, err
	}
	return true, nil
}

// copyTree recursively copies src into dst, preserving file modes.
// Skips build/, vendor/, libs/, and any dotfile or dot-directory.
func copyTree(src, dst string) error {
	return filepath.Walk(src,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return os.MkdirAll(dst, 0o755)
			}

			base := filepath.Base(path)
			if info.IsDir() {
				if base == "build" || base == "vendor" ||
					base == "libs" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				return os.MkdirAll(
					filepath.Join(dst, rel), info.Mode().Perm())
			}
			if strings.HasPrefix(base, ".") {
				return nil // skip dotfiles
			}

			return copyFile(path, filepath.Join(dst, rel), info.Mode().Perm())
		})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
