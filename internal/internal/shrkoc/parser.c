/* shrkoc/parser.c */
#include "parser.h"
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void set_err(ShrParser *p, int line, const char *fmt, ...) {
    if (!p) return;
    p->had_error = true;
    if (p->err_msg[0] == '\0') {
        p->err_line = line;
        va_list args;
        va_start(args, fmt);
        vsnprintf(p->err_msg, sizeof(p->err_msg), fmt, args);
        va_end(args);
    }
}

static void advance(ShrParser *p) {
    p->cur = shr_lexer_next(&p->lex);
}

static bool check(const ShrParser *p, ShrTokenKind k) {
    return p->cur.kind == k;
}

static bool match(ShrParser *p, ShrTokenKind k) {
    if (p->cur.kind == k) {
        advance(p);
        return true;
    }
    return false;
}

static bool is_at_end(const ShrParser *p) {
    return p->cur.kind == TOK_EOF;
}

static bool tok_eq(ShrToken t, const char *s) {
    size_t n = strlen(s);
    return t.len == n && memcmp(t.start, s, n) == 0;
}

static ShrTypeKind parse_base_type_tok(ShrToken t) {
    if (tok_eq(t, "int"))    return SHR_TY_I32;
    if (tok_eq(t, "i32"))    return SHR_TY_I32;
    if (tok_eq(t, "i64"))    return SHR_TY_I64;
    if (tok_eq(t, "u32"))    return SHR_TY_U32;
    if (tok_eq(t, "u64"))    return SHR_TY_U64;
    if (tok_eq(t, "f32"))    return SHR_TY_F32;
    if (tok_eq(t, "float"))  return SHR_TY_F64;
    if (tok_eq(t, "f64"))    return SHR_TY_F64;
    if (tok_eq(t, "byte"))   return SHR_TY_U8;
    if (tok_eq(t, "u8"))     return SHR_TY_U8;
    if (tok_eq(t, "bool"))   return SHR_TY_BOOL;
    if (tok_eq(t, "string")) return SHR_TY_STRING;
    if (tok_eq(t, "void"))   return SHR_TY_VOID;
    return SHR_TY_I32;
}

static ShrTypeList parse_type_list_paren(ShrParser *p);

static ShrTypeSpec parse_type_spec(ShrParser *p) {
    ShrTypeSpec sp = {0};
    if (check(p, TOK_LBRACKET)) {
        advance(p);
        if (match(p, TOK_RBRACKET)) {
            sp.kind = SHR_TY_LIST;
            if (check(p, TOK_IDENT)) {
                sp.elem_type = parse_base_type_tok(p->cur);
                advance(p);
            } else {
                sp.elem_type = SHR_TY_I32;
            }
            return sp;
        }
    }
    if (check(p, TOK_KW_FN)) {
        advance(p);
        sp.kind = SHR_TY_FN;
        if (check(p, TOK_LPAREN)) {
            ShrTypeList *fp = calloc(1, sizeof(ShrTypeList));
            if (fp) *fp = parse_type_list_paren(p);
            sp.fn_params = fp;
        }
        if (match(p, TOK_ARROW)) {
            ShrTypeList *fr = calloc(1, sizeof(ShrTypeList));
            if (fr) {
                if (check(p, TOK_LPAREN)) {
                    *fr = parse_type_list_paren(p);
                } else {
                    ShrTypeSpec one = parse_type_spec(p);
                    fr->count = 1;
                    fr->items = malloc(sizeof(ShrTypeKind));
                    fr->specs = malloc(sizeof(ShrTypeSpec));
                    if (fr->items && fr->specs) {
                        fr->items[0] = one.kind;
                        fr->specs[0] = one;
                    }
                }
            }
            sp.fn_rets = fr;
        }
        return sp;
    }
    if (check(p, TOK_IDENT)) {
        sp.kind = parse_base_type_tok(p->cur);
        advance(p);
        return sp;
    }
    sp.kind = SHR_TY_I32;
    return sp;
}

static ShrTypeList parse_type_list_paren(ShrParser *p) {
    ShrTypeList tl = {0};
    if (!match(p, TOK_LPAREN)) return tl;
    size_t cap = 4;
    tl.items = malloc(cap * sizeof(ShrTypeKind));
    tl.specs = calloc(cap, sizeof(ShrTypeSpec));
    while (!check(p, TOK_RPAREN) && !is_at_end(p)) {
        ShrTypeSpec sp = parse_type_spec(p);
        if (tl.count == cap) {
            cap *= 2;
            ShrTypeKind *ni = realloc(tl.items, cap * sizeof(ShrTypeKind));
            ShrTypeSpec *ns = realloc(tl.specs, cap * sizeof(ShrTypeSpec));
            if (!ni || !ns) {
                free(ni); free(ns);
                set_err(p, p->cur.line, "out of memory in type list");
                break;
            }
            tl.items = ni;
            tl.specs = ns;
        }
        if (tl.items) tl.items[tl.count] = sp.kind;
        if (tl.specs) tl.specs[tl.count] = sp;
        tl.count++;
        if (!match(p, TOK_COMMA)) break;
    }
    match(p, TOK_RPAREN);
    return tl;
}

