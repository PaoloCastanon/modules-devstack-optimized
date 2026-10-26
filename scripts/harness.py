#!/usr/bin/env python3
"""Local repository checks; never contacts libvirt, Terraform providers or OpenStack."""

import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
ENTRY_POINTS = ('README.md', 'AGENTS.md', 'PLANS.md', 'harness/build-log.md')
SKIP_DIRS = {'.git', '.terraform', '.cache', '.venv', 'venv', 'vendor', '__pycache__'}


def go_files(root):
    files = []
    for directory, subdirs, names in os.walk(root):
        subdirs[:] = sorted(name for name in subdirs if name not in SKIP_DIRS)
        files.extend(Path(directory) / name for name in sorted(names) if name.endswith('.go'))
    return files


def check_navigation(root):
    errors = []
    for name in ENTRY_POINTS:
        source = root / name
        if not source.is_file() or not source.stat().st_size:
            errors.append(f'Missing or empty harness entry point: {name}')
            continue
        # Entry-point documents use inline Markdown links, not reference definitions.
        for target in re.findall(r'\]\(([^\s)]+)\)', source.read_text()):
            parsed = urlsplit(target)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            destination = (source.parent / unquote(parsed.path)).resolve()
            if not destination.is_relative_to(root.resolve()):
                errors.append(f'{name}: link outside repository: {target}')
            elif not destination.exists():
                errors.append(f'{name}: broken local link: {target}')
    return errors


def verify_go(root):
    sources = go_files(root)
    manifest = root / 'go.mod'
    if not sources and not manifest.exists():
        print('PENDING Go: no go.mod or .go sources; no Go tests/build were run.', flush=True)
        return 0
    if not manifest.is_file() or not sources:
        print('FAIL Go: both root go.mod and Go sources are required.', file=sys.stderr)
        return 1
    runtime = [path for path in sources if not path.name.endswith('_test.go')]
    tests = [path for path in sources if path.name.endswith('_test.go')]
    if not runtime or not tests:
        print('FAIL Go: runtime sources and behavioral tests are required.', file=sys.stderr)
        return 1
    for domain in ('placement', 'glance'):
        path = root / 'internal' / domain
        if any(source.is_relative_to(path) for source in runtime) and not any(
            source.is_relative_to(path) for source in tests
        ):
            print(f'FAIL Go: internal/{domain} has no tests.', file=sys.stderr)
            return 1
    for tool in ('go', 'gofmt'):
        if not shutil.which(tool):
            print(f'FAIL Go: required tool is missing: {tool}', file=sys.stderr)
            return 1
    formatted = subprocess.run(
        ['gofmt', '-l', *(str(path) for path in sources)],
        cwd=root, capture_output=True, text=True, check=False,
    )
    if formatted.returncode or formatted.stdout.strip():
        print('FAIL Go: gofmt failed or found unformatted sources.', file=sys.stderr)
        print(formatted.stdout + formatted.stderr, file=sys.stderr, end='')
        return 1
    for args in (
        ['go', 'vet', './...'],
        ['go', 'test', '-race', '-count=1', './...'],
        ['go', 'build', './...'],
    ):
        print('+ ' + ' '.join(args), flush=True)
        result = subprocess.run(args, cwd=root, check=False)
        if result.returncode:
            return result.returncode
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('check', 'status', 'go'))
    args = parser.parse_args()
    if args.command == 'go':
        return verify_go(ROOT)
    if args.command == 'status':
        sources = go_files(ROOT)
        runtime = sum(not path.name.endswith('_test.go') for path in sources)
        print(f'Go runtime files: {runtime}; test files: {len(sources) - runtime}; '
              f'root go.mod: {"present" if (ROOT / "go.mod").is_file() else "absent"}')
        if not sources and not (ROOT / 'go.mod').exists():
            print('Service reimplementation has not started; next functional task: B07.')
        else:
            print('Files alone do not establish acceptance. Run make go-check and inspect the build log.')
        print('Accepted phase and verification limits: harness/build-log.md; planned work: PLANS.md')
        return 0
    errors = check_navigation(ROOT)
    for error in errors:
        print(error, file=sys.stderr)
    if errors:
        return 1
    print('Harness navigation OK (local links only; no remote availability check).')
    return 0


if __name__ == '__main__':
    sys.exit(main())
