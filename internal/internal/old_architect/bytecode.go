package main

import (
	"fmt"
	"strings"
)

const (
	JCC_EQ  uint64 = 1 // ==
	JCC_NEQ uint64 = 2 // !=
	JCC_LT  uint64 = 3 // <
	JCC_LTE uint64 = 4 // <=
	JCC_GT  uint64 = 5 // >
	JCC_GTE uint64 = 6 // >=
)

type OpCode uint64

const (
	OP_HALT OpCode = iota + 1
	OP_MOV
	OP_CONST
	OP_ADD
	OP_SUB
	OP_MUL
	OP_DIV
	OP_INC
	OP_DEC
	OP_NEG
	OP_NOT
	OP_CMP
	OP_USE
	OP_LOAD_GLOBAL
	OP_STORE_GLOBAL
	OP_NEW_STRUCT
	OP_GET_FIELD
	OP_SET_FIELD
	OP_NEW_LIST
	OP_INDEX_GET
	OP_INDEX_SET
	OP_LIST_LEN
	OP_GET_UPVALUE
	OP_SET_UPVALUE
	OP_JMP
	OP_JMP_IF_FALSE
	OP_JMP_IF_TRUE
	OP_CLOSURE
	OP_CALL
	OP_RET
	OP_FOR_STEP
)

type FieldInit struct {
	NameImm uint64
	ValMem  uint64
}

type UpvalueDesc struct {
	Name    string
	IsLocal bool
	Index   uint64
}

type CompiledFunction struct {
	Name         string
	Arity        uint64
	ReturnType   Type
	MemCount     uint64
	UpvalueCount uint64
	Upvalues     []UpvalueDesc
	Code         []uint64
	Constants    []any
}

type BytecodeProgram struct {
	Main *CompiledFunction
}

func AddConst(fn *CompiledFunction, v any) uint64 {
	fn.Constants = append(fn.Constants, v)
	return uint64(len(fn.Constants) - 1)
}

func EmitWords(fn *CompiledFunction, words ...uint64) int {
	startIdx := len(fn.Code)
	fn.Code = append(fn.Code, words...)
	return startIdx
}

func EncodeRel(curInstIdx int, targetInstIdx int) uint64 {
	return uint64(int64(targetInstIdx - curInstIdx))
}

func DecodeRel(raw uint64) int64 {
	return int64(raw)
}

// PatchRel vá giá trị jump nằm tại relWordIdx (chỉ số phần tử uint64 chứa rel trong fn.Code).
func PatchRel(fn *CompiledFunction, relWordIdx int, targetWordIdx int) {
	if relWordIdx < 0 || relWordIdx >= len(fn.Code) {
		return
	}
	fn.Code[relWordIdx] = uint64(int64(targetWordIdx - relWordIdx))
}

func InvertJCC(jcc uint64) uint64 {
	switch jcc {
	case JCC_EQ:
		return JCC_NEQ
	case JCC_NEQ:
		return JCC_EQ
	case JCC_LT:
		return JCC_GTE
	case JCC_LTE:
		return JCC_GT
	case JCC_GT:
		return JCC_LTE
	case JCC_GTE:
		return JCC_LT
	default:
		return JCC_EQ
	}
}

func JCCName(jcc uint64) string {
	switch jcc {
	case JCC_EQ:
		return "JE(==)"
	case JCC_NEQ:
		return "JNE(!=)"
	case JCC_LT:
		return "JL(<)"
	case JCC_LTE:
		return "JLE(<=)"
	case JCC_GT:
		return "JG(>)"
	case JCC_GTE:
		return "JGE(>=)"
	default:
		return fmt.Sprintf("JCC(%d)", jcc)
	}
}

func fmtImm(fn *CompiledFunction, idx uint64) string {
	if int(idx) < len(fn.Constants) {
		return fmt.Sprintf("[imm:%d=%#v]", idx, fn.Constants[idx])
	}
	return fmt.Sprintf("[imm:%d]", idx)
}