static ShrExpr *parse_expr(ShrParser *p);
static ShrStmt *parse_stmt_block(ShrParser *p, size_t *out_count);

static ShrExpr *parse_typed_list_literal(ShrParser *p, ShrToken start_tok) {
    if (!match(p, TOK_RBRACKET)) {
        set_err(p, p->cur.line, "expected ']' in list type");
        return nullptr;
    }
    ShrTypeKind elem_ty = SHR_TY_I32;
    if (check(p, TOK_IDENT)) {
        elem_ty = parse_base_type_tok(p->cur);
        advance(p);
    }
    if (!match(p, TOK_LBRACE)) {
        set_err(p, p->cur.line, "expected '{' in list literal");
        return nullptr;
    }

    size_t cap = 8, count = 0;
    ShrExpr **items = malloc(cap * sizeof(ShrExpr *));
    if (!items) {
        set_err(p, p->cur.line, "out of memory creating list literal");
        return nullptr;
    }

    while (!check(p, TOK_RBRACE) && !is_at_end(p)) {
        ShrExpr *el = parse_expr(p);
        if (!el || p->had_error) {
            shr_expr_free(el);
            for (size_t i = 0; i < count; i++) shr_expr_free(items[i]);
            free(items);
            return nullptr;
        }
        if (count == cap) {
            cap *= 2;
            ShrExpr **tmp = realloc(items, cap * sizeof(ShrExpr *));
            if (!tmp) {
                shr_expr_free(el);
                for (size_t i = 0; i < count; i++) shr_expr_free(items[i]);
                free(items);
                set_err(p, p->cur.line, "out of memory growing list literal");
                return nullptr;
            }
            items = tmp;
        }
        items[count++] = el;
        if (!match(p, TOK_COMMA)) break;
    }

    if (!match(p, TOK_RBRACE)) {
        for (size_t i = 0; i < count; i++) shr_expr_free(items[i]);
        free(items);
        set_err(p, p->cur.line, "expected '}' at end of list literal");
        return nullptr;
    }

    ShrExpr *node = calloc(1, sizeof(ShrExpr));
    if (!node) {
        for (size_t i = 0; i < count; i++) shr_expr_free(items[i]);
        free(items);
        return nullptr;
    }
    node->kind = EXPR_LIST;
    node->tok = start_tok;
    node->as.list.elem_type = elem_ty;
    node->as.list.items = items;
    node->as.list.item_count = count;
    return node;
}

