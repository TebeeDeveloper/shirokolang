// shiroko/cnv/cnv.go

package cnv

import (
	"fmt"
	"strings"

	"shiroko/core/emitter"
	"shiroko/core/lower"
	"shiroko/core/parser"
	"shiroko/core/sema"
)

func PrintError(err error) string {
	return fmt.Sprintf("shiroko error: %v", err)
}

// CliError wraps a raw CLI error message. It takes a string rather
// than a cli.Cmd so that cnv stays free of the cli package — cli
// imports core, core imports cnv, and a cnv → cli edge would close
// the loop.
func CliError(msg string) error {
	return fmt.Errorf("%s", msg)
}

func ParserError(errs []*parser.ParseError) error {
	var sb strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&sb, "%s\n", e.Error())
	}
	return fmt.Errorf("Parser Error(s):\n%s", sb.String())
}

func SemaError(errs []*sema.SemaError) error {
	var sb strings.Builder
	for _, se := range errs {
		fmt.Fprintf(&sb, "%s\n", se.Error())
	}
	return fmt.Errorf("Semantic Error(s):\n%s", sb.String())
}

func LowerError(errs []*lower.LowerError) error {
	var sb strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&sb, "%s\n", e.Error())
	}
	return fmt.Errorf("Lower Error(s):\n%s", sb.String())
}

func EmitterError(errs []*emitter.CodegenError) error {
	var sb strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&sb, "%s\n", e.Error())
	}
	return fmt.Errorf("Emitter Error(s):\n%s", sb.String())
}

func CompileError(err error) error {
	return fmt.Errorf("[Compiler] error: %v", err)
}
