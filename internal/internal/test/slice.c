#define _POSIX_C_SOURCE 199309L
#include "shrkobc.h"
#include <time.h>

typedef struct {
    void  *ptr;
    size_t len;
    size_t cap;
} ShrkoSlice;

static inline void vm_push_i32(ShrkoVM *vm, int32_t v) {
    vm_push_bytes(vm, &v, sizeof(v));
}

static inline int32_t vm_pop_i32(ShrkoVM *vm) {
    int32_t v = 0;
    vm_pop_bytes(vm, &v, sizeof(v));
    return v;
}

static inline void vm_push_slice_ref(ShrkoVM *vm, ShrkoSlice *s) {
    vm_push_bytes(vm, &s, sizeof(s));
}

static inline ShrkoSlice *vm_pop_slice_ref(ShrkoVM *vm) {
    ShrkoSlice *s = NULL;
    vm_pop_bytes(vm, &s, sizeof(s));
    return s;
}

#if defined(__GNUC__) || defined(__clang__)
#define SHRKO_NOINLINE __attribute__((noinline))
#else
#define SHRKO_NOINLINE
#endif

static SHRKO_NOINLINE void shrko_slice_grow(ShrkoVM *vm, ShrkoSlice *s, size_t elem_sz) {
    size_t new_cap = (s->cap == 0) ? 4 : (s->cap * 2);
    if (new_cap <= s->cap || new_cap > SIZE_MAX / elem_sz) {
        vm_trap(vm, 11, "slice capacity overflow");
    }
    void *new_ptr = realloc(s->ptr, new_cap * elem_sz);
    if (!new_ptr) {
        vm_trap(vm, 11, "slice realloc failed");
    }
    s->ptr = new_ptr;
    s->cap = new_cap;
}

static ShrkoSlice *shrko_slice_new_from_array(ShrkoVM *vm, const void *items, size_t len, size_t elem_sz) {
    ShrkoSlice *s = (ShrkoSlice *)malloc(sizeof(ShrkoSlice));
    if (!s) {
        vm_trap(vm, 10, "slice header alloc failed");
    }

    size_t cap = (len > 0) ? len : 4;
    void *ptr = malloc(cap * elem_sz);
    if (!ptr) {
        free(s);
        vm_trap(vm, 10, "slice buffer alloc failed");
    }

    if (len > 0 && items != NULL) {
        memcpy(ptr, items, len * elem_sz);
    }

    s->ptr = ptr;
    s->len = len;
    s->cap = cap;
    return s;
}

static inline void shrko_slice_append_i32(ShrkoVM *vm, ShrkoSlice *s, int32_t val) {
    if (s->len >= s->cap) {
        shrko_slice_grow(vm, s, sizeof(int32_t));
    }
    ((int32_t *)s->ptr)[s->len] = val;
    s->len += 1;
}

static inline void shrko_slice_append_bytes(ShrkoVM *vm, ShrkoSlice *s, const void *elem_ptr, size_t elem_sz) {
    if (s->len >= s->cap) {
        shrko_slice_grow(vm, s, elem_sz);
    }
    uint8_t *dst = (uint8_t *)s->ptr + (s->len * elem_sz);
    memcpy(dst, elem_ptr, elem_sz);
    s->len += 1;
}

static void shrko_slice_destroy(ShrkoSlice *s) {
    if (!s) return;
    free(s->ptr);
    free(s);
}

static double elapsed_sec(struct timespec start, struct timespec end) {
    return (double)(end.tv_sec - start.tv_sec) +
           (double)(end.tv_nsec - start.tv_nsec) * 1e-9;
}

int main(void) {
    const int64_t N_ITERS = 1000000;

    struct timespec t0, t1;
    clock_gettime(CLOCK_MONOTONIC, &t0);

    ShrkoVM vm;
    vm_init(&vm);
    vm_enter_frame(&vm);

    /* let data = []i32{1, 2, 3} -> lưu reference (ShrkoSlice*) tại OFF_DATA = 0 */
    const int32_t init_items[] = {1, 2, 3};
    ShrkoSlice *init_data = shrko_slice_new_from_array(&vm, init_items, 3, sizeof(int32_t));
    vm_push_slice_ref(&vm, init_data);

    /* let i: int64 = 0 -> nằm ngay sau con trỏ slice */
    const ptrdiff_t OFF_DATA = 0;
    const ptrdiff_t OFF_I    = (ptrdiff_t)sizeof(ShrkoSlice *);
    vm_push_i64(&vm, 0);

loop_start:
    /* i < N_ITERS */
    vm_load_i64(&vm, OFF_I);
    vm_push_i64(&vm, N_ITERS);
    vm_lt_i64(&vm);
    VM_JMP_IF_FALSE(&vm, loop_end);

    /* append(data, 4) -> chỉ đẩy/pop con trỏ slice (8B) + i32 (4B), cập nhật tại chỗ */
    ShrkoSlice *cur_ref = NULL;
    vm_read_at(&vm, (size_t)((ptrdiff_t)vm.bp + OFF_DATA), &cur_ref, sizeof(cur_ref));
    vm_push_slice_ref(&vm, cur_ref);
    vm_push_i32(&vm, 4);

    int32_t new_elem = vm_pop_i32(&vm);
    ShrkoSlice *target_ref = vm_pop_slice_ref(&vm);
    shrko_slice_append_i32(&vm, target_ref, new_elem);

    /* i = i + 1 */
    vm_load_i64(&vm, OFF_I);
    vm_push_i64(&vm, 1);
    vm_add_i64(&vm);
    vm_store_i64(&vm, OFF_I);

    VM_JMP(loop_start);

loop_end:;
    ShrkoSlice *final_ref = NULL;
    vm_read_at(&vm, (size_t)((ptrdiff_t)vm.bp + OFF_DATA), &final_ref, sizeof(final_ref));
    size_t final_len = final_ref->len;
    size_t final_cap = final_ref->cap;
    int32_t last_val = ((const int32_t *)final_ref->ptr)[final_len - 1];
    shrko_slice_destroy(final_ref);

    vm_leave_frame_bytes(&vm, 0, NULL, 0);

    clock_gettime(CLOCK_MONOTONIC, &t1);

    printf("len = %zu, cap = %zu, last = %d, sp = %zu, time = %.6fs\n",
           final_len, final_cap, last_val, vm.sp, elapsed_sec(t0, t1));
    return 0;
}
