GO ?= go
BINARY_DIR ?= bin
VERSION ?= dev
WARNING_BYTES ?= 8388608
FAIL_BYTES ?= 12582912
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test build package check-size clean

test:
	$(GO) test ./...

build: $(BINARY_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/burpvalve ./cmd/burpvalve
	$(MAKE) check-size

package:
	./scripts/package-skill.sh

$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

check-size:
	@for path in "$(BINARY_DIR)/burpvalve"; do \
		if [ ! -f "$$path" ]; then \
			echo "missing binary: $$path"; \
			exit 1; \
		fi; \
		size=$$(wc -c < "$$path"); \
		if [ "$$size" -gt "$(FAIL_BYTES)" ]; then \
			echo "error: $$path is $$size bytes, above hard limit $(FAIL_BYTES)"; \
			exit 1; \
		fi; \
		if [ "$$size" -gt "$(WARNING_BYTES)" ]; then \
			echo "warning: $$path is $$size bytes, above warning threshold $(WARNING_BYTES)"; \
		fi; \
	done

clean:
	rm -rf $(BINARY_DIR)
