/* shrko/vm.c */
#include "vm.h"
#include "module.h"
#include "codegen.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <limits.h>

#define SHR_STACK_SLOTS 65536u
#define SHR_CALL_DEPTH  1024u

static uint16_t rd_u16_le(const uint8_t *p) {
    return (uint16_t)p[0] | ((uint16_t)p[1] << 8);
}

static int16_t rd_i16_le(const uint8_t *p) {
    return (int16_t)rd_u16_le(p);
}

static uint32_t rd_u32_le(const uint8_t *p) {
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) |
           ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

static int32_t rd_i32_le(const uint8_t *p) {
    return (int32_t)rd_u32_le(p);
}

static uint64_t rd_u64_le(const uint8_t *p) {
    uint64_t v = 0;
    for (int i = 0; i < 8; i++) v |= ((uint64_t)p[i]) << (i * 8);
    return v;
}

static int64_t rd_i64_le(const uint8_t *p) {
    return (int64_t)rd_u64_le(p);
}

static float rd_f32_le(const uint8_t *p) {
    uint32_t bits = rd_u32_le(p);
    float out = 0.0f;
    memcpy(&out, &bits, sizeof(out));
    return out;
}

static double rd_f64_le(const uint8_t *p) {
    uint64_t bits = rd_u64_le(p);
    double out = 0.0;
    memcpy(&out, &bits, sizeof(out));
    return out;
}

static void free_vm_lists(ShrVm *vm) {
    if (!vm) return;
    for (size_t i = 0; i < vm->list_count; i++) {
        ShrListObj *lst = vm->lists[i];
        if (lst) {
            free(lst->items);
            free(lst);
            vm->lists[i] = nullptr;
        }
    }
    vm->list_count = 0;
}

bool shr_program_load(ShrProgram *p, const char *path) {
    memset(p, 0, sizeof(*p));
    FILE *fp = fopen(path, "rb");
    if (!fp) {
        fprintf(stderr, "vm load error: cannot open file '%s'\n", path ? path : "(null)");
        return false;
    }

    uint8_t hb[16];
    if (fread(hb, 1, sizeof(hb), fp) != sizeof(hb)) {
        fprintf(stderr, "vm load error: truncated header in '%s'\n", path);
        fclose(fp);
        return false;
    }
    if (hb[0] != SHRBC_MAGIC_0 || hb[1] != SHRBC_MAGIC_1 ||
        hb[2] != SHRBC_MAGIC_2 || hb[3] != SHRBC_MAGIC_3) {
        fprintf(stderr, "vm load error: invalid magic bytes in '%s'\n", path);
        fclose(fp);
        return false;
    }

    p->hdr = (ShrBcHeader){
        .magic = { hb[0], hb[1], hb[2], hb[3] },
        .version = rd_u16_le(hb + 4),
        .flags = rd_u16_le(hb + 6),
        .rodata_size = rd_u16_le(hb + 8),
        .module_count = rd_u16_le(hb + 10),
        .func_count = rd_u16_le(hb + 12),
        .code_size = rd_u16_le(hb + 14),
    };

    if (p->hdr.rodata_size > 0) {
        p->rodata = malloc(p->hdr.rodata_size);
        if (!p->rodata || fread(p->rodata, 1, p->hdr.rodata_size, fp) != p->hdr.rodata_size) {
            fprintf(stderr, "vm load error: failed reading rodata in '%s'\n", path);
            shr_program_unload(p);
            fclose(fp);
            return false;
        }
    }

    if (p->hdr.module_count > 0) {
        p->modules = malloc(p->hdr.module_count * sizeof(ShrBcModuleEntry));
        if (!p->modules) {
            fprintf(stderr, "vm load error: out of memory for modules\n");
            shr_program_unload(p);
            fclose(fp);
            return false;
        }
        for (size_t i = 0; i < p->hdr.module_count; i++) {
            uint8_t mb[6];
            if (fread(mb, 1, 6, fp) != 6) {
                fprintf(stderr, "vm load error: truncated module entry %zu in '%s'\n", i, path);
                shr_program_unload(p);
                fclose(fp);
                return false;
            }
            p->modules[i] = (ShrBcModuleEntry){ rd_u16_le(mb), rd_u16_le(mb + 2), rd_u16_le(mb + 4) };
        }
    }

    if (p->hdr.func_count > 0) {
        p->funcs = malloc(p->hdr.func_count * sizeof(ShrBcFuncEntry));
        if (!p->funcs) {
            fprintf(stderr, "vm load error: out of memory for functions\n");
            shr_program_unload(p);
            fclose(fp);
            return false;
        }
        for (size_t i = 0; i < p->hdr.func_count; i++) {
            uint8_t fb[12];
            if (fread(fb, 1, 12, fp) != 12) {
                fprintf(stderr, "vm load error: truncated function entry %zu in '%s'\n", i, path);
                shr_program_unload(p);
                fclose(fp);
                return false;
            }
            p->funcs[i] = (ShrBcFuncEntry){
                rd_u16_le(fb), rd_u16_le(fb + 2), rd_u16_le(fb + 4), rd_u16_le(fb + 6),
                fb[8], fb[9], fb[10], fb[11]
            };
        }
    }

    if (p->hdr.code_size > 0) {
        p->code = malloc(p->hdr.code_size);
        if (!p->code || fread(p->code, 1, p->hdr.code_size, fp) != p->hdr.code_size) {
            fprintf(stderr, "vm load error: failed reading code in '%s'\n", path);
            shr_program_unload(p);
            fclose(fp);
            return false;
        }
    }

    fclose(fp);
    return true;
}

