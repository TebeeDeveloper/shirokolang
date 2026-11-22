/* shrkc/sema.c */
#include "sema.h"
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void shr_sema_init(ShrSema *s) {
    memset(s, 0, sizeof(*s));
    s->ok = true;
}

void shr_sema_fail(ShrSema *s, int line, const char *fmt, ...) {
    if (!s) return;
    s->ok = false;
    if (s->err_msg[0] == '\0') {
        s->err_line = line;
        va_list args;
        va_start(args, fmt);
        vsnprintf(s->err_msg, sizeof(s->err_msg), fmt, args);
        va_end(args);
    }
}

static int find_var(const ShrSema *s, const char *name, size_t len) {
    for (size_t i = 0; i < s->var_count; i++) {
        if (s->vars[i].name_len == len && memcmp(s->vars[i].name, name, len) == 0) {
            return (int)i;
        }
    }
    return -1;
}

static const ShrFuncSym *find_func(const ShrSema *s, const char *name, size_t len) {
    for (size_t i = 0; i < s->func_count; i++) {
        if (s->funcs[i].name_len == len && memcmp(s->funcs[i].name, name, len) == 0) {
            return &s->funcs[i];
        }
    }
    return nullptr;
}

static size_t find_closure_in_stmts(const ShrStmt *stmts, size_t count, const char *name, size_t len) {
    if (!stmts || !name || len == 0) return SIZE_MAX;
    for (size_t i = 0; i < count; i++) {
        const ShrStmt *st = &stmts[i];
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
            size_t r = find_closure_in_stmts(st->as.for_cond.body, st->as.for_cond.body_count, name, len);
            if (r != SIZE_MAX) return r;
        } else if (st->kind == STMT_FOR_C) {
            if (st->as.for_c.init) {
                size_t r = find_closure_in_stmts(st->as.for_c.init, 1, name, len);
                if (r != SIZE_MAX) return r;
            }
            size_t r = find_closure_in_stmts(st->as.for_c.body, st->as.for_c.body_count, name, len);
            if (r != SIZE_MAX) return r;
        } else if (st->kind == STMT_FOR_IN) {
            size_t r = find_closure_in_stmts(st->as.for_in.body, st->as.for_in.body_count, name, len);
            if (r != SIZE_MAX) return r;
        }
    }
    return SIZE_MAX;
}

static size_t find_closure_idx_by_name(const ShrProgramAst *prog, const char *name, size_t len) {
    if (!prog || prog->closure_count == 0) return SIZE_MAX;
    size_t idx = find_closure_in_stmts(prog->toplevel_stmts, prog->toplevel_stmt_count, name, len);
    if (idx != SIZE_MAX) return idx;
    for (size_t f = 0; f < prog->func_count; f++) {
        idx = find_closure_in_stmts(prog->funcs[f].stmts, prog->funcs[f].stmt_count, name, len);
        if (idx != SIZE_MAX) return idx;
    }
    return SIZE_MAX;
}

static ShrTypeList make_single_type(ShrTypeKind t) {
    ShrTypeList res = {0};
    res.items = malloc(sizeof(ShrTypeKind));
    res.specs = calloc(1, sizeof(ShrTypeSpec));
    if (!res.items || !res.specs) {
        free(res.items);
        free(res.specs);
        res.items = nullptr;
        res.specs = nullptr;
        return res;
    }
    res.items[0] = t;
    res.specs[0].kind = t;
    res.count = 1;
    return res;
}

static ShrTypeList make_single_spec(ShrTypeSpec sp) {
    ShrTypeList res = {0};
    res.items = malloc(sizeof(ShrTypeKind));
    res.specs = calloc(1, sizeof(ShrTypeSpec));
    if (!res.items || !res.specs) {
        free(res.items);
        free(res.specs);
        res.items = nullptr;
        res.specs = nullptr;
        return res;
    }
    res.items[0] = sp.kind;
    res.specs[0] = sp;
    res.count = 1;
    return res;
}

