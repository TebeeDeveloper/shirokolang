// webApp/main.go
package main

import (
	"shiroko/cnv"
	"shiroko/cli"
	"shiroko/thread"
	"syscall/js"
)

func api(this js.Value, args []js.Value) interface{} {
	if len(args) == 0 {
		return js.ValueOf(map[string]interface{}{
			"status": 404,
			"message": "Args not found.",
		})
	}
	
	var arg []string
	var a js.Value
	for _, a = range args {
		arg = append(arg, a.String())
	}
	
	var err error = thread.Run(cli.ParseJS("shiroko", arg))
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"status": 501,
			"message": cnv.PrintError(err),
		})
	}
	return js.ValueOf(map[string]interface{}{
		"status": 200,
		"message": "Success.",
	})
}

func main() {
	js.Global().Set("CompileJS", js.FuncOf(api))
	select{}
}
