#!/usr/bin/env python3
"""Glance memory sampler: cgroup v2 memory of each unit at 10 Hz.

Complements the 1 Hz PSS collector (placement_resources.py, unchanged).
memory.stat 'anon' is the unit's private memory (heap) of all its processes,
uWSGI workers included, without counting shared pages twice; 'file' is page
cache charged to the unit (image bytes it wrote or read), reported apart and
never added to the service's RAM. One small file read per group per sample,
so peaks shorter than a second are still seen.
"""
import argparse
import json
from pathlib import Path
import time

FIELDS = ('anon', 'file', 'kernel', 'sock', 'shmem')


def read_group(group, root=Path('/sys/fs/cgroup')):
    base = root / group.lstrip('/')
    stat = {}
    for line in (base / 'memory.stat').read_text().splitlines():
        key, _, value = line.partition(' ')
        if key in FIELDS:
            stat[key] = int(value)
    return {'current': int((base / 'memory.current').read_text()), **stat}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--groups', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--stop-file', type=Path, required=True)
    parser.add_argument('--maximum-seconds', type=int, default=900)
    parser.add_argument('--hz', type=int, default=10)
    args = parser.parse_args()
    groups = json.loads(args.groups.read_text())
    if not 10 <= args.maximum_seconds <= 3600 or not 1 <= args.hz <= 50 or not groups or \
            any(not v.startswith('/') or '..' in v for v in groups.values()):
        raise SystemExit('bounded duration, rate and absolute cgroups required')
    begin, count = time.monotonic(), 0
    with args.output.open('x') as stream:
        while time.monotonic() - begin < args.maximum_seconds and not args.stop_file.exists():
            row = {'utc_ns': time.time_ns(), 'groups': {}}
            for label, group in groups.items():
                try:
                    row['groups'][label] = read_group(group)
                except (OSError, ValueError):
                    row['groups'][label] = None
            stream.write(json.dumps(row) + '\n')
            count += 1
            time.sleep(max(0, begin + count / args.hz - time.monotonic()))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
