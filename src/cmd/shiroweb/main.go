//go:build js && wasm

package main

import (
	"bytes"
	"fmt"
	"syscall/js"

	"shiroko/internal/errors"
	"shiroko/internal/color"
	"shiroko/internal/lexer"
	"shiroko/internal/lower"
	"shiroko/internal/parser"
	"shiroko/internal/sema"
	"shiroko/internal/source"
	"shiroko/internal/interpreter"
)

const (
	statusOK         = 200
	statusUserError  = 400
	statusNotFound   = 404
	statusRuntimeErr = 500
	statusInternal   = 501
)

// =====================================================================
// Build helpers
// =====================================================================

type buildError struct {
	phase   string
	message string
}

func (e *buildError) Error() string { return e.message }

// buildSemaOnly runs lexer → parser → sema. Enough for hover/completion.
func buildSemaOnly(code string) (s *sema.Sema, berr *buildError) {
	defer func() {
		if r := recover(); r != nil {
			s = nil
			berr = &buildError{"internal", fmt.Sprintf("panic: %v", r)}
		}
	}()

	toks, err := lexer.Lex(code)
	if err != nil {
		return nil, &buildError{"lex", err.Error()}
	}

	src := source.New(code)
	p := parser.New(toks, src)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, &buildError{"parse", errors.ParserError(p.Errors).Error()}
	}

	s = sema.New(src)
	if errs := s.Analyze(prog); len(errs) > 0 {
		return nil, &buildError{"sema", errors.SemaError(errs).Error()}
	}

	return s, nil
}

// buildFull runs lexer → parser → sema → lower.
func buildFull(code string) (ir []any, berr *buildError) {
	defer func() {
		if r := recover(); r != nil {
			ir = nil
			berr = &buildError{"internal", fmt.Sprintf("panic: %v", r)}
		}
	}()

	toks, err := lexer.Lex(code)
	if err != nil {
		return nil, &buildError{"lex", err.Error()}
	}

	src := source.New(code)
	p := parser.New(toks, src)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return nil, &buildError{"parse", errors.ParserError(p.Errors).Error()}
	}

	s := sema.New(src)
	if errs := s.Analyze(prog); len(errs) > 0 {
		return nil, &buildError{"sema", errors.SemaError(errs).Error()}
	}

	lw := lower.New(s)
	ir, lerrs := lw.LowerProgram(prog)
	if len(lerrs) > 0 {
		return nil, &buildError{"lower", errors.LowerError(lerrs).Error()}
	}

	return ir, nil
}

// =====================================================================
// Response helpers
// =====================================================================

func ok(output string) js.Value {
	return js.ValueOf(map[string]interface{}{
		"status": statusOK,
		"output": output,
	})
}

func fail(status int, phase, message string) js.Value {
	return js.ValueOf(map[string]interface{}{
		"status": status,
		"phase":  phase,
		"error":  message,
	})
}

// =====================================================================
// ShiroInterpret
// =====================================================================

// ShiroInterpret is the main WASM entry point.
//
// JS: ShiroInterpret(src, stdin?)
//
//	→ { status: 200, output: "..." }
//	→ { status: 400|500|501, phase: "...", error: "..." }
func ShiroInterpret(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return fail(statusUserError, "input", "missing source argument")
	}
	src := args[0].String()

	stdin := ""
	if len(args) >= 2 {
		stdin = args[1].String()
	}

	ir, berr := buildFull(src)
	if berr != nil {
		return fail(statusUserError, berr.phase, berr.message)
	}

	var out bytes.Buffer
	ip := interpreter.New(&out)
	ip.In = bytes.NewBufferString(stdin)

	if err := ip.Run(ir); err != nil {
		return fail(statusRuntimeErr, "runtime", err.Error())
	}

	return ok(out.String())
}

// =====================================================================
// ShiroTypeAt — hover
// =====================================================================

// ShiroTypeAt returns the type of the expression at (line, col).
//
// JS: ShiroTypeAt(src, line, col)
//
//	→ { status: 200, type: "int" }
//	→ { status: 404 }               (no type info at position)
//	→ { status: 400, phase, error } (source invalid)
func ShiroTypeAt(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return fail(statusUserError, "input", "missing args")
	}
	src := args[0].String()
	line := args[1].Int()
	col := args[2].Int()

	s, berr := buildSemaOnly(src)
	if berr != nil {
		return fail(statusUserError, berr.phase, berr.message)
	}

	ty := s.TypeAt(line, col)
	if ty == nil {
		return js.ValueOf(map[string]interface{}{
			"status": statusNotFound,
		})
	}

	return js.ValueOf(map[string]interface{}{
		"status": statusOK,
		"type":   sema.TypeString(ty),
	})
}

// =====================================================================
// ShiroComplete — intellisense
// =====================================================================

// ShiroComplete returns completion items collected from user code.
//
// JS: ShiroComplete(src, line, col)
//
//	→ { status: 200, items: [{label, kind, detail}, ...] }
//
// Never fails hard — on parse/sema error returns empty items, so the
// caller keeps its static suggestions.
func ShiroComplete(this js.Value, args []js.Value) interface{} {
	empty := js.ValueOf(map[string]interface{}{
		"status": statusOK,
		"items":  []interface{}{},
	})

	if len(args) < 3 {
		return empty
	}
	src := args[0].String()

	s, berr := buildSemaOnly(src)
	if berr != nil {
		return empty
	}

	items := make([]interface{}, 0, len(s.AllSymbols))
	for _, sym := range s.AllSymbols {
		items = append(items, map[string]interface{}{
			"label":  sym.Name,
			"kind":   sym.Kind,
			"detail": sema.TypeString(sym.Type),
		})
	}

	return js.ValueOf(map[string]interface{}{
		"status": statusOK,
		"items":  items,
	})
}

// =====================================================================
// main
// =====================================================================

func main() {
	color.Enabled = false
	js.Global().Set("ShiroInterpret", js.FuncOf(ShiroInterpret))
	js.Global().Set("ShiroTypeAt", js.FuncOf(ShiroTypeAt))
	js.Global().Set("ShiroComplete", js.FuncOf(ShiroComplete))
	select {}
}
