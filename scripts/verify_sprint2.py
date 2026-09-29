#!/usr/bin/env python3
"""Non-destructive artifact and evidence gate for Sprint 2."""
from pathlib import Path
import argparse
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
missing = [p for p in required if not (root / p).is_file()]
if not args.docs_only:
    missing += [p for p in critical if not (root / p).is_file()]
for p in missing:
    print(f'MISSING {p}', file=sys.stderr)
if missing:
    sys.exit(1)
print('Sprint 2 docs gate OK' if args.docs_only else 'Sprint 2 evidence gate OK')
