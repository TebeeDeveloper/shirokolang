/* shrkc/bc_writer.h */
#pragma once

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

#define SHRBC_MAGIC_0 'S'
#define SHRBC_MAGIC_1 'H'
#define SHRBC_MAGIC_2 'B'
#define SHRBC_MAGIC_3 'C'

#define SHRBC_FUNC_ELLIPSIS 0x01u

enum {
    OP_NOP = 0x00,
    OP_GC  = 0x01,
    OP_MOV_I32 = 0x10,
    OP_MOV_I64,
    OP_MOV_U32,
    OP_MOV_U64,
    OP_MOV_F32,
    OP_MOV_F64,
    OP_MOV_BYTE,
    OP_MOV_BOOL,
    OP_MOV_RODATA,
    OP_COPY_SLOT,
    OP_ADD_I32 = 0x20,
    OP_ADD_I64,
    OP_ADD_U32,
    OP_ADD_U64,
    OP_ADD_F32,
    OP_ADD_F64,
    OP_ADD_BYTE,
    OP_SUB_I32,
    OP_SUB_I64,
    OP_SUB_U32,
    OP_SUB_U64,
    OP_SUB_F32,
    OP_SUB_F64,
    OP_MUL_I32,
    OP_MUL_I64,
    OP_MUL_U32,
    OP_MUL_U64,
    OP_MUL_F32,
    OP_MUL_F64,
    OP_DIV_I32,
    OP_DIV_U32,
    OP_DIV_F32,
    OP_DIV_F64,
    OP_MOD_I32,
    OP_MOD_U32,
    OP_FMA_I32,
    OP_FMA_I64,
    OP_FMA_U32,
    OP_FMA_U64,
    OP_FMA_F32,
    OP_FMA_F64,
    OP_AND_BOOL,
    OP_OR_BOOL,
    OP_JE_I32 = 0x40,
    OP_JNE_I32,
    OP_JL_I32,
    OP_JLE_I32,
    OP_JG_I32,
    OP_JGE_I32,
    OP_JE_BOOL,
    OP_JMP,
    OP_CALL      = 0x50,
    OP_RET_SLOT  = 0x51,
    OP_RET_VOID  = 0x52,
    OP_RET_MULTI = 0x53,
};

typedef struct {
    uint8_t  magic[4];
    uint16_t version;
    uint16_t flags;
    uint16_t rodata_size;
    uint16_t module_count;
    uint16_t func_count;
    uint16_t code_size;
} ShrBcHeader;

typedef struct {
    uint16_t name_off;
    uint16_t name_len;
    uint16_t flags;
} ShrBcModuleEntry;

typedef struct {
    uint16_t name_off;
    uint16_t name_len;
    uint16_t code_offset;
    uint16_t code_size;
    uint8_t  param_count;
    uint8_t  slot_count;
    uint8_t  ret_count;
    uint8_t  flags;
} ShrBcFuncEntry;

typedef struct {
    uint8_t *rodata;
    size_t rodata_size, rodata_cap;
    ShrBcModuleEntry *modules;
    size_t module_count, module_cap;
    ShrBcFuncEntry *funcs;
    size_t func_count, func_cap;
    uint8_t *code;
    size_t code_size, code_cap;
} ShrBcWriter;

void shr_bc_writer_init(ShrBcWriter *w);
uint16_t shr_bc_writer_add_rodata(ShrBcWriter *w, const void *data, size_t len);
uint16_t shr_bc_writer_add_module(ShrBcWriter *w, const char *name, size_t len, uint16_t flags);
uint16_t shr_bc_writer_add_func(ShrBcWriter *w, ShrBcFuncEntry fn);
size_t shr_bc_writer_emit_u8(ShrBcWriter *w, uint8_t v);
size_t shr_bc_writer_emit_u16(ShrBcWriter *w, uint16_t v);
size_t shr_bc_writer_emit_u32(ShrBcWriter *w, uint32_t v);
size_t shr_bc_writer_emit_i32(ShrBcWriter *w, int32_t v);
size_t shr_bc_writer_emit_f64(ShrBcWriter *w, double v);
bool shr_bc_writer_write_file(const ShrBcWriter *w, const char *path);
void shr_bc_writer_free(ShrBcWriter *w);

