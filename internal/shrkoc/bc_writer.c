/* shrkc/bc_writer.c */
#include "bc_writer.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void shr_bc_writer_init(ShrBcWriter *w) {
    memset(w, 0, sizeof(*w));
}

static void wr_u16_le(uint8_t *p, uint16_t v) {
    p[0] = (uint8_t)(v & 0xffu);
    p[1] = (uint8_t)((v >> 8) & 0xffu);
}

static void wr_u32_le(uint8_t *p, uint32_t v) {
    p[0] = (uint8_t)(v & 0xffu);
    p[1] = (uint8_t)((v >> 8) & 0xffu);
    p[2] = (uint8_t)((v >> 16) & 0xffu);
    p[3] = (uint8_t)((v >> 24) & 0xffu);
}

uint16_t shr_bc_writer_add_rodata(ShrBcWriter *w, const void *data, size_t len) {
    if (w->rodata_size + len > w->rodata_cap) {
        size_t ncap = w->rodata_cap ? w->rodata_cap * 2 : 256;
        while (ncap < w->rodata_size + len) ncap *= 2;
        w->rodata = realloc(w->rodata, ncap);
        w->rodata_cap = ncap;
    }
    uint16_t off = (uint16_t)w->rodata_size;
    if (len > 0 && data) memcpy(w->rodata + w->rodata_size, data, len);
    w->rodata_size += len;
    return off;
}

uint16_t shr_bc_writer_add_module(ShrBcWriter *w, const char *name, size_t len, uint16_t flags) {
    if (w->module_count == w->module_cap) {
        size_t ncap = w->module_cap ? w->module_cap * 2 : 8;
        w->modules = realloc(w->modules, ncap * sizeof(ShrBcModuleEntry));
        w->module_cap = ncap;
    }
    uint16_t off = shr_bc_writer_add_rodata(w, name, len);
    uint16_t idx = (uint16_t)w->module_count;
    w->modules[w->module_count++] = (ShrBcModuleEntry){
        .name_off = off,
        .name_len = (uint16_t)len,
        .flags = flags,
    };
    return idx;
}

uint16_t shr_bc_writer_add_func(ShrBcWriter *w, ShrBcFuncEntry fn) {
    if (w->func_count == w->func_cap) {
        size_t ncap = w->func_cap ? w->func_cap * 2 : 16;
        w->funcs = realloc(w->funcs, ncap * sizeof(ShrBcFuncEntry));
        w->func_cap = ncap;
    }
    uint16_t idx = (uint16_t)w->func_count;
    w->funcs[w->func_count++] = fn;
    return idx;
}

static size_t ensure_code(ShrBcWriter *w, size_t extra) {
    if (w->code_size + extra > w->code_cap) {
        size_t ncap = w->code_cap ? w->code_cap * 2 : 512;
        while (ncap < w->code_size + extra) ncap *= 2;
        w->code = realloc(w->code, ncap);
        w->code_cap = ncap;
    }
    size_t pos = w->code_size;
    w->code_size += extra;
    return pos;
}

size_t shr_bc_writer_emit_u8(ShrBcWriter *w, uint8_t v) {
    size_t p = ensure_code(w, 1);
    w->code[p] = v;
    return p;
}

size_t shr_bc_writer_emit_u16(ShrBcWriter *w, uint16_t v) {
    size_t p = ensure_code(w, 2);
    wr_u16_le(w->code + p, v);
    return p;
}

size_t shr_bc_writer_emit_u32(ShrBcWriter *w, uint32_t v) {
    size_t p = ensure_code(w, 4);
    wr_u32_le(w->code + p, v);
    return p;
}

size_t shr_bc_writer_emit_i32(ShrBcWriter *w, int32_t v) {
    uint32_t u = 0;
    memcpy(&u, &v, sizeof(u));
    return shr_bc_writer_emit_u32(w, u);
}

size_t shr_bc_writer_emit_f64(ShrBcWriter *w, double v) {
    uint64_t u = 0;
    memcpy(&u, &v, sizeof(u));
    size_t p = ensure_code(w, 8);
    for (int i = 0; i < 8; i++) w->code[p + i] = (uint8_t)((u >> (i * 8)) & 0xffu);
    return p;
}

bool shr_bc_writer_write_file(const ShrBcWriter *w, const char *path) {
    FILE *fp = fopen(path, "wb");
    if (!fp) return false;

    uint8_t hb[16] = {
        SHRBC_MAGIC_0, SHRBC_MAGIC_1, SHRBC_MAGIC_2, SHRBC_MAGIC_3
    };
    wr_u16_le(hb + 4, 1);
    wr_u16_le(hb + 6, 0);
    wr_u16_le(hb + 8, (uint16_t)w->rodata_size);
    wr_u16_le(hb + 10, (uint16_t)w->module_count);
    wr_u16_le(hb + 12, (uint16_t)w->func_count);
    wr_u16_le(hb + 14, (uint16_t)w->code_size);

    fwrite(hb, 1, sizeof(hb), fp);
    if (w->rodata_size) fwrite(w->rodata, 1, w->rodata_size, fp);

    for (size_t i = 0; i < w->module_count; i++) {
        uint8_t mb[6];
        wr_u16_le(mb, w->modules[i].name_off);
        wr_u16_le(mb + 2, w->modules[i].name_len);
        wr_u16_le(mb + 4, w->modules[i].flags);
        fwrite(mb, 1, 6, fp);
    }

    for (size_t i = 0; i < w->func_count; i++) {
        const ShrBcFuncEntry *f = &w->funcs[i];
        uint8_t fb[12];
        wr_u16_le(fb, f->name_off);
        wr_u16_le(fb + 2, f->name_len);
        wr_u16_le(fb + 4, f->code_offset);
        wr_u16_le(fb + 6, f->code_size);
        fb[8] = f->param_count;
        fb[9] = f->slot_count;
        fb[10] = f->ret_count;
        fb[11] = f->flags;
        fwrite(fb, 1, 12, fp);
    }

    if (w->code_size) fwrite(w->code, 1, w->code_size, fp);
    fclose(fp);
    return true;
}

void shr_bc_writer_free(ShrBcWriter *w) {
    free(w->rodata);
    free(w->modules);
    free(w->funcs);
    free(w->code);
    memset(w, 0, sizeof(*w));
}
