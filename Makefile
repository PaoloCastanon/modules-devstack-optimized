.PHONY: lab-up lab-status lab-provision lab-verify devstack-setup devstack-verify smoke baseline docs-check sprint2-verify
.PHONY: check check-tools evidence-check harness-check harness-status harness-test infra-check go-check nova-pin-check unit-test

# Local gates only. Do not add lab-up, smoke, baseline or sprint2-verify here.
check: check-tools docs-check evidence-check harness-check unit-test nova-pin-check infra-check go-check

check-tools:
	@command -v shellcheck >/dev/null || { echo 'Missing required tool: shellcheck' >&2; exit 1; }
	@command -v terraform >/dev/null || { echo 'Missing required tool: terraform' >&2; exit 1; }
	@command -v ansible-playbook >/dev/null || { echo 'Missing required tool: ansible-playbook' >&2; exit 1; }

evidence-check:
	@python3 scripts/verify_sprint2.py

harness-check: harness-test
	@python3 scripts/harness.py check

harness-status:
	@python3 scripts/harness.py status

harness-test:
	@python3 -m unittest discover -s tests -p 'test_*.py' -v

# Discover this suite explicitly: tests/unit need not be an importable package.
unit-test:
	@if [ -d tests/unit ]; then python3 -m unittest discover -s tests/unit -p 'test_*.py' -v; else echo 'Python unit suite not present on this revision.'; fi

# This isolated template test is introduced by PR #1; baseline scripts contact the lab.
nova-pin-check:
	@if [ -f tests/compatibility/nova_pin.sh ]; then bash tests/compatibility/nova_pin.sh; else echo 'Nova pin regression test not present on this revision.'; fi

infra-check:
	terraform fmt -check -recursive infra/lab-vm/terraform
	ansible-playbook --syntax-check -i localhost, infra/lab-vm/ansible/site.yml

go-check:
	@python3 scripts/harness.py go

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

.PHONY: placement-build placement-test placement-test-mysql placement-test-binary
placement-build:
	@mkdir -p bin
	go build -trimpath -o bin/placement ./cmd/placement

placement-test:
	go test -race -count=1 ./cmd/placement ./internal/placement/...

# Each integration suite requires its own NEW EMPTY independently prepared schema.
# DSN files and explicit PLACEMENT_GO_MYSQL_TEST=1 are required by the tests.
placement-test-mysql:
	go test -race -count=1 -tags=integration ./internal/placement/repository/mysql -run '^TestB07MySQLHTTP$$' -v

placement-test-binary:
	go test -race -count=1 -tags=integration ./cmd/placement -run '^TestBinaryMySQLLifecycle$$' -v

.PHONY: placement-test-upgrade
# Another NEW EMPTY schema. Verifies a failed v1→v2 upgrade preserves both rows.
placement-test-upgrade:
	go test -race -count=1 -tags=integration ./internal/placement/repository/mysql -run '^TestV1CollisionStopsUpgrade$$' -v
