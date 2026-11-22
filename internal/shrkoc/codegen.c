/* shrkc/codegen.c */
#include "codegen.h"
#include "vm.h"
#include <limits.h>
#include <string.h>

#define SHR_CG_MAX_CALL_ARGS 32u

void shr_codegen_init(ShrCodegen *cg) {
    memset(cg, 0, sizeof(*cg));
    shr_bc_writer_init(&cg->bc);
}

static uint8_t alloc_slot(ShrCodegen *cg) {
    if (cg->next_slot >= 254u) return 254u;
    return cg->next_slot++;
}

static bool get_var_slot(const ShrCodegen *cg, const char *name, size_t len,
                         uint8_t *out_slot, ShrTypeKind *out_t, size_t *out_closure_idx) {
    for (size_t i = 0; i < cg->var_count; i++) {
        if (cg->vars[i].name_len == len && memcmp(cg->vars[i].name, name, len) == 0) {
            if (out_slot) *out_slot = cg->vars[i].slot;
            if (out_t) *out_t = cg->vars[i].type;
            if (out_closure_idx) *out_closure_idx = cg->vars[i].closure_func_idx;
            return true;
        }
    }
    return false;
}

static uint16_t find_func_idx(const ShrProgramAst *prog, const char *name, size_t len, uint16_t base_offset) {
    for (uint16_t i = 0; i < prog->func_count; i++) {
        if (prog->funcs[i].name.len == len &&
            memcmp(prog->funcs[i].name.start, name, len) == 0) {
            return (uint16_t)(base_offset + i);
        }
    }
    return 0xFFFFu;
}

static uint8_t pick_binop(ShrTokenKind op, ShrTypeKind ty) {
    if (ty == SHR_TY_F64) {
        switch (op) {
            case TOK_PLUS:     return OP_ADD_F64;
            case TOK_PLUS_EQ:  return OP_ADD_F64;
            case TOK_MINUS:    return OP_SUB_F64;
            case TOK_MINUS_EQ: return OP_SUB_F64;
            case TOK_STAR:     return OP_MUL_F64;
            case TOK_STAR_EQ:  return OP_MUL_F64;
            case TOK_SLASH:    return OP_DIV_F64;
            case TOK_SLASH_EQ: return OP_DIV_F64;
            default:           return OP_ADD_F64;
        }
    }
    if (ty == SHR_TY_I64) {
        switch (op) {
            case TOK_PLUS:     return OP_ADD_I64;
            case TOK_PLUS_EQ:  return OP_ADD_I64;
            case TOK_MINUS:    return OP_SUB_I64;
            case TOK_MINUS_EQ: return OP_SUB_I64;
            case TOK_STAR:     return OP_MUL_I64;
            case TOK_STAR_EQ:  return OP_MUL_I64;
            default:           return OP_ADD_I64;
        }
    }
    switch (op) {
        case TOK_PLUS:     return OP_ADD_I32;
        case TOK_PLUS_EQ:  return OP_ADD_I32;
        case TOK_MINUS:    return OP_SUB_I32;
        case TOK_MINUS_EQ: return OP_SUB_I32;
        case TOK_STAR:     return OP_MUL_I32;
        case TOK_STAR_EQ:  return OP_MUL_I32;
        case TOK_SLASH:    return OP_DIV_I32;
        case TOK_SLASH_EQ: return OP_DIV_I32;
        case TOK_PERCENT:  return OP_MOD_I32;
        default:           return OP_ADD_I32;
    }
}

static void emit_rel_patch(ShrCodegen *cg, size_t op_pos, size_t target_pos) {
    if (op_pos >= cg->bc.code_size) return;
    uint8_t op = cg->bc.code[op_pos];
    size_t rel_pos = (op == OP_JMP) ? (op_pos + 1) : (op_pos + 3);
    size_t next_ip = (op == OP_JMP) ? (op_pos + 3) : (op_pos + 5);

    if (rel_pos + 1 < cg->bc.code_size) {
        int64_t diff = (int64_t)target_pos - (int64_t)next_ip;
        if (diff < INT16_MIN || diff > INT16_MAX) return;
        int16_t rel = (int16_t)diff;
        uint16_t u = (uint16_t)rel;
        cg->bc.code[rel_pos]     = (uint8_t)(u & 0xffu);
        cg->bc.code[rel_pos + 1] = (uint8_t)((u >> 8) & 0xffu);
    }
}

static void emit_jmp_rel_to(ShrCodegen *cg, size_t target_pos) {
    size_t op_pos = cg->bc.code_size;
    shr_bc_writer_emit_u8(&cg->bc, OP_JMP);
    shr_bc_writer_emit_u16(&cg->bc, 0);
    emit_rel_patch(cg, op_pos, target_pos);
}