static ShrTypeKind promote_bin_type(ShrTypeKind a, ShrTypeKind b) {
    if (a == SHR_TY_F64 || b == SHR_TY_F64) return SHR_TY_F64;
    if (a == SHR_TY_I64 || b == SHR_TY_I64) return SHR_TY_I64;
    if (a == SHR_TY_U64 || b == SHR_TY_U64) return SHR_TY_U64;
    if (a == SHR_TY_F32 || b == SHR_TY_F32) return SHR_TY_F32;
    if (a == SHR_TY_U32 || b == SHR_TY_U32) return SHR_TY_U32;
    return SHR_TY_I32;
}

static bool expr_uses_ident(const ShrExpr *e, const char *name, size_t len) {
    if (!e) return false;
    if (e->kind == EXPR_IDENT) {
        return e->tok.len == len && memcmp(e->tok.start, name, len) == 0;
    }
    if (e->kind == EXPR_MEMBER) {
        return expr_uses_ident(e->as.member.obj, name, len);
    }
    if (e->kind == EXPR_INDEX) {
        return expr_uses_ident(e->as.index.target, name, len) ||
               expr_uses_ident(e->as.index.idx, name, len);
    }
    if (e->kind == EXPR_LIST) {
        for (size_t i = 0; i < e->as.list.item_count; i++) {
            if (expr_uses_ident(e->as.list.items[i], name, len)) return true;
        }
    }
    if (e->kind == EXPR_BINARY) {
        return expr_uses_ident(e->as.binary.lhs, name, len) ||
               expr_uses_ident(e->as.binary.rhs, name, len);
    }
    if (e->kind == EXPR_CALL) {
        if (expr_uses_ident(e->as.call.callee, name, len)) return true;
        for (size_t i = 0; i < e->as.call.arg_count; i++) {
            if (expr_uses_ident(e->as.call.args[i], name, len)) return true;
        }
    }
    return false;
}

static bool stmts_use_ident(const ShrStmt *stmts, size_t count, const char *name, size_t len) {
    for (size_t i = 0; i < count; i++) {
        const ShrStmt *st = &stmts[i];
        if (st->kind == STMT_LET_MULTI) {
            for (size_t k = 0; k < st->as.let_multi.expr_count; k++) {
                if (expr_uses_ident(st->as.let_multi.exprs[k], name, len)) return true;
            }
        } else if (st->kind == STMT_ASSIGN_MULTI) {
            for (size_t k = 0; k < st->as.assign_multi.target_count; k++) {
                if (expr_uses_ident(st->as.assign_multi.targets[k], name, len)) return true;
            }
            for (size_t k = 0; k < st->as.assign_multi.value_count; k++) {
                if (expr_uses_ident(st->as.assign_multi.values[k], name, len)) return true;
            }
        } else if (st->kind == STMT_COMPOUND_ASSIGN) {
            if (expr_uses_ident(st->as.compound_assign.target, name, len) ||
                expr_uses_ident(st->as.compound_assign.value, name, len)) return true;
        } else if (st->kind == STMT_INC_DEC) {
            if (expr_uses_ident(st->as.inc_dec.target, name, len)) return true;
        } else if (st->kind == STMT_RETURN_MULTI) {
            for (size_t k = 0; k < st->as.ret_multi.expr_count; k++) {
                if (expr_uses_ident(st->as.ret_multi.exprs[k], name, len)) return true;
            }
        } else if (st->kind == STMT_EXPR) {
            if (expr_uses_ident(st->as.expr.expr, name, len)) return true;
        } else if (st->kind == STMT_FOR_COND) {
            if (expr_uses_ident(st->as.for_cond.cond, name, len) ||
                stmts_use_ident(st->as.for_cond.body, st->as.for_cond.body_count, name, len)) return true;
        } else if (st->kind == STMT_FOR_C) {
            if ((st->as.for_c.init && stmts_use_ident(st->as.for_c.init, 1, name, len)) ||
                expr_uses_ident(st->as.for_c.cond, name, len) ||
                (st->as.for_c.post && stmts_use_ident(st->as.for_c.post, 1, name, len)) ||
                stmts_use_ident(st->as.for_c.body, st->as.for_c.body_count, name, len)) return true;
        } else if (st->kind == STMT_FOR_IN) {
            if (expr_uses_ident(st->as.for_in.iterable, name, len) ||
                stmts_use_ident(st->as.for_in.body, st->as.for_in.body_count, name, len)) return true;
        }
    }
    return false;
}

