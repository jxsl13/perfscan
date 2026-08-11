GO ?= go

.PHONY: all build test check fmt vet lint selfscan docs hooks clean

all: check build

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

fmt:
	gofmt -l -w .

vet:
	$(GO) vet ./...

# check is what the pre-commit hook and CI run.
check: vet test
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; echo "$$out"; exit 1; fi

# Dogfood: run perfscan on itself at every level.
selfscan:
	$(GO) run ./cmd/perfscan -level 3 ./...

# Regenerate docs/checks/ from the registry.
docs:
	$(GO) run ./cmd/gendocs

hooks:
	git config core.hooksPath .githooks
	@echo "git hooks installed (core.hooksPath=.githooks)"

clean:
	$(GO) clean ./...