static size_t find_closure_idx_by_let_name(const ShrStmt *stmts, size_t stmt_count,
                                             const char *name, size_t len) {
    if (!stmts || !name || len == 0) return SIZE_MAX;
    for (size_t s = 0; s < stmt_count; s++) {
        const ShrStmt *st = &stmts[s];
        if (st->kind == STMT_LET_MULTI) {
            for (size_t k = 0; k < st->as.let_multi.name_count && k < st->as.let_multi.expr_count; k++) {
                if (st->as.let_multi.names[k].len == len &&
                    memcmp(st->as.let_multi.names[k].start, name, len) == 0 &&
                    st->as.let_multi.exprs[k] &&
                    st->as.let_multi.exprs[k]->kind == EXPR_FN) {
                    return st->as.let_multi.exprs[k]->as.fn.func_idx;
                }
            }
        } else if (st->kind == STMT_FOR_COND) {
            size_t r = find_closure_idx_by_let_name(st->as.for_cond.body, st->as.for_cond.body_count, name, len);
            if (r != SIZE_MAX) return r;
        } else if (st->kind == STMT_FOR_C) {
            if (st->as.for_c.init) {
                size_t r = find_closure_idx_by_let_name(st->as.for_c.init, 1, name, len);
                if (r != SIZE_MAX) return r;
            }
            size_t r = find_closure_idx_by_let_name(st->as.for_c.body, st->as.for_c.body_count, name, len);
            if (r != SIZE_MAX) return r;
        } else if (st->kind == STMT_FOR_IN) {
            size_t r = find_closure_idx_by_let_name(st->as.for_in.body, st->as.for_in.body_count, name, len);
            if (r != SIZE_MAX) return r;
        }
    }
    return SIZE_MAX;
}

static size_t resolve_closure_ast_idx(const ShrCodegen *cg, const ShrProgramAst *prog,
                                      const ShrExpr *call_e, size_t cur_fn_idx, bool cur_is_closure) {
    if (!call_e || !call_e->as.call.callee || call_e->as.call.callee->kind != EXPR_IDENT ||
        !prog || prog->closure_count == 0) {
        return SIZE_MAX;
    }
    const ShrExpr *callee = call_e->as.call.callee;

    size_t c_idx = SIZE_MAX;
    if (get_var_slot(cg, callee->tok.start, callee->tok.len, nullptr, nullptr, &c_idx) &&
        c_idx < prog->closure_count) {
        return c_idx;
    }

    if (!cur_is_closure && cur_fn_idx < prog->func_count) {
        const ShrFuncDecl *cur_fn = &prog->funcs[cur_fn_idx];
        c_idx = find_closure_idx_by_let_name(cur_fn->stmts, cur_fn->stmt_count,
                                              callee->tok.start, callee->tok.len);
        if (c_idx < prog->closure_count) return c_idx;

        for (size_t p = 0; p < cur_fn->param_count; p++) {
            if (cur_fn->params[p].spec.kind == SHR_TY_FN &&
                cur_fn->params[p].name.len == callee->tok.len &&
                memcmp(cur_fn->params[p].name.start, callee->tok.start, callee->tok.len) == 0) {
                for (size_t c = 0; c < prog->closure_count; c++) {
                    if (prog->closures[c].user_param_count == call_e->as.call.arg_count) {
                        return c;
                    }
                }
            }
        }
    }

    c_idx = find_closure_idx_by_let_name(prog->toplevel_stmts, prog->toplevel_stmt_count,
                                         callee->tok.start, callee->tok.len);
    if (c_idx < prog->closure_count) return c_idx;

    for (size_t f = 0; f < prog->func_count; f++) {
        c_idx = find_closure_idx_by_let_name(prog->funcs[f].stmts, prog->funcs[f].stmt_count,
                                             callee->tok.start, callee->tok.len);
        if (c_idx < prog->closure_count) return c_idx;
    }

    for (size_t c = 0; c < prog->closure_count; c++) {
        if (prog->closures[c].name.len == callee->tok.len &&
            memcmp(prog->closures[c].name.start, callee->tok.start, callee->tok.len) == 0) {
            return c;
        }
    }
    return SIZE_MAX;
}

static bool get_captured_val_from_ast(const ShrProgramAst *prog, size_t cur_fn_idx, bool cur_is_closure,
                                      const char *name, size_t len, int32_t *out_val) {
    if (!prog) return false;
    for (size_t f = 0; f < prog->func_count; f++) {
        if (!cur_is_closure && f == cur_fn_idx) continue;
        const ShrFuncDecl *fn = &prog->funcs[f];
        for (size_t s = 0; s < fn->stmt_count; s++) {
            const ShrStmt *st = &fn->stmts[s];
            if (st->kind == STMT_LET_MULTI) {
                for (size_t k = 0; k < st->as.let_multi.name_count && k < st->as.let_multi.expr_count; k++) {
                    if (st->as.let_multi.names[k].len == len &&
                        memcmp(st->as.let_multi.names[k].start, name, len) == 0 &&
                        st->as.let_multi.exprs[k] &&
                        st->as.let_multi.exprs[k]->kind == EXPR_INT) {
                        *out_val = (int32_t)st->as.let_multi.exprs[k]->as.i_val;
                        return true;
                    }
                }
            }
        }
    }
    for (size_t s = 0; s < prog->toplevel_stmt_count; s++) {
        const ShrStmt *st = &prog->toplevel_stmts[s];
        if (st->kind == STMT_LET_MULTI) {
            for (size_t k = 0; k < st->as.let_multi.name_count && k < st->as.let_multi.expr_count; k++) {
                if (st->as.let_multi.names[k].len == len &&
                    memcmp(st->as.let_multi.names[k].start, name, len) == 0 &&
                    st->as.let_multi.exprs[k] &&
                    st->as.let_multi.exprs[k]->kind == EXPR_INT) {
                    *out_val = (int32_t)st->as.let_multi.exprs[k]->as.i_val;
                    return true;
                }
            }
        }
    }
    return false;
}

