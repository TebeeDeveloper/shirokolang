package pkg

import (
	"os"
	"path/filepath"
)

// FindProjectRoot walks up from `start` looking for the nearest
// directory that contains a shiroko.shrkomod. Returns "" if none found.
//
// `start` may be a directory or a file path; file paths are reduced to
// their parent directory first.
func FindProjectRoot(start string) string {
	if start == "" {
		return ""
	}
	if fi, err := os.Stat(start); err == nil && !fi.IsDir() {
		start = filepath.Dir(start)
	}
	dir := start
	for {
		if isFile(filepath.Join(dir, ShrkoModName)) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
