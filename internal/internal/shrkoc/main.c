/* shrkc/main.c */
#include "lexer.h"
#include "parser.h"
#include "sema.h"
#include "codegen.h"
#include "bc_writer.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void hoist_toplevel_stmts(ShrProgramAst *prog) {
    if (!prog || prog->func_count == 0) return;
    /* Nếu có logic hoist statement top-level vào hàm main, đặt tại đây */
}

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <input.shrko> [-o output.shrb]\n", argv[0]);
        return 1;
    }

    const char *input_path = argv[1];
    char default_out[4096];
    const char *output_path = default_out;

    size_t in_len = strlen(input_path);
    const char *ext = ".shrko";
    size_t ext_len = strlen(ext);
    if (in_len > ext_len && strcmp(input_path + in_len - ext_len, ext) == 0) {
        size_t base_len = in_len - ext_len;
        if (base_len + 5 >= sizeof(default_out)) base_len = sizeof(default_out) - 6;
        memcpy(default_out, input_path, base_len);
        memcpy(default_out + base_len, ".shrb", 6);
    } else {
        int n = snprintf(default_out, sizeof(default_out), "%s.shrb", input_path);
        if (n < 0 || (size_t)n >= sizeof(default_out)) {
            fprintf(stderr, "shrkc: output path too long\n");
            return 1;
        }
    }

    for (int i = 2; i + 1 < argc; i++) {
        if (strcmp(argv[i], "-o") == 0) {
            output_path = argv[i + 1];
            i++;
        }
    }

    FILE *fp = fopen(input_path, "rb");
    if (!fp) {
        perror("fopen");
        return 1;
    }
    if (fseek(fp, 0, SEEK_END) != 0) {
        fclose(fp);
        return 1;
    }
    long sz = ftell(fp);
    if (sz < 0 || fseek(fp, 0, SEEK_SET) != 0) {
        fclose(fp);
        return 1;
    }

    size_t src_len = (size_t)sz;
    char *src = malloc(src_len + 1);
    if (!src || (src_len > 0 && fread(src, 1, src_len, fp) != src_len)) {
        free(src);
        fclose(fp);
        return 1;
    }
    src[src_len] = '\0';
    fclose(fp);

    ShrParser parser;
    shr_parser_init(&parser, src, src_len);

    ShrProgramAst prog = {0};
    if (!shr_parse_program(&parser, &prog) || parser.had_error) {
        fprintf(stderr, "shrkc: parse error in '%s'\n", input_path);
        shr_program_ast_free(&prog);
        free(src);
        return 1;
    }

    hoist_toplevel_stmts(&prog);

    ShrSema sema;
    shr_sema_init(&sema);
    if (!shr_sema_analyze(&sema, &prog) || !sema.ok) {
        if (sema.err_line > 0 && sema.err_msg[0] != '\0') {
            fprintf(stderr, "shrkc: sema error in '%s' line %d: %s\n",
                    input_path, sema.err_line, sema.err_msg);
        } else if (sema.err_msg[0] != '\0') {
            fprintf(stderr, "shrkc: sema error in '%s': %s\n", input_path, sema.err_msg);
        } else {
            fprintf(stderr, "shrkc: sema error in '%s'\n", input_path);
        }
        shr_program_ast_free(&prog);
        free(src);
        return 1;
    }

    ShrCodegen cg;
    shr_codegen_init(&cg);
    bool emitted = shr_codegen_emit_program(&cg, &prog);
    if (!emitted) {
        fprintf(stderr, "shrkc: codegen error in '%s'\n", input_path);
        shr_codegen_free(&cg);
        shr_program_ast_free(&prog);
        free(src);
        return 1;
    }

    if (!shr_bc_writer_write_file(&cg.bc, output_path)) {
        fprintf(stderr, "shrkc: failed to write '%s'\n", output_path);
        shr_codegen_free(&cg);
        shr_program_ast_free(&prog);
        free(src);
        return 1;
    }

    shr_codegen_free(&cg);
    shr_program_ast_free(&prog);
    free(src);
    return 0;
}