static uint8_t emit_expr_slots(ShrCodegen *cg, const ShrProgramAst *prog,
                               uint16_t fn_base, uint16_t cl_base,
                               size_t cur_fn_idx, bool cur_is_closure,
                               ShrExpr *e, uint8_t *out_slots);

static size_t emit_cond_jump_to_exit(ShrCodegen *cg, const ShrProgramAst *prog,
                                     uint16_t fn_base, uint16_t cl_base,
                                     size_t cur_fn_idx, bool cur_is_closure, ShrExpr *cond) {
    if (!cond) return SIZE_MAX;

    if (cond->kind == EXPR_BINARY &&
        (cond->as.binary.op == TOK_LT || cond->as.binary.op == TOK_LE ||
         cond->as.binary.op == TOK_GT || cond->as.binary.op == TOK_GE ||
         cond->as.binary.op == TOK_EQ_EQ || cond->as.binary.op == TOK_BANG_EQ)) {
        uint8_t a[32] = {0}, b[32] = {0};
        emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, cond->as.binary.lhs, a);
        emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, cond->as.binary.rhs, b);

        uint8_t jmp_op = OP_JGE_I32;
        switch (cond->as.binary.op) {
            case TOK_LT:      jmp_op = OP_JGE_I32; break;
            case TOK_LE:      jmp_op = OP_JG_I32;  break;
            case TOK_GT:      jmp_op = OP_JLE_I32; break;
            case TOK_GE:      jmp_op = OP_JL_I32;  break;
            case TOK_EQ_EQ:   jmp_op = OP_JNE_I32; break;
            case TOK_BANG_EQ: jmp_op = OP_JE_I32;  break;
            default:          jmp_op = OP_JGE_I32; break;
        }
        size_t op_pos = cg->bc.code_size;
        shr_bc_writer_emit_u8(&cg->bc, jmp_op);
        shr_bc_writer_emit_u8(&cg->bc, a[0]);
        shr_bc_writer_emit_u8(&cg->bc, b[0]);
        shr_bc_writer_emit_u16(&cg->bc, 0);
        return op_pos;
    }

    uint8_t cslot[32] = {0};
    emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, cond, cslot);
    uint8_t zero = alloc_slot(cg);
    shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
    shr_bc_writer_emit_u8(&cg->bc, zero);
    shr_bc_writer_emit_i32(&cg->bc, 0);
    size_t op_pos = cg->bc.code_size;
    shr_bc_writer_emit_u8(&cg->bc, OP_JE_I32);
    shr_bc_writer_emit_u8(&cg->bc, cslot[0]);
    shr_bc_writer_emit_u8(&cg->bc, zero);
    shr_bc_writer_emit_u16(&cg->bc, 0);
    return op_pos;
}