static ShrExpr *parse_primary(ShrParser *p) {
    ShrToken t = p->cur;
    if (check(p, TOK_INT)) {
        advance(p);
        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_INT;
        e->tok = t;
        e->as.i_val = strtoll(t.start, nullptr, 10);
        return e;
    }
    if (check(p, TOK_FLOAT)) {
        advance(p);
        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_FLOAT;
        e->tok = t;
        e->as.f_val = strtod(t.start, nullptr);
        return e;
    }
    if (check(p, TOK_KW_TRUE) || check(p, TOK_KW_FALSE)) {
        advance(p);
        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_BOOL;
        e->tok = t;
        e->as.b_val = (t.kind == TOK_KW_TRUE);
        return e;
    }
    if (check(p, TOK_STRING)) {
        advance(p);
        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_STRING;
        e->tok = t;
        return e;
    }
    if (check(p, TOK_LBRACKET)) {
        advance(p);
        return parse_typed_list_literal(p, t);
    }
    if (check(p, TOK_KW_FN)) {
        advance(p);
        ShrParamDecl *params = nullptr;
        size_t param_count = 0, cap = 4;
        if (match(p, TOK_LPAREN)) {
            params = malloc(cap * sizeof(ShrParamDecl));
            while (!check(p, TOK_RPAREN) && !is_at_end(p)) {
                if (!check(p, TOK_IDENT)) break;
                ShrToken pname = p->cur;
                advance(p);
                ShrTypeSpec psp = parse_type_spec(p);
                if (params && param_count == cap) {
                    cap *= 2;
                    ShrParamDecl *tmp = realloc(params, cap * sizeof(ShrParamDecl));
                    if (tmp) params = tmp;
                }
                if (params) {
                    params[param_count++] = (ShrParamDecl){
                        .name = pname,
                        .type = psp.kind,
                        .spec = psp,
                    };
                }
                if (!match(p, TOK_COMMA)) break;
            }
            match(p, TOK_RPAREN);
        }

        ShrTypeList ret_types = {0};
        if (match(p, TOK_ARROW)) {
            ShrTypeSpec rsp = parse_type_spec(p);
            ret_types.count = 1;
            ret_types.items = malloc(sizeof(ShrTypeKind));
            ret_types.specs = malloc(sizeof(ShrTypeSpec));
            if (ret_types.items && ret_types.specs) {
                ret_types.items[0] = rsp.kind;
                ret_types.specs[0] = rsp;
            }
        }

        size_t body_count = 0;
        ShrStmt *body = nullptr;
        if (match(p, TOK_LBRACE)) {
            body = parse_stmt_block(p, &body_count);
            match(p, TOK_RBRACE);
        }

        size_t cidx = SIZE_MAX;
        if (p->prog) {
            ShrFuncDecl *tmp = realloc(p->prog->closures,
                                       (p->prog->closure_count + 1) * sizeof(ShrFuncDecl));
            if (tmp) {
                p->prog->closures = tmp;
                cidx = p->prog->closure_count++;
                p->prog->closures[cidx] = (ShrFuncDecl){
                    .name = (ShrToken){ .kind = TOK_IDENT, .start = t.start, .len = t.len, .line = t.line },
                    .params = params,
                    .param_count = param_count,
                    .user_param_count = param_count,
                    .captures = nullptr,
                    .capture_count = 0,
                    .ret_types = ret_types,
                    .stmts = body,
                    .stmt_count = body_count,
                };
            }
        }

        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_FN;
        e->tok = t;
        e->as.fn.func_idx = (cidx == SIZE_MAX) ? 0 : cidx;
        return e;
    }
    if (check(p, TOK_IDENT)) {
        advance(p);
        ShrExpr *e = calloc(1, sizeof(ShrExpr));
        if (!e) return nullptr;
        e->kind = EXPR_IDENT;
        e->tok = t;
        return e;
    }
    if (match(p, TOK_LPAREN)) {
        ShrExpr *inner = parse_expr(p);
        match(p, TOK_RPAREN);
        return inner;
    }

    set_err(p, t.line, "unexpected token '%.*s'", (int)t.len, t.start);
    return nullptr;
}

static ShrExpr *parse_unary(ShrParser *p) {
    if (check(p, TOK_BANG)) {
        ShrToken op_tok = p->cur;
        advance(p);
        ShrExpr *operand = parse_unary(p);
        if (!operand || p->had_error) {
            shr_expr_free(operand);
            return nullptr;
        }
        ShrExpr *u = calloc(1, sizeof(ShrExpr));
        if (!u) {
            shr_expr_free(operand);
            return nullptr;
        }
        u->kind = EXPR_UNARY;
        u->tok = op_tok;
        u->as.unary.op = TOK_BANG;
        u->as.unary.operand = operand;
        return u;
    }
    return parse_primary(p);
}

