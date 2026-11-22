package main

import (
	"fmt"
	"strings"
)

func formatWithPlaceholders(vm *VM, args []uint64, argTypes []Type) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	rawObj, ok := vm.derefHeap(args[0])
	if !ok {
		return "", fmt.Errorf("first argument to fmt is not a valid string")
	}
	tmpl, ok := rawObj.(string)
	if !ok {
		return "", fmt.Errorf("first argument to fmt must be a string, got %T", rawObj)
	}

	var b strings.Builder
	argIdx := 1
	for i := 0; i < len(tmpl); {
		if i+3 < len(tmpl) && tmpl[i] == '\\' && tmpl[i+1] == '{' && tmpl[i+2] == '\\' && tmpl[i+3] == '}' {
			b.WriteString("{}")
			i += 4
			continue
		}

		if i+1 < len(tmpl) && tmpl[i] == '{' && tmpl[i+1] == '}' {
			if argIdx < len(args) {
				var typ Type
				if argIdx < len(argTypes) {
					typ = argTypes[argIdx]
				}
				b.WriteString(vm.stringifyTypedVal(args[argIdx], typ))
				argIdx++
			} else {
				b.WriteString("{}")
			}
			i += 2
			continue
		}

		b.WriteRune(rune(tmpl[i]))
		i++
	}

	return b.String(), nil
}


func FmtModuleDef() *ModuleDef {
	printlnFn := func(vm *VM, args []uint64, argTypes []Type) (uint64, error) {
		s, err := formatWithPlaceholders(vm, args, argTypes)
		if err != nil {
			return 0, err
		}
		fmt.Println(s)
		return vm.unitAddr(), nil
	}

	printFn := func(vm *VM, args []uint64, argTypes []Type) (uint64, error) {
		s, err := formatWithPlaceholders(vm, args, argTypes)
		if err != nil {
			return 0, err
		}
		fmt.Print(s)
		return vm.unitAddr(), nil
	}

	return &ModuleDef{
		Name: "fmt",
		Exports: map[string]ModuleExport{
			"println": {
				Type: &FuncType{
					Params:     []Type{TypeString},
					ReturnType: TypeUnit,
					Variadic:   true,
				},
				Native: printlnFn,
			},
			"print": {
				Type: &FuncType{
					Params:     []Type{TypeString},
					ReturnType: TypeUnit,
					Variadic:   true,
				},
				Native: printFn,
			},
		},
	}
}