void shr_program_unload(ShrProgram *p) {
    free(p->rodata); free(p->modules); free(p->funcs); free(p->code);
    memset(p, 0, sizeof(*p));
}

void shr_vm_init(ShrVm *vm, const ShrProgram *prog) {
    memset(vm, 0, sizeof(*vm));
    vm->prog = prog;
    vm->closure_count = 0;
    vm->list_count = 0;
    vm->ok = true;
}

int32_t shr_vm_alloc_closure(ShrVm *vm, uint16_t func_idx, const ShrSlot *caps, uint8_t cap_count) {
    if (!vm || vm->closure_count >= SHR_MAX_CLOSURES) {
        fprintf(stderr, "vm error: closure table full or invalid vm\n");
        return -1;
    }
    size_t id = vm->closure_count++;
    ShrClosure *cl = &vm->closures[id];
    memset(cl, 0, sizeof(*cl));
    cl->func_idx = func_idx;
    uint8_t n = cap_count > SHR_MAX_CAPTURES ? SHR_MAX_CAPTURES : cap_count;
    cl->capture_count = n;
    if (caps && n > 0) {
        memcpy(cl->captures, caps, n * sizeof(ShrSlot));
    }
    return (int32_t)id;
}

#if defined(__GNUC__) || defined(__clang__)
#define SHR_HAS_COMPUTED_GOTO 1
#else
#define SHR_HAS_COMPUTED_GOTO 0
#endif

