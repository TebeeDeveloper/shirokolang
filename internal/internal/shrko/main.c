/* shrko/main.c */
#include "vm.h"
#include "module.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void print_usage(const char *prog) {
    fprintf(stderr, "Usage: %s <bytecode.shrb>\n", prog);
}

int main(int argc, char **argv) {
    if (argc != 2 || strcmp(argv[1], "-h") == 0 || strcmp(argv[1], "--help") == 0) {
        print_usage(argv[0]);
        return (argc == 2) ? 0 : 1;
    }

    char bc_path[4096];
    int n = snprintf(bc_path, sizeof(bc_path), "%s.shrb", argv[1]);
    if (n < 0 || (size_t)n >= sizeof(bc_path)) {
        fprintf(stderr, "shrko: input path too long\n");
        return 1;
    }

    ShrProgram prog = {0};
    if (!shr_program_load(&prog, bc_path)) {
        fprintf(stderr, "shrko: failed to load '%s'\n", bc_path);
        return 1;
    }

    ShrModuleTable mods;
    if (!shr_module_table_init(&mods, argv[0])) {
        fprintf(stderr, "shrko: failed to resolve binary directory\n");
        shr_program_unload(&prog);
        return 1;
    }

    for (size_t i = 0; i < prog.hdr.module_count; i++) {
        const char *mname = (const char *)(prog.rodata + prog.modules[i].name_off);
        size_t mlen = prog.modules[i].name_len;
        if (!shr_module_load(&mods, mname, mlen)) {
            shr_module_table_free(&mods);
            shr_program_unload(&prog);
            return 1;
        }
    }

    ShrVm vm = {0};
    shr_vm_init(&vm, &prog);
    vm.mods = &mods;

    bool ok = shr_vm_run_entry(&vm);
    if (!ok || !vm.ok) {
        fprintf(stderr, "shrko: runtime error while executing '%s'\n", bc_path);
        shr_module_table_free(&mods);
        shr_program_unload(&prog);
        return 1;
    }

    shr_module_table_free(&mods);
    shr_program_unload(&prog);
    return 0;
}
