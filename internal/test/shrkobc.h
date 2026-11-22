#ifndef SHRKOBC_H
#define SHRKOBC_H

#include <math.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifndef VM_STACK_SIZE
#define VM_STACK_SIZE 4096u
#endif

typedef struct {
    uint8_t stack[VM_STACK_SIZE];
    size_t sp;
    size_t bp;
    int err;
} ShrkoVM;

static inline void vm_init(ShrkoVM *vm) {
    memset(vm->stack, 0, sizeof(vm->stack));
    vm->sp = 0;
    vm->bp = 0;
    vm->err = 0;
}

static inline void vm_trap(ShrkoVM *vm, int code, const char *msg) {
    vm->err = code;
    fprintf(stderr, "VM error (%d): %s\n", code, msg);
    exit(1);
}

/* --- Thao tác byte thô (nền cho mọi kiểu & struct) --- */
static inline void vm_push_bytes(ShrkoVM *vm, const void *src, size_t sz) {
    if (vm->sp + sz > VM_STACK_SIZE) {
        vm_trap(vm, 1, "stack overflow");
    }
    memcpy(vm->stack + vm->sp, src, sz);
    vm->sp += sz;
}

static inline void vm_pop_bytes(ShrkoVM *vm, void *dst, size_t sz) {
    if (vm->sp < sz) {
        vm_trap(vm, 2, "stack underflow");
    }
    vm->sp -= sz;
    memcpy(dst, vm->stack + vm->sp, sz);
}

static inline void vm_read_at(const ShrkoVM *vm, size_t addr, void *dst, size_t sz) {
    if (addr + sz > vm->sp) {
        fprintf(stderr, "VM error: read out of bounds\n");
        exit(1);
    }
    memcpy(dst, vm->stack + addr, sz);
}

static inline void vm_write_at(ShrkoVM *vm, size_t addr, const void *src, size_t sz) {
    if (addr + sz > vm->sp) {
        vm_trap(vm, 3, "write out of bounds");
    }
    memcpy(vm->stack + addr, src, sz);
}

/* --- Khôi phục / dọn stack --- */
static inline void vm_restore_sp(ShrkoVM *vm, size_t target_sp) {
    if (target_sp > vm->sp || target_sp < vm->bp) {
        vm_trap(vm, 4, "invalid target_sp");
    }
    vm->sp = target_sp;
}

static inline void vm_discard(ShrkoVM *vm, size_t bytes) {
    if (bytes > vm->sp - vm->bp) {
        vm_trap(vm, 5, "discard exceeds frame");
    }
    vm->sp -= bytes;
}

/* --- Khung gọi hàm theo ABI --- */
static inline void vm_enter_frame(ShrkoVM *vm) {
    size_t old_bp = vm->bp;
    vm_push_bytes(vm, &old_bp, sizeof(old_bp));
    vm->bp = vm->sp;
}

static inline void vm_leave_frame_bytes(ShrkoVM *vm, size_t args_bytes, const void *ret_ptr, size_t ret_sz) {
    vm->sp = vm->bp;
    size_t old_bp = 0;
    vm_pop_bytes(vm, &old_bp, sizeof(old_bp));
    vm->bp = old_bp;

    if (vm->sp < args_bytes) {
        vm_trap(vm, 6, "invalid args_bytes on return");
    }
    vm->sp -= args_bytes;

    if (ret_ptr != NULL && ret_sz > 0) {
        vm_push_bytes(vm, ret_ptr, ret_sz);
    }
}

/* --- Helper kiểu int64_t --- */
static inline void vm_push_i64(ShrkoVM *vm, int64_t v) { vm_push_bytes(vm, &v, sizeof(v)); }
static inline int64_t vm_pop_i64(ShrkoVM *vm) {
    int64_t v = 0;
    vm_pop_bytes(vm, &v, sizeof(v));
    return v;
}
static inline void vm_load_i64(ShrkoVM *vm, ptrdiff_t off) {
    int64_t v = 0;
    size_t addr = (size_t)((ptrdiff_t)vm->bp + off);
    vm_read_at(vm, addr, &v, sizeof(v));
    vm_push_i64(vm, v);
}
static inline void vm_store_i64(ShrkoVM *vm, ptrdiff_t off) {
    int64_t v = vm_pop_i64(vm);
    size_t addr = (size_t)((ptrdiff_t)vm->bp + off);
    vm_write_at(vm, addr, &v, sizeof(v));
}

/* --- Helper kiểu bool --- */
static inline void vm_push_bool(ShrkoVM *vm, bool v) { vm_push_bytes(vm, &v, sizeof(v)); }
static inline bool vm_pop_bool(ShrkoVM *vm) {
    bool v = false;
    vm_pop_bytes(vm, &v, sizeof(v));
    return v;
}
static inline void vm_load_bool(ShrkoVM *vm, ptrdiff_t off) {
    bool v = false;
    size_t addr = (size_t)((ptrdiff_t)vm->bp + off);
    vm_read_at(vm, addr, &v, sizeof(v));
    vm_push_bool(vm, v);
}
static inline void vm_store_bool(ShrkoVM *vm, ptrdiff_t off) {
    bool v = vm_pop_bool(vm);
    size_t addr = (size_t)((ptrdiff_t)vm->bp + off);
    vm_write_at(vm, addr, &v, sizeof(v));
}

/* --- Số học & so sánh cho int64_t --- */
static inline void vm_add_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_i64(vm, a + b); }
static inline void vm_sub_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_i64(vm, a - b); }
static inline void vm_mul_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_i64(vm, a * b); }
static inline void vm_div_i64(ShrkoVM *vm) {
    int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm);
    if (b == 0) vm_trap(vm, 7, "division by zero");
    vm_push_i64(vm, a / b);
}
static inline void vm_mod_i64(ShrkoVM *vm) {
    int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm);
    if (b == 0) vm_trap(vm, 8, "modulo by zero");
    vm_push_i64(vm, a % b);
}

static inline void vm_lt_i64(ShrkoVM *vm)  { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a < b); }
static inline void vm_gt_i64(ShrkoVM *vm)  { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a > b); }
static inline void vm_lte_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a <= b); }
static inline void vm_gte_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a >= b); }
static inline void vm_eq_i64(ShrkoVM *vm)  { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a == b); }
static inline void vm_neq_i64(ShrkoVM *vm) { int64_t b = vm_pop_i64(vm), a = vm_pop_i64(vm); vm_push_bool(vm, a != b); }

static inline void vm_ret_i64(ShrkoVM *vm, size_t args_bytes) {
    int64_t ret = vm_pop_i64(vm);
    vm_leave_frame_bytes(vm, args_bytes, &ret, sizeof(ret));
}

#define VM_JMP(label) goto label
#define VM_JMP_IF_FALSE(vm_ptr, label) \
    do { if (!vm_pop_bool((vm_ptr))) goto label; } while (0)

#endif