static void analyze_func_decl(ShrSema *s, ShrProgramAst *prog, ShrFuncDecl *fn);
static void analyze_stmts_in_scope(ShrSema *s, ShrProgramAst *prog, ShrStmt *stmts, size_t stmt_count, bool reset_vars);

static ShrTypeList infer_expr_types(ShrSema *s, ShrProgramAst *prog, ShrExpr *e) {
    ShrTypeList res = {0};
    if (!e) return res;
    if (e->inferred_types.count > 0 && e->inferred_types.items) {
        res.count = e->inferred_types.count;
        res.items = malloc(res.count * sizeof(ShrTypeKind));
        res.specs = calloc(res.count, sizeof(ShrTypeSpec));
        if (res.items && res.specs) {
            memcpy(res.items, e->inferred_types.items, res.count * sizeof(ShrTypeKind));
            if (e->inferred_types.specs) {
                memcpy(res.specs, e->inferred_types.specs, res.count * sizeof(ShrTypeSpec));
            }
            return res;
        }
        free(res.items);
        free(res.specs);
        res.items = nullptr;
        res.specs = nullptr;
        res.count = 0;
        return res;
    }

    switch (e->kind) {
        case EXPR_INT:
            res = make_single_type(SHR_TY_I32);
            break;
        case EXPR_FLOAT:
            res = make_single_type(SHR_TY_F64);
            break;
        case EXPR_BOOL:
            res = make_single_type(SHR_TY_BOOL);
            break;
        case EXPR_STRING:
            res = make_single_type(SHR_TY_STRING);
            break;
        case EXPR_LIST: {
            for (size_t i = 0; i < e->as.list.item_count; i++) {
                ShrTypeList it = infer_expr_types(s, prog, e->as.list.items[i]);
                free(it.items);
                free(it.specs);
            }
            ShrTypeSpec sp = {0};
            sp.kind = SHR_TY_LIST;
            sp.elem_type = e->as.list.elem_type;
            res = make_single_spec(sp);
            break;
        }
        case EXPR_INDEX: {
            ShrTypeList obj_t = infer_expr_types(s, prog, e->as.index.target);
            ShrTypeList idx_t = infer_expr_types(s, prog, e->as.index.idx);
            ShrTypeKind elem_k = SHR_TY_I32;
            if (obj_t.count > 0 && obj_t.items) {
                if (obj_t.items[0] == SHR_TY_LIST && obj_t.specs) {
                    elem_k = obj_t.specs[0].elem_type ? obj_t.specs[0].elem_type : SHR_TY_I32;
                } else if (obj_t.items[0] == SHR_TY_STRING) {
                    elem_k = SHR_TY_U8;
                } else {
                    shr_sema_fail(s, e->tok.line, "cannot index non-list expression");
                }
            }
            free(obj_t.items); free(obj_t.specs);
            free(idx_t.items); free(idx_t.specs);
            res = make_single_type(elem_k);
            break;
        }
        case EXPR_FN: {
            size_t cidx = e->as.fn.func_idx;
            if (prog && cidx < prog->closure_count) {
                ShrFuncDecl *cfn = &prog->closures[cidx];
                free(cfn->captures);
                cfn->captures = nullptr;
                cfn->capture_count = 0;

                ShrVarSym outer_vars[256];
                size_t outer_count = s->var_count;
                if (outer_count > 0) {
                    memcpy(outer_vars, s->vars, outer_count * sizeof(ShrVarSym));
                }

                size_t cap_cap = 8;
                cfn->captures = malloc(cap_cap * sizeof(ShrCaptureDecl));

                for (size_t ov = 0; ov < outer_count; ov++) {
                    bool is_param = false;
                    for (size_t up = 0; up < cfn->user_param_count; up++) {
                        if (cfn->params[up].name.len == outer_vars[ov].name_len &&
                            memcmp(cfn->params[up].name.start, outer_vars[ov].name, outer_vars[ov].name_len) == 0) {
                            is_param = true;
                            break;
                        }
                    }
                    if (!is_param && cfn->captures) {
                        bool used = stmts_use_ident(cfn->stmts, cfn->stmt_count,
                                                    outer_vars[ov].name, outer_vars[ov].name_len);
                        if (used) {
                            if (cfn->capture_count == cap_cap) {
                                cap_cap *= 2;
                                ShrCaptureDecl *tmp = realloc(cfn->captures, cap_cap * sizeof(ShrCaptureDecl));
                                if (!tmp) {
                                    shr_sema_fail(s, e->tok.line, "out of memory while collecting captures");
                                    break;
                                }
                                cfn->captures = tmp;
                            }
                            cfn->captures[cfn->capture_count++] = (ShrCaptureDecl){
                                .name = (ShrToken){
                                    .kind = TOK_IDENT,
                                    .start = outer_vars[ov].name,
                                    .len = outer_vars[ov].name_len,
                                    .line = e->tok.line,
                                },
                                .type = outer_vars[ov].type,
                                .spec = outer_vars[ov].spec,
                            };
                        }
                    }
                }

                size_t total_p = cfn->capture_count + cfn->user_param_count;
                ShrParamDecl *new_params = calloc(total_p ? total_p : 1, sizeof(ShrParamDecl));
                if (new_params) {
                    for (size_t c = 0; c < cfn->capture_count; c++) {
                        new_params[c] = (ShrParamDecl){
                            .name = cfn->captures[c].name,
                            .type = cfn->captures[c].type,
                            .spec = cfn->captures[c].spec,
                        };
                    }
                    for (size_t u = 0; u < cfn->user_param_count; u++) {
                        new_params[cfn->capture_count + u] = cfn->params[u];
                    }
                    free(cfn->params);
                    cfn->params = new_params;
                    cfn->param_count = total_p;
                }

                ShrSema sub = *s;
                analyze_func_decl(&sub, prog, cfn);
                if (!sub.ok) {
                    shr_sema_fail(s, sub.err_line ? sub.err_line : e->tok.line, "%s",
                                  sub.err_msg[0] ? sub.err_msg : "error inside closure");
                }
                if (outer_count > 0) {
                    memcpy(s->vars, outer_vars, outer_count * sizeof(ShrVarSym));
                }
                s->var_count = outer_count;
            }
            res = make_single_type(SHR_TY_FN);
            break;
        }
        case EXPR_IDENT: {
            int idx = find_var(s, e->tok.start, e->tok.len);
            if (idx < 0) {
                shr_sema_fail(s, e->tok.line, "undefined variable '%.*s'",
                              (int)e->tok.len, e->tok.start);
                res = make_single_type(SHR_TY_I32);
                break;
            }
            res = make_single_spec(s->vars[idx].spec);
            break;
        }
        case EXPR_BINARY: {
            ShrTypeList lt = infer_expr_types(s, prog, e->as.binary.lhs);
            ShrTypeList rt = infer_expr_types(s, prog, e->as.binary.rhs);
            ShrTypeKind lk = (lt.count > 0) ? lt.items[0] : SHR_TY_I32;
            ShrTypeKind rk = (rt.count > 0) ? rt.items[0] : SHR_TY_I32;
            ShrTypeKind k = promote_bin_type(lk, rk);
            free(lt.items); free(lt.specs);
            free(rt.items); free(rt.specs);
            res = make_single_type(k);
            break;
        }
        case EXPR_CALL: {
            for (size_t i = 0; i < e->as.call.arg_count; i++) {
                ShrTypeList at = infer_expr_types(s, prog, e->as.call.args[i]);
                free(at.items); free(at.specs);
            }
            if (e->as.call.callee && e->as.call.callee->kind == EXPR_IDENT) {
                size_t cidx = SIZE_MAX;
                int vidx = find_var(s, e->as.call.callee->tok.start, e->as.call.callee->tok.len);
                if (vidx >= 0) {
                    cidx = s->vars[vidx].closure_func_idx;
                    if (cidx == SIZE_MAX) {
                        cidx = find_closure_idx_by_name(prog, e->as.call.callee->tok.start, e->as.call.callee->tok.len);
                    }
                    if (prog && cidx < prog->closure_count) {
                        const ShrTypeList *cr = &prog->closures[cidx].ret_types;
                        if (cr->count > 0 && cr->items) {
                            res.count = cr->count;
                            res.items = malloc(res.count * sizeof(ShrTypeKind));
                            res.specs = calloc(res.count, sizeof(ShrTypeSpec));
                            if (res.items && res.specs) {
                                memcpy(res.items, cr->items, res.count * sizeof(ShrTypeKind));
                                break;
                            }
                            free(res.items); free(res.specs);
                            res.items = nullptr; res.specs = nullptr; res.count = 0;
                        }
                    }
                    if (s->vars[vidx].spec.kind == SHR_TY_FN && s->vars[vidx].spec.fn_rets &&
                        s->vars[vidx].spec.fn_rets->count > 0 && s->vars[vidx].spec.fn_rets->items) {
                        ShrTypeList *fr = s->vars[vidx].spec.fn_rets;
                        res.count = fr->count;
                        res.items = malloc(res.count * sizeof(ShrTypeKind));
                        res.specs = calloc(res.count, sizeof(ShrTypeSpec));
                        if (res.items && res.specs) {
                            memcpy(res.items, fr->items, res.count * sizeof(ShrTypeKind));
                            break;
                        }
                        free(res.items); free(res.specs);
                        res.items = nullptr; res.specs = nullptr; res.count = 0;
                    }
                } else {
                    cidx = find_closure_idx_by_name(prog, e->as.call.callee->tok.start, e->as.call.callee->tok.len);
                    if (prog && cidx < prog->closure_count) {
                        const ShrTypeList *cr = &prog->closures[cidx].ret_types;
                        if (cr->count > 0 && cr->items) {
                            res.count = cr->count;
                            res.items = malloc(res.count * sizeof(ShrTypeKind));
                            res.specs = calloc(res.count, sizeof(ShrTypeSpec));
                            if (res.items && res.specs) {
                                memcpy(res.items, cr->items, res.count * sizeof(ShrTypeKind));
                                break;
                            }
                            free(res.items); free(res.specs);
                            res.items = nullptr; res.specs = nullptr; res.count = 0;
                        }
                    }
                }
                const ShrFuncSym *fs = find_func(s, e->as.call.callee->tok.start, e->as.call.callee->tok.len);
                if (fs && fs->ret_types.count > 0 && fs->ret_types.items) {
                    res.count = fs->ret_types.count;
                    res.items = malloc(res.count * sizeof(ShrTypeKind));
                    res.specs = calloc(res.count, sizeof(ShrTypeSpec));
                    if (res.items && res.specs) {
                        memcpy(res.items, fs->ret_types.items, res.count * sizeof(ShrTypeKind));
                        break;
                    }
                    free(res.items); free(res.specs);
                    res.items = nullptr; res.specs = nullptr; res.count = 0;
                }
            } else if (e->as.call.callee && e->as.call.callee->kind == EXPR_MEMBER) {
                char sym_buf[128];
                ShrExpr *obj = e->as.call.callee->as.member.obj;
                ShrToken mem = e->as.call.callee->as.member.member;
                if (obj && obj->kind == EXPR_IDENT && obj->tok.len + 1 + mem.len < sizeof(sym_buf)) {
                    size_t p = 0;
                    memcpy(sym_buf + p, obj->tok.start, obj->tok.len);
                    p += obj->tok.len;
                    sym_buf[p++] = '_';
                    memcpy(sym_buf + p, mem.start, mem.len);
                    p += mem.len;
                    sym_buf[p] = '\0';
                    const ShrFuncSym *fs = find_func(s, sym_buf, p);
                    if (fs && fs->ret_types.count > 0 && fs->ret_types.items) {
                        res.count = fs->ret_types.count;
                        res.items = malloc(res.count * sizeof(ShrTypeKind));
                        res.specs = calloc(res.count, sizeof(ShrTypeSpec));
                        if (res.items && res.specs) {
                            memcpy(res.items, fs->ret_types.items, res.count * sizeof(ShrTypeKind));
                            break;
                        }
                        free(res.items); free(res.specs);
                        res.items = nullptr; res.specs = nullptr; res.count = 0;
                    }
                }
                res = make_single_type(SHR_TY_VOID);
                break;
            }
            res = make_single_type(SHR_TY_VOID);
            break;
        }
        case EXPR_MEMBER:
            res = make_single_type(SHR_TY_I32);
            break;
    }

    if (res.count > 0 && res.items && res.specs) {
        free(e->inferred_types.items);
        free(e->inferred_types.specs);
        e->inferred_types.count = res.count;
        e->inferred_types.items = malloc(res.count * sizeof(ShrTypeKind));
        e->inferred_types.specs = calloc(res.count, sizeof(ShrTypeSpec));
        if (e->inferred_types.items && e->inferred_types.specs) {
            memcpy(e->inferred_types.items, res.items, res.count * sizeof(ShrTypeKind));
            memcpy(e->inferred_types.specs, res.specs, res.count * sizeof(ShrTypeSpec));
        } else {
            free(e->inferred_types.items);
            free(e->inferred_types.specs);
            e->inferred_types.items = nullptr;
            e->inferred_types.specs = nullptr;
            e->inferred_types.count = 0;
        }
    }
    return res;
}