func DisassembleFunction(fn *CompiledFunction) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== %s (arity=%d, mem=%d, upvals=%d) ===\n",
		fn.Name, fn.Arity, fn.MemCount, fn.UpvalueCount)

	for ip := 0; ip < len(fn.Code); {
		idx := ip
		op := OpCode(fn.Code[ip])
		ip++
		fmt.Fprintf(&sb, "%04d  ", idx)

		switch op {
		case OP_MOV:
			dst, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_MOV [mem:%d], [mem:%d]", dst, src)

		case OP_CONST:
			dst, imm := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_CONST [mem:%d], %s", dst, fmtImm(fn, imm))

		case OP_ADD:
			dst, lhs, rhs := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_ADD [mem:%d], [mem:%d], [mem:%d]", dst, lhs, rhs)

		case OP_SUB:
			dst, lhs, rhs := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_SUB [mem:%d], [mem:%d], [mem:%d]", dst, lhs, rhs)

		case OP_MUL:
			dst, lhs, rhs := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_MUL [mem:%d], [mem:%d], [mem:%d]", dst, lhs, rhs)

		case OP_DIV:
			dst, lhs, rhs := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_DIV [mem:%d], [mem:%d], [mem:%d]", dst, lhs, rhs)

		case OP_INC:
			dst, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_INC [mem:%d], [mem:%d]", dst, src)

		case OP_DEC:
			dst, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_DEC [mem:%d], [mem:%d]", dst, src)

		case OP_NEG:
			dst, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_NEG [mem:%d], [mem:%d]", dst, src)

		case OP_NOT:
			dst, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_NOT [mem:%d], [mem:%d]", dst, src)

		case OP_CMP:
			lhs, rhs, jcc := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			relPos := ip + 3
			rel := DecodeRel(fn.Code[relPos])
			ip += 4
			fmt.Fprintf(&sb, "OP_CMP [mem:%d], [mem:%d], %s, %+d (-> %d)",
				lhs, rhs, JCCName(jcc), rel, int64(relPos)+rel)

		case OP_USE:
			dst, nameImm := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_USE [mem:%d], %s", dst, fmtImm(fn, nameImm))

		case OP_LOAD_GLOBAL:
			dst, nameImm := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_LOAD_GLOBAL [mem:%d], %s", dst, fmtImm(fn, nameImm))

		case OP_STORE_GLOBAL:
			nameImm, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_STORE_GLOBAL %s, [mem:%d]", fmtImm(fn, nameImm), src)

		case OP_NEW_STRUCT:
			dst, typeImm, fieldCount := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3 + int(fieldCount)*2
			fmt.Fprintf(&sb, "OP_NEW_STRUCT [mem:%d], %s, fields=%d", dst, fmtImm(fn, typeImm), fieldCount)

		case OP_GET_FIELD:
			dst, objMem, fieldImm := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_GET_FIELD [mem:%d], [mem:%d], %s", dst, objMem, fmtImm(fn, fieldImm))

		case OP_SET_FIELD:
			objMem, fieldImm, valMem := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_SET_FIELD [mem:%d], %s, [mem:%d]", objMem, fmtImm(fn, fieldImm), valMem)

		case OP_NEW_LIST:
			dst, elemTypeImm, isRepeat := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			if isRepeat != 0 {
				ip += 2
			} else {
				elemCount := fn.Code[ip]
				ip += 1 + int(elemCount)
			}
			fmt.Fprintf(&sb, "OP_NEW_LIST [mem:%d], %s, repeat=%d", dst, fmtImm(fn, elemTypeImm), isRepeat)

		case OP_INDEX_GET:
			dst, listMem, idxMem := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_INDEX_GET [mem:%d], [mem:%d][[mem:%d]]", dst, listMem, idxMem)

		case OP_INDEX_SET:
			listMem, idxMem, valMem := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3
			fmt.Fprintf(&sb, "OP_INDEX_SET [mem:%d][[mem:%d]], [mem:%d]", listMem, idxMem, valMem)

		case OP_LIST_LEN:
			dst, listMem := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_LIST_LEN [mem:%d], len([mem:%d])", dst, listMem)

		case OP_GET_UPVALUE:
			dst, upImm := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_GET_UPVALUE [mem:%d], [up:%d]", dst, upImm)

		case OP_SET_UPVALUE:
			upImm, src := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_SET_UPVALUE [up:%d], [mem:%d]", upImm, src)

		case OP_JMP:
			relPos := ip
			rel := DecodeRel(fn.Code[relPos])
			ip++
			fmt.Fprintf(&sb, "OP_JMP %+d (-> %d)", rel, int64(relPos)+rel)

		case OP_JMP_IF_FALSE:
			cond := fn.Code[ip]
			relPos := ip + 1
			rel := DecodeRel(fn.Code[relPos])
			ip += 2
			fmt.Fprintf(&sb, "OP_JMP_IF_FALSE [mem:%d], %+d (-> %d)", cond, rel, int64(relPos)+rel)

		case OP_JMP_IF_TRUE:
			cond := fn.Code[ip]
			relPos := ip + 1
			rel := DecodeRel(fn.Code[relPos])
			ip += 2
			fmt.Fprintf(&sb, "OP_JMP_IF_TRUE [mem:%d], %+d (-> %d)", cond, rel, int64(relPos)+rel)

		case OP_CLOSURE:
			dst, fnImm := fn.Code[ip], fn.Code[ip+1]
			ip += 2
			fmt.Fprintf(&sb, "OP_CLOSURE [mem:%d], %s", dst, fmtImm(fn, fnImm))

		case OP_CALL:
			fnMem, dstCount, argCount := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2]
			ip += 3 + int(dstCount) + int(argCount)
			fmt.Fprintf(&sb, "OP_CALL [mem:%d], dsts=%d, argc=%d", fnMem, dstCount, argCount)

		case OP_RET:
			valCount := fn.Code[ip]
			ip += 1 + int(valCount)
			fmt.Fprintf(&sb, "OP_RET vals=%d", valCount)

		case OP_FOR_STEP:
			idxMem, limMem, stepRaw, jcc := fn.Code[ip], fn.Code[ip+1], fn.Code[ip+2], fn.Code[ip+3]
			relPos := ip + 4
			rel := DecodeRel(fn.Code[relPos])
			ip += 5
			fmt.Fprintf(&sb, "OP_FOR_STEP [mem:%d], [mem:%d], step=%+d, %s, %+d (-> %d)",
				idxMem, limMem, int64(stepRaw), JCCName(jcc), rel, int64(relPos)+rel)

		case OP_HALT:
			fmt.Fprint(&sb, "OP_HALT")

		default:
			fmt.Fprintf(&sb, "UNKNOWN_OP(%d)", uint64(op))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
