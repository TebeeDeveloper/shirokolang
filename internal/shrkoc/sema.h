/* shrkc/sema.h */
#pragma once

#include "parser.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
    const char *name;
    size_t name_len;
    uint8_t slot;
    ShrTypeKind type;
    ShrTypeSpec spec;
    size_t closure_func_idx;
} ShrVarSym;

typedef struct {
    const char *name;
    size_t name_len;
    uint16_t func_idx;
    ShrTypeList ret_types;
    bool is_native;
} ShrFuncSym;

typedef struct {
    ShrVarSym vars[256];
    size_t var_count;
    ShrFuncSym funcs[256];
    size_t func_count;
    ShrTypeList cur_ret_types;
    bool ok;
    int err_line;
    char err_msg[256];
} ShrSema;

void shr_sema_init(ShrSema *s);
void shr_sema_fail(ShrSema *s, int line, const char *fmt, ...);
bool shr_sema_analyze(ShrSema *s, ShrProgramAst *prog);