static ShrExpr *parse_postfix(ShrParser *p) {
    ShrExpr *e = parse_unary(p);
    if (!e || p->had_error) return e;

    for (;;) {
        if (match(p, TOK_DOT)) {
            if (!check(p, TOK_IDENT)) {
                set_err(p, p->cur.line, "expected member name after '.'");
                shr_expr_free(e);
                return nullptr;
            }
            ShrToken mem = p->cur;
            advance(p);
            ShrExpr *m = calloc(1, sizeof(ShrExpr));
            if (!m) { shr_expr_free(e); return nullptr; }
            m->kind = EXPR_MEMBER;
            m->tok = mem;
            m->as.member.obj = e;
            m->as.member.member = mem;
            e = m;
        } else if (match(p, TOK_LBRACKET)) {
            ShrExpr *idx = parse_expr(p);
            if (!idx || !match(p, TOK_RBRACKET)) {
                shr_expr_free(idx);
                shr_expr_free(e);
                set_err(p, p->cur.line, "expected ']' after index");
                return nullptr;
            }
            ShrExpr *ix = calloc(1, sizeof(ShrExpr));
            if (!ix) { shr_expr_free(idx); shr_expr_free(e); return nullptr; }
            ix->kind = EXPR_INDEX;
            ix->tok = e->tok;
            ix->as.index.target = e;
            ix->as.index.idx = idx;
            e = ix;
        } else if (match(p, TOK_LPAREN)) {
            size_t cap = 4, argc = 0;
            ShrExpr **args = malloc(cap * sizeof(ShrExpr *));
            while (!check(p, TOK_RPAREN) && !is_at_end(p)) {
                ShrExpr *arg = parse_expr(p);
                if (!arg || p->had_error) {
                    shr_expr_free(arg);
                    for (size_t i = 0; i < argc; i++) shr_expr_free(args[i]);
                    free(args);
                    shr_expr_free(e);
                    return nullptr;
                }
                if (args && argc == cap) {
                    cap *= 2;
                    ShrExpr **tmp = realloc(args, cap * sizeof(ShrExpr *));
                    if (tmp) args = tmp;
                }
                if (args) args[argc++] = arg;
                if (!match(p, TOK_COMMA)) break;
            }
            if (!match(p, TOK_RPAREN)) {
                for (size_t i = 0; i < argc; i++) shr_expr_free(args[i]);
                free(args);
                shr_expr_free(e);
                set_err(p, p->cur.line, "expected ')' after call arguments");
                return nullptr;
            }
            ShrExpr *call = calloc(1, sizeof(ShrExpr));
            if (!call) {
                for (size_t i = 0; i < argc; i++) shr_expr_free(args[i]);
                free(args);
                shr_expr_free(e);
                return nullptr;
            }
            call->kind = EXPR_CALL;
            call->tok = e->tok;
            call->as.call.callee = e;
            call->as.call.args = args;
            call->as.call.arg_count = argc;
            e = call;
        } else {
            break;
        }
    }
    return e;
}

static int bin_prec(ShrTokenKind k) {
    switch (k) {
        case TOK_STAR:
        case TOK_SLASH:
        case TOK_PERCENT:  return 40;
        case TOK_PLUS:
        case TOK_MINUS:    return 30;
        case TOK_LT:
        case TOK_LE:
        case TOK_GT:
        case TOK_GE:       return 20;
        case TOK_EQ_EQ:
        case TOK_BANG_EQ:  return 15;
        case TOK_AND_AND:  return 10;
        case TOK_OR_OR:    return 5;
        default:           return -1;
    }
}

static ShrExpr *parse_bin_rhs(ShrParser *p, ShrExpr *lhs, int min_prec) {
    for (;;) {
        ShrTokenKind op = p->cur.kind;
        int prec = bin_prec(op);
        if (prec < min_prec) break;
        ShrToken op_tok = p->cur;
        advance(p);
        ShrExpr *rhs = parse_postfix(p);
        if (!rhs || p->had_error) {
            shr_expr_free(rhs);
            shr_expr_free(lhs);
            return nullptr;
        }
        while (bin_prec(p->cur.kind) > prec) {
            rhs = parse_bin_rhs(p, rhs, prec + 1);
            if (!rhs || p->had_error) {
                shr_expr_free(lhs);
                return nullptr;
            }
        }
        ShrExpr *bin = calloc(1, sizeof(ShrExpr));
        if (!bin) {
            shr_expr_free(lhs);
            shr_expr_free(rhs);
            return nullptr;
        }
        bin->kind = EXPR_BINARY;
        bin->tok = op_tok;
        bin->as.binary.op = op;
        bin->as.binary.lhs = lhs;
        bin->as.binary.rhs = rhs;
        lhs = bin;
    }
    return lhs;
}

static ShrExpr *parse_expr(ShrParser *p) {
    ShrExpr *lhs = parse_postfix(p);
    if (!lhs || p->had_error) return lhs;
    return parse_bin_rhs(p, lhs, 0);
}

static ShrStmt parse_stmt(ShrParser *p);

static ShrStmt *parse_stmt_block(ShrParser *p, size_t *out_count) {
    size_t cap = 8, count = 0;
    ShrStmt *arr = calloc(cap, sizeof(ShrStmt));
    if (!arr) {
        *out_count = 0;
        return nullptr;
    }
    while (!check(p, TOK_RBRACE) && !is_at_end(p) && !p->had_error) {
        if (count == cap) {
            cap *= 2;
            ShrStmt *tmp = realloc(arr, cap * sizeof(ShrStmt));
            if (!tmp) {
                set_err(p, p->cur.line, "out of memory in statement block");
                break;
            }
            arr = tmp;
        }
        arr[count++] = parse_stmt(p);
    }
    *out_count = count;
    return arr;
}