static uint8_t emit_expr_slots(ShrCodegen *cg, const ShrProgramAst *prog,
                               uint16_t fn_base, uint16_t cl_base,
                               size_t cur_fn_idx, bool cur_is_closure,
                               ShrExpr *e, uint8_t *out_slots) {
    if (!e || !out_slots) return 0;

    switch (e->kind) {
        case EXPR_INT: {
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_i32(&cg->bc, (int32_t)e->as.i_val);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_FLOAT: {
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_F64);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_f64(&cg->bc, e->as.f_val);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_BOOL: {
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_BOOL);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_u8(&cg->bc, e->as.b_val ? 1 : 0);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_STRING: {
            uint8_t d = alloc_slot(cg);
            size_t raw_len = e->tok.len >= 2 ? e->tok.len - 2 : 0;
            const char *raw = e->tok.len >= 2 ? e->tok.start + 1 : e->tok.start;
            uint16_t off = shr_bc_writer_add_rodata(&cg->bc, raw, raw_len);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_RODATA);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_u16(&cg->bc, off);
            shr_bc_writer_emit_u16(&cg->bc, (uint16_t)raw_len);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_LIST: {
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_NEW_LIST);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_u8(&cg->bc, (uint8_t)e->as.list.elem_type);
            shr_bc_writer_emit_u16(&cg->bc, (uint16_t)e->as.list.item_count);

            for (size_t i = 0; i < e->as.list.item_count; i++) {
                uint8_t elem_s[32] = {0};
                emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure,
                                e->as.list.items[i], elem_s);
                shr_bc_writer_emit_u8(&cg->bc, OP_LIST_APPEND);
                shr_bc_writer_emit_u8(&cg->bc, d);
                shr_bc_writer_emit_u8(&cg->bc, elem_s[0]);
            }
            out_slots[0] = d;
            return 1;
        }
        case EXPR_INDEX: {
            uint8_t obj_s[32] = {0};
            uint8_t idx_s[32] = {0};
            emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, e->as.index.target, obj_s);
            emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, e->as.index.idx, idx_s);
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_LIST_GET);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_u8(&cg->bc, obj_s[0]);
            shr_bc_writer_emit_u8(&cg->bc, idx_s[0]);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_FN: {
            uint8_t d = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_i32(&cg->bc, (int32_t)(cl_base + e->as.fn.func_idx));
            out_slots[0] = d;
            return 1;
        }
        case EXPR_IDENT: {
            uint8_t s = 0;
            if (!get_var_slot(cg, e->tok.start, e->tok.len, &s, nullptr, nullptr)) {
                int32_t cap_val = 0;
                uint8_t d = alloc_slot(cg);
                if (get_captured_val_from_ast(prog, cur_fn_idx, cur_is_closure,
                                              e->tok.start, e->tok.len, &cap_val)) {
                    shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
                    shr_bc_writer_emit_u8(&cg->bc, d);
                    shr_bc_writer_emit_i32(&cg->bc, cap_val);
                } else {
                    shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
                    shr_bc_writer_emit_u8(&cg->bc, d);
                    shr_bc_writer_emit_i32(&cg->bc, 0);
                }
                out_slots[0] = d;
                return 1;
            }
            out_slots[0] = s;
            return 1;
        }
        case EXPR_BINARY: {
            uint8_t a[32] = {0}, b[32] = {0};
            emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, e->as.binary.lhs, a);
            emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, e->as.binary.rhs, b);
            uint8_t d = alloc_slot(cg);
            ShrTypeKind ty = (e->inferred_types.count > 0 && e->inferred_types.items)
                                 ? e->inferred_types.items[0] : SHR_TY_I32;
            uint8_t op = pick_binop(e->as.binary.op, ty);
            shr_bc_writer_emit_u8(&cg->bc, op);
            shr_bc_writer_emit_u8(&cg->bc, d);
            shr_bc_writer_emit_u8(&cg->bc, a[0]);
            shr_bc_writer_emit_u8(&cg->bc, b[0]);
            out_slots[0] = d;
            return 1;
        }
        case EXPR_CALL: {
            uint8_t arg_slots[SHR_CG_MAX_CALL_ARGS] = {0};
            uint8_t arg_types[SHR_CG_MAX_CALL_ARGS] = {0};
            uint8_t argc = 0;

            uint16_t fidx = 0xFFFFu;
            uint16_t sym_off = 0;
            uint16_t sym_len = 0;

            size_t cl_ast_idx = resolve_closure_ast_idx(cg, prog, e, cur_fn_idx, cur_is_closure);
            if (cl_ast_idx < prog->closure_count) {
                fidx = (uint16_t)(cl_base + cl_ast_idx);
                const ShrFuncDecl *cfn = &prog->closures[cl_ast_idx];
                for (size_t c = 0; c < cfn->capture_count && argc < SHR_CG_MAX_CALL_ARGS; c++) {
                    uint8_t cslot = 0;
                    ShrTypeKind cty = cfn->captures[c].type;
                    if (get_var_slot(cg, cfn->captures[c].name.start, cfn->captures[c].name.len,
                                     &cslot, &cty, nullptr)) {
                        arg_slots[argc] = cslot;
                        arg_types[argc] = (uint8_t)cty;
                        argc++;
                    } else {
                        int32_t cap_val = 0;
                        uint8_t tmp_cap = alloc_slot(cg);
                        if (get_captured_val_from_ast(prog, cur_fn_idx, cur_is_closure,
                                                      cfn->captures[c].name.start,
                                                      cfn->captures[c].name.len, &cap_val)) {
                            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
                            shr_bc_writer_emit_u8(&cg->bc, tmp_cap);
                            shr_bc_writer_emit_i32(&cg->bc, cap_val);
                        } else {
                            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
                            shr_bc_writer_emit_u8(&cg->bc, tmp_cap);
                            shr_bc_writer_emit_i32(&cg->bc, 0);
                        }
                        arg_slots[argc] = tmp_cap;
                        arg_types[argc] = (uint8_t)cty;
                        argc++;
                    }
                }
            } else if (e->as.call.callee && e->as.call.callee->kind == EXPR_IDENT) {
                fidx = find_func_idx(prog, e->as.call.callee->tok.start, e->as.call.callee->tok.len, fn_base);
            } else if (e->as.call.callee && e->as.call.callee->kind == EXPR_MEMBER) {
                char sym_buf[128];
                ShrExpr *obj = e->as.call.callee->as.member.obj;
                ShrToken mem = e->as.call.callee->as.member.member;
                size_t p = 0;
                if (obj && obj->kind == EXPR_IDENT && obj->tok.len + 1 + mem.len < sizeof(sym_buf)) {
                    memcpy(sym_buf + p, obj->tok.start, obj->tok.len);
                    p += obj->tok.len;
                    sym_buf[p++] = '_';
                    memcpy(sym_buf + p, mem.start, mem.len);
                    p += mem.len;
                    sym_buf[p] = '\0';
                    fidx = find_func_idx(prog, sym_buf, p, fn_base);
                }
            }

            for (size_t i = 0; i < e->as.call.arg_count && argc < SHR_CG_MAX_CALL_ARGS; i++) {
                uint8_t tmp[32] = {0};
                uint8_t k = emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, e->as.call.args[i], tmp);
                for (uint8_t j = 0; j < k && argc < SHR_CG_MAX_CALL_ARGS; j++) {
                    uint8_t snap = alloc_slot(cg);
                    shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                    shr_bc_writer_emit_u8(&cg->bc, snap);
                    shr_bc_writer_emit_u8(&cg->bc, tmp[j]);
                    arg_slots[argc] = snap;
                    arg_types[argc] = (e->as.call.args[i]->inferred_types.count > j && e->as.call.args[i]->inferred_types.items)
                                          ? (uint8_t)e->as.call.args[i]->inferred_types.items[j]
                                          : (uint8_t)SHR_TY_I32;
                    argc++;
                }
            }

            uint8_t ret_n = (e->inferred_types.count > 0)
                                ? (uint8_t)e->inferred_types.count
                                : 1;
            if (ret_n == 0) ret_n = 1;
            if (ret_n > 32) ret_n = 32;
            for (uint8_t r = 0; r < ret_n; r++) {
                out_slots[r] = alloc_slot(cg);
            }

            if (fidx == 0xFFFFu) {
                if (e->as.call.callee && e->as.call.callee->kind == EXPR_IDENT) {
                    sym_off = shr_bc_writer_add_rodata(&cg->bc, e->as.call.callee->tok.start, e->as.call.callee->tok.len);
                    sym_len = (uint16_t)e->as.call.callee->tok.len;
                } else if (e->as.call.callee && e->as.call.callee->kind == EXPR_MEMBER) {
                    ShrToken mem = e->as.call.callee->as.member.member;
                    sym_off = shr_bc_writer_add_rodata(&cg->bc, mem.start, mem.len);
                    sym_len = (uint16_t)mem.len;
                }
            }

            shr_bc_writer_emit_u8(&cg->bc, OP_CALL);
            shr_bc_writer_emit_u8(&cg->bc, ret_n);
            for (uint8_t r = 0; r < ret_n; r++) {
                shr_bc_writer_emit_u8(&cg->bc, out_slots[r]);
            }
            shr_bc_writer_emit_u16(&cg->bc, fidx);
            shr_bc_writer_emit_u8(&cg->bc, argc);
            shr_bc_writer_emit_u8(&cg->bc, argc);
            for (uint8_t i = 0; i < argc; i++) {
                shr_bc_writer_emit_u8(&cg->bc, arg_types[i]);
                shr_bc_writer_emit_u8(&cg->bc, arg_slots[i]);
            }
            if (fidx == 0xFFFFu) {
                shr_bc_writer_emit_u16(&cg->bc, sym_off);
                shr_bc_writer_emit_u16(&cg->bc, sym_len);
            }
            return ret_n;
        }
        default:
            return 0;
    }
}

