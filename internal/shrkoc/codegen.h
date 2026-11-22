/* shrkc/codegen.h */
#pragma once

#include "sema.h"
#include "bc_writer.h"

typedef struct {
    const char *name;
    size_t name_len;
    uint8_t slot;
    ShrTypeKind type;
    size_t closure_func_idx;
} ShrCgVar;

typedef struct {
    ShrBcWriter bc;
    ShrCgVar vars[256];
    size_t var_count;
    uint8_t next_slot;
} ShrCodegen;

void shr_codegen_init(ShrCodegen *cg);
bool shr_codegen_emit_program(ShrCodegen *cg, const ShrProgramAst *prog);
void shr_codegen_free(ShrCodegen *cg);
