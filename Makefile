.PHONY: devstack-setup devstack-verify smoke baseline docs-check sprint2-verify

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