static ShrStmt parse_if_stmt(ShrParser *p) {
    ShrStmt st = {0};
    st.kind = STMT_IF;

    advance(p); /* ăn TOK_KW_IF */
    ShrExpr *cond = parse_expr(p);
    if (!cond || p->had_error) {
        shr_expr_free(cond);
        return st;
    }

    size_t then_count = 0;
    ShrStmt *then_body = nullptr;
    if (match(p, TOK_LBRACE)) {
        then_body = parse_stmt_block(p, &then_count);
        if (!match(p, TOK_RBRACE)) {
            set_err(p, p->cur.line, "expected '}' after if block");
        }
    } else {
        set_err(p, p->cur.line, "expected '{' after if condition");
    }

    st.as.if_stmt.cond = cond;
    st.as.if_stmt.then_body = then_body;
    st.as.if_stmt.then_count = then_count;

    if (!p->had_error && match(p, TOK_KW_ELSE)) {
        if (check(p, TOK_KW_IF)) {
            ShrStmt *nested = malloc(sizeof(ShrStmt));
            if (nested) {
                *nested = parse_if_stmt(p);
                st.as.if_stmt.else_stmt = nested;
                st.as.if_stmt.else_stmt_count = 1;
                st.as.if_stmt.else_is_block = false;
            }
        } else if (match(p, TOK_LBRACE)) {
            size_t ecount = 0;
            ShrStmt *ebody = parse_stmt_block(p, &ecount);
            if (!match(p, TOK_RBRACE)) {
                set_err(p, p->cur.line, "expected '}' after else block");
            }
            st.as.if_stmt.else_stmt = ebody;
            st.as.if_stmt.else_stmt_count = ecount;
            st.as.if_stmt.else_is_block = true;
        } else {
            set_err(p, p->cur.line, "expected 'if' or '{' after else");
        }
    }

    return st;
}