static bool exec_stack_vm(ShrVm *vm) {
    if (!vm->prog || vm->prog->hdr.func_count == 0) {
        fprintf(stderr, "vm error: program has no functions\n");
        return false;
    }

    ShrSlot *stack = calloc(SHR_STACK_SLOTS, sizeof(ShrSlot));
    if (!stack) {
        fprintf(stderr, "vm error: failed to allocate stack\n");
        vm->ok = false;
        return false;
    }

    typedef struct {
        const uint8_t *ret_ip;
        ShrSlot *caller_fp;
        const ShrBcFuncEntry *caller_fn;
        uint8_t ret_dsts[SHR_MAX_MULTI_RET];
        uint8_t ret_dst_count;
    } CallFrame;

    CallFrame call_stack[SHR_CALL_DEPTH];
    size_t call_sp = 0;

    const uint8_t *code = vm->prog->code;
    const uint8_t *code_end = code + vm->prog->hdr.code_size;
    const ShrBcFuncEntry *fn = &vm->prog->funcs[0];
    ShrSlot *fp = stack;
    const uint8_t *ip = code + fn->code_offset;
    const char *err_reason = "unhandled or invalid opcode";

#if SHR_HAS_COMPUTED_GOTO
    static const void *dispatch_table[256] = {
        [0 ... 255]      = &&L_ERR,
        [OP_NOP]         = &&L_OP_NOP,
        [OP_GC]          = &&L_OP_GC,
        [OP_MOV_I32]     = &&L_OP_MOV_I32,
        [OP_MOV_I64]     = &&L_OP_MOV_I64,
        [OP_MOV_U32]     = &&L_OP_MOV_U32,
        [OP_MOV_U64]     = &&L_OP_MOV_U64,
        [OP_MOV_F32]     = &&L_OP_MOV_F32,
        [OP_MOV_F64]     = &&L_OP_MOV_F64,
        [OP_MOV_BYTE]    = &&L_OP_MOV_BYTE,
        [OP_MOV_BOOL]    = &&L_OP_MOV_BOOL,
        [OP_MOV_RODATA]  = &&L_OP_MOV_RODATA,
        [OP_COPY_SLOT]   = &&L_OP_COPY_SLOT,
        [OP_NEW_LIST]    = &&L_OP_NEW_LIST,
        [OP_LIST_APPEND] = &&L_OP_LIST_APPEND,
        [OP_LIST_GET]    = &&L_OP_LIST_GET,
        [OP_LIST_SET]    = &&L_OP_LIST_SET,
        [OP_ADD_I32]     = &&L_OP_ADD_I32,
        [OP_ADD_I64]     = &&L_OP_ADD_I64,
        [OP_ADD_U32]     = &&L_OP_ADD_U32,
        [OP_ADD_U64]     = &&L_OP_ADD_U64,
        [OP_ADD_F32]     = &&L_OP_ADD_F32,
        [OP_ADD_F64]     = &&L_OP_ADD_F64,
        [OP_ADD_BYTE]    = &&L_OP_ADD_BYTE,
        [OP_SUB_I32]     = &&L_OP_SUB_I32,
        [OP_SUB_I64]     = &&L_OP_SUB_I64,
        [OP_SUB_U32]     = &&L_OP_SUB_U32,
        [OP_SUB_U64]     = &&L_OP_SUB_U64,
        [OP_SUB_F32]     = &&L_OP_SUB_F32,
        [OP_SUB_F64]     = &&L_OP_SUB_F64,
        [OP_MUL_I32]     = &&L_OP_MUL_I32,
        [OP_MUL_I64]     = &&L_OP_MUL_I64,
        [OP_MUL_U32]     = &&L_OP_MUL_U32,
        [OP_MUL_U64]     = &&L_OP_MUL_U64,
        [OP_MUL_F32]     = &&L_OP_MUL_F32,
        [OP_MUL_F64]     = &&L_OP_MUL_F64,
        [OP_DIV_I32]     = &&L_OP_DIV_I32,
        [OP_DIV_U32]     = &&L_OP_DIV_U32,
        [OP_DIV_F32]     = &&L_OP_DIV_F32,
        [OP_DIV_F64]     = &&L_OP_DIV_F64,
        [OP_MOD_I32]     = &&L_OP_MOD_I32,
        [OP_MOD_U32]     = &&L_OP_MOD_U32,
        [OP_FMA_I32]     = &&L_OP_FMA_I32,
        [OP_FMA_I64]     = &&L_OP_FMA_I64,
        [OP_FMA_U32]     = &&L_OP_FMA_U32,
        [OP_FMA_U64]     = &&L_OP_FMA_U64,
        [OP_FMA_F32]     = &&L_OP_FMA_F32,
        [OP_FMA_F64]     = &&L_OP_FMA_F64,
        [OP_AND_BOOL]    = &&L_OP_AND_BOOL,
        [OP_OR_BOOL]     = &&L_OP_OR_BOOL,
        [OP_JE_I32]      = &&L_OP_JE_I32,
        [OP_JNE_I32]     = &&L_OP_JNE_I32,
        [OP_JL_I32]      = &&L_OP_JL_I32,
        [OP_JLE_I32]     = &&L_OP_JLE_I32,
        [OP_JG_I32]      = &&L_OP_JG_I32,
        [OP_JGE_I32]     = &&L_OP_JGE_I32,
        [OP_JE_BOOL]     = &&L_OP_JE_BOOL,
        [OP_JMP]         = &&L_OP_JMP,
        [OP_CALL]        = &&L_OP_CALL,
        [OP_RET_SLOT]    = &&L_OP_RET_SLOT,
        [OP_RET_VOID]    = &&L_OP_RET_VOID,
        [OP_RET_MULTI]   = &&L_OP_RET_MULTI,
    };
#define SHR_REL_JMP(cond, rel_val) do { \
    if ((cond)) { \
        const uint8_t *_dst = ip + (rel_val); \
        if (_dst < code || _dst > code_end) { \
            err_reason = "relative jump target out of code bounds"; \
            fprintf(stderr, "vm error: rel=%d target_off=%td (code_size=%u)\n", \
                    (int)(rel_val), _dst - code, vm->prog->hdr.code_size); \
            vm->ok = false; \
            goto L_ERR; \
        } \
        ip = _dst; \
    } \
} while (0)
#define DISPATCH() goto *dispatch_table[*ip++]
#define CASE(op)   L_##op:
#define NEXT()     goto *dispatch_table[*ip++]
    DISPATCH();