static size_t expand_rhs_types(ShrSema *s, ShrProgramAst *prog, ShrExpr **exprs, size_t expr_count,
                               ShrTypeKind **out_types, ShrTypeSpec **out_specs) {
    size_t cap = 16, total = 0;
    ShrTypeKind *arr = malloc(cap * sizeof(ShrTypeKind));
    ShrTypeSpec *sarr = calloc(cap, sizeof(ShrTypeSpec));
    if (!arr || !sarr) {
        free(arr); free(sarr);
        *out_types = nullptr;
        if (out_specs) *out_specs = nullptr;
        shr_sema_fail(s, 0, "out of memory while expanding expression types");
        return 0;
    }
    for (size_t i = 0; i < expr_count; i++) {
        ShrTypeList tl = infer_expr_types(s, prog, exprs[i]);
        for (size_t j = 0; j < tl.count; j++) {
            if (total == cap) {
                cap *= 2;
                ShrTypeKind *tmp = realloc(arr, cap * sizeof(ShrTypeKind));
                ShrTypeSpec *stmp = realloc(sarr, cap * sizeof(ShrTypeSpec));
                if (!tmp || !stmp) {
                    free(tmp ? tmp : arr);
                    free(stmp ? stmp : sarr);
                    free(tl.items); free(tl.specs);
                    *out_types = nullptr;
                    if (out_specs) *out_specs = nullptr;
                    shr_sema_fail(s, exprs[i] ? exprs[i]->tok.line : 0,
                                  "out of memory while expanding expression types");
                    return 0;
                }
                arr = tmp;
                sarr = stmp;
            }
            arr[total] = tl.items[j];
            sarr[total] = (tl.specs) ? tl.specs[j] : (ShrTypeSpec){ .kind = tl.items[j] };
            total++;
        }
        free(tl.items); free(tl.specs);
    }
    *out_types = arr;
    if (out_specs) *out_specs = sarr;
    else free(sarr);
    return total;
}

