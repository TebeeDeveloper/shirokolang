/* fmt.c */
#include "shiroko.h"
#include <stdio.h>
#include <stdint.h>
#include <stdbool.h>
#include <math.h>

static bool is_valid_rodata_str(const ShrProgram *prog, ShrSlot v) {
    if (!prog || !prog->rodata || prog->hdr.rodata_size == 0) return false;
    if (v.rodata_ref.len == 0) return false;
    if ((size_t)v.rodata_ref.off + (size_t)v.rodata_ref.len > prog->hdr.rodata_size) return false;
    for (uint16_t i = 0; i < v.rodata_ref.len; i++) {
        unsigned char ch = (unsigned char)prog->rodata[v.rodata_ref.off + i];
        if (ch < 0x20 && ch != '\n' && ch != '\r' && ch != '\t') return false;
    }
    return true;
}

static const ShrListObj *as_vm_list(const ShrVm *vm, ShrSlot v) {
    if (!vm || !v.ptr) return nullptr;
    for (size_t i = 0; i < vm->list_count; i++) {
        if (vm->lists[i] == v.ptr) {
            return vm->lists[i];
        }
    }
    return nullptr;
}

static void print_slot(const ShrVm *vm, ShrSlot v);

static bool try_print_list(const ShrVm *vm, ShrSlot v) {
    const ShrListObj *lst = as_vm_list(vm, v);
    if (!lst) return false;

    putchar('[');
    for (size_t i = 0; i < lst->len; i++) {
        if (i > 0) printf(", ");
        if (lst->elem_type == SHR_TY_F64 || lst->elem_type == SHR_TY_F32) {
            printf("%g", lst->elem_type == SHR_TY_F32 ? (double)lst->items[i].f32 : lst->items[i].f64);
        } else if (lst->elem_type == SHR_TY_BOOL) {
            printf("%s", lst->items[i].b ? "true" : "false");
        } else if (lst->elem_type == SHR_TY_STRING) {
            print_slot(vm, lst->items[i]);
        } else {
            printf("%d", lst->items[i].i32);
        }
    }
    putchar(']');
    return true;
}

static void print_slot(const ShrVm *vm, ShrSlot v) {
    if (try_print_list(vm, v)) {
        return;
    }
    if (vm && vm->prog && is_valid_rodata_str(vm->prog, v)) {
        printf("%.*s", (int)v.rodata_ref.len,
               (const char *)(vm->prog->rodata + v.rodata_ref.off));
        return;
    }
    if (v.i32 >= -1000000000 && v.i32 <= 1000000000) {
        printf("%d", v.i32);
        return;
    }
    if (isfinite(v.f64) && fabs(v.f64) >= 1e-100 && fabs(v.f64) <= 1e100) {
        printf("%g", v.f64);
        return;
    }
    printf("%lld", (long long)v.i64);
}

static ShrSlot get_arg_at(const ShrVm *vm, uint8_t argv, uint8_t idx) {
    if (!vm || !vm->caller_fp) return (ShrSlot){0};
    if (idx < vm->arg_count) {
        return vm->caller_fp[vm->arg_slots[idx]];
    }
    return vm->caller_fp[(uint8_t)(argv + idx)];
}

void println(ShrVm *vm, uint8_t argc, uint8_t argv, ShrSlot slot) {
    if (!vm || !vm->prog) {
        if (vm) vm->ok = false;
        return;
    }
    if (argc == 0) {
        putchar('\n');
        return;
    }

    if (is_valid_rodata_str(vm->prog, slot)) {
        const char *s = (const char *)(vm->prog->rodata + slot.rodata_ref.off);
        size_t len = slot.rodata_ref.len;
        uint8_t next_arg = 1;

        for (size_t i = 0; i < len; i++) {
            if (i + 1 < len && s[i] == '{' && s[i + 1] == '}' && next_arg < argc) {
                print_slot(vm, get_arg_at(vm, argv, next_arg));
                next_arg++;
                i++;
            } else {
                putchar(s[i]);
            }
        }
        for (; next_arg < argc; next_arg++) {
            putchar(' ');
            print_slot(vm, get_arg_at(vm, argv, next_arg));
        }
    } else {
        print_slot(vm, slot);
        for (uint8_t i = 1; i < argc; i++) {
            putchar(' ');
            print_slot(vm, get_arg_at(vm, argv, i));
        }
    }

    putchar('\n');
}