static void emit_stmt_block(ShrCodegen *cg, const ShrProgramAst *prog,
                            uint16_t fn_base, uint16_t cl_base,
                            size_t cur_fn_idx, bool cur_is_closure,
                            const ShrStmt *stmts, size_t stmt_count) {
    for (size_t s = 0; s < stmt_count; s++) {
        const ShrStmt *st = &stmts[s];

        if (st->kind == STMT_LET_MULTI) {
            uint8_t rhs_slots[64] = {0};
            uint8_t rhs_types[64] = {0};
            size_t rhs_closure[64];
            for (int i = 0; i < 64; i++) rhs_closure[i] = SIZE_MAX;
            uint8_t total_rhs = 0;

            for (size_t e = 0; e < st->as.let_multi.expr_count; e++) {
                ShrExpr *re = st->as.let_multi.exprs[e];
                uint8_t tmp[32] = {0};
                uint8_t k = emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, re, tmp);
                for (uint8_t j = 0; j < k && total_rhs < 64; j++) {
                    rhs_slots[total_rhs] = tmp[j];
                    rhs_types[total_rhs] = (re && re->inferred_types.count > j && re->inferred_types.items)
                                             ? (uint8_t)re->inferred_types.items[j]
                                             : (uint8_t)SHR_TY_I32;
                    if (re && re->kind == EXPR_FN) {
                        rhs_closure[total_rhs] = re->as.fn.func_idx;
                    } else if (re && re->kind == EXPR_IDENT) {
                        size_t c_idx = SIZE_MAX;
                        if (get_var_slot(cg, re->tok.start, re->tok.len, nullptr, nullptr, &c_idx)) {
                            rhs_closure[total_rhs] = c_idx;
                        }
                    }
                    total_rhs++;
                }
            }

            for (size_t n = 0; n < st->as.let_multi.name_count; n++) {
                uint8_t vslot = alloc_slot(cg);
                ShrTypeKind vt = (n < total_rhs) ? (ShrTypeKind)rhs_types[n] : SHR_TY_I32;
                if (n < total_rhs && rhs_slots[n] != vslot) {
                    shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                    shr_bc_writer_emit_u8(&cg->bc, vslot);
                    shr_bc_writer_emit_u8(&cg->bc, rhs_slots[n]);
                }
                if (cg->var_count < 256) {
                    cg->vars[cg->var_count++] = (ShrCgVar){
                        .name = st->as.let_multi.names[n].start,
                        .name_len = st->as.let_multi.names[n].len,
                        .slot = vslot,
                        .type = vt,
                        .closure_func_idx = (n < total_rhs) ? rhs_closure[n] : SIZE_MAX,
                    };
                }
            }
        }
        else if (st->kind == STMT_ASSIGN_MULTI) {
            uint8_t rhs_slots[64] = {0};
            uint8_t total_rhs = 0;
            for (size_t e = 0; e < st->as.assign_multi.value_count; e++) {
                uint8_t tmp[32] = {0};
                uint8_t k = emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.assign_multi.values[e], tmp);
                for (uint8_t j = 0; j < k && total_rhs < 64; j++) {
                    uint8_t snap = alloc_slot(cg);
                    shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                    shr_bc_writer_emit_u8(&cg->bc, snap);
                    shr_bc_writer_emit_u8(&cg->bc, tmp[j]);
                    rhs_slots[total_rhs++] = snap;
                }
            }

            size_t r_idx = 0;
            for (size_t t = 0; t < st->as.assign_multi.target_count; t++) {
                ShrExpr *tgt = st->as.assign_multi.targets[t];
                if (!tgt || r_idx >= total_rhs) continue;
                if (tgt->kind == EXPR_IDENT) {
                    uint8_t dst = 0;
                    if (get_var_slot(cg, tgt->tok.start, tgt->tok.len, &dst, nullptr, nullptr)) {
                        shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                        shr_bc_writer_emit_u8(&cg->bc, dst);
                        shr_bc_writer_emit_u8(&cg->bc, rhs_slots[r_idx++]);
                    }
                } else if (tgt->kind == EXPR_INDEX) {
                    uint8_t obj_s[32] = {0};
                    uint8_t idx_s[32] = {0};
                    emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, tgt->as.index.target, obj_s);
                    emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, tgt->as.index.idx, idx_s);
                    shr_bc_writer_emit_u8(&cg->bc, OP_LIST_SET);
                    shr_bc_writer_emit_u8(&cg->bc, obj_s[0]);
                    shr_bc_writer_emit_u8(&cg->bc, idx_s[0]);
                    shr_bc_writer_emit_u8(&cg->bc, rhs_slots[r_idx++]);
                }
            }
        }
        else if (st->kind == STMT_COMPOUND_ASSIGN) {
            ShrExpr *tgt = st->as.compound_assign.target;
            uint8_t dst = 0;
            ShrTypeKind ty = SHR_TY_I32;
            if (tgt && tgt->kind == EXPR_IDENT &&
                get_var_slot(cg, tgt->tok.start, tgt->tok.len, &dst, &ty, nullptr)) {
                uint8_t rhs[32] = {0};
                emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.compound_assign.value, rhs);
                uint8_t res_slot = alloc_slot(cg);
                uint8_t op = pick_binop(st->as.compound_assign.op, ty);
                shr_bc_writer_emit_u8(&cg->bc, op);
                shr_bc_writer_emit_u8(&cg->bc, res_slot);
                shr_bc_writer_emit_u8(&cg->bc, dst);
                shr_bc_writer_emit_u8(&cg->bc, rhs[0]);
                shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                shr_bc_writer_emit_u8(&cg->bc, dst);
                shr_bc_writer_emit_u8(&cg->bc, res_slot);
            }
        }
        else if (st->kind == STMT_INC_DEC) {
            ShrExpr *tgt = st->as.inc_dec.target;
            uint8_t dst = 0;
            ShrTypeKind ty = SHR_TY_I32;
            if (tgt && tgt->kind == EXPR_IDENT &&
                get_var_slot(cg, tgt->tok.start, tgt->tok.len, &dst, &ty, nullptr)) {
                uint8_t one = alloc_slot(cg);
                shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
                shr_bc_writer_emit_u8(&cg->bc, one);
                shr_bc_writer_emit_i32(&cg->bc, 1);

                uint8_t res_slot = alloc_slot(cg);
                uint8_t op = (st->as.inc_dec.op == TOK_PLUS_PLUS) ? OP_ADD_I32 : OP_SUB_I32;
                shr_bc_writer_emit_u8(&cg->bc, op);
                shr_bc_writer_emit_u8(&cg->bc, res_slot);
                shr_bc_writer_emit_u8(&cg->bc, dst);
                shr_bc_writer_emit_u8(&cg->bc, one);

                shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
                shr_bc_writer_emit_u8(&cg->bc, dst);
                shr_bc_writer_emit_u8(&cg->bc, res_slot);
            }
        }
        else if (st->kind == STMT_FOR_COND) {
            size_t loop_start = cg->bc.code_size;
            size_t exit_op = emit_cond_jump_to_exit(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_cond.cond);
            emit_stmt_block(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_cond.body, st->as.for_cond.body_count);
            emit_jmp_rel_to(cg, loop_start);
            if (exit_op != SIZE_MAX) {
                emit_rel_patch(cg, exit_op, cg->bc.code_size);
            }
        }
        else if (st->kind == STMT_FOR_C) {
            if (st->as.for_c.init) {
                emit_stmt_block(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_c.init, 1);
            }
            size_t loop_start = cg->bc.code_size;
            size_t exit_op = emit_cond_jump_to_exit(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_c.cond);
            emit_stmt_block(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_c.body, st->as.for_c.body_count);
            if (st->as.for_c.post) {
                emit_stmt_block(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_c.post, 1);
            }
            emit_jmp_rel_to(cg, loop_start);
            if (exit_op != SIZE_MAX) {
                emit_rel_patch(cg, exit_op, cg->bc.code_size);
            }
        }
        else if (st->kind == STMT_FOR_IN) {
            uint8_t idx_slot = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
            shr_bc_writer_emit_u8(&cg->bc, idx_slot);
            shr_bc_writer_emit_i32(&cg->bc, 0);

            int32_t total_len = 0;
            if (st->as.for_in.iterable && st->as.for_in.iterable->kind == EXPR_STRING) {
                ShrExpr *ie = st->as.for_in.iterable;
                total_len = (ie->tok.len >= 2) ? (int32_t)(ie->tok.len - 2) : 0;
            }
            uint8_t len_slot = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
            shr_bc_writer_emit_u8(&cg->bc, len_slot);
            shr_bc_writer_emit_i32(&cg->bc, total_len);

            uint8_t item_slot = alloc_slot(cg);
            if (cg->var_count < 256) {
                cg->vars[cg->var_count++] = (ShrCgVar){
                    .name = st->as.for_in.item_name.start,
                    .name_len = st->as.for_in.item_name.len,
                    .slot = item_slot,
                    .type = SHR_TY_I32,
                    .closure_func_idx = SIZE_MAX,
                };
            }

            size_t loop_start = cg->bc.code_size;
            size_t exit_op = cg->bc.code_size;
            shr_bc_writer_emit_u8(&cg->bc, OP_JGE_I32);
            shr_bc_writer_emit_u8(&cg->bc, idx_slot);
            shr_bc_writer_emit_u8(&cg->bc, len_slot);
            shr_bc_writer_emit_u16(&cg->bc, 0);

            shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
            shr_bc_writer_emit_u8(&cg->bc, item_slot);
            shr_bc_writer_emit_u8(&cg->bc, idx_slot);

            emit_stmt_block(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.for_in.body, st->as.for_in.body_count);

            uint8_t one = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_MOV_I32);
            shr_bc_writer_emit_u8(&cg->bc, one);
            shr_bc_writer_emit_i32(&cg->bc, 1);
            uint8_t next_idx = alloc_slot(cg);
            shr_bc_writer_emit_u8(&cg->bc, OP_ADD_I32);
            shr_bc_writer_emit_u8(&cg->bc, next_idx);
            shr_bc_writer_emit_u8(&cg->bc, idx_slot);
            shr_bc_writer_emit_u8(&cg->bc, one);
            shr_bc_writer_emit_u8(&cg->bc, OP_COPY_SLOT);
            shr_bc_writer_emit_u8(&cg->bc, idx_slot);
            shr_bc_writer_emit_u8(&cg->bc, next_idx);

            emit_jmp_rel_to(cg, loop_start);
            emit_rel_patch(cg, exit_op, cg->bc.code_size);
        }
        else if (st->kind == STMT_RETURN_MULTI) {
            uint8_t ret_slots[64] = {0};
            uint8_t ret_types[64] = {0};
            uint8_t total_ret = 0;

            for (size_t e = 0; e < st->as.ret_multi.expr_count; e++) {
                ShrExpr *re = st->as.ret_multi.exprs[e];
                uint8_t tmp[32] = {0};
                uint8_t k = emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, re, tmp);
                for (uint8_t j = 0; j < k && total_ret < 64; j++) {
                    ret_slots[total_ret] = tmp[j];
                    ret_types[total_ret] = (re && re->inferred_types.count > j && re->inferred_types.items)
                                             ? (uint8_t)re->inferred_types.items[j]
                                             : (uint8_t)SHR_TY_I32;
                    total_ret++;
                }
            }

            if (total_ret == 0) {
                shr_bc_writer_emit_u8(&cg->bc, OP_RET_VOID);
            } else {
                shr_bc_writer_emit_u8(&cg->bc, OP_RET_MULTI);
                shr_bc_writer_emit_u8(&cg->bc, total_ret);
                for (uint8_t r = 0; r < total_ret; r++) {
                    shr_bc_writer_emit_u8(&cg->bc, ret_slots[r]);
                    shr_bc_writer_emit_u8(&cg->bc, ret_types[r]);
                }
            }
        }
        else if (st->kind == STMT_EXPR) {
            uint8_t tmp[32] = {0};
            emit_expr_slots(cg, prog, fn_base, cl_base, cur_fn_idx, cur_is_closure, st->as.expr.expr, tmp);
        }
    }
}