static void analyze_stmts_in_scope(ShrSema *s, ShrProgramAst *prog, ShrStmt *stmts, size_t stmt_count, bool reset_vars) {
    if (reset_vars) {
        s->var_count = 0;
    }

    for (size_t j = 0; j < stmt_count; j++) {
        ShrStmt *st = &stmts[j];
        if (st->kind == STMT_LET_MULTI) {
            ShrTypeKind *rhs_t = nullptr;
            ShrTypeSpec *rhs_s = nullptr;
            size_t rhs_n = expand_rhs_types(s, prog, st->as.let_multi.exprs, st->as.let_multi.expr_count, &rhs_t, &rhs_s);
            int ln = (st->as.let_multi.name_count > 0) ? st->as.let_multi.names[0].line : 0;
            if (st->as.let_multi.expr_count > 0 && rhs_n != st->as.let_multi.name_count) {
                shr_sema_fail(s, ln, "let count mismatch: expected %zu value(s), got %zu",
                              st->as.let_multi.name_count, rhs_n);
            }
            for (size_t k = 0; k < st->as.let_multi.name_count && s->var_count < 256; k++) {
                ShrTypeKind tk = (k < rhs_n && rhs_t) ? rhs_t[k] : SHR_TY_I32;
                size_t c_idx = SIZE_MAX;
                ShrTypeSpec sp = (k < rhs_n && rhs_s) ? rhs_s[k] : (ShrTypeSpec){ .kind = tk };
                if (k < st->as.let_multi.expr_count && st->as.let_multi.exprs[k]) {
                    if (st->as.let_multi.exprs[k]->kind == EXPR_FN) {
                        c_idx = st->as.let_multi.exprs[k]->as.fn.func_idx;
                    } else if (st->as.let_multi.exprs[k]->kind == EXPR_IDENT) {
                        int src_v = find_var(s, st->as.let_multi.exprs[k]->tok.start, st->as.let_multi.exprs[k]->tok.len);
                        if (src_v >= 0) {
                            c_idx = s->vars[src_v].closure_func_idx;
                            sp = s->vars[src_v].spec;
                        }
                    }
                }
                size_t v_idx = s->var_count++;
                s->vars[v_idx] = (ShrVarSym){
                    .name = st->as.let_multi.names[k].start,
                    .name_len = st->as.let_multi.names[k].len,
                    .slot = (uint8_t)v_idx,
                    .type = tk,
                    .spec = sp,
                    .closure_func_idx = c_idx,
                };
            }
            free(rhs_t);
            free(rhs_s);
        } else if (st->kind == STMT_ASSIGN_MULTI) {
            ShrTypeKind *rhs_t = nullptr;
            size_t rhs_n = expand_rhs_types(s, prog, st->as.assign_multi.values, st->as.assign_multi.value_count, &rhs_t, nullptr);
            int ln = (st->as.assign_multi.target_count > 0 && st->as.assign_multi.targets[0])
                         ? st->as.assign_multi.targets[0]->tok.line : 0;
            if (rhs_n != st->as.assign_multi.target_count) {
                shr_sema_fail(s, ln, "assign count mismatch: expected %zu value(s), got %zu",
                              st->as.assign_multi.target_count, rhs_n);
            }
            for (size_t k = 0; k < st->as.assign_multi.target_count; k++) {
                ShrTypeList tt = infer_expr_types(s, prog, st->as.assign_multi.targets[k]);
                free(tt.items); free(tt.specs);
            }
            free(rhs_t);
        } else if (st->kind == STMT_COMPOUND_ASSIGN) {
            ShrTypeList tt = infer_expr_types(s, prog, st->as.compound_assign.target);
            ShrTypeList vt = infer_expr_types(s, prog, st->as.compound_assign.value);
            free(tt.items); free(tt.specs);
            free(vt.items); free(vt.specs);
        } else if (st->kind == STMT_INC_DEC) {
            ShrTypeList tt = infer_expr_types(s, prog, st->as.inc_dec.target);
            free(tt.items); free(tt.specs);
        } else if (st->kind == STMT_RETURN_MULTI) {
            ShrTypeKind *rhs_t = nullptr;
            size_t rhs_n = expand_rhs_types(s, prog, st->as.ret_multi.exprs, st->as.ret_multi.expr_count, &rhs_t, nullptr);
            int ln = (st->as.ret_multi.expr_count > 0 && st->as.ret_multi.exprs[0])
                         ? st->as.ret_multi.exprs[0]->tok.line : 0;
            if (s->cur_ret_types.count == 1 && s->cur_ret_types.items && s->cur_ret_types.items[0] == SHR_TY_VOID) {
                if (rhs_n > 0) {
                    shr_sema_fail(s, ln, "void function cannot return %zu value(s)", rhs_n);
                }
            } else if (rhs_n != s->cur_ret_types.count) {
                shr_sema_fail(s, ln, "return count mismatch: expected %zu, got %zu",
                              s->cur_ret_types.count, rhs_n);
            }
            free(rhs_t);
        } else if (st->kind == STMT_EXPR) {
            ShrTypeList et = infer_expr_types(s, prog, st->as.expr.expr);
            free(et.items); free(et.specs);
        } else if (st->kind == STMT_FOR_COND) {
            if (st->as.for_cond.cond) {
                ShrTypeList ct = infer_expr_types(s, prog, st->as.for_cond.cond);
                free(ct.items); free(ct.specs);
            }
            analyze_stmts_in_scope(s, prog, st->as.for_cond.body, st->as.for_cond.body_count, false);
        } else if (st->kind == STMT_FOR_C) {
            if (st->as.for_c.init) {
                analyze_stmts_in_scope(s, prog, st->as.for_c.init, 1, false);
            }
            if (st->as.for_c.cond) {
                ShrTypeList ct = infer_expr_types(s, prog, st->as.for_c.cond);
                free(ct.items); free(ct.specs);
            }
            if (st->as.for_c.post) {
                analyze_stmts_in_scope(s, prog, st->as.for_c.post, 1, false);
            }
            analyze_stmts_in_scope(s, prog, st->as.for_c.body, st->as.for_c.body_count, false);
        } else if (st->kind == STMT_FOR_IN) {
            ShrTypeKind elem_ty = SHR_TY_I32;
            if (st->as.for_in.iterable) {
                ShrTypeList it = infer_expr_types(s, prog, st->as.for_in.iterable);
                if (it.count > 0 && it.items[0] == SHR_TY_STRING) elem_ty = SHR_TY_I32;
                free(it.items); free(it.specs);
            }
            if (s->var_count < 256) {
                size_t v_idx = s->var_count++;
                s->vars[v_idx] = (ShrVarSym){
                    .name = st->as.for_in.item_name.start,
                    .name_len = st->as.for_in.item_name.len,
                    .slot = (uint8_t)v_idx,
                    .type = elem_ty,
                    .spec = (ShrTypeSpec){ .kind = elem_ty },
                    .closure_func_idx = SIZE_MAX,
                };
            }
            analyze_stmts_in_scope(s, prog, st->as.for_in.body, st->as.for_in.body_count, false);
        }
    }
}

