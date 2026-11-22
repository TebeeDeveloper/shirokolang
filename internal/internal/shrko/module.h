/* shrko/module.h */
#pragma once

#include "vm.h"
#include <stddef.h>
#include <stdint.h>
#include <stdbool.h>

typedef void (*ShrNativeFn)(ShrVm *vm, uint8_t argc, uint8_t argv, ShrSlot slot);

typedef struct {
    char *name;
    char *so_path;
    void *handle;
} ShrModule;

typedef struct ShrModuleTable {
    ShrModule *items;
    size_t count;
    size_t cap;
    char exe_dir[4096];
} ShrModuleTable;

bool shr_module_table_init(ShrModuleTable *tbl, const char *argv0);
void *shr_module_load(ShrModuleTable *tbl, const char *name, size_t name_len);
void *shr_module_sym(ShrModuleTable *tbl, const char *mod_name, const char *sym_name);
void shr_module_table_free(ShrModuleTable *tbl);
