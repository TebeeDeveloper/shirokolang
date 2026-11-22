CC       := tcc
CFLAGS   := -Wall -Wextra
BUILD    := build
REL_DIR  := $(BUILD)/release

SRCS     := $(wildcard *.c src/*.c)
HDRS     := $(wildcard *.h src/*.h)
BINS     := $(patsubst %.c,$(BUILD)/%,$(SRCS))
REL_BINS := $(patsubst %.c,$(REL_DIR)/%,$(SRCS))

REL_CC     := gcc
REL_CFLAGS := -std=c23 -O2 -Wall -Wextra

.PHONY: all build run release release-run clean

all: run

# Dev build nhanh bằng tcc (chỉ build lại file đổi)
build: $(BINS)

$(BUILD)/%: %.c $(HDRS) Makefile
	@mkdir -p $(dir $@)
	$(CC) $(CFLAGS) "$<" -o "$@"

# Chạy lần lượt các binary dev
run: build
	@if [ -z "$(SRCS)" ]; then \
		echo "Khong tim thay file .c nao!"; exit 1; \
	fi
	@for src in $(SRCS); do \
		bin="$(BUILD)/$${src%.c}"; \
		echo "--- Running $$src ---"; \
		"$$bin" || exit $$?; \
	done

# Build tối ưu bằng gcc -std=c23 -O2 vào build/release/...
release: $(REL_BINS)

$(REL_DIR)/%: %.c $(HDRS) Makefile
	@mkdir -p $(dir $@)
	$(REL_CC) $(REL_CFLAGS) "$<" -o "$@"

release-run: release
	@if [ -z "$(SRCS)" ]; then \
		echo "Khong tim thay file .c nao!"; exit 1; \
	fi
	@for src in $(SRCS); do \
		bin="$(REL_DIR)/$${src%.c}"; \
		echo "--- Running release $$src ---"; \
		"$$bin" || exit $$?; \
	done

# Gõ tắt: `make slice.c` hoặc `make src/test.c` -> build dev bằng tcc rồi chạy
%.c:
	@$(MAKE) --no-print-directory "$(BUILD)/$*"
	@echo "--- Running $@ ---"
	@./"$(BUILD)/$*"

clean:
	rm -rf $(BUILD)