static ShrStmt parse_stmt(ShrParser *p) {
    ShrStmt st = {0};

    if (check(p, TOK_KW_IF)) {
        return parse_if_stmt(p);
    }

    if (check(p, TOK_KW_LET)) {
        advance(p);
        size_t ncap = 4, ncount = 0;
        ShrToken *names = malloc(ncap * sizeof(ShrToken));
        while (check(p, TOK_IDENT)) {
            if (names && ncount == ncap) {
                ncap *= 2;
                ShrToken *tmp = realloc(names, ncap * sizeof(ShrToken));
                if (tmp) names = tmp;
            }
            if (names) names[ncount++] = p->cur;
            advance(p);
            if (!match(p, TOK_COMMA)) break;
        }
        size_t ecap = 4, ecount = 0;
        ShrExpr **exprs = malloc(ecap * sizeof(ShrExpr *));
        if (match(p, TOK_EQ)) {
            while (!is_at_end(p) && !p->had_error) {
                ShrExpr *ex = parse_expr(p);
                if (!ex || p->had_error) {
                    shr_expr_free(ex);
                    break;
                }
                if (exprs && ecount == ecap) {
                    ecap *= 2;
                    ShrExpr **tmp = realloc(exprs, ecap * sizeof(ShrExpr *));
                    if (tmp) exprs = tmp;
                }
                if (exprs) exprs[ecount++] = ex;
                if (!match(p, TOK_COMMA)) break;
            }
        }
        st.kind = STMT_LET_MULTI;
        st.as.let_multi.names = names;
        st.as.let_multi.name_count = ncount;
        st.as.let_multi.exprs = exprs;
        st.as.let_multi.expr_count = ecount;
        return st;
    }

    if (check(p, TOK_KW_RETURN)) {
        advance(p);
        size_t ecap = 4, ecount = 0;
        ShrExpr **exprs = malloc(ecap * sizeof(ShrExpr *));
        while (!check(p, TOK_RBRACE) && !is_at_end(p) && !p->had_error) {
            ShrExpr *ex = parse_expr(p);
            if (!ex || p->had_error) {
                shr_expr_free(ex);
                break;
            }
            if (exprs && ecount == ecap) {
                ecap *= 2;
                ShrExpr **tmp = realloc(exprs, ecap * sizeof(ShrExpr *));
                if (tmp) exprs = tmp;
            }
            if (exprs) exprs[ecount++] = ex;
            if (!match(p, TOK_COMMA)) break;
        }
        st.kind = STMT_RETURN_MULTI;
        st.as.ret_multi.exprs = exprs;
        st.as.ret_multi.expr_count = ecount;
        return st;
    }

    if (check(p, TOK_KW_FOR)) {
        advance(p);
        ShrStmt *init_st = nullptr;
        if (check(p, TOK_KW_LET)) {
            init_st = malloc(sizeof(ShrStmt));
            if (init_st) *init_st = parse_stmt(p);
            match(p, TOK_SEMI);
            ShrExpr *cond = parse_expr(p);
            match(p, TOK_SEMI);
            ShrStmt *post_st = nullptr;
            if (!check(p, TOK_LBRACE) && !is_at_end(p)) {
                post_st = malloc(sizeof(ShrStmt));
                if (post_st) *post_st = parse_stmt(p);
            }
            size_t bcount = 0;
            ShrStmt *body = nullptr;
            if (match(p, TOK_LBRACE)) {
                body = parse_stmt_block(p, &bcount);
                match(p, TOK_RBRACE);
            }
            st.kind = STMT_FOR_C;
            st.as.for_c.init = init_st;
            st.as.for_c.cond = cond;
            st.as.for_c.post = post_st;
            st.as.for_c.body = body;
            st.as.for_c.body_count = bcount;
            return st;
        }
        ShrExpr *cond = parse_expr(p);
        size_t bcount = 0;
        ShrStmt *body = nullptr;
        if (match(p, TOK_LBRACE)) {
            body = parse_stmt_block(p, &bcount);
            match(p, TOK_RBRACE);
        }
        st.kind = STMT_FOR_COND;
        st.as.for_cond.cond = cond;
        st.as.for_cond.body = body;
        st.as.for_cond.body_count = bcount;
        return st;
    }

    ShrExpr *first = parse_expr(p);
    if (!first || p->had_error) {
        shr_expr_free(first);
        st.kind = STMT_EXPR;
        return st;
    }

    if (check(p, TOK_PLUS_PLUS) || check(p, TOK_MINUS_MINUS)) {
        ShrTokenKind op = p->cur.kind;
        advance(p);
        st.kind = STMT_INC_DEC;
        st.as.inc_dec.op = op;
        st.as.inc_dec.target = first;
        return st;
    }

    if (check(p, TOK_PLUS_EQ) || check(p, TOK_MINUS_EQ) ||
        check(p, TOK_STAR_EQ) || check(p, TOK_SLASH_EQ)) {
        ShrTokenKind op = p->cur.kind;
        advance(p);
        ShrExpr *val = parse_expr(p);
        st.kind = STMT_COMPOUND_ASSIGN;
        st.as.compound_assign.op = op;
        st.as.compound_assign.target = first;
        st.as.compound_assign.value = val;
        return st;
    }

    if (match(p, TOK_EQ)) {
        size_t tcap = 4, tcount = 1;
        ShrExpr **targets = malloc(tcap * sizeof(ShrExpr *));
        if (targets) targets[0] = first;
        size_t vcap = 4, vcount = 0;
        ShrExpr **vals = malloc(vcap * sizeof(ShrExpr *));
        while (!is_at_end(p) && !p->had_error) {
            ShrExpr *v = parse_expr(p);
            if (!v || p->had_error) {
                shr_expr_free(v);
                break;
            }
            if (vals && vcount == vcap) {
                vcap *= 2;
                ShrExpr **tmp = realloc(vals, vcap * sizeof(ShrExpr *));
                if (tmp) vals = tmp;
            }
            if (vals) vals[vcount++] = v;
            if (!match(p, TOK_COMMA)) break;
        }
        st.kind = STMT_ASSIGN_MULTI;
        st.as.assign_multi.targets = targets;
        st.as.assign_multi.target_count = tcount;
        st.as.assign_multi.values = vals;
        st.as.assign_multi.value_count = vcount;
        return st;
    }

    st.kind = STMT_EXPR;
    st.as.expr.expr = first;
    return st;
}