#else
#define SHR_REL_JMP(cond, rel_val) do { \
    if ((cond)) { \
        const uint8_t *_dst = ip + (rel_val); \
        if (_dst < code || _dst > code_end) { \
            err_reason = "relative jump target out of code bounds"; \
            fprintf(stderr, "vm error: rel=%d target_off=%td (code_size=%u)\n", \
                    (int)(rel_val), _dst - code, vm->prog->hdr.code_size); \
            vm->ok = false; \
            goto L_ERR; \
        } \
        ip = _dst; \
    } \
} while (0)
#define DISPATCH() for (;;) { switch (*ip++) {
#define CASE(op)   case op:
#define NEXT()     break;
    DISPATCH();
#endif

    CASE(OP_NOP) NEXT();
    CASE(OP_GC) NEXT();

    CASE(OP_MOV_I32)    { uint8_t d = *ip++; fp[d].i32 = rd_i32_le(ip); ip += 4; NEXT(); }
    CASE(OP_MOV_I64)    { uint8_t d = *ip++; fp[d].i64 = rd_i64_le(ip); ip += 8; NEXT(); }
    CASE(OP_MOV_U32)    { uint8_t d = *ip++; fp[d].u32 = rd_u32_le(ip); ip += 4; NEXT(); }
    CASE(OP_MOV_U64)    { uint8_t d = *ip++; fp[d].u64 = rd_u64_le(ip); ip += 8; NEXT(); }
    CASE(OP_MOV_F32)    { uint8_t d = *ip++; fp[d].f32 = rd_f32_le(ip); ip += 4; NEXT(); }
    CASE(OP_MOV_F64)    { uint8_t d = *ip++; fp[d].f64 = rd_f64_le(ip); ip += 8; NEXT(); }
    CASE(OP_MOV_BYTE)   { uint8_t d = *ip++; fp[d].u8  = *ip++; NEXT(); }
    CASE(OP_MOV_BOOL)   { uint8_t d = *ip++; fp[d].b   = (*ip++) != 0; NEXT(); }
    CASE(OP_MOV_RODATA) {
        uint8_t d = *ip++;
        memset(&fp[d], 0, sizeof(fp[d]));
        fp[d].rodata_ref.off = rd_u16_le(ip); ip += 2;
        fp[d].rodata_ref.len = rd_u16_le(ip); ip += 2;
        NEXT();
    }
    CASE(OP_COPY_SLOT) {
        uint8_t d = *ip++;
        uint8_t s = *ip++;
        fp[d] = fp[s];
        NEXT();
    }

    CASE(OP_NEW_LIST) {
        uint8_t dst = *ip++;
        uint8_t elem_ty = *ip++;
        uint16_t init_cap = rd_u16_le(ip); ip += 2;

        ShrListObj *lst = calloc(1, sizeof(ShrListObj));
        if (!lst) {
            err_reason = "out of memory allocating list object";
            vm->ok = false;
            goto L_ERR;
        }
        lst->elem_type = elem_ty;
        lst->len = 0;
        lst->cap = (init_cap > 0) ? (size_t)init_cap : 4u;
        lst->items = calloc(lst->cap, sizeof(ShrSlot));
        if (!lst->items) {
            free(lst);
            lst = nullptr;
            err_reason = "out of memory allocating list elements";
            vm->ok = false;
            goto L_ERR;
        }
        if (vm->list_count < SHR_MAX_LISTS) {
            vm->lists[vm->list_count++] = lst;
        }
        memset(&fp[dst], 0, sizeof(fp[dst]));
        fp[dst].ptr = lst;
        NEXT();
    }

    CASE(OP_LIST_APPEND) {
        uint8_t lst_s = *ip++;
        uint8_t val_s = *ip++;
        ShrListObj *lst = (ShrListObj *)fp[lst_s].ptr;
        if (!lst) {
            err_reason = "null list in OP_LIST_APPEND";
            vm->ok = false;
            goto L_ERR;
        }
        if (lst->len == lst->cap) {
            size_t new_cap = (lst->cap > 0) ? (lst->cap * 2u) : 4u;
            ShrSlot *tmp = realloc(lst->items, new_cap * sizeof(ShrSlot));
            if (!tmp) {
                err_reason = "out of memory growing list";
                vm->ok = false;
                goto L_ERR;
            }
            lst->items = tmp;
            lst->cap = new_cap;
        }
        lst->items[lst->len++] = fp[val_s];
        NEXT();
    }

    CASE(OP_LIST_GET) {
        uint8_t dst = *ip++;
        uint8_t lst_s = *ip++;
        uint8_t idx_s = *ip++;
        ShrListObj *lst = (ShrListObj *)fp[lst_s].ptr;
        int32_t idx = fp[idx_s].i32;
        if (!lst || idx < 0 || (size_t)idx >= lst->len) {
            err_reason = "list index out of bounds";
            fprintf(stderr, "vm error: list index %d out of bounds (len=%zu)\n",
                    idx, lst ? lst->len : 0u);
            vm->ok = false;
            goto L_ERR;
        }
        fp[dst] = lst->items[(size_t)idx];
        NEXT();
    }

    CASE(OP_LIST_SET) {
        uint8_t lst_s = *ip++;
        uint8_t idx_s = *ip++;
        uint8_t val_s = *ip++;
        ShrListObj *lst = (ShrListObj *)fp[lst_s].ptr;
        int32_t idx = fp[idx_s].i32;
        if (!lst || idx < 0 || (size_t)idx >= lst->len) {
            err_reason = "list index out of bounds on write";
            fprintf(stderr, "vm error: list index %d out of bounds on write (len=%zu)\n",
                    idx, lst ? lst->len : 0u);
            vm->ok = false;
            goto L_ERR;
        }
        lst->items[(size_t)idx] = fp[val_s];
        NEXT();
    }

    CASE(OP_ADD_I32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i32  = fp[a].i32  + fp[b].i32;  NEXT(); }
    CASE(OP_ADD_I64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i64  = fp[a].i64  + fp[b].i64;  NEXT(); }
    CASE(OP_ADD_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u32  = fp[a].u32  + fp[b].u32;  NEXT(); }
    CASE(OP_ADD_U64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u64  = fp[a].u64  + fp[b].u64;  NEXT(); }
    CASE(OP_ADD_F32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f32  = fp[a].f32  + fp[b].f32;  NEXT(); }
    CASE(OP_ADD_F64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f64  = fp[a].f64  + fp[b].f64;  NEXT(); }
    CASE(OP_ADD_BYTE) { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u8   = (uint8_t)(fp[a].u8 + fp[b].u8); NEXT(); }

    CASE(OP_SUB_I32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i32  = fp[a].i32  - fp[b].i32;  NEXT(); }
    CASE(OP_SUB_I64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i64  = fp[a].i64  - fp[b].i64;  NEXT(); }
    CASE(OP_SUB_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u32  = fp[a].u32  - fp[b].u32;  NEXT(); }
    CASE(OP_SUB_U64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u64  = fp[a].u64  - fp[b].u64;  NEXT(); }
    CASE(OP_SUB_F32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f32  = fp[a].f32  - fp[b].f32;  NEXT(); }
    CASE(OP_SUB_F64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f64  = fp[a].f64  - fp[b].f64;  NEXT(); }

    CASE(OP_MUL_I32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i32  = fp[a].i32  * fp[b].i32;  NEXT(); }
    CASE(OP_MUL_I64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].i64  = fp[a].i64  * fp[b].i64;  NEXT(); }
    CASE(OP_MUL_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u32  = fp[a].u32  * fp[b].u32;  NEXT(); }
    CASE(OP_MUL_U64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u64  = fp[a].u64  * fp[b].u64;  NEXT(); }
    CASE(OP_MUL_F32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f32  = fp[a].f32  * fp[b].f32;  NEXT(); }
    CASE(OP_MUL_F64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f64  = fp[a].f64  * fp[b].f64;  NEXT(); }

    CASE(OP_DIV_I32)  {
        uint8_t d = *ip++, a = *ip++, b = *ip++;
        if (fp[b].i32 == 0 || (fp[a].i32 == INT32_MIN && fp[b].i32 == -1)) fp[d].i32 = 0;
        else fp[d].i32 = fp[a].i32 / fp[b].i32;
        NEXT();
    }
    CASE(OP_DIV_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u32  = fp[b].u32 ? fp[a].u32 / fp[b].u32 : 0; NEXT(); }
    CASE(OP_DIV_F32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f32  = fp[a].f32 / fp[b].f32; NEXT(); }
    CASE(OP_DIV_F64)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].f64  = fp[a].f64 / fp[b].f64; NEXT(); }

    CASE(OP_MOD_I32)  {
        uint8_t d = *ip++, a = *ip++, b = *ip++;
        if (fp[b].i32 == 0 || (fp[a].i32 == INT32_MIN && fp[b].i32 == -1)) fp[d].i32 = 0;
        else fp[d].i32 = fp[a].i32 % fp[b].i32;
        NEXT();
    }
    CASE(OP_MOD_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].u32  = fp[b].u32 ? fp[a].u32 % fp[b].u32 : 0; NEXT(); }

    CASE(OP_FMA_I32)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].i32 = fp[a].i32 * fp[b].i32 + fp[c].i32; NEXT(); }
    CASE(OP_FMA_I64)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].i64 = fp[a].i64 * fp[b].i64 + fp[c].i64; NEXT(); }
    CASE(OP_FMA_U32)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].u32 = fp[a].u32 * fp[b].u32 + fp[c].u32; NEXT(); }
    CASE(OP_FMA_U64)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].u64 = fp[a].u64 * fp[b].u64 + fp[c].u64; NEXT(); }
    CASE(OP_FMA_F32)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].f32 = fmaf(fp[a].f32, fp[b].f32, fp[c].f32); NEXT(); }
    CASE(OP_FMA_F64)  { uint8_t d = *ip++, a = *ip++, b = *ip++, c = *ip++; fp[d].f64 = fma(fp[a].f64, fp[b].f64, fp[c].f64); NEXT(); }

    CASE(OP_AND_BOOL) { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].b = fp[a].b && fp[b].b; NEXT(); }
    CASE(OP_OR_BOOL)  { uint8_t d = *ip++, a = *ip++, b = *ip++; fp[d].b = fp[a].b || fp[b].b; NEXT(); }

    CASE(OP_JE_I32)  { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 == fp[b].i32, r); NEXT(); }
    CASE(OP_JNE_I32) { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 != fp[b].i32, r); NEXT(); }
    CASE(OP_JL_I32)  { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 <  fp[b].i32, r); NEXT(); }
    CASE(OP_JLE_I32) { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 <= fp[b].i32, r); NEXT(); }
    CASE(OP_JG_I32)  { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 >  fp[b].i32, r); NEXT(); }
    CASE(OP_JGE_I32) { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].i32 >= fp[b].i32, r); NEXT(); }
    CASE(OP_JE_BOOL) { uint8_t a = *ip++, b = *ip++; int16_t r = rd_i16_le(ip); ip += 2; SHR_REL_JMP(fp[a].b   == fp[b].b,   r); NEXT(); }

    CASE(OP_JMP) {
        int16_t r = rd_i16_le(ip);
        ip += 2;
        SHR_REL_JMP(1, r);
        NEXT();
    }

    CASE(OP_CALL) {
        uint8_t ret_n = *ip++;
        if (ret_n > SHR_MAX_MULTI_RET) ret_n = SHR_MAX_MULTI_RET;
        uint8_t ret_dsts[SHR_MAX_MULTI_RET];
        for (uint8_t r = 0; r < ret_n; r++) {
            ret_dsts[r] = *ip++;
        }

        uint16_t fidx = rd_u16_le(ip); ip += 2;
        uint8_t argc = *ip++;
        uint8_t ell_start = *ip++;

        if (fidx == 0xFFFFu) {
            uint8_t arg_tks[SHR_MAX_CALL_ARGS] = {0};
            uint8_t arg_slots[SHR_MAX_CALL_ARGS] = {0};
            uint8_t take_a = argc > SHR_MAX_CALL_ARGS ? SHR_MAX_CALL_ARGS : argc;
            for (unsigned int i = 0; i < (unsigned int)argc; i++) {
                uint8_t tk = *ip++;
                uint8_t s = *ip++;
                if (i < (unsigned int)take_a) {
                    arg_tks[i] = tk;
                    arg_slots[i] = s;
                }
            }
            uint16_t sym_off = rd_u16_le(ip); ip += 2;
            uint16_t sym_len = rd_u16_le(ip); ip += 2;

            char sym_name[128];
            if (sym_len == 0 || sym_len >= sizeof(sym_name) ||
                (size_t)sym_off + sym_len > vm->prog->hdr.rodata_size || !vm->mods) {
                err_reason = "invalid native symbol metadata or missing module table";
                fprintf(stderr, "vm error: invalid native symbol metadata\n");
                vm->ok = false;
                goto L_ERR;
            }
            memcpy(sym_name, vm->prog->rodata + sym_off, sym_len);
            sym_name[sym_len] = '\0';

            ShrNativeFn nfn = nullptr;
            for (size_t m = 0; m < vm->prog->hdr.module_count; m++) {
                const ShrBcModuleEntry *me = &vm->prog->modules[m];
                char mod_name[128];
                if (me->name_len == 0 || me->name_len >= sizeof(mod_name) ||
                    (size_t)me->name_off + me->name_len > vm->prog->hdr.rodata_size) continue;
                memcpy(mod_name, vm->prog->rodata + me->name_off, me->name_len);
                mod_name[me->name_len] = '\0';
                void *p = shr_module_sym(vm->mods, mod_name, sym_name);
                if (p) {
                    memcpy(&nfn, &p, sizeof(nfn));
                    break;
                }
            }

            if (!nfn) {
                err_reason = "unresolved native symbol";
                fprintf(stderr, "vm error: unresolved native symbol '%s'\n", sym_name);
                vm->ok = false;
                goto L_ERR;
            }

            vm->caller_fp = fp;
            vm->arg_count = take_a;
            for (uint8_t a = 0; a < take_a; a++) {
                vm->arg_slots[a] = arg_slots[a];
            }
            vm->ret_dst_count = ret_n;
            for (uint8_t r = 0; r < ret_n; r++) vm->ret_dsts[r] = ret_dsts[r];
            (void)arg_tks;
            nfn(vm, argc, take_a > 0 ? arg_slots[0] : 0, take_a > 0 ? fp[arg_slots[0]] : (ShrSlot){0});
            if (!vm->ok) {
                err_reason = "native function failed";
                fprintf(stderr, "vm error: native function '%s' returned failure\n", sym_name);
                goto L_ERR;
            }
            NEXT();
        }

        if (fidx >= vm->prog->hdr.func_count) {
            err_reason = "function index out of range";
            fprintf(stderr, "vm error: call fidx=%u >= func_count=%u\n", fidx, vm->prog->hdr.func_count);
            vm->ok = false;
            goto L_ERR;
        }
        if (call_sp >= SHR_CALL_DEPTH) {
            err_reason = "call stack overflow";
            fprintf(stderr, "vm error: call stack overflow\n");
            vm->ok = false;
            goto L_ERR;
        }

        const ShrBcFuncEntry *callee = &vm->prog->funcs[fidx];
        uint8_t caller_step = fn->slot_count ? fn->slot_count : 1;
        ShrSlot *next_fp = fp + caller_step;
        uint8_t needed = callee->slot_count;
        if (argc > needed) needed = argc;
        if (needed == 0) needed = 1;

        if ((size_t)(next_fp - stack) + (size_t)needed >= SHR_STACK_SLOTS) {
            err_reason = "value stack overflow";
            fprintf(stderr, "vm error: value stack overflow\n");
            vm->ok = false;
            goto L_ERR;
        }

        bool is_variadic = (callee->flags & SHRBC_FUNC_ELLIPSIS) != 0;
        uint8_t fixed_limit = is_variadic ? ell_start : callee->param_count;
        uint8_t var_slot = fixed_limit;

        for (unsigned int i = 0; i < (unsigned int)argc; i++) {
            uint8_t tk = *ip++;
            uint8_t s = *ip++;
            (void)tk;
            if (i < (unsigned int)fixed_limit) {
                next_fp[i] = fp[s];
            } else if (is_variadic && var_slot < needed) {
                next_fp[var_slot++] = fp[s];
            }
        }

        CallFrame *cf = &call_stack[call_sp++];
        cf->ret_ip = ip;
        cf->caller_fp = fp;
        cf->caller_fn = fn;
        cf->ret_dst_count = ret_n;
        for (uint8_t r = 0; r < ret_n; r++) {
            cf->ret_dsts[r] = ret_dsts[r];
            vm->ret_dsts[r] = ret_dsts[r];
        }
        vm->caller_fp = fp;
        vm->ret_dst_count = ret_n;

        fp = next_fp;
        fn = callee;
        ip = code + callee->code_offset;
        NEXT();
    }

    CASE(OP_RET_SLOT) {
        uint8_t src = *ip++;
        uint8_t rt = *ip++;
        ShrSlot ret_val = fp[src];
        if (rt == SHR_TY_I64) vm->last_ret_i64 = ret_val.i64;
        else if (rt == SHR_TY_U64) vm->last_ret_i64 = (int64_t)ret_val.u64;
        else if (rt == SHR_TY_F64) {
            int64_t bits = 0;
            memcpy(&bits, &ret_val.f64, sizeof(bits));
            vm->last_ret_i64 = bits;
        } else {
            vm->last_ret_i32 = ret_val.i32;
        }
        vm->last_ret_count = 1;

        if (call_sp == 0) {
            free_vm_lists(vm);
            free(stack);
            return true;
        }
        CallFrame cf = call_stack[--call_sp];
        if (cf.ret_dst_count > 0) {
            cf.caller_fp[cf.ret_dsts[0]] = ret_val;
        }
        fp = cf.caller_fp;
        fn = cf.caller_fn;
        ip = cf.ret_ip;
        vm->caller_fp = call_sp > 0 ? call_stack[call_sp - 1].caller_fp : nullptr;
        vm->ret_dst_count = 0;
        NEXT();
    }

    CASE(OP_RET_MULTI) {
        uint8_t ret_n = *ip++;
        uint8_t srcs[SHR_MAX_MULTI_RET];
        uint8_t rts[SHR_MAX_MULTI_RET];
        uint8_t take = ret_n > SHR_MAX_MULTI_RET ? SHR_MAX_MULTI_RET : ret_n;

        for (unsigned int i = 0; i < (unsigned int)ret_n; i++) {
            uint8_t s = *ip++;
            uint8_t t = *ip++;
            if (i < SHR_MAX_MULTI_RET) {
                srcs[i] = s;
                rts[i] = t;
            }
        }

        if (take > 0) {
            vm->last_ret_count = take;
            if (rts[0] == SHR_TY_I64) vm->last_ret_i64 = fp[srcs[0]].i64;
            else vm->last_ret_i32 = fp[srcs[0]].i32;
        } else {
            vm->last_ret_count = 0;
        }

        if (call_sp == 0) {
            free_vm_lists(vm);
            free(stack);
            return true;
        }
        CallFrame cf = call_stack[--call_sp];
        uint8_t copy_n = take < cf.ret_dst_count ? take : cf.ret_dst_count;
        for (uint8_t i = 0; i < copy_n; i++) {
            cf.caller_fp[cf.ret_dsts[i]] = fp[srcs[i]];
        }
        fp = cf.caller_fp;
        fn = cf.caller_fn;
        ip = cf.ret_ip;
        vm->caller_fp = call_sp > 0 ? call_stack[call_sp - 1].caller_fp : nullptr;
        vm->ret_dst_count = 0;
        NEXT();
    }

    CASE(OP_RET_VOID) {
        vm->last_ret_count = 0;
        if (call_sp == 0) {
            free_vm_lists(vm);
            free(stack);
            return true;
        }
        CallFrame cf = call_stack[--call_sp];
        fp = cf.caller_fp;
        fn = cf.caller_fn;
        ip = cf.ret_ip;
        vm->caller_fp = call_sp > 0 ? call_stack[call_sp - 1].caller_fp : nullptr;
        vm->ret_dst_count = 0;
        NEXT();
    }

#if SHR_HAS_COMPUTED_GOTO
L_ERR:
#else
    CASE(ERR)
#endif
    {
        size_t err_off = (ip > code) ? (size_t)((ip - 1) - code) : 0;
        uint8_t bad_op = (ip > code && (size_t)(ip - code) <= vm->prog->hdr.code_size) ? *(ip - 1) : 0;
        fprintf(stderr, "vm runtime error: %s (code_off=%zu, op=0x%02x, fn_idx=%zu, call_sp=%zu)\n",
                err_reason, err_off, bad_op, (size_t)(fn - vm->prog->funcs), call_sp);
        vm->ok = false;
        free_vm_lists(vm);
        free(stack);
        return false;
    }

#if !SHR_HAS_COMPUTED_GOTO
        }
    }
#endif
}

bool shr_vm_run_entry(ShrVm *vm) {
    if (!vm || !vm->prog || vm->prog->hdr.func_count == 0) {
        fprintf(stderr, "vm error: invalid vm state or empty function table\n");
        return false;
    }
    return exec_stack_vm(vm);
}
