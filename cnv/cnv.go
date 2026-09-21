// shiroko/cnv/cnv.go

package cnv

import (
	"fmt"
	"shiroko/core/parse"
	"shiroko/cli"
	"strings"
)

func PrintError(err error) string {
	return fmt.Sprintf("shiroko error: %v", err)
}

func CliError(cmd cli.Cmd) error {
	return fmt.Errorf("%s", cmd.Error)
}

func ParserError(p parse.Parser) error {
	var e string
	var sb strings.Builder
	for _, e = range p.Error {
		sb.WriteString(e)
		sb.WriteByte('\n')
	}

	return fmt.Errorf("[Parser] error {\n%s}", sb.String())
}

func CompileError(err error) error {
	return fmt.Errorf("[Compiler] error: %v", err)
}
