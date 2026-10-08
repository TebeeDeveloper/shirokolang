// Package backend remines to transpile your IR to the others programming
// language if you want to do.
// The choice is very quiet, about C++, Go, C, etc.
package backend

import (
	"shiroko/core/ir"
)

type Backend interface {
	Transpile(prog *ir.Program) (string, error)
}