static ShrFuncDecl parse_top_fn(ShrParser *p) {
    ShrFuncDecl fn = {0};
    advance(p); /* ăn TOK_KW_FN */
    if (check(p, TOK_IDENT)) {
        fn.name = p->cur;
        advance(p);
    }

    size_t pcap = 4;
    fn.params = malloc(pcap * sizeof(ShrParamDecl));
    if (match(p, TOK_LPAREN)) {
        while (!check(p, TOK_RPAREN) && !is_at_end(p)) {
            if (!check(p, TOK_IDENT)) break;
            ShrToken pname = p->cur;
            advance(p);
            ShrTypeSpec psp = parse_type_spec(p);
            if (fn.params && fn.param_count == pcap) {
                pcap *= 2;
                ShrParamDecl *tmp = realloc(fn.params, pcap * sizeof(ShrParamDecl));
                if (tmp) fn.params = tmp;
            }
            if (fn.params) {
                fn.params[fn.param_count++] = (ShrParamDecl){
                    .name = pname,
                    .type = psp.kind,
                    .spec = psp,
                };
            }
            if (!match(p, TOK_COMMA)) break;
        }
        match(p, TOK_RPAREN);
    }
    fn.user_param_count = fn.param_count;

    if (match(p, TOK_ARROW)) {
        ShrTypeSpec rsp = parse_type_spec(p);
        fn.ret_types.count = 1;
        fn.ret_types.items = malloc(sizeof(ShrTypeKind));
        fn.ret_types.specs = malloc(sizeof(ShrTypeSpec));
        if (fn.ret_types.items && fn.ret_types.specs) {
            fn.ret_types.items[0] = rsp.kind;
            fn.ret_types.specs[0] = rsp;
        }
    } else {
        fn.ret_types.count = 1;
        fn.ret_types.items = malloc(sizeof(ShrTypeKind));
        fn.ret_types.specs = calloc(1, sizeof(ShrTypeSpec));
        if (fn.ret_types.items) fn.ret_types.items[0] = SHR_TY_VOID;
        if (fn.ret_types.specs) fn.ret_types.specs[0].kind = SHR_TY_VOID;
    }

    if (match(p, TOK_LBRACE)) {
        fn.stmts = parse_stmt_block(p, &fn.stmt_count);
        match(p, TOK_RBRACE);
    }
    return fn;
}

void shr_parser_init(ShrParser *p, const char *src, size_t len) {
    memset(p, 0, sizeof(*p));
    shr_lexer_init(&p->lex, src, len);
    advance(p);
}

bool shr_parse_program(ShrParser *p, ShrProgramAst *prog) {
    memset(prog, 0, sizeof(*prog));
    p->prog = prog;

    size_t fcap = 8, tcap = 16, ucap = 4;
    prog->funcs = malloc(fcap * sizeof(ShrFuncDecl));
    prog->toplevel_stmts = malloc(tcap * sizeof(ShrStmt));
    prog->uses = malloc(ucap * sizeof(ShrToken));

    while (!is_at_end(p) && !p->had_error) {
        if (check(p, TOK_KW_USE)) {
            advance(p);
            if (check(p, TOK_IDENT)) {
                if (prog->uses && prog->use_count == ucap) {
                    ucap *= 2;
                    ShrToken *tmp = realloc(prog->uses, ucap * sizeof(ShrToken));
                    if (tmp) prog->uses = tmp;
                }
                if (prog->uses) prog->uses[prog->use_count++] = p->cur;
                advance(p);
            }
        } else if (check(p, TOK_KW_FN)) {
            if (prog->funcs && prog->func_count == fcap) {
                fcap *= 2;
                ShrFuncDecl *tmp = realloc(prog->funcs, fcap * sizeof(ShrFuncDecl));
                if (tmp) prog->funcs = tmp;
            }
            if (prog->funcs) prog->funcs[prog->func_count++] = parse_top_fn(p);
        } else {
            if (prog->toplevel_stmts && prog->toplevel_stmt_count == tcap) {
                tcap *= 2;
                ShrStmt *tmp = realloc(prog->toplevel_stmts, tcap * sizeof(ShrStmt));
                if (tmp) prog->toplevel_stmts = tmp;
            }
            if (prog->toplevel_stmts) prog->toplevel_stmts[prog->toplevel_stmt_count++] = parse_stmt(p);
        }
    }

    return !p->had_error;
}

void shr_expr_free(ShrExpr *e) {
    if (!e) return;
    free(e->inferred_types.items);
    free(e->inferred_types.specs);
    switch (e->kind) {
        case EXPR_UNARY:
            shr_expr_free(e->as.unary.operand);
            break;
        case EXPR_BINARY:
            shr_expr_free(e->as.binary.lhs);
            shr_expr_free(e->as.binary.rhs);
            break;
        case EXPR_CALL:
            shr_expr_free(e->as.call.callee);
            for (size_t i = 0; i < e->as.call.arg_count; i++) {
                shr_expr_free(e->as.call.args[i]);
            }
            free(e->as.call.args);
            break;
        case EXPR_MEMBER:
            shr_expr_free(e->as.member.obj);
            break;
        case EXPR_LIST:
            for (size_t i = 0; i < e->as.list.item_count; i++) {
                shr_expr_free(e->as.list.items[i]);
            }
            free(e->as.list.items);
            break;
        case EXPR_INDEX:
            shr_expr_free(e->as.index.target);
            shr_expr_free(e->as.index.idx);
            break;
        default:
            break;
    }
    free(e);
}

