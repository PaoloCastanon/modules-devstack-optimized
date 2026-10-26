.PHONY: lab-up lab-status lab-provision lab-verify devstack-setup devstack-verify smoke baseline docs-check sprint2-verify
.PHONY: check check-tools evidence-check harness-check harness-status harness-test infra-check go-check nova-pin-check

# Local gates only. Do not add lab-up, smoke, baseline or sprint2-verify here.
check: check-tools docs-check evidence-check harness-check nova-pin-check infra-check go-check

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

# This isolated template test is introduced by PR #1; baseline scripts contact the lab.
nova-pin-check:
	@if [ -f tests/compatibility/nova_pin.sh ]; then bash tests/compatibility/nova_pin.sh; else echo 'Nova pin regression test not present on this revision.'; fi

infra-check:
	terraform fmt -check -recursive infra/lab-vm/terraform
	ansible-playbook --syntax-check -i localhost, infra/lab-vm/ansible/site.yml

go-check:
	@python3 scripts/harness.py go

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
