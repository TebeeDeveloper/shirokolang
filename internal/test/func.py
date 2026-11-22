def calc(x, flag):
    bonus = 10
    if flag:
        bonus = bonus + x
    return bonus * 2


ret = calc(15, True)

print("result =", ret)
