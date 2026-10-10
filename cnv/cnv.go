// shiroko/cnv/cnv.go

package cnv

import (
	"fmt"
	"shiroko/cli"
	"strings"
	"shiroko/core/parser"
	"shiroko/core/sema"
	"shiroko/core/lower"
	"shiroko/backend/golang"
)

func PrintError(err error) string {
	return fmt.Sprintf("shiroko error: %v", err)
}

func CliError(cmd cli.Cmd) error {
	return fmt.Errorf("%s", cmd.Error)
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

func EmitterError(errs []*golang.CodegenError) error {
	var sb strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&sb, "%s\n", e.Error())
	}
	return fmt.Errorf("Lower Error(s):\n%s", sb.String())
}

func CompileError(err error) error {
	return fmt.Errorf("[Compiler] error: %v", err)
}
