package color

import "os"

// Enabled is false when NO_COLOR is set (https://no-color.org/) or when
// the caller explicitly disables it.
var Enabled = os.Getenv("NO_COLOR") == ""

const (
	reset   = "\x1b[0m"
	yellow  = "\x1b[33m"
	red     = "\x1b[31m"
	cyan    = "\x1b[36m"
	magenta = "\x1b[35m"
	dim     = "\x1b[2m"
	bold    = "\x1b[1m"
)

func wrap(code, s string) string {
	if !Enabled || s == "" {
		return s
	}
	return code + s + reset
}

func Yellow(s string) string  { return wrap(yellow, s) }
func Red(s string) string     { return wrap(red, s) }
func Cyan(s string) string    { return wrap(cyan, s) }
func Magenta(s string) string { return wrap(magenta, s) }
func Dim(s string) string     { return wrap(dim, s) }
func Bold(s string) string    { return wrap(bold, s) }