void shr_stmt_free(ShrStmt *st) {
    if (!st) return;
    switch (st->kind) {
        case STMT_LET_MULTI:
            free(st->as.let_multi.names);
            for (size_t i = 0; i < st->as.let_multi.expr_count; i++) {
                shr_expr_free(st->as.let_multi.exprs[i]);
            }
            free(st->as.let_multi.exprs);
            break;
        case STMT_ASSIGN_MULTI:
            for (size_t i = 0; i < st->as.assign_multi.target_count; i++) {
                shr_expr_free(st->as.assign_multi.targets[i]);
            }
            free(st->as.assign_multi.targets);
            for (size_t i = 0; i < st->as.assign_multi.value_count; i++) {
                shr_expr_free(st->as.assign_multi.values[i]);
            }
            free(st->as.assign_multi.values);
            break;
        case STMT_COMPOUND_ASSIGN:
            shr_expr_free(st->as.compound_assign.target);
            shr_expr_free(st->as.compound_assign.value);
            break;
        case STMT_INC_DEC:
            shr_expr_free(st->as.inc_dec.target);
            break;
        case STMT_RETURN_MULTI:
            for (size_t i = 0; i < st->as.ret_multi.expr_count; i++) {
                shr_expr_free(st->as.ret_multi.exprs[i]);
            }
            free(st->as.ret_multi.exprs);
            break;
        case STMT_EXPR:
            shr_expr_free(st->as.expr.expr);
            break;
        case STMT_FOR_COND:
            shr_expr_free(st->as.for_cond.cond);
            for (size_t i = 0; i < st->as.for_cond.body_count; i++) shr_stmt_free(&st->as.for_cond.body[i]);
            free(st->as.for_cond.body);
            break;
        case STMT_FOR_C:
            if (st->as.for_c.init) { shr_stmt_free(st->as.for_c.init); free(st->as.for_c.init); }
            shr_expr_free(st->as.for_c.cond);
            if (st->as.for_c.post) { shr_stmt_free(st->as.for_c.post); free(st->as.for_c.post); }
            for (size_t i = 0; i < st->as.for_c.body_count; i++) shr_stmt_free(&st->as.for_c.body[i]);
            free(st->as.for_c.body);
            break;
        case STMT_FOR_IN:
            shr_expr_free(st->as.for_in.iterable);
            for (size_t i = 0; i < st->as.for_in.body_count; i++) shr_stmt_free(&st->as.for_in.body[i]);
            free(st->as.for_in.body);
            break;
        case STMT_IF:
            shr_expr_free(st->as.if_stmt.cond);
            for (size_t i = 0; i < st->as.if_stmt.then_count; i++) {
                shr_stmt_free(&st->as.if_stmt.then_body[i]);
            }
            free(st->as.if_stmt.then_body);
            if (st->as.if_stmt.else_stmt) {
                if (st->as.if_stmt.else_is_block) {
                    for (size_t i = 0; i < st->as.if_stmt.else_stmt_count; i++) {
                        shr_stmt_free(&st->as.if_stmt.else_stmt[i]);
                    }
                    free(st->as.if_stmt.else_stmt);
                } else {
                    shr_stmt_free(st->as.if_stmt.else_stmt);
                    free(st->as.if_stmt.else_stmt);
                }
            }
            break;
        default:
            break;
    }
}

static void free_type_list_contents(ShrTypeList *tl) {
    if (!tl) return;
    free(tl->items);
    free(tl->specs);
    tl->items = nullptr;
    tl->specs = nullptr;
    tl->count = 0;
}

static void free_func_decl_contents(ShrFuncDecl *fn) {
    if (!fn) return;
    for (size_t i = 0; i < fn->param_count; i++) {
        free_type_list_contents(fn->params[i].spec.fn_params);
        free(fn->params[i].spec.fn_params);
        free_type_list_contents(fn->params[i].spec.fn_rets);
        free(fn->params[i].spec.fn_rets);
    }
    free(fn->params);
    free(fn->captures);
    free_type_list_contents(&fn->ret_types);
    for (size_t i = 0; i < fn->stmt_count; i++) {
        shr_stmt_free(&fn->stmts[i]);
    }
    free(fn->stmts);
}

void shr_program_ast_free(ShrProgramAst *prog) {
    if (!prog) return;
    free(prog->uses);
    for (size_t i = 0; i < prog->func_count; i++) {
        free_func_decl_contents(&prog->funcs[i]);
    }
    free(prog->funcs);
    for (size_t i = 0; i < prog->closure_count; i++) {
        free_func_decl_contents(&prog->closures[i]);
    }
    free(prog->closures);
    for (size_t i = 0; i < prog->toplevel_stmt_count; i++) {
        shr_stmt_free(&prog->toplevel_stmts[i]);
    }
    free(prog->toplevel_stmts);
    memset(prog, 0, sizeof(*prog));
}
