import time

N_ITERS = 1_000_000

t0 = time.perf_counter()
data = [1, 2, 3]
for _ in range(N_ITERS):
    data.append(4)
t1 = time.perf_counter()

print(f"len = {len(data)}, last = {data[-1]}, time = {t1 - t0:.6f}s")
