#include "shrkobc.h"

void fn_calc(ShrkoVM *vm) {
    vm_enter_frame(vm);

    const ptrdiff_t OFF_FLAG  = -(ptrdiff_t)(sizeof(size_t) + sizeof(bool));
    const ptrdiff_t OFF_X     = OFF_FLAG - (ptrdiff_t)sizeof(int64_t);
    const ptrdiff_t OFF_BONUS = 0;

    /* let bonus: int64 = 10 */
    vm_push_i64(vm, 10);

    /* if flag */
    vm_load_bool(vm, OFF_FLAG);
    VM_JMP_IF_FALSE(vm, skip_bonus);

    /* bonus = bonus + x */
    vm_load_i64(vm, OFF_BONUS);
    vm_load_i64(vm, OFF_X);
    vm_add_i64(vm);
    vm_store_i64(vm, OFF_BONUS);

skip_bonus:
    /* return bonus * 2 */
    vm_load_i64(vm, OFF_BONUS);
    vm_push_i64(vm, 2);
    vm_mul_i64(vm);
    vm_ret_i64(vm, sizeof(int64_t) + sizeof(bool));
}

int main(void) {
    ShrkoVM vm;
    vm_init(&vm);

    vm_push_i64(&vm, 15);
    vm_push_bool(&vm, true);
    fn_calc(&vm);

    int64_t out = vm_pop_i64(&vm);
    printf("result = %lld, sp = %zu\n", (long long)out, vm.sp);
    return 0;
}
