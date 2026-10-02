#!/usr/bin/env python3
"""Non-destructive artifact and evidence gate for Sprint 2."""
from pathlib import Path
import argparse
import json
import sys

root = Path(__file__).resolve().parents[1]
required = [
    'docs/sprint2/excel-requirements-map.md',
    'docs/sprint2/placement-architecture.md',
    'docs/sprint2/placement-api-scope.md',
    'docs/sprint2/glance-architecture.md',
    'docs/sprint2/glance-api-scope.md',
    'docs/sprint2/go-target-architecture.md',
    'docs/sprint2/compatibility-strategy.md',
    'docs/sprint2/test-strategy.md',
    'docs/sprint2/sprint2-evidence.md',
    'docs/sprint2/sprint2-checklist.md',
    'docs/sprint2/final-traceability.md',
    'docs/sprint2/diagrams/general-architecture.mmd',
    'docs/sprint2/diagrams/placement-components.mmd',
    'docs/sprint2/diagrams/nova-placement-sequence.mmd',
    'docs/sprint2/diagrams/placement-data-model.mmd',
    'docs/sprint2/diagrams/glance-components.mmd',
    'docs/sprint2/diagrams/nova-glance-sequence.mmd',
    'docs/sprint2/diagrams/glance-data-model.mmd',
    'docs/sprint2/diagrams/go-target-architecture.mmd',
    'docs/sprint2/informe_etapa2.tex',
    'docs/sprint2/informe_etapa2.pdf',
]
critical = [
    'evidence/sprint2/devstack/service-list.json',
    'evidence/sprint2/devstack/resource-provider-list.json',
    'evidence/sprint2/tests/boot-states.txt',
    'evidence/sprint2/placement/versions.response.json',
    'evidence/sprint2/glance/images.response.json',
    'evidence/sprint2/nova/scheduler-placement.log',
]
parser = argparse.ArgumentParser()
parser.add_argument('--docs-only', action='store_true')
args = parser.parse_args()
missing = [p for p in required if not (root / p).is_file() or (root / p).stat().st_size == 0]
if not args.docs_only:
    missing += [p for p in critical if not (root / p).is_file() or (root / p).stat().st_size == 0]
    boot = root / 'evidence/sprint2/tests/boot-states.txt'
    if boot.is_file() and 'ACTIVE' not in boot.read_text():
        missing.append('boot state ACTIVE')
    lock = (root / 'infra/devstack/versions.lock').read_text()
    if 'UNINSTALLED' in lock:
        missing.append('runtime versions in infra/devstack/versions.lock')
    def require_status(path, expected):
        target = root / path
        if target.is_file() and target.read_text().strip() != str(expected):
            missing.append(f'{path}: expected HTTP {expected}')

    for name, code in {
        'placement/providers': 200,
        'placement/provider-create': 201,
        'placement/inventory-put': 200,
        'placement/provider-delete': 204,
        'placement/provider-not-found': 404,
        'glance/images': 200,
        'glance/image-create': 201,
        'glance/image-upload': 204,
        'glance/image-active': 200,
        'glance/image-patch': 200,
        'glance/image-download': 200,
        'glance/image-delete': 204,
        'glance/image-not-found': 404,
    }.items():
        path = f'evidence/sprint2/{name}.status.txt'
        if not (root / path).is_file():
            missing.append(path)
        else:
            require_status(path, code)

    versions = root / 'evidence/sprint2/placement/versions.response.json'
    if versions.is_file():
        data = json.loads(versions.read_text())
        offered = data.get('versions', [])
        if not any(v.get('min_version') == '1.0' and v.get('max_version') == '1.39' for v in offered):
            missing.append('Placement server microversions 1.0–1.39')
    flow = root / 'evidence/sprint2/placement/nova-requests.log'
    if flow.is_file():
        lines = flow.read_text().splitlines()
        for method, path, code, micro in (
            ('GET', '/allocation_candidates', '200', '1.36'),
            ('GET', '/allocations/', '200', '1.28'),
            ('PUT', '/allocations/', '204', '1.36'),
            ('DELETE', '/allocations/', '204', '1.0'),
        ):
            if not any(f'"{method} /placement{path}' in line and f'status: {code}' in line and f'microversion: {micro}' in line and 'service nova' in line for line in lines):
                missing.append(f'Nova→Placement {method} {path} HTTP {code} v{micro}')
    else:
        missing.append('evidence/sprint2/placement/nova-requests.log')
    hashes = root / 'evidence/sprint2/glance/image-download.sha256.txt'
    if hashes.is_file():
        values = hashes.read_text().splitlines()
        if len(values) != 2 or values[0] != values[1]:
            missing.append('Glance upload/download SHA256 equality')
    else:
        missing.append('evidence/sprint2/glance/image-download.sha256.txt')
for p in missing:
    print(f'MISSING {p}', file=sys.stderr)
if missing:
    sys.exit(1)
print('Sprint 2 docs gate OK' if args.docs_only else 'Sprint 2 evidence gate OK')
