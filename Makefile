.PHONY: lab-up lab-status lab-provision lab-verify devstack-setup devstack-verify smoke baseline docs-check sprint2-verify

.PHONY: glance-fmt-check glance-vet glance-test glance-build glance-test-mysql
.PHONY: glance-test-mysql-catalog
# Go is selected per invocation; no global go env or installation changes.
GLANCE_TOOLCHAIN ?= go1.27.3
GLANCE_GOFMT ?= gofmt
GLANCE_GO = GIT_OPTIONAL_LOCKS=0 GOTOOLCHAIN=$(GLANCE_TOOLCHAIN) GOMAXPROCS=2 go

glance-fmt-check:
	@files="$$($(GLANCE_GOFMT) -l cmd/glance internal)" || exit 1; \
	if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi

glance-vet:
	$(GLANCE_GO) vet -p 1 ./cmd/glance/... ./internal/...

glance-test:
	$(GLANCE_GO) test -p 1 -count=1 ./cmd/glance/... ./internal/...

glance-build:
	@mkdir -p bin
	$(GLANCE_GO) build -p 1 -trimpath -buildvcs=false -o bin/glance ./cmd/glance

# Explicitly mutates an independently authorized, empty disposable test schema.
glance-test-mysql:
	$(GLANCE_GO) test -p 1 -count=1 -tags=integration ./internal/glance/repository/mysql -run '^TestMySQLPersistenceAndTransactions$$' -v

# NEW EMPTY independently authorized schema; never run alongside the A suite on it.
glance-test-mysql-catalog:
	$(GLANCE_GO) test -p 1 -count=1 -tags=integration ./internal/glance/repository/mysql -run '^TestMySQLCatalogOperations$$' -v

lab-up:
	./infra/lab-vm/up.sh

lab-status:
	./infra/lab-vm/status.sh

lab-provision:
	./infra/lab-vm/provision.sh

lab-verify:
	./infra/lab-vm/verify-vm.sh

devstack-setup:
	./infra/devstack/setup-devstack.sh

devstack-verify:
	./infra/devstack/verify-devstack.sh

smoke:
	./infra/devstack/smoke-test.sh

baseline:
	./infra/devstack/capture-baseline.sh

docs-check:
	@find infra scripts tests -name '*.sh' -print0 | xargs -0 -n1 bash -n
	@if command -v shellcheck >/dev/null; then find infra scripts tests -name '*.sh' -print0 | xargs -0 shellcheck -S warning; else echo 'shellcheck unavailable; bash -n completed'; fi
	@python3 scripts/verify_sprint2.py --docs-only

sprint2-verify: docs-check
	@if [ -n "$${OS_AUTH_URL:-}" ] || [ -f /opt/stack/devstack/openrc ]; then ./infra/devstack/verify-devstack.sh; fi
	@python3 scripts/verify_sprint2.py
