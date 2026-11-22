package main

import (
	"fmt"
	"strings"
)

type NativeFunc func(vm *VM, args []uint64, argTypes []Type) (uint64, error)

type ModuleType struct {
	Name    string
	Exports map[string]Type
}

func (m *ModuleType) String() string { return "module " + m.Name }
func (m *ModuleType) Equals(other Type) bool {
	o, ok := other.(*ModuleType)
	return ok && m == o
}

type ModuleExport struct {
	Type   Type
	Native NativeFunc
}

type ModuleDef struct {
	Name    string
	Exports map[string]ModuleExport
}

func (d *ModuleDef) StaticType() *ModuleType {
	exports := make(map[string]Type, len(d.Exports))
	for name, exp := range d.Exports {
		exports[name] = exp.Type
	}
	return &ModuleType{
		Name:    d.Name,
		Exports: exports,
	}
}

type ModuleRegistry struct {
	defs map[string]*ModuleDef
}

func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		defs: make(map[string]*ModuleDef),
	}
}

func (r *ModuleRegistry) Register(def *ModuleDef) {
	if def == nil || def.Name == "" {
		return
	}
	r.defs[def.Name] = def
}

func (r *ModuleRegistry) Get(name string) (*ModuleDef, bool) {
	def, ok := r.defs[name]
	return def, ok
}

func DefaultModuleRegistry() *ModuleRegistry {
	reg := NewModuleRegistry()
	reg.Register(FmtModuleDef())
	return reg
}