bool shr_codegen_emit_program(ShrCodegen *cg, const ShrProgramAst *prog) {
    if (!cg || !prog) return false;
    for (size_t i = 0; i < prog->use_count; i++) {
        shr_bc_writer_add_module(&cg->bc, prog->uses[i].start, prog->uses[i].len, 0);
    }

    uint16_t script_offset = (prog->toplevel_stmt_count > 0) ? 1u : 0u;
    uint16_t fn_base = script_offset;
    uint16_t cl_base = (uint16_t)(script_offset + prog->func_count);

    if (prog->toplevel_stmt_count > 0) {
        cg->var_count = 0;
        cg->next_slot = 0;
        uint16_t name_off = shr_bc_writer_add_rodata(&cg->bc, "<script>", 8);
        uint16_t code_off = (uint16_t)cg->bc.code_size;

        emit_stmt_block(cg, prog, fn_base, cl_base, SIZE_MAX, false, prog->toplevel_stmts, prog->toplevel_stmt_count);
        shr_bc_writer_emit_u8(&cg->bc, OP_RET_VOID);

        uint16_t code_sz = (uint16_t)(cg->bc.code_size - code_off);
        shr_bc_writer_add_func(&cg->bc, (ShrBcFuncEntry){
            .name_off = name_off,
            .name_len = 8,
            .code_offset = code_off,
            .code_size = code_sz,
            .param_count = 0,
            .slot_count = cg->next_slot ? cg->next_slot : 1,
            .ret_count = 0,
            .flags = 0,
        });
    }

    for (size_t i = 0; i < prog->func_count; i++) {
        const ShrFuncDecl *fn = &prog->funcs[i];
        cg->var_count = 0;
        cg->next_slot = 0;

        for (size_t p = 0; p < fn->param_count; p++) {
            uint8_t pslot = alloc_slot(cg);
            size_t c_idx = SIZE_MAX;
            if (fn->params[p].spec.kind == SHR_TY_FN && prog) {
                size_t want_params = fn->params[p].spec.fn_params ? fn->params[p].spec.fn_params->count : 0;
                for (size_t c = 0; c < prog->closure_count; c++) {
                    if (prog->closures[c].user_param_count == want_params) {
                        c_idx = c;
                        break;
                    }
                }
            }
            if (cg->var_count < 256) {
                cg->vars[cg->var_count++] = (ShrCgVar){
                    .name = fn->params[p].name.start,
                    .name_len = fn->params[p].name.len,
                    .slot = pslot,
                    .type = fn->params[p].type,
                    .closure_func_idx = c_idx,
                };
            }
        }

        uint16_t name_off = shr_bc_writer_add_rodata(&cg->bc, fn->name.start, fn->name.len);
        uint16_t code_off = (uint16_t)cg->bc.code_size;

        emit_stmt_block(cg, prog, fn_base, cl_base, i, false, fn->stmts, fn->stmt_count);
        shr_bc_writer_emit_u8(&cg->bc, OP_RET_VOID);

        uint16_t code_sz = (uint16_t)(cg->bc.code_size - code_off);
        shr_bc_writer_add_func(&cg->bc, (ShrBcFuncEntry){
            .name_off = name_off,
            .name_len = (uint16_t)fn->name.len,
            .code_offset = code_off,
            .code_size = code_sz,
            .param_count = (uint8_t)fn->param_count,
            .slot_count = cg->next_slot ? cg->next_slot : 1,
            .ret_count = (uint8_t)fn->ret_types.count,
            .flags = 0,
        });
    }

    for (size_t i = 0; i < prog->closure_count; i++) {
        const ShrFuncDecl *cl = &prog->closures[i];
        cg->var_count = 0;
        cg->next_slot = 0;

        for (size_t c = 0; c < cl->capture_count; c++) {
            uint8_t cslot = alloc_slot(cg);
            if (cg->var_count < 256) {
                cg->vars[cg->var_count++] = (ShrCgVar){
                    .name = cl->captures[c].name.start,
                    .name_len = cl->captures[c].name.len,
                    .slot = cslot,
                    .type = cl->captures[c].type,
                    .closure_func_idx = SIZE_MAX,
                };
            }
        }

        for (size_t p = 0; p < cl->user_param_count; p++) {
            uint8_t pslot = alloc_slot(cg);
            if (cg->var_count < 256) {
                cg->vars[cg->var_count++] = (ShrCgVar){
                    .name = cl->params[cl->capture_count + p].name.start,
                    .name_len = cl->params[cl->capture_count + p].name.len,
                    .slot = pslot,
                    .type = cl->params[cl->capture_count + p].type,
                    .closure_func_idx = SIZE_MAX,
                };
            }
        }

        uint16_t name_off = shr_bc_writer_add_rodata(&cg->bc, cl->name.start, cl->name.len);
        uint16_t code_off = (uint16_t)cg->bc.code_size;

        emit_stmt_block(cg, prog, fn_base, cl_base, i, true, cl->stmts, cl->stmt_count);
        shr_bc_writer_emit_u8(&cg->bc, OP_RET_VOID);

        uint16_t code_sz = (uint16_t)(cg->bc.code_size - code_off);
        shr_bc_writer_add_func(&cg->bc, (ShrBcFuncEntry){
            .name_off = name_off,
            .name_len = (uint16_t)cl->name.len,
            .code_offset = code_off,
            .code_size = code_sz,
            .param_count = (uint8_t)(cl->capture_count + cl->user_param_count),
            .slot_count = cg->next_slot ? cg->next_slot : 1,
            .ret_count = (uint8_t)cl->ret_types.count,
            .flags = 0,
        });
    }

    return true;
}

void shr_codegen_free(ShrCodegen *cg) {
    shr_bc_writer_free(&cg->bc);
}
