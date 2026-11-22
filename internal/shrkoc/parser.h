/* shrkc/parser.h */
#pragma once

#include "lexer.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

typedef enum {
    SHR_TY_VOID = 0,
    SHR_TY_I32,
    SHR_TY_I64,
    SHR_TY_U32,
    SHR_TY_U64,
    SHR_TY_F32,
    SHR_TY_F64,
    SHR_TY_U8,
    SHR_TY_BOOL,
    SHR_TY_STRING,
    SHR_TY_FN,
    SHR_TY_LIST,
} ShrTypeKind;

typedef struct ShrTypeList ShrTypeList;

typedef struct {
    ShrTypeKind kind;
    ShrTypeKind elem_type;
    ShrTypeList *fn_params;
    ShrTypeList *fn_rets;
} ShrTypeSpec;

struct ShrTypeList {
    ShrTypeKind *items;
    ShrTypeSpec *specs;
    size_t count;
};

typedef enum {
    EXPR_INT = 0,
    EXPR_FLOAT,
    EXPR_BOOL,
    EXPR_STRING,
    EXPR_IDENT,
    EXPR_BINARY,
    EXPR_CALL,
    EXPR_MEMBER,
    EXPR_FN,
    EXPR_LIST,
    EXPR_INDEX,
} ShrExprKind;

typedef struct ShrExpr ShrExpr;

struct ShrExpr {
    ShrExprKind kind;
    ShrToken tok;
    ShrTypeList inferred_types;
    union {
        int64_t i_val;
        double f_val;
        bool b_val;
        struct {
            ShrTokenKind op;
            ShrExpr *lhs;
            ShrExpr *rhs;
        } binary;
        struct {
            ShrExpr *callee;
            ShrExpr **args;
            size_t arg_count;
        } call;
        struct {
            ShrExpr *obj;
            ShrToken member;
        } member;
        struct {
            size_t func_idx;
        } fn;
        struct {
            ShrTypeKind elem_type;
            ShrExpr **items;
            size_t item_count;
        } list;
        struct {
            ShrExpr *target;
            ShrExpr *idx;
        } index;
    } as;
};

typedef struct {
    ShrToken name;
    ShrTypeKind type;
    ShrTypeSpec spec;
} ShrParamDecl;

typedef ShrParamDecl ShrCaptureDecl;

typedef enum {
    STMT_LET_MULTI = 0,
    STMT_ASSIGN_MULTI,
    STMT_COMPOUND_ASSIGN,
    STMT_INC_DEC,
    STMT_RETURN_MULTI,
    STMT_EXPR,
    STMT_FOR_COND,
    STMT_FOR_C,
    STMT_FOR_IN,
} ShrStmtKind;

typedef struct ShrStmt ShrStmt;

struct ShrStmt {
    ShrStmtKind kind;
    union {
        struct {
            ShrToken *names;
            size_t name_count;
            ShrExpr **exprs;
            size_t expr_count;
        } let_multi;
        struct {
            ShrExpr **targets;
            size_t target_count;
            ShrExpr **values;
            size_t value_count;
        } assign_multi;
        struct {
            ShrTokenKind op;
            ShrExpr *target;
            ShrExpr *value;
        } compound_assign;
        struct {
            ShrTokenKind op;
            ShrExpr *target;
        } inc_dec;
        struct {
            ShrExpr **exprs;
            size_t expr_count;
        } ret_multi;
        struct {
            ShrExpr *expr;
        } expr;
        struct {
            ShrExpr *cond;
            ShrStmt *body;
            size_t body_count;
        } for_cond;
        struct {
            ShrStmt *init;
            ShrExpr *cond;
            ShrStmt *post;
            ShrStmt *body;
            size_t body_count;
        } for_c;
        struct {
            ShrToken item_name;
            ShrExpr *iterable;
            ShrStmt *body;
            size_t body_count;
        } for_in;
    } as;
};

typedef struct {
    ShrToken name;
    ShrParamDecl *params;
    size_t param_count;
    size_t user_param_count;
    ShrCaptureDecl *captures;
    size_t capture_count;
    ShrTypeList ret_types;
    ShrStmt *stmts;
    size_t stmt_count;
} ShrFuncDecl;

typedef struct {
    ShrToken *uses;
    size_t use_count;
    ShrFuncDecl *funcs;
    size_t func_count;
    ShrFuncDecl *closures;
    size_t closure_count;
    ShrStmt *toplevel_stmts;
    size_t toplevel_stmt_count;
} ShrProgramAst;

typedef struct {
    ShrLexer lex;
    ShrToken cur;
    bool had_error;
    int err_line;
    char err_msg[256];
    ShrProgramAst *prog;
} ShrParser;

void shr_parser_init(ShrParser *p, const char *src, size_t len);
bool shr_parse_program(ShrParser *p, ShrProgramAst *prog);
void shr_expr_free(ShrExpr *e);
void shr_stmt_free(ShrStmt *st);
void shr_program_ast_free(ShrProgramAst *prog);
