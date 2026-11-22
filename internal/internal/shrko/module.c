/* shrko/module.c */
#define _POSIX_C_SOURCE 200809L
#include "module.h"
#include <dlfcn.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static bool resolve_exe_dir(const char *argv0, char out_dir[4096]) {
    char exe_path[PATH_MAX];
    ssize_t len = readlink("/proc/self/exe", exe_path, sizeof(exe_path) - 1);
    if (len > 0) {
        exe_path[len] = '\0';
    } else if (argv0 && argv0[0] != '\0') {
        if (!realpath(argv0, exe_path)) {
            if (strlen(argv0) >= sizeof(exe_path)) return false;
            strcpy(exe_path, argv0);
        }
    } else {
        return false;
    }

    char *slash = strrchr(exe_path, '/');
    if (!slash) {
        if (strlen(".") >= 4096) return false;
        strcpy(out_dir, ".");
        return true;
    }
    *slash = '\0';
    if (strlen(exe_path) >= 4096) return false;
    strcpy(out_dir, exe_path);
    return true;
}

static char *dup_slice(const char *s, size_t len) {
    char *p = malloc(len + 1);
    if (!p) return nullptr;
    if (len > 0 && s) memcpy(p, s, len);
    p[len] = '\0';
    return p;
}

bool shr_module_table_init(ShrModuleTable *tbl, const char *argv0) {
    if (!tbl) return false;
    memset(tbl, 0, sizeof(*tbl));
    return resolve_exe_dir(argv0, tbl->exe_dir);
}

void *shr_module_load(ShrModuleTable *tbl, const char *name, size_t name_len) {
    if (!tbl || !name || name_len == 0) return nullptr;

    for (size_t i = 0; i < tbl->count; i++) {
        if (strlen(tbl->items[i].name) == name_len &&
            memcmp(tbl->items[i].name, name, name_len) == 0) {
            return tbl->items[i].handle;
        }
    }

    char *mod_name = dup_slice(name, name_len);
    if (!mod_name) return nullptr;

    char so_path[4096];
    int n = snprintf(so_path, sizeof(so_path), "%s/../../lib/%s.so", tbl->exe_dir, mod_name);
    if (n < 0 || (size_t)n >= sizeof(so_path)) {
        free(mod_name);
        return nullptr;
    }

    dlerror();
    void *handle = dlopen(so_path, RTLD_LAZY | RTLD_LOCAL);
    if (!handle) {
        const char *err = dlerror();
        fprintf(stderr, "shrko: dlopen('%s') failed: %s\n", so_path, err ? err : "unknown error");
        free(mod_name);
        return nullptr;
    }

    char *path_copy = dup_slice(so_path, strlen(so_path));
    if (!path_copy) {
        dlclose(handle);
        free(mod_name);
        return nullptr;
    }

    if (tbl->count == tbl->cap) {
        size_t ncap = tbl->cap == 0 ? 8 : tbl->cap * 2;
        ShrModule *g = realloc(tbl->items, ncap * sizeof(ShrModule));
        if (!g) {
            free(path_copy);
            dlclose(handle);
            free(mod_name);
            return nullptr;
        }
        tbl->items = g;
        tbl->cap = ncap;
    }

    tbl->items[tbl->count++] = (ShrModule){
        .name = mod_name,
        .so_path = path_copy,
        .handle = handle,
    };
    return handle;
}

void *shr_module_sym(ShrModuleTable *tbl, const char *mod_name, const char *sym_name) {
    if (!tbl || !mod_name || !sym_name) return nullptr;
    void *handle = shr_module_load(tbl, mod_name, strlen(mod_name));
    if (!handle) return nullptr;
    dlerror();
    void *sym = dlsym(handle, sym_name);
    if (!sym) {
        const char *err = dlerror();
        fprintf(stderr, "shrko: dlsym('%s' in '%s') failed: %s\n", sym_name, mod_name, err ? err : "unknown error");
    }
    return sym;
}

void shr_module_table_free(ShrModuleTable *tbl) {
    if (!tbl) return;
    for (size_t i = 0; i < tbl->count; i++) {
        if (tbl->items[i].handle) dlclose(tbl->items[i].handle);
        free(tbl->items[i].name);
        free(tbl->items[i].so_path);
    }
    free(tbl->items);
    memset(tbl, 0, sizeof(*tbl));
}