static void analyze_stmt_list(ShrSema *s, ShrProgramAst *prog, const ShrParamDecl *params, size_t param_count,
                              ShrStmt *stmts, size_t stmt_count, ShrTypeList ret_types) {
    s->var_count = 0;
    s->cur_ret_types = ret_types;

    for (size_t p = 0; p < param_count && s->var_count < 256; p++) {
        size_t c_idx = SIZE_MAX;
        if (params[p].spec.kind == SHR_TY_FN && prog) {
            size_t want_params = params[p].spec.fn_params ? params[p].spec.fn_params->count : 0;
            for (size_t c = 0; c < prog->closure_count; c++) {
                if (prog->closures[c].user_param_count == want_params) {
                    c_idx = c;
                    break;
                }
            }
        }
        s->vars[s->var_count++] = (ShrVarSym){
            .name = params[p].name.start,
            .name_len = params[p].name.len,
            .slot = (uint8_t)p,
            .type = params[p].type,
            .spec = params[p].spec,
            .closure_func_idx = c_idx,
        };
    }

    analyze_stmts_in_scope(s, prog, stmts, stmt_count, false);
}

static void analyze_func_decl(ShrSema *s, ShrProgramAst *prog, ShrFuncDecl *fn) {
    if (!fn) return;
    analyze_stmt_list(s, prog, fn->params, fn->param_count, fn->stmts, fn->stmt_count, fn->ret_types);
}

bool shr_sema_analyze(ShrSema *s, ShrProgramAst *prog) {
    s->func_count = 0;
    for (size_t i = 0; i < prog->func_count && s->func_count < 256; i++) {
        ShrFuncDecl *fn = &prog->funcs[i];
        s->funcs[s->func_count++] = (ShrFuncSym){
            .name = fn->name.start,
            .name_len = fn->name.len,
            .func_idx = (uint16_t)i,
            .ret_types = fn->ret_types,
            .is_native = false,
        };
    }

    if (prog->toplevel_stmt_count > 0) {
        ShrTypeKind void_ty = SHR_TY_VOID;
        ShrTypeList top_ret = { .items = &void_ty, .count = 1 };
        analyze_stmt_list(s, prog, nullptr, 0, prog->toplevel_stmts, prog->toplevel_stmt_count, top_ret);
    }

    for (size_t i = 0; i < prog->func_count; i++) {
        analyze_func_decl(s, prog, &prog->funcs[i]);
        s->funcs[i].ret_types = prog->funcs[i].ret_types;
    }

    for (size_t c = 0; c < prog->closure_count; c++) {
        analyze_func_decl(s, prog, &prog->closures[c]);
    }

    return s->ok;
}
