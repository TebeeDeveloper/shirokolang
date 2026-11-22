/* shrko/vm.h */
#pragma once

#include "bc_writer.h"
#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

#define SHR_MAX_MULTI_RET 64u
#define SHR_MAX_CLOSURES  4096u
#define SHR_MAX_CAPTURES  32u
#define SHR_MAX_LISTS     4096u
#define SHR_MAX_CALL_ARGS 32u

#ifndef OP_NEW_LIST
#define OP_NEW_LIST    0xA0u
#endif
#ifndef OP_LIST_APPEND
#define OP_LIST_APPEND 0xA1u
#endif
#ifndef OP_LIST_GET
#define OP_LIST_GET    0xA2u
#endif
#ifndef OP_LIST_SET
#define OP_LIST_SET    0xA3u
#endif

typedef struct ShrModuleTable ShrModuleTable;

typedef union ShrSlot {
    int32_t  i32;
    int64_t  i64;
    uint32_t u32;
    uint64_t u64;
    float    f32;
    double   f64;
    uint8_t  u8;
    bool     b;
    void    *ptr;
    struct {
        uint16_t off;
        uint16_t len;
    } rodata_ref;
} ShrSlot;

typedef struct ShrListObj {
    union ShrSlot *items;
    size_t len;
    size_t cap;
    uint8_t elem_type;
} ShrListObj;

typedef struct {
    uint16_t func_idx;
    uint8_t  capture_count;
    ShrSlot  captures[SHR_MAX_CAPTURES];
} ShrClosure;

typedef struct {
    ShrBcHeader hdr;
    uint8_t *rodata;
    ShrBcModuleEntry *modules;
    ShrBcFuncEntry *funcs;
    uint8_t *code;
} ShrProgram;

typedef struct ShrVm ShrVm;

struct ShrVm {
    const ShrProgram *prog;
    ShrModuleTable *mods;
    ShrSlot *caller_fp;
    uint8_t arg_slots[SHR_MAX_CALL_ARGS];
    uint8_t arg_count;
    uint8_t ret_dsts[SHR_MAX_MULTI_RET];
    uint8_t ret_dst_count;
    uint8_t last_ret_count;
    int32_t last_ret_i32;
    int64_t last_ret_i64;
    ShrClosure closures[SHR_MAX_CLOSURES];
    size_t closure_count;
    ShrListObj *lists[SHR_MAX_LISTS];
    size_t list_count;
    bool ok;
};

bool shr_program_load(ShrProgram *p, const char *path);
void shr_program_unload(ShrProgram *p);
void shr_vm_init(ShrVm *vm, const ShrProgram *prog);
int32_t shr_vm_alloc_closure(ShrVm *vm, uint16_t func_idx, const ShrSlot *caps, uint8_t cap_count);
bool shr_vm_run_entry(ShrVm *vm);
