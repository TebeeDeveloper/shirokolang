CC       ?= gcc
INCLUDES := -I. -Ishrkoc -Ishrko
CFLAGS   := -std=c23 -O3 -Wall -Wextra -Wpedantic $(INCLUDES) -w -ldl
SOFLAGS  := -fPIC -shared
LDFLAGS  := -lm

BUILD_DIR     := build
LIB_OUT_DIR   := ../lib
OBJ_SHRK_DIR  := $(BUILD_DIR)/obj_shrkc
OBJ_SHRKO_DIR := $(BUILD_DIR)/obj_shrko

SHRK_BIN  := $(BUILD_DIR)/shrkoc
SHRKO_BIN := $(BUILD_DIR)/shrko

SHRK_SRCS  := shrkoc/main.c shrkoc/lexer.c shrkoc/parser.c shrkoc/sema.c shrkoc/bc_writer.c shrkoc/codegen.c
SHRKO_SRCS := shrko/main.c shrko/vm.c shrko/module.c
MOD_SRCS   := $(wildcard mod/*.c)
MOD_SOS    := $(patsubst mod/%.c,$(LIB_OUT_DIR)/%.so,$(MOD_SRCS))

SHRK_OBJS  := $(patsubst %.c,$(OBJ_SHRK_DIR)/%.o,$(SHRK_SRCS))
SHRKO_OBJS := $(patsubst %.c,$(OBJ_SHRKO_DIR)/%.o,$(SHRKO_SRCS))

.PHONY: all mods run clean

all: $(SHRK_BIN) $(SHRKO_BIN) mods

mods: $(MOD_SOS)

$(SHRK_BIN): $(SHRK_OBJS)
	$(CC) $(CFLAGS) $^ -o $@ $(LDFLAGS)

$(SHRKO_BIN): $(SHRKO_OBJS)
	$(CC) $(CFLAGS) $^ -o $@ $(LDFLAGS)

$(LIB_OUT_DIR)/%.so: mod/%.c
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(SOFLAGS) $< -o $@ $(LDFLAGS)

$(OBJ_SHRK_DIR)/%.o: %.c
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) -MMD -MP -c $< -o $@

$(OBJ_SHRKO_DIR)/%.o: %.c
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) -MMD -MP -c $< -o $@

-include $(SHRK_OBJS:.o=.d) $(SHRKO_OBJS:.o=.d)

run: all
	$(SHRK_BIN) test.shrko
	$(SHRKO_BIN) test

clean:
	rm -rf $(BUILD_DIR) $(LIB_OUT_DIR)/*.so
